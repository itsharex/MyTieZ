package app

import (
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"github.com/egoist/mygo/ui"
	"golang.org/x/net/html"
)

const (
	maxRichPreviewHTMLBytes = 32 * 1024
	maxRichCheckHTMLBytes   = 64 * 1024
	maxRichPreviewSpans     = 160
	maxRichPreviewChars     = 1000
	maxRichSpanCacheEntries = 128
)

type spanStyle struct {
	weight        int
	size          float32
	baselineShift float32
	italic        bool
	underline     bool
	strikethrough bool
	code          bool
	kbd           bool
	inPre         bool
	inQuote       bool
	color         ui.Color
	hasColor      bool
	bg            ui.Color
	hasBG         bool
	paddingX      float32
	radius        float32
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

type listState struct {
	ordered bool
	index   int
}

func maybeDecodeClipboardHTML(raw string) string {
	if len(raw) >= 2 && ((raw[0] == 0xFF && raw[1] == 0xFE) || (raw[0] == 0xFE && raw[1] == 0xFF)) {
		return decodeClipboardHTML([]byte(raw))
	}
	if strings.IndexByte(raw, 0) >= 0 {
		return decodeClipboardHTML([]byte(raw))
	}
	return raw
}

// parseHTMLPreviewSpans extracts lightweight inline-styled spans from clipboard HTML
// for card previews without generating screenshots or layout-breaking sizes.
func parseHTMLPreviewSpans(rawHTML string, t *ui.Theme) []ui.Span {
	frag := extractHTMLFragment(maybeDecodeClipboardHTML(rawHTML))
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
	var listStack []listState

	appendText := func(txt string, st spanStyle) {
		if txt == "" || len(spans) >= maxRichPreviewSpans || totalChars >= maxRichPreviewChars {
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
	}

	var walk func(n *html.Node, st spanStyle)
	walk = func(n *html.Node, st spanStyle) {
		if len(spans) >= maxRichPreviewSpans || totalChars >= maxRichPreviewChars {
			return
		}
		switch n.Type {
		case html.TextNode:
			var txt string
			if st.inPre {
				txt = normalizePreText(n.Data)
			} else {
				txt = collapsePreviewWhitespace(n.Data)
			}
			if txt == "" {
				return
			}
			if txt == " " {
				if len(spans) == 0 {
					return
				}
				prev := spans[len(spans)-1].Text
				if strings.HasSuffix(prev, " ") || strings.HasSuffix(prev, "\n") {
					return
				}
			}
			if strings.HasPrefix(txt, " ") && len(spans) > 0 && strings.HasSuffix(spans[len(spans)-1].Text, "\n") {
				txt = strings.TrimLeft(txt, " ")
				if txt == "" {
					return
				}
			}
			appendText(txt, st)
			return
		case html.ElementNode:
			tag := strings.ToLower(n.Data)
			if tag == "script" || tag == "style" || tag == "head" || tag == "meta" || tag == "title" || tag == "noscript" || tag == "link" {
				return
			}
			if tag == "br" {
				appendLineBreakSpan(&spans, &totalChars)
				return
			}
			if tag == "hr" {
				appendLineBreakSpan(&spans, &totalChars)
				appendText("───", spanStyle{color: t.TextMuted, hasColor: true})
				appendLineBreakSpan(&spans, &totalChars)
				return
			}

			switch tag {
			case "ul":
				listStack = append(listStack, listState{ordered: false})
				defer func() { listStack = listStack[:len(listStack)-1] }()
			case "ol":
				listStack = append(listStack, listState{ordered: true})
				defer func() { listStack = listStack[:len(listStack)-1] }()
			case "li":
				appendLineBreakSpan(&spans, &totalChars)
				indent := ""
				if len(listStack) > 1 {
					indent = strings.Repeat("  ", min(len(listStack)-1, 3))
				}
				marker := "• "
				if n := len(listStack); n > 0 && listStack[n-1].ordered {
					listStack[n-1].index++
					marker = strconv.Itoa(listStack[n-1].index) + ". "
				}
				appendText(indent+marker, spanStyle{color: t.TextMuted, hasColor: true, weight: 600})
			case "blockquote":
				appendLineBreakSpan(&spans, &totalChars)
				appendText("▎ ", spanStyle{color: t.Accent.Alpha(0.75), hasColor: true, weight: 700})
			}

			next := applyTagAndStyle(st, tag, n.Attr, t)
			firstCell := true
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if (tag == "tr") && c.Type == html.ElementNode {
					ctag := strings.ToLower(c.Data)
					if ctag == "td" || ctag == "th" {
						if !firstCell {
							appendText(" │ ", spanStyle{color: t.TextMuted, hasColor: true})
						}
						firstCell = false
					}
				}
				walk(c, next)
			}
			if isBlockHTMLTag(tag) {
				appendLineBreakSpan(&spans, &totalChars)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, st)
		}
	}

	walk(doc, spanStyle{})
	for len(spans) > 0 && strings.Trim(spans[0].Text, " \n") == "" {
		spans = spans[1:]
	}
	if len(spans) > 0 {
		spans[0].Text = strings.TrimLeft(spans[0].Text, " \n")
		for len(spans) > 0 {
			last := len(spans) - 1
			spans[last].Text = strings.TrimRight(spans[last].Text, " \n")
			if spans[last].Text != "" {
				break
			}
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

func normalizePreText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	return strings.ReplaceAll(s, "\t", "  ")
}

func appendLineBreakSpan(spans *[]ui.Span, totalChars *int) {
	if len(*spans) == 0 || *totalChars >= maxRichPreviewChars {
		return
	}
	last := &(*spans)[len(*spans)-1]
	last.Text = strings.TrimRight(last.Text, " ")
	if strings.HasSuffix(last.Text, "\n") {
		return
	}
	if last.Text == "" && len(*spans) > 1 {
		*spans = (*spans)[:len(*spans)-1]
		last = &(*spans)[len(*spans)-1]
		if strings.HasSuffix(last.Text, "\n") {
			return
		}
	}
	last.Text += "\n"
	*totalChars++
}

func isBlockHTMLTag(tag string) bool {
	switch tag {
	case "p", "div", "li", "tr", "h1", "h2", "h3", "h4", "h5", "h6", "blockquote", "pre", "section", "article", "dt", "dd":
		return true
	default:
		return false
	}
}

func applyTagAndStyle(st spanStyle, tag string, attrs []html.Attribute, t *ui.Theme) spanStyle {
	switch tag {
	case "h1":
		st.weight = 700
		st.size = 15.5
	case "h2":
		st.weight = 700
		st.size = 14.5
	case "h3", "h4", "h5", "h6", "b", "strong", "th":
		st.weight = 700
	case "i", "em", "cite":
		st.italic = true
	case "u", "ins":
		st.underline = true
	case "s", "del", "strike":
		st.strikethrough = true
	case "mark":
		st.bg = ui.RGBA(250, 204, 21, 0.28)
		if t.Dark {
			st.bg = ui.RGBA(250, 204, 21, 0.22)
		}
		st.hasBG = true
		st.paddingX = 2
		st.radius = 3
	case "sub":
		st.size = 11
		st.baselineShift = -2.5
	case "sup":
		st.size = 11
		st.baselineShift = 3.5
	case "blockquote":
		st.inQuote = true
		if !st.hasColor {
			st.color = t.TextMuted
			st.hasColor = true
		}
	case "pre":
		st.inPre = true
		st.code = true
	case "code", "tt", "samp":
		st.code = true
	case "kbd":
		st.kbd = true
		st.code = true
	case "a":
		st.color = t.Accent
		st.hasColor = true
		st.underline = true
	case "font":
		if colStr := attrValue(attrs, "color"); colStr != "" {
			if r, g, b, a, ok := parseCSSColorRGBA(colStr); ok {
				col := ui.RGBA(r, g, b, float32(a)/255)
				if isReadableForeground(col, t.Dark) {
					st.color = col
					st.hasColor = true
				}
			}
		}
	}

	forEachInlineStyle(attrs, func(key, val string) {
		lval := strings.ToLower(val)
		switch key {
		case "font-weight":
			if strings.Contains(lval, "bold") {
				st.weight = 700
			} else if w, err := strconv.Atoi(val); err == nil && w >= 600 {
				st.weight = 700
			} else if lval == "normal" || lval == "400" || lval == "300" || lval == "lighter" {
				st.weight = 400
			}
		case "font-style":
			if strings.Contains(lval, "italic") || strings.Contains(lval, "oblique") {
				st.italic = true
			} else if strings.Contains(lval, "normal") {
				st.italic = false
			}
		case "text-decoration", "text-decoration-line":
			if strings.Contains(lval, "underline") {
				st.underline = true
			}
			if strings.Contains(lval, "line-through") {
				st.strikethrough = true
			}
			if lval == "none" {
				st.underline = false
				st.strikethrough = false
			}
		case "color":
			if r, g, b, a, ok := parseCSSColorRGBA(val); ok {
				col := ui.RGBA(r, g, b, float32(a)/255)
				if isReadableForeground(col, t.Dark) {
					st.color = col
					st.hasColor = true
				}
			}
		case "background-color", "background":
			if r, g, b, a, ok := parseCSSBackgroundColorRGBA(val); ok && !isPageCanvasColor(r, g, b, a) {
				if a > 110 {
					a = 110
				}
				st.bg = ui.RGBA(r, g, b, float32(a)/255)
				st.hasBG = true
			}
		}
	})
	return st
}

func buildUISpan(txt string, st spanStyle, t *ui.Theme) ui.Span {
	sp := ui.Span{
		Text:          txt,
		Size:          st.size,
		BaselineShift: st.baselineShift,
		Weight:        st.weight,
		Italic:        st.italic,
		Underline:     st.underline,
		Strikethrough: st.strikethrough,
		PaddingX:      st.paddingX,
		Radius:        st.radius,
	}
	if st.code {
		sp.Font = "Consolas"
		if sp.Size == 0 {
			sp.Size = 13.5
		}
		if !st.inPre {
			sp.Background = t.Text.Alpha(0.08)
			if st.kbd {
				sp.Background = t.Text.Alpha(0.12)
				sp.Weight = 600
				sp.PaddingX = 4
				sp.Radius = 4
			} else {
				sp.PaddingX = 3
				sp.Radius = 3
			}
		}
	}
	if st.hasColor {
		sp.Color = st.color
	}
	if st.hasBG {
		sp.Background = st.bg
		if sp.Radius == 0 {
			sp.Radius = 3
		}
	}
	return sp
}

func sameSpanStyle(a, b ui.Span) bool {
	return a.Font == b.Font &&
		a.Size == b.Size &&
		a.BaselineShift == b.BaselineShift &&
		a.Weight == b.Weight &&
		a.Italic == b.Italic &&
		a.Underline == b.Underline &&
		a.Strikethrough == b.Strikethrough &&
		a.Color == b.Color &&
		a.Background == b.Background &&
		a.PaddingX == b.PaddingX &&
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

func extractHTMLFragment(raw string) string {
	const startMarker = "<!--StartFragment-->"
	const endMarker = "<!--EndFragment-->"
	if i := strings.Index(raw, startMarker); i >= 0 {
		s := raw[i+len(startMarker):]
		if j := strings.Index(s, endMarker); j >= 0 {
			s = s[:j]
		}
		s = strings.TrimSpace(s)
		if strings.Contains(s, "<") {
			return s
		}
		return ""
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

func forEachInlineStyle(attrs []html.Attribute, fn func(key, val string)) {
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
			val := stripCSSImportant(v)
			if key == "" || val == "" {
				continue
			}
			fn(key, val)
		}
	}
}

func stripCSSImportant(v string) string {
	v = strings.TrimSpace(v)
	if len(v) >= len("!important") && strings.EqualFold(v[len(v)-len("!important"):], "!important") {
		v = strings.TrimSpace(v[:len(v)-len("!important")])
	}
	return v
}

func parseCSSBackgroundColorRGBA(raw string) (r, g, b, a uint8, ok bool) {
	if r, g, b, a, ok = parseCSSColorRGBA(raw); ok {
		return r, g, b, a, true
	}
	// Handle CSS `background` shorthand (e.g. "rgb(255 240 240) no-repeat" or "#fff none").
	s := strings.TrimSpace(raw)
	for i := 0; i < len(s); {
		for i < len(s) && (s[i] == ' ' || s[i] == '\t' || s[i] == '\n' || s[i] == '\r') {
			i++
		}
		if i >= len(s) {
			break
		}
		start := i
		depth := 0
		for i < len(s) {
			ch := s[i]
			if ch == '(' {
				depth++
			} else if ch == ')' && depth > 0 {
				depth--
			} else if depth == 0 && (ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r') {
				break
			}
			i++
		}
		if r, g, b, a, ok = parseCSSColorRGBA(s[start:i]); ok {
			return r, g, b, a, true
		}
	}
	return 0, 0, 0, 0, false
}

func parseCSSColorRGBA(raw string) (r, g, b, a uint8, ok bool) {
	s := strings.ToLower(stripCSSImportant(raw))
	if s == "" || s == "inherit" || s == "initial" || s == "unset" || s == "transparent" || s == "none" {
		return 0, 0, 0, 0, false
	}
	if strings.HasPrefix(s, "#") {
		hex := s[1:]
		switch len(hex) {
		case 3:
			r1, err1 := strconv.ParseUint(strings.Repeat(hex[0:1], 2), 16, 8)
			g1, err2 := strconv.ParseUint(strings.Repeat(hex[1:2], 2), 16, 8)
			b1, err3 := strconv.ParseUint(strings.Repeat(hex[2:3], 2), 16, 8)
			if err1 == nil && err2 == nil && err3 == nil {
				return uint8(r1), uint8(g1), uint8(b1), 255, true
			}
		case 6, 8:
			r1, err1 := strconv.ParseUint(hex[0:2], 16, 8)
			g1, err2 := strconv.ParseUint(hex[2:4], 16, 8)
			b1, err3 := strconv.ParseUint(hex[4:6], 16, 8)
			a1 := uint64(255)
			var err4 error
			if len(hex) == 8 {
				a1, err4 = strconv.ParseUint(hex[6:8], 16, 8)
			}
			if err1 == nil && err2 == nil && err3 == nil && err4 == nil && a1 > 0 {
				return uint8(r1), uint8(g1), uint8(b1), uint8(a1), true
			}
		}
		return 0, 0, 0, 0, false
	}
	if strings.HasPrefix(s, "rgb(") || strings.HasPrefix(s, "rgba(") {
		open := strings.IndexByte(s, '(')
		closeIdx := strings.LastIndexByte(s, ')')
		if open < 0 || closeIdx <= open || closeIdx != len(s)-1 {
			return 0, 0, 0, 0, false
		}
		inner := strings.TrimSpace(s[open+1 : closeIdx])
		var rgbParts []string
		alphaRaw := ""
		if strings.Contains(inner, ",") {
			parts := strings.Split(inner, ",")
			if len(parts) < 3 {
				return 0, 0, 0, 0, false
			}
			rgbParts = parts[:3]
			if len(parts) >= 4 {
				alphaRaw = strings.TrimSpace(parts[3])
			}
		} else {
			rgbSide, aSide, hasSlash := strings.Cut(inner, "/")
			rgbParts = strings.Fields(rgbSide)
			if len(rgbParts) != 3 {
				return 0, 0, 0, 0, false
			}
			if hasSlash {
				alphaRaw = strings.TrimSpace(aSide)
			}
		}
		rv, err1 := strconv.Atoi(strings.TrimSpace(rgbParts[0]))
		gv, err2 := strconv.Atoi(strings.TrimSpace(rgbParts[1]))
		bv, err3 := strconv.Atoi(strings.TrimSpace(rgbParts[2]))
		if err1 != nil || err2 != nil || err3 != nil {
			return 0, 0, 0, 0, false
		}
		af := 1.0
		if alphaRaw != "" {
			if pct, isPct := strings.CutSuffix(alphaRaw, "%"); isPct {
				parsedA, err := strconv.ParseFloat(strings.TrimSpace(pct), 64)
				if err != nil {
					return 0, 0, 0, 0, false
				}
				af = parsedA / 100.0
			} else {
				parsedA, err := strconv.ParseFloat(alphaRaw, 64)
				if err != nil {
					return 0, 0, 0, 0, false
				}
				af = parsedA
			}
		}
		if af <= 0 {
			return 0, 0, 0, 0, false
		}
		if af > 1 {
			af = 1
		}
		return clampByte(rv), clampByte(gv), clampByte(bv), uint8(af*255 + 0.5), true
	}
	switch s {
	case "black":
		return 0, 0, 0, 255, true
	case "white":
		return 255, 255, 255, 255, true
	case "red":
		return 220, 38, 38, 255, true
	case "blue":
		return 37, 99, 235, 255, true
	case "green":
		return 22, 163, 74, 255, true
	case "orange":
		return 234, 88, 12, 255, true
	case "purple":
		return 147, 51, 234, 255, true
	case "yellow":
		return 234, 179, 8, 255, true
	case "gray", "grey":
		return 107, 114, 128, 255, true
	}
	return 0, 0, 0, 0, false
}

// isPageCanvasColor reports whether an RGBA color is a neutral light/dark page
// background (white/near-white or black/near-black) commonly serialized by browsers
// onto plain text selections.
func isPageCanvasColor(r, g, b, a uint8) bool {
	if a <= 20 {
		return true
	}
	chroma := max(r, max(g, b)) - min(r, min(g, b))
	if chroma > 20 {
		return false
	}
	return (r >= 235 && g >= 235 && b >= 235) || (r <= 36 && g <= 36 && b <= 36)
}

func isChromaticColor(r, g, b uint8) bool {
	return max(r, max(g, b))-min(r, min(g, b)) > 24
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

type richCheckStyle struct {
	bold          bool
	italic        bool
	underline     bool
	strikethrough bool
	inPre         bool
	fgKey         string
	fgChromatic   bool
	bgKey         string
	bgCanvas      bool
	fontSize      string
	fontFamily    string
}

// isMeaningfulRichHTML reports whether rawHTML carries actual rich-text structure
// or visual formatting beyond plain-text wrappers emitted by browsers/editors.
func isMeaningfulRichHTML(rawHTML, plainText string) bool {
	frag := extractHTMLFragment(rawHTML)
	if frag == "" {
		return false
	}
	if len(frag) > maxRichCheckHTMLBytes {
		frag = truncateUTF8Bytes(frag, maxRichCheckHTMLBytes)
	}
	doc, err := html.Parse(strings.NewReader(frag))
	if err != nil {
		return false
	}

	var (
		hasRichStructure bool
		hasStyledText    bool
		nonLinkTextSeen  bool
		linkCount        int
		linkMeaningful   bool
		fgColors         = make(map[string]bool)
		bgColors         = make(map[string]bool)
		fontSizes        = make(map[string]struct{})
		fontFamilies     = make(map[string]struct{})
	)

	var walk func(n *html.Node, st richCheckStyle, inLink bool)
	walk = func(n *html.Node, st richCheckStyle, inLink bool) {
		if hasRichStructure || hasStyledText {
			return
		}
		switch n.Type {
		case html.TextNode:
			if strings.TrimSpace(n.Data) == "" {
				return
			}
			if !inLink {
				nonLinkTextSeen = true
			}
			if st.bold || st.italic || st.underline || st.strikethrough {
				hasStyledText = true
				return
			}
			if st.fgKey != "" {
				fgColors[st.fgKey] = st.fgChromatic
			} else {
				fgColors["default"] = false
			}
			if st.bgKey != "" {
				bgColors[st.bgKey] = !st.bgCanvas
			} else {
				bgColors["default"] = false
			}
			if st.fontSize != "" {
				fontSizes[st.fontSize] = struct{}{}
			} else {
				fontSizes["default"] = struct{}{}
			}
			if st.fontFamily != "" {
				fontFamilies[st.fontFamily] = struct{}{}
			} else {
				fontFamilies["default"] = struct{}{}
			}
			return
		case html.ElementNode:
			tag := strings.ToLower(n.Data)
			switch tag {
			case "script", "style", "head", "meta", "title", "noscript", "link":
				return
			case "img", "hr":
				hasRichStructure = true
				return
			case "svg":
				if n.FirstChild != nil {
					hasRichStructure = true
					return
				}
			case "ul", "ol", "li", "table", "thead", "tbody", "tfoot", "tr", "td", "th", "blockquote", "dl", "dt", "dd":
				if hasNonEmptyText(n) {
					hasRichStructure = true
					return
				}
			case "kbd":
				if hasNonEmptyText(n) {
					hasRichStructure = true
					return
				}
			case "code", "tt", "samp":
				if !st.inPre && hasNonEmptyText(n) {
					hasRichStructure = true
					return
				}
			case "a":
				if href := attrValue(n.Attr, "href"); isMeaningfulHref(href) {
					anchorText := strings.TrimSpace(nodePlainText(n))
					if anchorText != "" {
						linkCount++
						if !sameURLTarget(anchorText, href) && !sameURLTarget(strings.TrimSpace(plainText), href) {
							linkMeaningful = true
						}
						next := applyRichCheckStyle(st, tag, n.Attr)
						for c := n.FirstChild; c != nil; c = c.NextSibling {
							walk(c, next, true)
						}
						return
					}
				}
			}

			next := applyRichCheckStyle(st, tag, n.Attr)
			if tag == "font" && (attrValue(n.Attr, "color") != "" || attrValue(n.Attr, "face") != "" || attrValue(n.Attr, "size") != "") && hasNonEmptyText(n) {
				hasRichStructure = true
				return
			}
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				walk(c, next, inLink)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c, st, inLink)
		}
	}

	walk(doc, richCheckStyle{}, false)

	if hasRichStructure || hasStyledText {
		return true
	}
	if linkCount > 0 && (linkMeaningful || nonLinkTextSeen || linkCount > 1) {
		return true
	}
	if len(fgColors) >= 2 {
		return true
	}
	for k, chromatic := range fgColors {
		if k != "default" && chromatic {
			return true
		}
	}
	if len(bgColors) >= 2 {
		return true
	}
	for k, nonCanvas := range bgColors {
		if k != "default" && nonCanvas {
			return true
		}
	}
	if len(fontSizes) >= 2 || len(fontFamilies) >= 2 {
		return true
	}
	return false
}

func applyRichCheckStyle(st richCheckStyle, tag string, attrs []html.Attribute) richCheckStyle {
	switch tag {
	case "b", "strong", "h1", "h2", "h3", "h4", "h5", "h6":
		st.bold = true
	case "i", "em", "cite":
		st.italic = true
	case "u", "ins":
		st.underline = true
	case "s", "del", "strike":
		st.strikethrough = true
	case "mark":
		st.bgKey = "mark"
		st.bgCanvas = false
	case "pre":
		st.inPre = true
	}

	forEachInlineStyle(attrs, func(key, val string) {
		lval := strings.ToLower(val)
		switch key {
		case "font-weight":
			if strings.Contains(lval, "bold") {
				st.bold = true
			} else if w, err := strconv.Atoi(val); err == nil && w >= 600 {
				st.bold = true
			} else if lval == "normal" || lval == "400" || lval == "300" || lval == "lighter" {
				st.bold = false
			}
		case "font-style":
			if strings.Contains(lval, "italic") || strings.Contains(lval, "oblique") {
				st.italic = true
			} else if strings.Contains(lval, "normal") {
				st.italic = false
			}
		case "text-decoration", "text-decoration-line":
			if strings.Contains(lval, "underline") {
				st.underline = true
			}
			if strings.Contains(lval, "line-through") {
				st.strikethrough = true
			}
			if lval == "none" {
				st.underline = false
				st.strikethrough = false
			}
		case "color":
			if r, g, b, _, ok := parseCSSColorRGBA(val); ok {
				st.fgKey = fmt.Sprintf("%d,%d,%d", r, g, b)
				st.fgChromatic = isChromaticColor(r, g, b)
			}
		case "background-color", "background":
			if r, g, b, a, ok := parseCSSBackgroundColorRGBA(val); ok {
				st.bgKey = fmt.Sprintf("%d,%d,%d,%d", r, g, b, a)
				st.bgCanvas = isPageCanvasColor(r, g, b, a)
			}
		case "font-size":
			if lval != "" && lval != "inherit" && lval != "initial" && lval != "unset" {
				st.fontSize = lval
			}
		case "font-family":
			if lval != "" && lval != "inherit" && lval != "initial" && lval != "unset" {
				st.fontFamily = lval
			}
		}
	})
	return st
}

func attrValue(attrs []html.Attribute, key string) string {
	for _, a := range attrs {
		if strings.EqualFold(a.Key, key) {
			return strings.TrimSpace(a.Val)
		}
	}
	return ""
}

func isMeaningfulHref(href string) bool {
	if href == "" || href == "#" {
		return false
	}
	return !strings.HasPrefix(strings.ToLower(href), "javascript:")
}

func sameURLTarget(text, href string) bool {
	a := strings.TrimRight(strings.TrimSpace(text), "/")
	b := strings.TrimRight(strings.TrimSpace(href), "/")
	return a != "" && strings.EqualFold(a, b)
}

func hasNonEmptyText(n *html.Node) bool {
	return strings.TrimSpace(nodePlainText(n)) != ""
}

func nodePlainText(n *html.Node) string {
	var b strings.Builder
	var walk func(*html.Node)
	walk = func(cur *html.Node) {
		if cur.Type == html.TextNode {
			b.WriteString(cur.Data)
			return
		}
		for c := cur.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(n)
	return b.String()
}
