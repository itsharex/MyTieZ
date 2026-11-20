package store

import (
	"regexp"
	"strings"
	"sync"
	"unicode"
)

const SensitiveTag = "sensitive"

var AllPrivacyKinds = []string{"phone", "idcard", "email", "url", "secret", "password"}

func DefaultPrivacyKinds() []string {
	return []string{"phone", "idcard", "email", "secret"}
}

var (
	rePrivacyURL    = regexp.MustCompile(`(?i)(?:[a-zA-Z][a-zA-Z0-9+\-.]*://|www\.)\S+`)
	rePrivacyPhone  = regexp.MustCompile(`(?:\+?86)?[-\s\(]*1[3-9]\d{1}[-\s\)]*\d{4}[-\s]*\d{4}`)
	rePrivacyIDCard = regexp.MustCompile(`\b[1-9]\d{5}[1-9]\d{3}((0\d)|(1[0-2]))(([0|1|2]\d)|3[0-1])\d{3}([0-9Xx])\b`)
	rePrivacyEmail  = regexp.MustCompile(`[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}`)
	rePrivacySecret = regexp.MustCompile(`(?i)((?:sk|pk|ghp|gho|github_pat|AIza|AKIA|ya29)[-_][\w\-]{20,}|(?:password|secret|api[_-]?key|access[_-]?key|token|bearer)[\s:=]+[\w\-]{16,})`)
)

// HasSensitiveTag reports whether tags mark an item as sensitive.
func HasSensitiveTag(tags []string) bool {
	for _, t := range tags {
		switch strings.ToLower(strings.TrimSpace(t)) {
		case "sensitive", "密码", "password":
			return true
		}
	}
	return false
}

var (
	customRulesMu     sync.RWMutex
	cachedCustomInput string
	cachedCustomRegex []*regexp.Regexp
)

func compiledCustomRules(customRules string) []*regexp.Regexp {
	if strings.TrimSpace(customRules) == "" {
		return nil
	}
	customRulesMu.RLock()
	if customRules == cachedCustomInput {
		res := cachedCustomRegex
		customRulesMu.RUnlock()
		return res
	}
	customRulesMu.RUnlock()

	var compiled []*regexp.Regexp
	for _, rawLine := range strings.Split(customRules, "\n") {
		rule := strings.TrimSpace(rawLine)
		if rule == "" || strings.HasPrefix(rule, "#") {
			continue
		}
		if re, err := regexp.Compile(rule); err == nil {
			compiled = append(compiled, re)
		}
	}

	customRulesMu.Lock()
	cachedCustomInput = customRules
	cachedCustomRegex = compiled
	customRulesMu.Unlock()
	return compiled
}

// InvalidCustomRuleLines returns 1-based line numbers of non-empty custom rules
// that fail Go regexp compilation.
func InvalidCustomRuleLines(customRules string) []int {
	var invalid []int
	for i, rawLine := range strings.Split(customRules, "\n") {
		rule := strings.TrimSpace(rawLine)
		if rule == "" || strings.HasPrefix(rule, "#") {
			continue
		}
		if _, err := regexp.Compile(rule); err != nil {
			invalid = append(invalid, i+1)
		}
	}
	return invalid
}

// ContainsSensitiveInfo checks text against enabled built-in kinds and custom regex lines.
func ContainsSensitiveInfo(text string, kinds []string, customRules string) bool {
	if len(text) > 5000 || strings.HasPrefix(text, "data:") {
		return false
	}

	hasKind := func(k string) bool {
		for _, item := range kinds {
			if item == k {
				return true
			}
		}
		return false
	}

	if hasKind("url") && rePrivacyURL.MatchString(text) {
		return true
	}
	if hasKind("phone") && rePrivacyPhone.MatchString(text) {
		return true
	}
	if hasKind("idcard") && rePrivacyIDCard.MatchString(text) {
		return true
	}
	if hasKind("email") && rePrivacyEmail.MatchString(text) {
		return true
	}
	if hasKind("secret") && rePrivacySecret.MatchString(text) {
		return true
	}
	if hasKind("password") && looksLikePassword(text) {
		return true
	}

	for _, re := range compiledCustomRules(customRules) {
		if re.MatchString(text) {
			return true
		}
	}
	return false
}

func looksLikePassword(text string) bool {
	if len(text) < 8 || len(text) > 64 || strings.ContainsAny(text, " \t\r\n") {
		return false
	}
	var hasUpper, hasLower, hasDigit, hasSpecial bool
	for _, r := range text {
		switch {
		case unicode.IsUpper(r):
			hasUpper = true
		case unicode.IsLower(r):
			hasLower = true
		case unicode.IsDigit(r):
			hasDigit = true
		default:
			hasSpecial = true
		}
	}
	return hasUpper && hasLower && hasDigit && hasSpecial
}

// MaskSensitiveText produces a masked preview of sensitive text for UI display.
func MaskSensitiveText(text string, prefix, suffix int, maskEmailDomain bool) string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return "••••••"
	}
	if prefix < 0 {
		prefix = 0
	}
	if suffix < 0 {
		suffix = 0
	}

	// Single email token gets domain-aware masking.
	if !strings.ContainsAny(trimmed, " \t\r\n") && rePrivacyEmail.MatchString(trimmed) {
		if at := strings.IndexByte(trimmed, '@'); at > 0 && at == strings.LastIndexByte(trimmed, '@') {
			local := []rune(trimmed[:at])
			domain := trimmed[at+1:]
			maskedLocal := maskRunes(local, min(prefix, max(1, len(local)-1)), 0)
			if maskEmailDomain {
				if dot := strings.LastIndexByte(domain, '.'); dot > 0 {
					domain = "•••" + domain[dot:]
				} else {
					domain = "••••"
				}
			}
			return maskedLocal + "@" + domain
		}
	}

	collapsed := strings.Join(strings.Fields(trimmed), " ")
	runes := []rune(collapsed)
	return maskRunes(runes, prefix, suffix)
}

func maskRunes(runes []rune, prefix, suffix int) string {
	n := len(runes)
	if n == 0 {
		return "••••••"
	}
	if n <= 4 || n <= prefix+suffix {
		return "••••••••"
	}
	var b strings.Builder
	if prefix > 0 {
		b.WriteString(string(runes[:prefix]))
	}
	b.WriteString("••••••")
	if suffix > 0 {
		b.WriteString(string(runes[n-suffix:]))
	}
	return b.String()
}
