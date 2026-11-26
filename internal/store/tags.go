package store

import (
	"encoding/json"
	"sort"
	"strings"
)

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

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var curText, curHTML, curPreview string
	if err := tx.QueryRow(`SELECT text_content, html_content, preview FROM clip_items WHERE id = ?`, id).Scan(&curText, &curHTML, &curPreview); err != nil {
		return nil, err
	}

	nextText, nextHTML, nextPreview, err := encryptSensitiveFields(curText, curHTML, curPreview, HasSensitiveTag(cleaned))
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(
		`UPDATE clip_items SET tags_json = ?, text_content = ?, html_content = ?, preview = ? WHERE id = ?`,
		raw, nextText, nextHTML, nextPreview, id,
	); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
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
