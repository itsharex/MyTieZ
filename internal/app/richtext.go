package app

import (
	"encoding/binary"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"golang.org/x/net/html"
)

const (
	maxRichPreviewHTMLBytes = 16 * 1024
	maxRichPreviewSpans     = 48
	maxRichPreviewChars     = 400
	maxRichSpanCacheEntries = 128
)

type spanStyle struct {
	weight        int
	italic        bool
	underline     bool
	strikethrough bool
	code          bool
	color         ui.Color
	hasColor      bool
	bg            ui.Color
	hasBG         bool
}

// decodeClipboardHTML converts Windows UTF-16LE/BE clipboard HTML payloads
// (such as Firefox/Zen "text/html" format) into clean UTF-8 without NUL bytes.
func decodeClipboardHTML(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	var s string
	switch {
	case len(b) >= 2 && b[0] == 0xFF && b[1] == 0xFE:
		s = decodeUTF16Bytes(b[2:], binary.LittleEndian)
	case len(b) >= 2 && b[0] == 0xFE && b[1] == 0xFF:
		s = decodeUTF16Bytes(b[2:], binary.BigEndian)
	case looksLikeUTF16(b, true):
		s = decodeUTF16Bytes(b, binary.LittleEndian)
	case looksLikeUTF16(b, false):
		s = decodeUTF16Bytes(b, binary.BigEndian)
	default:
		s = string(b)
	}
	return strings.TrimRight(strings.ReplaceAll(s, "\x00", ""), "\r\n ")
}

func looksLikeUTF16(b []byte, little bool) bool {
	if len(b) < 4 || len(b)%2 != 0 {
		return false
	}
	if little && b[0] == '<' && b[1] == 0 {
		return true
	}
	if !little && b[0] == 0 && b[1] == '<' {
		return true
	}
	sample := min(len(b), 128)
	zeros := 0
	for i := 0; i+1 < sample; i += 2 {
		if little && b[i+1] == 0 && b[i] != 0 {
			zeros++
		} else if !little && b[i] == 0 && b[i+1] != 0 {
			zeros++
		}
	}
	return zeros*4 >= sample/2
}

func decodeUTF16Bytes(b []byte, order binary.ByteOrder) string {
	n := len(b) / 2
	u := make([]uint16, n)
	for i := 0; i < n; i++ {
		u[i] = order.Uint16(b[i*2 : i*2+2])
	}
	return string(utf16.Decode(u))
}

// parseHTMLPreviewSpans extracts lightweight inline-styled spans from clipboard HTML
// for card previews without generating screenshots or layout-breaking sizes.
func parseHTMLPreviewSpans(rawHTML string, t *ui.Theme) []ui.Span {
	frag := extractHTMLFragment(decodeClipboardHTML([]byte(rawHTML)))
	if frag == "" {
		return nil
	}
	if len(frag) > maxRichPreviewHTMLBytes {
		frag = truncateUTF8Bytes(frag, maxRichPreviewHTMLBytes)
	}
	doc, err := html.Parse(strings.NewReader(frag))
	if err != nil {
		return nil
	}

	var spans []ui.Span
	totalChars := 0
	var walk func(n *html.Node, st spanStyle)
	walk = func(n *html.Node, st spanStyle) {
		if len(spans) >= maxRichPreviewSpans || totalChars >= maxRichPreviewChars {
			return
		}
		switch n.Type {
		case html.TextNode:
			txt := collapsePreviewWhitespace(n.Data)
			if txt == "" {
				return
			}
			if txt == " " && (len(spans) == 0 || strings.HasSuffix(spans[len(spans)-1].Text, " ")) {
				return
			}
			if remaining := maxRichPreviewChars - totalChars; len([]rune(txt)) > remaining {
				r := []rune(txt)
				txt = string(r[:remaining]) + "…"
			}
			totalChars += len([]rune(txt))
			sp := buildUISpan(txt, st, t)
			if len(spans) > 0 && sameSpanStyle(spans[len(spans)-1], sp) {
				spans[len(spans)-1].Text += sp.Text
			} else {
				spans = append(spans, sp)
			}
			return
		case html.ElementNode:
			tag := strings.ToLower(n.Data)
			if tag == "script" || tag == "style" || tag == "head" || tag == "meta" || tag == "title" || tag == "noscript" {
				return
			}
			if tag == "br" {
				appendSpaceSpan(&spans, &totalChars, st, t)
				return
			}
			next := applyTagAndStyle(st, tag, n.Attr, t)
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c, next)
			}
			if isBlockHTMLTag(tag) {
				appendSpaceSpan(&spans, &totalChars, st, t)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, st)
		}
	}

	walk(doc, spanStyle{})
	for len(spans) > 0 && strings.TrimSpace(spans[0].Text) == "" {
		spans = spans[1:]
	}
	if len(spans) > 0 {
		spans[0].Text = strings.TrimLeft(spans[0].Text, " ")
		last := len(spans) - 1
		spans[last].Text = strings.TrimRight(spans[last].Text, " ")
		if spans[last].Text == "" {
			spans = spans[:last]
		}
	}
	return spans
}

