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
	"sort"
	"strings"
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

// Settings holds persisted user preferences for the clipboard manager.
type Settings struct {
	AutoStart          bool   `json:"auto_start"`
	SilentStart        bool   `json:"silent_start"`
	FollowMouse        bool   `json:"follow_mouse"`
	NoFocusMode        bool   `json:"no_focus_mode"`
	AutoHideOnBlur     bool   `json:"auto_hide_on_blur"`
	EdgeDocking        bool   `json:"edge_docking"`
	PinSearchBox       bool   `json:"pin_search_box"`
	ScrollTopButton    bool   `json:"scroll_top_button"`
	Hotkey             string `json:"hotkey"`
	Deduplicate        bool   `json:"deduplicate"`
	MaxHistory         int    `json:"max_history"`
	CaptureFiles       bool   `json:"capture_files"`
	CaptureRichText    bool   `json:"capture_rich_text"`
	ClickCopyOnly      bool   `json:"click_copy_only"`
	MoveToTopOnPaste   bool   `json:"move_to_top_on_paste"`
	DeleteAfterPaste   bool   `json:"delete_after_paste"`
	PasteMethod              string   `json:"paste_method"`         // "shift_insert" | "ctrl_v" | "game_mode"
	QuickPasteModifier       string   `json:"quick_paste_modifier"` // "none" | "alt" | "ctrl"
	PrivacyProtection        bool     `json:"privacy_protection"`
	PrivacyKinds             []string `json:"privacy_kinds"`
	PrivacyCustomRules       string   `json:"privacy_custom_rules"`
	SensitiveMaskPrefix      int      `json:"sensitive_mask_prefix"`
	SensitiveMaskSuffix      int      `json:"sensitive_mask_suffix"`
	SensitiveMaskEmailDomain bool     `json:"sensitive_mask_email_domain"`
	ColorMode                string   `json:"color_mode"`    // "system" | "light" | "dark"
	BackdropMode             string   `json:"backdrop_mode"` // "solid" | "mica" | "mica_alt" | "acrylic_base" | "acrylic"
	CompactMode        bool   `json:"compact_mode"`
	HideCardActions    bool   `json:"hide_card_actions"`
	ShowSourceIcon     bool   `json:"show_source_icon"`
	OpenAppText        string `json:"open_app_text"`
	OpenAppRichText    string `json:"open_app_rich_text"`
	OpenAppImage       string `json:"open_app_image"`
	WindowWidth        int    `json:"window_width"`
	WindowHeight       int    `json:"window_height"`
	LastUpdateCheck    int64  `json:"last_update_check"`
}

