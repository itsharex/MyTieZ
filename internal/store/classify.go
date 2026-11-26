package store

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
)

func hashBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func TextContentHash(text, html string) string {
	if html == "" {
		return hashBytes([]byte(text))
	}
	return hashBytes([]byte(text + "\x00html\x00" + html))
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

func cleanFilePaths(paths []string) []string {
	var clean []string
	for _, p := range paths {
		p = strings.TrimSpace(p)
		if p != "" {
			clean = append(clean, p)
		}
	}
	return clean
}

// FilesContentHash computes the canonical deduplication hash for a file list.
func FilesContentHash(paths []string) (string, error) {
	clean := cleanFilePaths(paths)
	if len(clean) == 0 {
		return "", fmt.Errorf("empty file list")
	}
	raw, err := json.Marshal(clean)
	if err != nil {
		return "", err
	}
	return hashBytes(raw), nil
}

func truncateRunes(s string, maxRunes int) string {
	n := 0
	for i := range s {
		if n == maxRunes {
			return s[:i]
		}
		n++
	}
	return s
}

func makeTextPreview(s string) string {
	const (
		maxLines = 5
		maxRunes = 220
	)
	var b strings.Builder
	lines := 0
	runes := 0
	truncated := false

	for i := 0; i < len(s) && lines < maxLines; {
		lineStart := i
		for i < len(s) && s[i] != '\n' && s[i] != '\r' {
			i++
		}
		line := s[lineStart:i]
		if i < len(s) {
			if s[i] == '\r' && i+1 < len(s) && s[i+1] == '\n' {
				i += 2
			} else {
				i++
			}
		}

		trimmed := strings.TrimRight(line, " \t")
		if lines == 0 {
			trimmed = strings.TrimLeft(trimmed, " \t")
		}
		if strings.TrimSpace(trimmed) == "" {
			continue
		}

		if lines > 0 {
			if runes >= maxRunes {
				truncated = true
				break
			}
			b.WriteByte('\n')
			runes++
		}
		lines++

		for idx := range trimmed {
			if runes == maxRunes {
				b.WriteString(trimmed[:idx])
				truncated = true
				break
			}
			runes++
		}
		if truncated {
			break
		}
		b.WriteString(trimmed)
	}
	if truncated {
		b.WriteString("…")
	}
	return b.String()
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