func truncateUTF8Bytes(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	n := maxBytes
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func extractHTMLFragment(raw string) string {
	const startMarker = "<!--StartFragment-->"
	const endMarker = "<!--EndFragment-->"
	if i := strings.Index(raw, startMarker); i >= 0 {
		s := raw[i+len(startMarker):]
		if j := strings.Index(s, endMarker); j >= 0 {
			s = s[:j]
		}
		return strings.TrimSpace(s)
	}
	if i := strings.Index(strings.ToLower(raw), "<html"); i >= 0 {
		return strings.TrimSpace(raw[i:])
	}
	raw = stripCFHTMLHeaders(raw)
	if strings.Contains(raw, "<") {
		return strings.TrimSpace(raw)
	}
	return ""
}

func stripCFHTMLHeaders(raw string) string {
	head := raw
	if len(head) > 512 {
		head = head[:512]
	}
	if !strings.HasPrefix(head, "Version:") || (!strings.Contains(head, "StartHTML:") && !strings.Contains(head, "StartFragment:")) {
		return raw
	}
	s := raw
	for s != "" {
		line, rest, hasNL := strings.Cut(s, "\n")
		if !isCFHTMLHeaderLine(strings.TrimRight(line, "\r")) {
			return s
		}
		if !hasNL {
			return ""
		}
		s = rest
	}
	return ""
}

func isCFHTMLHeaderLine(line string) bool {
	for _, prefix := range []string{
		"Version:",
		"StartHTML:",
		"EndHTML:",
		"StartFragment:",
		"EndFragment:",
		"StartSelection:",
		"EndSelection:",
		"SourceURL:",
	} {
		if strings.HasPrefix(line, prefix) {
			return true
		}
	}
	return false
}

func collapsePreviewWhitespace(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	inSpace := false
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' || r == '\r' {
			if !inSpace {
				b.WriteByte(' ')
				inSpace = true
			}
			continue
		}
		inSpace = false
		b.WriteRune(r)
	}
	return b.String()
}

func appendSpaceSpan(spans *[]ui.Span, totalChars *int, st spanStyle, t *ui.Theme) {
	if len(*spans) == 0 || *totalChars >= maxRichPreviewChars {
		return
	}
	last := &(*spans)[len(*spans)-1]
	if strings.HasSuffix(last.Text, " ") {
		return
	}
	last.Text += " "
	*totalChars++
	_ = st
	_ = t
}

func isBlockHTMLTag(tag string) bool {
	switch tag {
	case "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "pre", "section", "article":
		return true
	default:
		return false
	}
}

func applyTagAndStyle(st spanStyle, tag string, attrs []html.Attribute, t *ui.Theme) spanStyle {
	switch tag {
	case "b", "strong", "h1", "h2", "h3", "h4", "h5", "h6":
		st.weight = 700
	case "i", "em":
		st.italic = true
	case "u", "ins":
		st.underline = true
	case "s", "del", "strike":
		st.strikethrough = true
	case "code", "pre", "kbd":
		st.code = true
	case "a":
		st.color = t.Accent
		st.hasColor = true
		st.underline = true
	}

	for _, a := range attrs {
		if !strings.EqualFold(a.Key, "style") {
			continue
		}
		for _, decl := range strings.Split(a.Val, ";") {
			k, v, ok := strings.Cut(decl, ":")
			if !ok {
				continue
			}
			key := strings.ToLower(strings.TrimSpace(k))
			val := strings.TrimSpace(v)
			switch key {
			case "font-weight":
				if strings.Contains(strings.ToLower(val), "bold") {
					st.weight = 700
				} else if w, err := strconv.Atoi(val); err == nil && w >= 600 {
					st.weight = 700
				} else if strings.EqualFold(val, "normal") || val == "400" {
					st.weight = 400
				}
			case "font-style":
				lval := strings.ToLower(val)
				if strings.Contains(lval, "italic") || strings.Contains(lval, "oblique") {
					st.italic = true
				} else if strings.Contains(lval, "normal") {
					st.italic = false
				}
			case "text-decoration", "text-decoration-line":
				lval := strings.ToLower(val)
				if strings.Contains(lval, "underline") {
					st.underline = true
				}
				if strings.Contains(lval, "line-through") {
					st.strikethrough = true
				}
			case "color":
				if col, ok := parseCSSColor(val); ok && isReadableForeground(col, t.Dark) {
					st.color = col
					st.hasColor = true
				}
			case "background-color", "background":
				if col, ok := parseCSSColor(val); ok && col.A > 20 {
					if col.A > 110 {
						col.A = 110
					}
					st.bg = col
					st.hasBG = true
				}
			}
		}
	}
	return st
}