func DefaultSettings() Settings {
	return Settings{
		AutoStart:          false,
		SilentStart:        false,
		FollowMouse:        false,
		NoFocusMode:        false,
		AutoHideOnBlur:     true,
		EdgeDocking:        false,
		PinSearchBox:       false,
		ScrollTopButton:    true,
		Hotkey:             "Alt+V",
		Deduplicate:        true,
		MaxHistory:         500,
		CaptureFiles:       true,
		CaptureRichText:    true,
		ClickCopyOnly:      false,
		MoveToTopOnPaste:   true,
		DeleteAfterPaste:   false,
		PasteMethod:         "shift_insert",
		QuickPasteModifier:  "none",
		PrivacyProtection:   true,
		PrivacyKinds:        DefaultPrivacyKinds(),
		SensitiveMaskPrefix: 3,
		SensitiveMaskSuffix: 3,
		ColorMode:           "system",
		BackdropMode:       "solid",
		CompactMode:        false,
		HideCardActions:    false,
		ShowSourceIcon:     true,
		WindowWidth:        400,
		WindowHeight:       660,
	}
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
	tags_json TEXT NOT NULL DEFAULT '[]',
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_clip_items_order ON clip_items(pinned DESC, updated_at DESC);
CREATE INDEX IF NOT EXISTS idx_clip_items_kind_hash ON clip_items(kind, content_hash);
CREATE INDEX IF NOT EXISTS idx_clip_items_tags ON clip_items(tags_json) WHERE tags_json != '[]' AND tags_json != '';
CREATE TABLE IF NOT EXISTS app_settings (
	key TEXT PRIMARY KEY,
	value TEXT NOT NULL
);
`
	if _, err := db.Exec(schema); err != nil {
		return err
	}
	return ensureColumn(db, "clip_items", "tags_json", "TEXT NOT NULL DEFAULT '[]'")
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
	if !cfg.NoFocusMode {
		var rawMap map[string]json.RawMessage
		if json.Unmarshal([]byte(raw), &rawMap) == nil {
			if _, ok := rawMap["no_focus_mode"]; !ok {
				var legacy struct {
					HotkeyNoFocus bool `json:"hotkey_no_focus"`
				}
				if json.Unmarshal([]byte(raw), &legacy) == nil && legacy.HotkeyNoFocus {
					cfg.NoFocusMode = true
				}
			}
		}
	}
	if cfg.ColorMode != "system" && cfg.ColorMode != "light" && cfg.ColorMode != "dark" {
		cfg.ColorMode = "system"
	}
	switch cfg.BackdropMode {
	case "solid", "mica", "mica_alt", "acrylic_base", "acrylic":
	default:
		cfg.BackdropMode = DefaultSettings().BackdropMode
	}
	if cfg.PasteMethod != "shift_insert" && cfg.PasteMethod != "ctrl_v" && cfg.PasteMethod != "game_mode" {
		cfg.PasteMethod = "shift_insert"
	}
	if cfg.QuickPasteModifier != "none" && cfg.QuickPasteModifier != "alt" && cfg.QuickPasteModifier != "ctrl" {
		cfg.QuickPasteModifier = "none"
	}
	if cfg.PrivacyKinds == nil {
		cfg.PrivacyKinds = DefaultPrivacyKinds()
	}
	cfg.SensitiveMaskPrefix = max(0, min(cfg.SensitiveMaskPrefix, 12))
	cfg.SensitiveMaskSuffix = max(0, min(cfg.SensitiveMaskSuffix, 12))
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
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
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
	kind := DetectTextKind(text)
	if opts.HTML != "" {
		kind = KindRichText
	}
	return s.save(ClipItem{
		Kind:     kind,
		Hash:     TextContentHash(text, opts.HTML),
		Text:     text,
		HTML:     opts.HTML,
		Preview:  makeTextPreview(text),
		ByteSize: int64(len(text) + len(opts.HTML)),
	}, opts)
}

var codeKeywords = []string{
	"import ",
	"const ",
	"let ",
	"var ",
	"function ",
	"class ",
	"func ",
	"pub fn ",
	"impl ",
	"#include",
	"package ",
	"interface ",
	"namespace ",
	"void ",
	"return ",
	"if (",
	"for (",
	"while (",
	"=>",
}

// DetectTextKind classifies plain text into URL, code, or text using tiez-clipboard heuristics.
func DetectTextKind(text string) ClipKind {
	trimmed := strings.TrimSpace(text)
	if trimmed != "" && !strings.ContainsAny(trimmed, " \t\r\n") {
		if strings.HasPrefix(strings.ToLower(trimmed), "www.") || hasValidURLScheme(trimmed) {
			return KindURL
		}
	}

	score := 0
	for _, kw := range codeKeywords {
		if strings.Contains(text, kw) {
			score++
		}
	}
	if strings.Contains(text, ";") {
		score++
	}
	if strings.Contains(text, "{") && strings.Contains(text, "}") {
		score++
	}
	if strings.Contains(text, "</") && strings.Contains(text, ">") {
		score += 2
	}
	if score >= 2 {
		return KindCode
	}

	if strings.HasPrefix(trimmed, "{") && strings.HasSuffix(trimmed, "}") &&
		strings.Contains(text, ":") && strings.Contains(text, "\"") {
		return KindCode
	}

	return KindText
}

func hasValidURLScheme(s string) bool {
	scheme, _, ok := strings.Cut(s, "://")
	if !ok || scheme == "" {
		return false
	}
	for i := 0; i < len(scheme); i++ {
		c := scheme[i]
		isAlphaNum := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
		if !isAlphaNum && c != '+' && c != '-' && c != '.' {
			return false
		}
	}
	return true
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

	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return ClipItem{}, err
	}
	defer func() { _ = tx.Rollback() }()

	var targetID int64
	if opts.Deduplicate {
		var existingID int64
		var existingApp, existingPath, existingTagsJSON string
		err := tx.QueryRow(`
SELECT id, source_app, source_path, tags_json FROM clip_items WHERE kind = ? AND content_hash = ? ORDER BY updated_at DESC, id DESC LIMIT 1
`, string(item.Kind), item.Hash).Scan(&existingID, &existingApp, &existingPath, &existingTagsJSON)
		if err == nil {
			keepApp, keepPath := sourceApp, sourcePath
			if keepPath == "" && existingPath != "" {
				keepApp, keepPath = existingApp, existingPath
			}
			mergedTagsJSON := existingTagsJSON
			if len(opts.Tags) > 0 {
				var cur []string
				_ = json.Unmarshal([]byte(existingTagsJSON), &cur)
				merged := normalizeTags(append(cur, opts.Tags...))
				if b, err := json.Marshal(merged); err == nil {
					mergedTagsJSON = string(b)
				}
			}
			if _, err := tx.Exec(`
UPDATE clip_items
SET text_content = ?, html_content = ?, image_path = ?, files_json = ?, preview = ?, source_app = ?, source_path = ?, byte_size = ?, tags_json = ?, updated_at = ?
WHERE id = ?
`, item.Text, item.HTML, item.ImagePath, filesJSON, item.Preview, keepApp, keepPath, item.ByteSize, mergedTagsJSON, now, existingID); err != nil {
				return ClipItem{}, err
			}
			targetID = existingID
		} else if err != sql.ErrNoRows {
			return ClipItem{}, err
		}
	}

	if targetID == 0 {
		tagsJSON := "[]"
		if cleanTags := normalizeTags(opts.Tags); len(cleanTags) > 0 {
			if b, err := json.Marshal(cleanTags); err == nil {
				tagsJSON = string(b)
			}
		}
		res, err := tx.Exec(`
INSERT INTO clip_items (kind, content_hash, text_content, html_content, image_path, files_json, preview, source_app, source_path, byte_size, pinned, tags_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?)
`, string(item.Kind), item.Hash, item.Text, item.HTML, item.ImagePath, filesJSON, item.Preview, sourceApp, sourcePath, item.ByteSize, tagsJSON, now, now)
		if err != nil {
			return ClipItem{}, err
		}
		targetID, err = res.LastInsertId()
		if err != nil {
			return ClipItem{}, err
		}
	}

	var orphanPaths []string
	var deleted int64
	if opts.MaxHistory > 0 {
		orphanPaths, deleted, _ = enforceLimitExec(tx, opts.MaxHistory)
	}

	row := tx.QueryRow(`
SELECT id, kind, content_hash, text_content, html_content, image_path, files_json, preview, source_app, source_path, byte_size, pinned, tags_json, created_at, updated_at
FROM clip_items WHERE id = ?
`, targetID)
	saved, err := scanItem(row)
	if err != nil {
		return ClipItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return ClipItem{}, err
	}
	s.removeOrphanImages(orphanPaths)
	s.bumpRevision(len(opts.Tags) > 0 || deleted > 0)
	return saved, nil
}

// GetByID returns the full clipboard item for id, including complete text and HTML.
func (s *Store) GetByID(id int64) (ClipItem, error) {
	row := s.db.QueryRow(`
SELECT id, kind, content_hash, text_content, html_content, image_path, files_json, preview, source_app, source_path, byte_size, pinned, tags_json, created_at, updated_at
FROM clip_items WHERE id = ?
`, id)
	return scanItem(row)
}

func (s *Store) List(query string, kind ClipKind, limit int) ([]ClipItem, error) {
	return s.listItems(query, kind, limit, false)
}

// ListSummaries returns matching items for UI list display without loading
// large text/HTML payloads into memory.
func (s *Store) ListSummaries(query string, kind ClipKind, limit int) ([]ClipItem, error) {
	return s.listItems(query, kind, limit, true)
}

func (s *Store) listItems(query string, kind ClipKind, limit int, summary bool) ([]ClipItem, error) {
	if limit <= 0 {
		limit = 200
	}
	var conds []string
	var args []any

	if kind != "" {
		conds = append(conds, "kind = ?")
		args = append(args, string(kind))
	}
	tagFilters, kw := splitQueryTags(query)
	for _, tag := range tagFilters {
		conds = append(conds, "tags_json LIKE ?")
		encoded, _ := json.Marshal(tag)
		args = append(args, "%"+string(encoded)+"%")
	}
	pinyinSearch := isASCIIAlphaNum(kw)
	if kw != "" && !pinyinSearch {
		like := "%" + kw + "%"
		conds = append(conds, "(text_content LIKE ? OR preview LIKE ? OR files_json LIKE ? OR source_app LIKE ? OR tags_json LIKE ?)")
		args = append(args, like, like, like, like, like)
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	queryLimit := limit
	if pinyinSearch {
		queryLimit = max(limit*5, 1000)
	}
	args = append(args, queryLimit)

	cols := "id, kind, content_hash, text_content, html_content, image_path, files_json, preview, source_app, source_path, byte_size, pinned, tags_json, created_at, updated_at"
	if summary {
		textCol := "CASE WHEN length(text_content) <= 1024 THEN text_content ELSE '' END"
		if pinyinSearch {
			// 拼音搜索最多匹配前 2048 个 rune，取前 8192 字符参与匹配后若超过 1024 字节再清空，兼顾召回与内存占用
			textCol = "substr(text_content, 1, 8192)"
		}
		cols = "id, kind, content_hash, " + textCol + ", CASE WHEN kind = 'rich_text' THEN substr(html_content, 1, 8192) ELSE '' END, image_path, files_json, preview, source_app, source_path, byte_size, pinned, tags_json, created_at, updated_at"
	}

	rows, err := s.db.Query(`
SELECT `+cols+`
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
		if pinyinSearch && !itemMatchesKeyword(it, kw) {
			continue
		}
		if summary && pinyinSearch && len(it.Text) > 1024 {
			it.Text = ""
		}
		items = append(items, it)
		if len(items) >= limit {
			break
		}
	}
	return items, rows.Err()
}

