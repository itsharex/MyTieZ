package store

import (
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	_ "modernc.org/sqlite"
)

type ClipKind string

const (
	KindText     ClipKind = "text"
	KindRichText ClipKind = "rich_text"
	KindURL      ClipKind = "url"
	KindCode     ClipKind = "code"
	KindImage    ClipKind = "image"
	KindFile     ClipKind = "file"
)

type ClipItem struct {
	ID         int64
	Kind       ClipKind
	Hash       string
	Text       string
	HTML       string
	ImagePath  string
	Files      []string
	Preview    string
	SourceApp  string
	SourcePath string
	ByteSize   int64
	Pinned     bool
	Tags       []string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

type SaveOptions struct {
	Deduplicate bool
	MaxHistory  int
	SourceApp   string
	SourcePath  string
	HTML        string
	Tags        []string
}

type Store struct {
	db          *sql.DB
	baseDir     string
	imageDir    string
	writeMu     sync.Mutex
	revision    atomic.Uint64
	tagRevision atomic.Uint64
}

type sqlExecer interface {
	Exec(query string, args ...any) (sql.Result, error)
	Query(query string, args ...any) (*sql.Rows, error)
	QueryRow(query string, args ...any) *sql.Row
}

func (s *Store) Revision() uint64 {
	if s == nil {
		return 0
	}
	return s.revision.Load()
}

func (s *Store) TagRevision() uint64 {
	if s == nil {
		return 0
	}
	return s.tagRevision.Load()
}

func (s *Store) bumpRevision(tagsChanged bool) {
	if s == nil {
		return
	}
	s.revision.Add(1)
	if tagsChanged {
		s.tagRevision.Add(1)
	}
}

func defaultDataDir() (string, error) {
	base, err := os.UserCacheDir()
	if err != nil || base == "" {
		base, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(base, "MyTieZ"), nil
}

func OpenStore(baseDir string) (*Store, error) {
	if baseDir == "" {
		var err error
		baseDir, err = defaultDataDir()
		if err != nil {
			return nil, err
		}
	}
	imageDir := filepath.Join(baseDir, "images")
	if err := os.MkdirAll(imageDir, 0o755); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(baseDir, "mytiez.db")
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	pragmas := []string{
		`PRAGMA journal_mode=WAL;`,
		`PRAGMA busy_timeout=5000;`,
		`PRAGMA synchronous=NORMAL;`,
		`PRAGMA cache_size=-2048;`,
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			_ = db.Close()
			return nil, err
		}
	}

	if err := initSchema(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	st := &Store{db: db, baseDir: baseDir, imageDir: imageDir}
	st.revision.Store(1)
	st.tagRevision.Store(1)
	st.alignSensitiveEncryption()
	return st, nil
}

func initSchema(db *sql.DB) error {
	const schema = `
CREATE TABLE IF NOT EXISTS clip_items (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	kind TEXT NOT NULL,
	content_hash TEXT NOT NULL,
	text_content TEXT NOT NULL DEFAULT '',
	html_content TEXT NOT NULL DEFAULT '',
	image_path TEXT NOT NULL DEFAULT '',
	files_json TEXT NOT NULL DEFAULT '[]',
	preview TEXT NOT NULL DEFAULT '',
	source_app TEXT NOT NULL DEFAULT '',
	source_path TEXT NOT NULL DEFAULT '',
	byte_size INTEGER NOT NULL DEFAULT 0,
	pinned INTEGER NOT NULL DEFAULT 0,
	pin_order INTEGER NOT NULL DEFAULT 0,
	tags_json TEXT NOT NULL DEFAULT '[]',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_clip_items_order ON clip_items(pinned DESC, pin_order ASC, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_clip_items_order_id ON clip_items(pinned DESC, pin_order ASC, updated_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_clip_items_kind_order ON clip_items(kind, pinned DESC, pin_order ASC, updated_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_clip_items_unpinned_recent ON clip_items(updated_at DESC, id DESC) WHERE pinned = 0;
CREATE INDEX IF NOT EXISTS idx_clip_items_kind_hash ON clip_items(kind, content_hash);
CREATE INDEX IF NOT EXISTS idx_clip_items_image_path ON clip_items(image_path) WHERE image_path != '';
CREATE INDEX IF NOT EXISTS idx_clip_items_tags ON clip_items(tags_json) WHERE tags_json != '[]' AND tags_json != '';
CREATE TABLE IF NOT EXISTS app_settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	if err := ensureColumn(db, "clip_items", "tags_json", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return err
	}
	return ensureColumn(db, "clip_items", "pin_order", "INTEGER NOT NULL DEFAULT 0")
}

func ensureColumn(db *sql.DB, table, col, def string) error {
	rows, err := db.Query("PRAGMA table_info(" + table + ")")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return err
		}
		if name == col {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.Exec("ALTER TABLE " + table + " ADD COLUMN " + col + " " + def)
	return err
}

func (s *Store) BaseDir() string {
	if s == nil {
		return ""
	}
	return s.baseDir
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// ShrinkMemory asks SQLite to release unused page cache memory.
func (s *Store) ShrinkMemory() {
	if s == nil || s.db == nil {
		return
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	_, _ = s.db.Exec(`PRAGMA shrink_memory;`)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanRawItem(r rowScanner) (ClipItem, error) {
	var it ClipItem
	var kindStr, filesJSON, tagsJSON string
	var pinnedInt int
	var createdMs, updatedMs int64
	if err := r.Scan(
		&it.ID,
		&kindStr,
		&it.Hash,
		&it.Text,
		&it.HTML,
		&it.ImagePath,
		&filesJSON,
		&it.Preview,
		&it.SourceApp,
		&it.SourcePath,
		&it.ByteSize,
		&pinnedInt,
		&tagsJSON,
		&createdMs,
		&updatedMs,
	); err != nil {
		return ClipItem{}, err
	}
	it.Kind = ClipKind(kindStr)
	it.Pinned = pinnedInt != 0
	it.CreatedAt = time.UnixMilli(createdMs)
	it.UpdatedAt = time.UnixMilli(updatedMs)
	if filesJSON != "" && filesJSON != "[]" {
		_ = json.Unmarshal([]byte(filesJSON), &it.Files)
	}
	if tagsJSON != "" && tagsJSON != "[]" {
		_ = json.Unmarshal([]byte(tagsJSON), &it.Tags)
	}
	return it, nil
}

func scanItem(r rowScanner) (ClipItem, error) {
	it, err := scanRawItem(r)
	if err != nil {
		return ClipItem{}, err
	}
	if err := decryptItemFields(&it); err != nil {
		return ClipItem{}, err
	}
	return it, nil
}

func encryptSensitiveFields(text, html, preview string, sensitive bool) (string, string, string, error) {
	fn := decryptString
	if sensitive {
		fn = encryptString
	}
	outText, err := fn(text)
	if err != nil {
		return "", "", "", err
	}
	outHTML, err := fn(html)
	if err != nil {
		return "", "", "", err
	}
	outPreview, err := fn(preview)
	if err != nil {
		return "", "", "", err
	}
	return outText, outHTML, outPreview, nil
}

func decryptItemFields(it *ClipItem) error {
	if it == nil {
		return nil
	}
	var err error
	if it.Text, err = decryptString(it.Text); err != nil {
		return err
	}
	if it.HTML, err = decryptString(it.HTML); err != nil {
		return err
	}
	if it.Preview, err = decryptString(it.Preview); err != nil {
		return err
	}
	return nil
}

func (s *Store) alignSensitiveEncryption() {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	rows, err := s.db.Query(`
SELECT id, text_content, html_content, preview, tags_json
FROM clip_items
WHERE text_content LIKE ? OR preview LIKE ? OR html_content LIKE ?
   OR lower(tags_json) LIKE '%sensitive%'
   OR tags_json LIKE '%密码%'
   OR lower(tags_json) LIKE '%password%'
`, EncryptPrefix+"%", EncryptPrefix+"%", EncryptPrefix+"%")
	if err != nil {
		return
	}
	type alignRow struct {
		id      int64
		text    string
		html    string
		preview string
	}
	var updates []alignRow
	for rows.Next() {
		var id int64
		var text, html, preview, tagsJSON string
		if err := rows.Scan(&id, &text, &html, &preview, &tagsJSON); err != nil {
			continue
		}
		var tags []string
		if tagsJSON != "" && tagsJSON != "[]" {
			_ = json.Unmarshal([]byte(tagsJSON), &tags)
		}
		nextText, nextHTML, nextPreview, err := encryptSensitiveFields(text, html, preview, HasSensitiveTag(tags))
		if err != nil {
			continue
		}
		if nextText != text || nextHTML != html || nextPreview != preview {
			updates = append(updates, alignRow{
				id:      id,
				text:    nextText,
				html:    nextHTML,
				preview: nextPreview,
			})
		}
	}
	_ = rows.Close()
	if len(updates) == 0 {
		return
	}
	tx, err := s.db.Begin()
	if err != nil {
		return
	}
	defer func() { _ = tx.Rollback() }()
	for _, u := range updates {
		if _, err := tx.Exec(
			`UPDATE clip_items SET text_content = ?, html_content = ?, preview = ? WHERE id = ?`,
			u.text, u.html, u.preview, u.id,
		); err != nil {
			return
		}
	}
	_ = tx.Commit()
}
