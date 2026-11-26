package app

import (
	"strings"
	"testing"

	"mytiez/internal/store"
)

func TestNormalizeCapturedHTMLAndClassification(t *testing.T) {
	nonRichCases := []struct {
		name     string
		text     string
		html     string
		wantKind store.ClipKind
	}{
		{
			name:     "zen tagless fragment",
			text:     "MyGo!!!!!",
			html:     "MyGo!!!!!",
			wantKind: store.KindText,
		},
		{
			name:     "cfhtml wrapper around plain text",
			text:     "MyGo!!!!!",
			html:     "Version:0.9\r\nStartHTML:000000\r\n<!--StartFragment-->MyGo!!!!!<!--EndFragment-->",
			wantKind: store.KindText,
		},
		{
			name:     "plain span white-space pre-wrap",
			text:     "screenshots",
			html:     `<span style="white-space: pre-wrap;">screenshots</span>`,
			wantKind: store.KindText,
		},
		{
			name:     "chrome uniform light webpage wrapper",
			text:     "Hello world",
			html:     `<span style="color: rgb(31, 35, 40); font-family: Arial, sans-serif; font-size: 14px; font-weight: 400; font-style: normal; text-decoration: none; background-color: rgb(255, 255, 255);">Hello world</span>`,
			wantKind: store.KindText,
		},
		{
			name:     "chrome uniform dark webpage wrapper",
			text:     "Hello dark",
			html:     `<span style="color: rgb(230, 237, 243); background-color: rgb(13, 17, 23);">Hello dark</span>`,
			wantKind: store.KindText,
		},
		{
			name:     "plain paragraph and line breaks",
			text:     "line 1\nline 2",
			html:     `<div><p>line 1</p><p>line 2<br></p></div>`,
			wantKind: store.KindText,
		},
		{
			name:     "nested bold reset to normal",
			text:     "normal text",
			html:     `<div style="font-weight: 700"><span style="font-weight: 400">normal text</span></div>`,
			wantKind: store.KindText,
		},
		{
			name:     "standalone unstyled pre code block",
			text:     "func main() {\n\treturn\n}",
			html:     "<pre><code>func main() {\n\treturn\n}</code></pre>",
			wantKind: store.KindCode,
		},
		{
			name:     "bare url wrapped in matching anchor",
			text:     "https://github.com/llxo/MyTieZ",
			html:     `<a href="https://github.com/llxo/MyTieZ/">https://github.com/llxo/MyTieZ</a>`,
			wantKind: store.KindURL,
		},
	}

	richCases := []struct {
		name string
		text string
		html string
	}{
		{
			name: "bold tag",
			text: "Hello",
			html: `<b>Hello</b>`,
		},
		{
			name: "css bold and italic",
			text: "Styled",
			html: `<span style="font-weight: 700; font-style: italic;">Styled</span>`,
		},
		{
			name: "chromatic color",
			text: "Red alert",
			html: `<span style="color: #dc2626">Red alert</span>`,
		},
		{
			name: "highlight background",
			text: "Marked",
			html: `<span style="background-color: #fef08a">Marked</span>`,
		},
		{
			name: "mixed inline code in sentence",
			text: "支持数字键快捷粘贴（Alt+1~9 / Ctrl+1~9）",
			html: `支持数字键快捷粘贴（<code>Alt+1~9</code> / <code>Ctrl+1~9</code>）`,
		},
		{
			name: "link with custom anchor text",
			text: "tiez-clipboard",
			html: `<a href="https://github.com/jimuzhe/tiez-clipboard">tiez-clipboard</a>`,
		},
		{
			name: "syntax highlighted code",
			text: "func main()",
			html: `<pre><span style="color:#569cd6">func</span> <span style="color:#dcdcaa">main</span>()</pre>`,
		},
		{
			name: "unordered list",
			text: "one\ntwo",
			html: `<ul><li>one</li><li>two</li></ul>`,
		},
	}

	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for _, tc := range nonRichCases {
		filtered := normalizeCapturedHTML([]byte(tc.html), tc.text)
		if filtered != "" {
			t.Errorf("%s: expected normalizeCapturedHTML=\"\", got %q", tc.name, filtered)
		}
		item, err := st.SaveText(tc.text, store.SaveOptions{HTML: filtered})
		if err != nil {
			t.Fatalf("%s: SaveText failed: %v", tc.name, err)
		}
		if item.Kind != tc.wantKind || item.HTML != "" {
			t.Errorf("%s: got kind=%q html=%q, want kind=%q html=\"\"", tc.name, item.Kind, item.HTML, tc.wantKind)
		}
	}

	for _, tc := range richCases {
		filtered := normalizeCapturedHTML([]byte(tc.html), tc.text)
		if filtered == "" {
			t.Errorf("%s: expected normalizeCapturedHTML non-empty for %q", tc.name, tc.html)
		}
		item, err := st.SaveText(tc.text, store.SaveOptions{HTML: filtered})
		if err != nil {
			t.Fatalf("%s: SaveText failed: %v", tc.name, err)
		}
		if item.Kind != store.KindRichText || item.HTML == "" {
			t.Errorf("%s: got kind=%q html=%q, want kind=%q with html", tc.name, item.Kind, item.HTML, store.KindRichText)
		}
	}

	// Non-rich HTML should deduplicate cleanly with an existing plain-text entry.
	plain, err := st.SaveText("dedup-zen", store.SaveOptions{Deduplicate: true})
	if err != nil {
		t.Fatal(err)
	}
	wrapped, err := st.SaveText("dedup-zen", store.SaveOptions{
		Deduplicate: true,
		HTML:        normalizeCapturedHTML([]byte(`<span style="white-space: pre-wrap;">dedup-zen</span>`), "dedup-zen"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if wrapped.ID != plain.ID || wrapped.Kind != store.KindText {
		t.Fatalf("expected non-rich wrapped HTML to deduplicate into plain item %d, got id=%d kind=%q", plain.ID, wrapped.ID, wrapped.Kind)
	}
}

func TestParseHTMLPreviewSpansRichStructures(t *testing.T) {
	joinedText := func(raw string) string {
		spans := parseHTMLPreviewSpans(raw, myTiezLightTheme)
		var b strings.Builder
		for _, sp := range spans {
			b.WriteString(sp.Text)
		}
		return b.String()
	}

	if got := joinedText(`<ul><li>One</li><li>Two</li></ul>`); got != "• One\n• Two" {
		t.Fatalf("unordered list = %q, want %q", got, "• One\n• Two")
	}
	if got := joinedText(`<ol><li>First</li><li>Second</li></ol>`); got != "1. First\n2. Second" {
		t.Fatalf("ordered list = %q, want %q", got, "1. First\n2. Second")
	}
	if got := joinedText(`<blockquote>Cited note</blockquote>`); got != "▎ Cited note" {
		t.Fatalf("blockquote = %q, want %q", got, "▎ Cited note")
	}
	if got := joinedText(`<table><tr><th>Name</th><th>Role</th></tr><tr><td>Tomori</td><td>Vocal</td></tr></table>`); got != "Name │ Role\nTomori │ Vocal" {
		t.Fatalf("table = %q, want %q", got, "Name │ Role\nTomori │ Vocal")
	}
	if got := joinedText("<pre><span style=\"color:#569cd6\">func</span> main() {\n\treturn\n}</pre>"); !strings.Contains(got, "\n  return\n") {
		t.Fatalf("pre indentation lost: %q", got)
	}

	hSpans := parseHTMLPreviewSpans(`<h2>Heading</h2><p>Press <kbd>Ctrl+V</kbd> to <mark>paste</mark> H<sub>2</sub>O</p>`, myTiezLightTheme)
	if len(hSpans) < 5 || hSpans[0].Size != 14.5 || hSpans[0].Weight != 700 {
		t.Fatalf("unexpected heading span: %+v", hSpans)
	}
	foundKbd, foundMark, foundSub := false, false, false
	for _, sp := range hSpans {
		if sp.Text == "Ctrl+V" && sp.Font == "Consolas" && sp.PaddingX == 4 {
			foundKbd = true
		}
		if sp.Text == "paste" && sp.Background.A > 0 && sp.PaddingX == 2 {
			foundMark = true
		}
		if sp.Text == "2" && sp.BaselineShift < 0 && sp.Size == 11 {
			foundSub = true
		}
	}
	if !foundKbd || !foundMark || !foundSub {
		t.Fatalf("missing styled spans (kbd=%v mark=%v sub=%v): %+v", foundKbd, foundMark, foundSub, hSpans)
	}
}

func TestRichTextCSSParsingAndEmptyContainers(t *testing.T) {
	if r, g, b, a, ok := parseCSSColorRGBA("rgb(220 38 38 / 50%) !important"); !ok || r != 220 || g != 38 || b != 38 || a != 128 {
		t.Fatalf("modern rgb with percent alpha failed: (%d,%d,%d,%d,%v)", r, g, b, a, ok)
	}
	if r, g, b, a, ok := parseCSSBackgroundColorRGBA("url(bg.png) rgb(254 240 138 / 0.8) no-repeat !important"); !ok || r != 254 || g != 240 || b != 138 || a != 204 {
		t.Fatalf("background shorthand parsing failed: (%d,%d,%d,%d,%v)", r, g, b, a, ok)
	}

	spans := parseHTMLPreviewSpans(`<a href="https://example.com" style="text-decoration: none !important; font-weight: 300 !important">Link</a>`, myTiezLightTheme)
	if len(spans) != 1 || spans[0].Underline || spans[0].Weight != 400 {
		t.Fatalf("expected text-decoration:none and font-weight:300 applied, got %+v", spans)
	}

	if isMeaningfulRichHTML(`<div><ul><li>   </li></ul><svg></svg>Plain text</div>`, "Plain text") {
		t.Fatalf("empty list and empty svg should not mark plain text as rich HTML")
	}
}