func (s *Store) TouchItem(id int64) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := s.db.Exec(`UPDATE clip_items SET updated_at = ? WHERE id = ?`, time.Now().UnixMilli(), id); err != nil {
		return err
	}
	s.bumpRevision(false)
	return nil
}

func (s *Store) TogglePin(id int64) (bool, error) {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := s.db.Exec(`UPDATE clip_items SET pinned = 1 - pinned WHERE id = ?`, id); err != nil {
		return false, err
	}
	s.bumpRevision(false)
	var pinned int
	if err := s.db.QueryRow(`SELECT pinned FROM clip_items WHERE id = ?`, id).Scan(&pinned); err != nil {
		return false, err
	}
	return pinned != 0, nil
}

func normalizeTags(tags []string) []string {
	seen := make(map[string]struct{}, len(tags))
	out := make([]string, 0, len(tags))
	for _, raw := range tags {
		t := strings.TrimSpace(raw)
		if t == "" {
			continue
		}
		if _, ok := seen[t]; ok {
			continue
		}
		seen[t] = struct{}{}
		out = append(out, t)
	}
	return out
}

func (s *Store) UpdateTags(id int64, tags []string) ([]string, error) {
	cleaned := normalizeTags(tags)
	raw := "[]"
	if len(cleaned) > 0 {
		b, err := json.Marshal(cleaned)
		if err != nil {
			return nil, err
		}
		raw = string(b)
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	if _, err := s.db.Exec(`UPDATE clip_items SET tags_json = ? WHERE id = ?`, raw, id); err != nil {
		return nil, err
	}
	s.bumpRevision(true)
	return cleaned, nil
}

func (s *Store) ListAllTags() ([]string, error) {
	rows, err := s.db.Query(`SELECT tags_json FROM clip_items WHERE tags_json != '[]' AND tags_json != ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		var tags []string
		if err := json.Unmarshal([]byte(raw), &tags); err != nil {
			continue
		}
		for _, t := range tags {
			t = strings.TrimSpace(t)
			if t != "" {
				counts[t]++
			}
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]string, 0, len(counts))
	for t := range counts {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool {
		if counts[out[i]] != counts[out[j]] {
			return counts[out[i]] > counts[out[j]]
		}
		return out[i] < out[j]
	})
	return out, nil
}

func splitQueryTags(query string) (tags []string, keyword string) {
	fields := strings.Fields(strings.TrimSpace(query))
	if len(fields) == 0 {
		return nil, ""
	}
	var words []string
	for _, f := range fields {
		if strings.HasPrefix(f, "#") && len(f) > 1 {
			tags = append(tags, strings.TrimPrefix(f, "#"))
		} else {
			words = append(words, f)
		}
	}
	return tags, strings.Join(words, " ")
}

func (s *Store) Delete(id int64) error {
	return s.deleteItemsWhere("id = ?", id)
}

func (s *Store) ClearUnpinned() error {
	return s.deleteItemsWhere("pinned = 0 AND (tags_json IS NULL OR tags_json = '' OR tags_json = '[]')")
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

func enforceLimitExec(exec sqlExecer, maxHistory int) ([]string, int64, error) {
	if maxHistory <= 0 {
		return nil, 0, nil
	}
	return deleteItemsWhereExec(exec, `id IN (
		SELECT id FROM clip_items
		WHERE pinned = 0
		ORDER BY updated_at DESC, id DESC
		LIMIT -1 OFFSET ?
	)`, maxHistory)
}

func (s *Store) deleteItemsWhere(where string, args ...any) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	paths, deleted, err := deleteItemsWhereExec(s.db, where, args...)
	if err != nil {
		return err
	}
	s.removeOrphanImages(paths)
	if deleted > 0 {
		s.bumpRevision(true)
	}
	return nil
}

func deleteItemsWhereExec(exec sqlExecer, where string, args ...any) ([]string, int64, error) {
	rows, err := exec.Query(`SELECT DISTINCT image_path FROM clip_items WHERE `+where+` AND image_path != ''`, args...)
	if err != nil {
		return nil, 0, err
	}
	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err == nil && p != "" {
			paths = append(paths, p)
		}
	}
	_ = rows.Close()

	res, err := exec.Exec(`DELETE FROM clip_items WHERE `+where, args...)
	if err != nil {
		return nil, 0, err
	}
	n, _ := res.RowsAffected()
	return paths, n, nil
}

func (s *Store) removeOrphanImages(paths []string) {
	for _, p := range paths {
		s.removeOrphanImage(p)
	}
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