func buildUISpan(txt string, st spanStyle, t *ui.Theme) ui.Span {
	sp := ui.Span{
		Text:          txt,
		Weight:        st.weight,
		Italic:        st.italic,
		Underline:     st.underline,
		Strikethrough: st.strikethrough,
	}
	if st.code {
		sp.Font = "Consolas"
		sp.Background = t.Text.Alpha(0.08)
		sp.Radius = 3
	}
	if st.hasColor {
		sp.Color = st.color
	}
	if st.hasBG {
		sp.Background = st.bg
		sp.Radius = 3
	}
	return sp
}

func sameSpanStyle(a, b ui.Span) bool {
	return a.Font == b.Font &&
		a.Weight == b.Weight &&
		a.Italic == b.Italic &&
		a.Underline == b.Underline &&
		a.Strikethrough == b.Strikethrough &&
		a.Color == b.Color &&
		a.Background == b.Background &&
		a.Radius == b.Radius
}

func isReadableForeground(col ui.Color, dark bool) bool {
	// Perceived luminance in 0..255.
	lum := (299*int(col.R) + 587*int(col.G) + 114*int(col.B)) / 1000
	if dark {
		return lum >= 65
	}
	return lum <= 215
}

func parseCSSColor(raw string) (ui.Color, bool) {
	s := strings.ToLower(strings.TrimSpace(raw))
	if s == "" || s == "inherit" || s == "initial" || s == "transparent" || s == "none" {
		return ui.Color{}, false
	}
	if strings.HasPrefix(s, "#") {
		hex := s[1:]
		if len(hex) == 3 {
			r, err1 := strconv.ParseUint(strings.Repeat(hex[0:1], 2), 16, 8)
			g, err2 := strconv.ParseUint(strings.Repeat(hex[1:2], 2), 16, 8)
			b, err3 := strconv.ParseUint(strings.Repeat(hex[2:3], 2), 16, 8)
			if err1 == nil && err2 == nil && err3 == nil {
				return ui.RGB(uint8(r), uint8(g), uint8(b)), true
			}
		} else if len(hex) == 6 {
			r, err1 := strconv.ParseUint(hex[0:2], 16, 8)
			g, err2 := strconv.ParseUint(hex[2:4], 16, 8)
			b, err3 := strconv.ParseUint(hex[4:6], 16, 8)
			if err1 == nil && err2 == nil && err3 == nil {
				return ui.RGB(uint8(r), uint8(g), uint8(b)), true
			}
		}
		return ui.Color{}, false
	}
	if strings.HasPrefix(s, "rgb(") || strings.HasPrefix(s, "rgba(") {
		inner := strings.TrimSuffix(s[strings.IndexByte(s, '(')+1:], ")")
		parts := strings.Split(inner, ",")
		if len(parts) < 3 {
			return ui.Color{}, false
		}
		r, err1 := strconv.Atoi(strings.TrimSpace(parts[0]))
		g, err2 := strconv.Atoi(strings.TrimSpace(parts[1]))
		b, err3 := strconv.Atoi(strings.TrimSpace(parts[2]))
		if err1 != nil || err2 != nil || err3 != nil {
			return ui.Color{}, false
		}
		a := float64(1)
		if len(parts) >= 4 {
			if af, err := strconv.ParseFloat(strings.TrimSpace(parts[3]), 64); err == nil {
				a = af
			}
		}
		if a <= 0 {
			return ui.Color{}, false
		}
		return ui.RGBA(clampByte(r), clampByte(g), clampByte(b), float32(a)), true
	}
	switch s {
	case "black":
		return ui.RGB(0, 0, 0), true
	case "white":
		return ui.RGB(255, 255, 255), true
	case "red":
		return ui.RGB(220, 38, 38), true
	case "blue":
		return ui.RGB(37, 99, 235), true
	case "green":
		return ui.RGB(22, 163, 74), true
	case "orange":
		return ui.RGB(234, 88, 12), true
	case "purple":
		return ui.RGB(147, 51, 234), true
	case "yellow":
		return ui.RGB(234, 179, 8), true
	case "gray", "grey":
		return ui.RGB(107, 114, 128), true
	}
	return ui.Color{}, false
}

func clampByte(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}
