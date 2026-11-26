package store

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"
)

func (s *Store) UpsertText(text string) (ClipItem, error) {
	return s.SaveText(text, SaveOptions{Deduplicate: true})
}

func (s *Store) UpsertImage(pngData []byte) (ClipItem, error) {
	return s.SaveImage(pngData, SaveOptions{Deduplicate: true})
}

func (s *Store) UpsertFiles(paths []string) (ClipItem, error) {
	return s.SaveFiles(paths, SaveOptions{Deduplicate: true})
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
	clean := cleanFilePaths(paths)
	if len(clean) == 0 {
		return ClipItem{}, fmt.Errorf("empty file list")
	}
	h, err := FilesContentHash(clean)
	if err != nil {
		return ClipItem{}, err
	}
	names := make([]string, 0, len(clean))
	for _, p := range clean {
		names = append(names, filepath.Base(p))
	}
	return s.save(ClipItem{
		Kind:     KindFile,
		Hash:     h,
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

	saved := ClipItem{
		Kind:       item.Kind,
		Hash:       item.Hash,
		Text:       item.Text,
		HTML:       item.HTML,
		ImagePath:  item.ImagePath,
		Preview:    item.Preview,
		SourceApp:  sourceApp,
		SourcePath: sourcePath,
		ByteSize:   item.ByteSize,
		UpdatedAt:  time.UnixMilli(now),
	}
	if len(item.Files) > 0 {
		saved.Files = append([]string(nil), item.Files...)
	}

	var targetID int64
	if opts.Deduplicate {
		var existingID, existingCreatedMs int64
		var existingPinned int
		var existingApp, existingPath, existingTagsJSON string
		err := tx.QueryRow(`
SELECT id, source_app, source_path, tags_json, created_at, pinned FROM clip_items WHERE kind = ? AND content_hash = ? ORDER BY updated_at DESC, id DESC LIMIT 1
`, string(item.Kind), item.Hash).Scan(&existingID, &existingApp, &existingPath, &existingTagsJSON, &existingCreatedMs, &existingPinned)
		if err == nil {
			keepApp, keepPath := sourceApp, sourcePath
			if keepPath == "" && existingPath != "" {
				keepApp, keepPath = existingApp, existingPath
			}
			mergedTagsJSON := existingTagsJSON
			var mergedTags []string
			if existingTagsJSON != "" && existingTagsJSON != "[]" {
				_ = json.Unmarshal([]byte(existingTagsJSON), &mergedTags)
			}
			if len(opts.Tags) > 0 {
				mergedTags = normalizeTags(append(mergedTags, opts.Tags...))
				if b, err := json.Marshal(mergedTags); err == nil {
					mergedTagsJSON = string(b)
				}
			}
			storeText, storeHTML, storePreview, err := encryptSensitiveFields(item.Text, item.HTML, item.Preview, HasSensitiveTag(mergedTags))
			if err != nil {
				return ClipItem{}, err
			}
			if _, err := tx.Exec(`
UPDATE clip_items
SET text_content = ?, html_content = ?, image_path = ?, files_json = ?, preview = ?, source_app = ?, source_path = ?, byte_size = ?, tags_json = ?, updated_at = ?
WHERE id = ?
`, storeText, storeHTML, item.ImagePath, filesJSON, storePreview, keepApp, keepPath, item.ByteSize, mergedTagsJSON, now, existingID); err != nil {
				return ClipItem{}, err
			}
			targetID = existingID
			saved.ID = existingID
			saved.SourceApp = keepApp
			saved.SourcePath = keepPath
			saved.Pinned = existingPinned != 0
			saved.CreatedAt = time.UnixMilli(existingCreatedMs)
			if len(mergedTags) > 0 {
				saved.Tags = mergedTags
			}
		} else if err != sql.ErrNoRows {
			return ClipItem{}, err
		}
	}

	if targetID == 0 {
		tagsJSON := "[]"
		cleanTags := normalizeTags(opts.Tags)
		if len(cleanTags) > 0 {
			if b, err := json.Marshal(cleanTags); err == nil {
				tagsJSON = string(b)
			}
		}
		storeText, storeHTML, storePreview, err := encryptSensitiveFields(item.Text, item.HTML, item.Preview, HasSensitiveTag(cleanTags))
		if err != nil {
			return ClipItem{}, err
		}
		res, err := tx.Exec(`
INSERT INTO clip_items (kind, content_hash, text_content, html_content, image_path, files_json, preview, source_app, source_path, byte_size, pinned, tags_json, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, ?, ?)
`, string(item.Kind), item.Hash, storeText, storeHTML, item.ImagePath, filesJSON, storePreview, sourceApp, sourcePath, item.ByteSize, tagsJSON, now, now)
		if err != nil {
			return ClipItem{}, err
		}
		targetID, err = res.LastInsertId()
		if err != nil {
			return ClipItem{}, err
		}
		saved.ID = targetID
		saved.CreatedAt = time.UnixMilli(now)
		if len(cleanTags) > 0 {
			saved.Tags = cleanTags
		}
	}

	var orphanPaths []string
	var deleted int64
	if opts.MaxHistory > 0 {
		var err error
		orphanPaths, deleted, err = enforceLimitExec(tx, opts.MaxHistory)
		if err != nil {
			return ClipItem{}, err
		}
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
		conds = append(conds, "EXISTS (SELECT 1 FROM json_each(tags_json) WHERE value = ?)")
		args = append(args, tag)
	}
	pinyinSearch := isASCIIAlphaNum(kw)
	if kw != "" && !pinyinSearch {
		like := "%" + escapeLike(kw) + "%"
		conds = append(conds, "((text_content LIKE ? ESCAPE '\\' OR preview LIKE ? ESCAPE '\\' OR files_json LIKE ? ESCAPE '\\' OR source_app LIKE ? ESCAPE '\\' OR tags_json LIKE ? ESCAPE '\\') OR text_content LIKE ? OR preview LIKE ?)")
		encPrefixLike := EncryptPrefix + "%"
		args = append(args, like, like, like, like, like, encPrefixLike, encPrefixLike)
	}

	where := ""
	if len(conds) > 0 {
		where = "WHERE " + strings.Join(conds, " AND ")
	}
	limitClause := ""
	if !pinyinSearch && kw == "" {
		limitClause = "\nLIMIT ?"
		args = append(args, limit)
	}

	cols := "id, kind, content_hash, text_content, html_content, image_path, files_json, preview, source_app, source_path, byte_size, pinned, tags_json, created_at, updated_at"
	if summary {
		textCol := "CASE WHEN text_content LIKE 'dpapi:%' OR length(text_content) <= 1024 THEN text_content ELSE '' END"
		if pinyinSearch || kw != "" {
			// 关键词/拼音搜索时加密字段需完整取出以便解密后匹配，未加密字段限制在 8192 字符以内
			textCol = "CASE WHEN text_content LIKE 'dpapi:%' THEN text_content ELSE substr(text_content, 1, 8192) END"
		}
		cols = "id, kind, content_hash, " + textCol + ", CASE WHEN kind = 'rich_text' THEN (CASE WHEN html_content LIKE 'dpapi:%' THEN html_content ELSE substr(html_content, 1, 8192) END) ELSE '' END, image_path, files_json, preview, source_app, source_path, byte_size, pinned, tags_json, created_at, updated_at"
	}

	rows, err := s.db.Query(`
SELECT `+cols+`
FROM clip_items
`+where+`
ORDER BY pinned DESC, pin_order ASC, updated_at DESC, id DESC`+limitClause, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []ClipItem
	for rows.Next() {
		it, err := scanRawItem(rows)
		if err != nil {
			return nil, err
		}
		// 过滤阶段先仅解密 Text 与 Preview，避免对未命中的富文本条目做无谓的 HTML DPAPI 解密
		if it.Text, err = decryptString(it.Text); err != nil {
			continue
		}
		if it.Preview, err = decryptString(it.Preview); err != nil {
			continue
		}
		if pinyinSearch && !itemMatchesKeyword(it, kw) {
			continue
		} else if !pinyinSearch && kw != "" && !itemMatchesPlainKeyword(it, kw) {
			continue
		}
		if it.HTML, err = decryptString(it.HTML); err != nil {
			continue
		}
		if summary && len(it.Text) > 1024 && utf8.RuneCountInString(it.Text) > 1024 {
			it.Text = ""
		}
		if summary && it.Kind == KindRichText && len(it.HTML) > 8192 && utf8.RuneCountInString(it.HTML) > 8192 {
			it.HTML = truncateRunes(it.HTML, 8192)
		}
		items = append(items, it)
		if len(items) >= limit {
			break
		}
	}
	return items, rows.Err()
}

func escapeLike(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}

func itemMatchesPlainKeyword(it ClipItem, kw string) bool {
	lowerKW := strings.ToLower(kw)
	if strings.Contains(strings.ToLower(it.Text), lowerKW) ||
		strings.Contains(strings.ToLower(it.Preview), lowerKW) ||
		strings.Contains(strings.ToLower(it.SourceApp), lowerKW) {
		return true
	}
	for _, f := range it.Files {
		if strings.Contains(strings.ToLower(f), lowerKW) {
			return true
		}
	}
	for _, tag := range it.Tags {
		if strings.Contains(strings.ToLower(tag), lowerKW) {
			return true
		}
	}
	return false
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
	var pinned int
	if err := s.db.QueryRow(`SELECT pinned FROM clip_items WHERE id = ?`, id).Scan(&pinned); err != nil {
		return false, err
	}
	if pinned == 0 {
		if _, err := s.db.Exec(
			`UPDATE clip_items SET pinned = 1, pin_order = COALESCE((SELECT MIN(pin_order) FROM clip_items WHERE pinned = 1), 0) - 1 WHERE id = ?`,
			id,
		); err != nil {
			return false, err
		}
		s.bumpRevision(false)
		return true, nil
	}
	if _, err := s.db.Exec(`UPDATE clip_items SET pinned = 0, pin_order = 0 WHERE id = ?`, id); err != nil {
		return false, err
	}
	s.bumpRevision(false)
	return false, nil
}

// ReorderPinned persists the relative order of the given pinned item IDs while
// preserving the slots of any pinned items not included in orderedIDs (e.g. when
// dragging inside a filtered view).
func (s *Store) ReorderPinned(orderedIDs []int64) error {
	if len(orderedIDs) == 0 {
		return nil
	}
	s.writeMu.Lock()
	defer s.writeMu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.Query(`SELECT id FROM clip_items WHERE pinned = 1 ORDER BY pin_order ASC, updated_at DESC, id DESC`)
	if err != nil {
		return err
	}
	var allPinned []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			_ = rows.Close()
			return err
		}
		allPinned = append(allPinned, id)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	_ = rows.Close()

	subset := make(map[int64]struct{}, len(orderedIDs))
	for _, id := range orderedIDs {
		subset[id] = struct{}{}
	}
	orderedIdx := 0
	for i, id := range allPinned {
		if _, ok := subset[id]; ok && orderedIdx < len(orderedIDs) {
			allPinned[i] = orderedIDs[orderedIdx]
			orderedIdx++
		}
	}

	stmt, err := tx.Prepare(`UPDATE clip_items SET pin_order = ? WHERE id = ? AND pinned = 1`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for i, id := range allPinned {
		if _, err := stmt.Exec(i+1, id); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	s.bumpRevision(false)
	return nil
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

const enforceLimitWhere = `id IN (
		SELECT id FROM clip_items
		WHERE pinned = 0
		ORDER BY updated_at DESC, id DESC
		LIMIT -1 OFFSET ?
	)`

// EnforceLimit trims unpinned items beyond maxHistory (keeping pinned items untouched).
func (s *Store) EnforceLimit(maxHistory int) error {
	if maxHistory <= 0 {
		return nil
	}
	return s.deleteItemsWhere(enforceLimitWhere, maxHistory)
}

func enforceLimitExec(exec sqlExecer, maxHistory int) ([]string, int64, error) {
	if maxHistory <= 0 {
		return nil, 0, nil
	}
	return deleteItemsWhereExec(exec, enforceLimitWhere, maxHistory)
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

// ThumbPath returns the on-disk thumbnail cache path corresponding to imagePath.
func ThumbPath(imagePath string) string {
	if imagePath == "" {
		return ""
	}
	base := filepath.Base(imagePath)
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if stem == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(imagePath), "thumbs", stem+".png")
}

func (s *Store) removeOrphanImage(imgPath string) {
	if imgPath == "" {
		return
	}
	var exists int
	if err := s.db.QueryRow(`SELECT EXISTS(SELECT 1 FROM clip_items WHERE image_path = ? LIMIT 1)`, imgPath).Scan(&exists); err == nil && exists == 0 {
		_ = os.Remove(imgPath)
		if tp := ThumbPath(imgPath); tp != "" {
			_ = os.Remove(tp)
		}
	}
}
