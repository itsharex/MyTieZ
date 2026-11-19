package store

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func isASCIIAlphaNum(s string) bool {
	if s == "" {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		isAlpha := (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
		isDigit := c >= '0' && c <= '9'
		if !isAlpha && !isDigit {
			return false
		}
	}
	return true
}

func runePinyins(r rune) string {
	if r < 0x4E00 || r > 0x9FFF {
		return ""
	}
	idx := pinyinIndex[r-0x4E00]
	if idx == 0 {
		return ""
	}
	return pinyinDict[idx]
}

// matchPinyin reports whether query (ASCII letters/digits) matches text
// by case-insensitive substring, full pinyin, initials, or mixed prefix.
func matchPinyin(text, query string) bool {
	if query == "" {
		return true
	}
	q := strings.ToLower(query)
	if strings.Contains(strings.ToLower(text), q) {
		return true
	}

	const maxRunes = 2048
	runes := make([]rune, 0, min(len(text), maxRunes))
	for _, r := range text {
		runes = append(runes, r)
		if len(runes) >= maxRunes {
			break
		}
	}
	for i := range runes {
		if matchPinyinAt(runes[i:], q) {
			return true
		}
	}
	return false
}

func matchPinyinAt(runes []rune, q string) bool {
	if q == "" {
		return true
	}
	if len(runes) == 0 {
		return false
	}
	r := runes[0]
	pys := runePinyins(r)
	if pys == "" {
		lr := unicode.ToLower(r)
		qr, size := utf8.DecodeRuneInString(q)
		if lr != qr {
			return false
		}
		return matchPinyinAt(runes[1:], q[size:])
	}

	for pys != "" {
		var py string
		if comma := strings.IndexByte(pys, ','); comma >= 0 {
			py = pys[:comma]
			pys = pys[comma+1:]
		} else {
			py = pys
			pys = ""
		}
		if py == "" {
			continue
		}
		// Full pinyin match or trailing prefix match (e.g. "weix" -> "微信")
		if strings.HasPrefix(q, py) {
			if matchPinyinAt(runes[1:], q[len(py):]) {
				return true
			}
		} else if strings.HasPrefix(py, q) {
			return true
		}
		// Initial match (e.g. "wx" -> "微信")
		if q[0] == py[0] && matchPinyinAt(runes[1:], q[1:]) {
			return true
		}
	}
	return false
}

func itemMatchesKeyword(it ClipItem, kw string) bool {
	if matchPinyin(it.Text, kw) ||
		matchPinyin(it.Preview, kw) ||
		matchPinyin(it.SourceApp, kw) {
		return true
	}
	for _, f := range it.Files {
		if matchPinyin(f, kw) {
			return true
		}
	}
	for _, tag := range it.Tags {
		if matchPinyin(tag, kw) {
			return true
		}
	}
	return false
}
