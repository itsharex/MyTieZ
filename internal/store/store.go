package store

import (
	"bytes"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type ClipKind string

const (
	KindText  ClipKind = "text"
	KindImage ClipKind = "image"
	KindFile  ClipKind = "file"
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
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// Settings holds persisted user preferences for the clipboard manager.
type Settings struct {
	AutoStart        bool   `json:"auto_start"`
	SilentStart      bool   `json:"silent_start"`
	FollowMouse      bool   `json:"follow_mouse"`
	AutoHideOnBlur   bool   `json:"auto_hide_on_blur"`
	Hotkey           string `json:"hotkey"`
	Deduplicate      bool   `json:"deduplicate"`
	MaxHistory       int    `json:"max_history"`
	CaptureFiles     bool   `json:"capture_files"`
	CaptureRichText  bool   `json:"capture_rich_text"`
	ClickCopyOnly    bool   `json:"click_copy_only"`
	MoveToTopOnPaste bool   `json:"move_to_top_on_paste"`
	ColorMode        string `json:"color_mode"`    // "system" | "light" | "dark"
	BackdropMode     string `json:"backdrop_mode"` // "mica" | "acrylic" | "solid"
	CompactMode      bool   `json:"compact_mode"`
	WindowWidth      int    `json:"window_width"`
	WindowHeight     int    `json:"window_height"`
}

func DefaultSettings() Settings {
	return Settings{
		AutoStart:        false,
		SilentStart:      false,
		FollowMouse:      false,
		AutoHideOnBlur:   true,
		Hotkey:           "Alt+V",
		Deduplicate:      true,
		MaxHistory:       500,
		CaptureFiles:     true,
		CaptureRichText:  true,
		ClickCopyOnly:    false,
		MoveToTopOnPaste: true,
		ColorMode:        "system",
		BackdropMode:     "mica",
		CompactMode:      false,
		WindowWidth:      420,
		WindowHeight:     600,
	}
}

type SaveOptions struct {
	Deduplicate bool
	MaxHistory  int
	SourceApp   string
	SourcePath  string
	HTML        string
}

type Store struct {
	db       *sql.DB
	baseDir  string
	imageDir string
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
	return &Store{db: db, baseDir: baseDir, imageDir: imageDir}, nil
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
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_clip_items_order ON clip_items(pinned DESC, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_clip_items_kind_hash ON clip_items(kind, content_hash);
CREATE TABLE IF NOT EXISTS app_settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`
	_, err := db.Exec(schema)
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

func (s *Store) LoadSettings() (Settings, error) {
	cfg := DefaultSettings()
	var raw string
	err := s.db.QueryRow(`SELECT value FROM app_settings WHERE key = 'config'`).Scan(&raw)
	if err == sql.ErrNoRows {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal([]byte(raw), &cfg); err != nil {
		return DefaultSettings(), err
	}
	if cfg.ColorMode != "system" && cfg.ColorMode != "light" && cfg.ColorMode != "dark" {
		cfg.ColorMode = "system"
	}
	if cfg.BackdropMode != "mica" && cfg.BackdropMode != "acrylic" && cfg.BackdropMode != "solid" {
		cfg.BackdropMode = "mica"
	}
	if cfg.Hotkey == "" {
		cfg.Hotkey = DefaultSettings().Hotkey
	}
	if cfg.MaxHistory < 0 {
		cfg.MaxHistory = 0
	}
	if cfg.WindowWidth < 360 {
		cfg.WindowWidth = DefaultSettings().WindowWidth
	}
	if cfg.WindowHeight < 420 {
		cfg.WindowHeight = DefaultSettings().WindowHeight
	}
	return cfg, nil
}

func (s *Store) SaveSettings(cfg Settings) error {
	raw, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
INSERT INTO app_settings (key, value) VALUES ('config', ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value
`, string(raw))
	return err
}

func (s *Store) ResetSettings() (Settings, error) {
	cfg := DefaultSettings()
	if err := s.SaveSettings(cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func (s *Store) UpsertText(text string) (ClipItem, error) {
	return s.SaveText(text, SaveOptions{Deduplicate: true})
}

func (s *Store) UpsertImage(pngData []byte) (ClipItem, error) {
	return s.SaveImage(pngData, SaveOptions{Deduplicate: true})
}

func (s *Store) UpsertFiles(paths []string) (ClipItem, error) {
	return s.SaveFiles(paths, SaveOptions{Deduplicate: true})
}

func TextContentHash(text, html string) string {
	if html == "" {
		return hashBytes([]byte(text))
	}
	return hashBytes([]byte(text + "\x00html\x00" + html))
}

func (s *Store) SaveText(text string, opts SaveOptions) (ClipItem, error) {
	if strings.TrimSpace(text) == "" {
		return ClipItem{}, fmt.Errorf("empty text")
	}
	return s.save(ClipItem{
		Kind:     KindText,
		Hash:     TextContentHash(text, opts.HTML),
		Text:     text,
		HTML:     opts.HTML,
		Preview:  makeTextPreview(text),
		ByteSize: int64(len(text) + len(opts.HTML)),
	}, opts)
}

func (s *Store) SaveImage(pngData []byte, opts SaveOptions) (ClipItem, error) {
	if len(pngData) == 0 {
		return ClipItem{}, fmt.Errorf("empty image")
	}
	h := hashBytes(pngData)
	imgPath := filepath.Join(s.imageDir, h+".png")
	if _, err := os.Stat(imgPath); os.IsNotExist(err) {
		if err := os.WriteFile(imgPath, pngData, 0o644); err != nil {
			return ClipItem{}, err
		}
	}
	preview := formatBytes(int64(len(pngData)))
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(pngData)); err == nil {
		preview = fmt.Sprintf("%d × %d · %s", cfg.Width, cfg.Height, formatBytes(int64(len(pngData))))
	}
	return s.save(ClipItem{
		Kind:      KindImage,
		Hash:      h,
		ImagePath: imgPath,
		Preview:   preview,
		ByteSize:  int64(len(pngData)),
	}, opts)
}

func (s *Store) SaveFiles(paths []string, opts SaveOptions) (ClipItem, error) {
	var clean []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p != "" {
			clean = append(clean, p)
		}
	}
	if len(clean) == 0 {
		return ClipItem{}, fmt.Errorf("empty file list")
	}
	raw, err := json.Marshal(clean)
	if err != nil {
		return ClipItem{}, err
	}
	names := make([]string, 0, len(clean))
	for _, p := range clean {
		names = append(names, filepath.Base(p))
	}
	return s.save(ClipItem{
		Kind:     KindFile,
		Hash:     hashBytes(raw),
		Files:    clean,
		Preview:  strings.Join(names, ", "),
		ByteSize: int64(len(clean)),
	}, opts)
}

func (s *Store) save(item ClipItem, opts SaveOptions) (ClipItem, error) {
	now := time.Now().UnixMilli()
	filesJSON := "[]"
	if len(item.Files) > 0 {
		b, err := json.Marshal(item.Files)
		if err != nil {
			return ClipItem{}, err
		}
		filesJSON = string(b)
	}
	sourceApp := strings.TrimSpace(opts.SourceApp)
	if sourceApp == "" {
		sourceApp = "Unknown"
	}
	sourcePath := strings.TrimSpace(opts.SourcePath)

	var targetID int64
	if opts.Deduplicate {
		var existingID int64
		var existingApp, existingPath string
		err := s.db.QueryRow(`
SELECT id, source_app, source_path FROM clip_items WHERE kind = ? AND content_hash = ? ORDER BY updated_at DESC LIMIT 1
`, string(item.Kind), item.Hash).Scan(&existingID, &existingApp, &existingPath)
		if err == nil {
			keepApp, keepPath := sourceApp, sourcePath
			if keepPath == "" && existingPath != "" {
				keepApp, keepPath = existingApp, existingPath
			}
			if _, err := s.db.Exec(`
UPDATE clip_items
SET text_content = ?, html_content = ?, image_path = ?, files_json = ?, preview = ?, source_app = ?, source_path = ?, byte_size = ?, updated_at = ?
WHERE id = ?
`, item.Text, item.HTML, item.ImagePath, filesJSON, item.Preview, keepApp, keepPath, item.ByteSize, now, existingID); err != nil {
				return ClipItem{}, err
			}
			targetID = existingID
		} else if err != sql.ErrNoRows {
			return ClipItem{}, err
		}
	}

	if targetID == 0 {
		res, err := s.db.Exec(`
INSERT INTO clip_items (kind, content_hash, text_content, html_content, image_path, files_json, preview, source_app, source_path, byte_size, pinned, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?)
`, string(item.Kind), item.Hash, item.Text, item.HTML, item.ImagePath, filesJSON, item.Preview, sourceApp, sourcePath, item.ByteSize, now, now)
		if err != nil {
			return ClipItem{}, err
		}
		targetID, err = res.LastInsertId()
		if err != nil {
			return ClipItem{}, err
		}
	}

	if opts.MaxHistory > 0 {
		_ = s.EnforceLimit(opts.MaxHistory)
	}

	row := s.db.QueryRow(`
SELECT id, kind, content_hash, text_content, html_content, image_path, files_json, preview, source_app, source_path, byte_size, pinned, created_at, updated_at
FROM clip_items WHERE id = ?
`, targetID)
	return scanItem(row)
}

func (s *Store) List(query string, kind ClipKind, limit int) ([]ClipItem, error) {
	if limit <= 0 {
		limit = 200
	}
	var conds []string
	var args []any

	if kind != "" {
		conds = append(conds, "kind = ?")
		args = append(args, string(kind))
	}
	q := strings.TrimSpace(query)
	if q != "" {
		like := "%" + q + "%"
		conds = append(conds, "(text_content LIKE ? OR preview LIKE ? OR files_json LIKE ? OR source_app LIKE ?)")
		args = append(args, like, like, like, like)
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	args = append(args, limit)

	rows, err := s.db.Query(`
SELECT id, kind, content_hash, text_content, html_content, image_path, files_json, preview, source_app, source_path, byte_size, pinned, created_at, updated_at
FROM clip_items
`+where+`
ORDER BY pinned DESC, updated_at DESC, id DESC
LIMIT ?
`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ClipItem
	for rows.Next() {
		it, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, it)
	}
	return items, rows.Err()
}

func (s *Store) TouchItem(id int64) error {
	_, err := s.db.Exec(`UPDATE clip_items SET updated_at = ? WHERE id = ?`, time.Now().UnixMilli(), id)
	return err
}

func (s *Store) TogglePin(id int64) (bool, error) {
	if _, err := s.db.Exec(`UPDATE clip_items SET pinned = 1 - pinned WHERE id = ?`, id); err != nil {
		return false, err
	}
	var pinned int
	if err := s.db.QueryRow(`SELECT pinned FROM clip_items WHERE id = ?`, id).Scan(&pinned); err != nil {
		return false, err
	}
	return pinned != 0, nil
}

func (s *Store) Delete(id int64) error {
	return s.deleteItemsWhere("id = ?", id)
}

func (s *Store) ClearUnpinned() error {
	return s.deleteItemsWhere("pinned = 0")
}

func (s *Store) ClearAll() error {
	return s.deleteItemsWhere("1 = 1")
}

// EnforceLimit trims unpinned items beyond maxHistory (keeping pinned items untouched).
func (s *Store) EnforceLimit(maxHistory int) error {
	if maxHistory <= 0 {
		return nil
	}
	return s.deleteItemsWhere(`id IN (
		SELECT id FROM clip_items
		WHERE pinned = 0
		ORDER BY updated_at DESC, id DESC
		LIMIT -1 OFFSET ?
	)`, maxHistory)
}

func (s *Store) deleteItemsWhere(where string, args ...any) error {
	rows, err := s.db.Query(`SELECT DISTINCT image_path FROM clip_items WHERE `+where+` AND image_path != ''`, args...)
	if err != nil {
		return err
	}
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err == nil && p != "" {
			paths = append(paths, p)
		}
	}
	_ = rows.Close()

	if _, err := s.db.Exec(`DELETE FROM clip_items WHERE `+where, args...); err != nil {
		return err
	}
	for _, p := range paths {
		s.removeOrphanImage(p)
	}
	return nil
}

func (s *Store) removeOrphanImage(imgPath string) {
	if imgPath == "" {
		return
	}
	var cnt int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM clip_items WHERE image_path = ?`, imgPath).Scan(&cnt); err == nil && cnt == 0 {
		_ = os.Remove(imgPath)
	}
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanItem(r rowScanner) (ClipItem, error) {
	var it ClipItem
	var kindStr, filesJSON string
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
	return it, nil
}

func makeTextPreview(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	lines := strings.Split(s, "\n")
	if len(lines) > 4 {
		lines = lines[:4]
	}
	out := strings.Join(lines, " ")
	runes := []rune(out)
	if len(runes) > 220 {
		return string(runes[:220]) + "…"
	}
	return out
}

func formatBytes(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	default:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	}
}
