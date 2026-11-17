package app

import (
	"path/filepath"
	"testing"
	"time"

	"mytiez/internal/store"

	"github.com/egoist/mygo/ui"
)

func TestViewInteractions(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if _, err := st.SaveText("alpha snippet", store.SaveOptions{
		Deduplicate: true,
		SourceApp:   "Antigravity.exe",
		SourcePath:  `C:\Program Files\Antigravity\Antigravity.exe`,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertFiles([]string{filepath.Join("C:\\", "work", "report.pdf")}); err != nil {
		t.Fatal(err)
	}

	a := newClipApp(st, nil)
	a.reload()

	tt := ui.NewTester(a.view, panelWidth, panelHeight)
	if a.selected != -1 {
		t.Fatalf("expected no card selected initially (selected=-1), got %d", a.selected)
	}
	if !tt.HasText("alpha snippet") || !tt.HasText("Antigravity.exe") || !tt.HasText("report.pdf") {
		t.Fatalf("expected initial items and source app in view, got texts: %q", tt.Texts())
	}
	if tt.HasText("↑↓ 选择") {
		t.Fatalf("expected bottom bar removed, got texts: %q", tt.Texts())
	}

	// 按向下键才选中第 0 张卡片
	tt.Key(0, ui.KeyDown)
	if a.selected != 0 {
		t.Fatalf("expected selected=0 after KeyDown, got %d", a.selected)
	}
	tt.Key(0, ui.KeyEscape)
	if a.selected != -1 {
		t.Fatalf("expected Escape to clear selection, got %d", a.selected)
	}

	// 切换到“文本”分类
	if err := tt.Click("文本"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("alpha snippet") || tt.HasText("report.pdf") {
		t.Fatalf("expected only text item after filter, got texts: %q", tt.Texts())
	}

	// 再次点击“文本”取消筛选回到全部，并用搜索过滤
	if err := tt.Click("文本"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("搜索剪贴板"); err != nil {
		t.Fatal(err)
	}
	tt.Type("report")
	if tt.HasText("alpha snippet") || !tt.HasText("report.pdf") {
		t.Fatalf("expected search filter to keep only report.pdf, got texts: %q", tt.Texts())
	}

	// 按 Esc 清空搜索
	tt.Key(0, ui.KeyEscape)
	if a.query != "" || !tt.HasText("alpha snippet") {
		t.Fatalf("expected Escape to clear search, query=%q, texts=%q", a.query, tt.Texts())
	}

	// 默认不置顶，点击固定窗口按钮可切换为置顶
	if a.winPinned {
		t.Fatal("expected winPinned to default to false")
	}
	if err := tt.Click("固定窗口"); err != nil {
		t.Fatal(err)
	}
	if !a.winPinned {
		t.Fatal("expected winPinned to toggle to true after click")
	}
}

func TestSettingsViewInteractions(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if _, err := st.UpsertText("keep item"); err != nil {
		t.Fatal(err)
	}

	a := newClipApp(st, nil)
	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	// 进入设置页，所有分组默认收起
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if !a.showSettings || !tt.HasText("常规设置") || !tt.HasText("剪贴板设置") {
		t.Fatalf("expected settings groups visible, texts=%q", tt.Texts())
	}
	if a.expanded.general || tt.HasText("开机自启动") {
		t.Fatalf("expected groups collapsed by default, texts=%q", tt.Texts())
	}

	// 展开“常规设置”，推进折叠展开动画，再测试“失焦自动隐藏”联动与收起
	if err := tt.Click("常规设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if !a.expanded.general || !tt.HasText("开机自启动") {
		t.Fatalf("expected general group expanded, texts=%q", tt.Texts())
	}
	if err := tt.Click("切换失焦自动隐藏"); err != nil {
		t.Fatal(err)
	}
	if a.settings.AutoHideOnBlur || !a.winPinned {
		t.Fatalf("expected AutoHideOnBlur=false and winPinned=true, got autoHide=%v winPinned=%v", a.settings.AutoHideOnBlur, a.winPinned)
	}
	if err := tt.Click("常规设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if a.expanded.general || tt.HasText("开机自启动") {
		t.Fatalf("expected general group collapsed again, texts=%q", tt.Texts())
	}

	// 展开“剪贴板设置”切换“自动去重”与上限
	if err := tt.Click("剪贴板设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("切换自动去重"); err != nil {
		t.Fatal(err)
	}
	if a.settings.Deduplicate {
		t.Fatal("expected Deduplicate toggled to false")
	}
	if !a.settings.CaptureRichText {
		t.Fatal("expected CaptureRichText to default to true")
	}
	if err := tt.Click("切换捕获带格式的文本"); err != nil {
		t.Fatal(err)
	}
	if a.settings.CaptureRichText {
		t.Fatal("expected CaptureRichText toggled to false")
	}
	if err := tt.Click("上限200"); err != nil {
		t.Fatal(err)
	}
	if a.settings.MaxHistory != 200 {
		t.Fatalf("expected MaxHistory=200, got %d", a.settings.MaxHistory)
	}
	if err := tt.Click("剪贴板设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)

	// 展开“打开方式”，验证默认显示与恢复默认交互
	a.settings.OpenAppText = `C:\Windows\System32\notepad.exe`
	if err := tt.Click("打开方式"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if !a.expanded.openApps || !tt.HasText("notepad") || !tt.HasText("系统默认") {
		t.Fatalf("expected openApps group expanded with notepad and 系统默认, texts=%q", tt.Texts())
	}
	if err := tt.Click("恢复文本默认打开方式"); err != nil {
		t.Fatal(err)
	}
	if a.settings.OpenAppText != "" {
		t.Fatalf("expected OpenAppText cleared after reset, got %q", a.settings.OpenAppText)
	}
	if err := tt.Click("打开方式"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)

	// 展开“界面设置”切换窗口材质，并验证 Vibrancy 模式下渲染正常
	tt.SetVibrancy(true)
	if err := tt.Click("界面设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("材质毛玻璃"); err != nil {
		t.Fatal(err)
	}
	if a.settings.BackdropMode != "acrylic" {
		t.Fatalf("expected BackdropMode=acrylic, got %q", a.settings.BackdropMode)
	}
	if err := tt.Click("切换悬停显示卡片操作"); err != nil {
		t.Fatal(err)
	}
	if !a.settings.HideCardActions {
		t.Fatal("expected HideCardActions toggled to true")
	}

	// 返回主列表
	if err := tt.Click("返回"); err != nil {
		t.Fatal(err)
	}
	if a.showSettings || !tt.HasText("keep item") {
		t.Fatalf("expected back to main list, showSettings=%v, texts=%q", a.showSettings, tt.Texts())
	}
}

func TestScrollRevealControls(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for i := 0; i < 15; i++ {
		if _, err := st.SaveText(
			string(rune('A'+i))+" clipboard entry line for scroll test",
			store.SaveOptions{Deduplicate: false},
		); err != nil {
			t.Fatal(err)
		}
	}

	a := newClipApp(st, nil)
	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	if a.controlsCollapsed {
		t.Fatal("expected controls initially expanded")
	}

	// 第一次在顶部向下滚动：只收起搜索栏，拦截列表滚动以免列表同时下跳
	tt.Wheel(panelWidth/2, panelHeight/2, 0, 200)
	tt.Advance(250 * time.Millisecond)
	if !a.controlsCollapsed || a.listScroll.Y != 0 {
		t.Fatalf("expected controls collapsed and list still at Y=0, got collapsed=%v scrollY=%v", a.controlsCollapsed, a.listScroll.Y)
	}

	// 继续向下滚动：列表离开顶部
	tt.Wheel(panelWidth/2, panelHeight/2, 0, 200)
	tt.Advance(250 * time.Millisecond)
	if a.atTop || a.listScroll.Y <= hideScrollOffset {
		t.Fatalf("expected list scrolled away from top, got atTop=%v scrollY=%v", a.atTop, a.listScroll.Y)
	}

	// 一次大滚轮直接滑回顶部：刚到顶时仍保持收起，不会立刻显示
	tt.Wheel(panelWidth/2, panelHeight/2, 0, -400)
	tt.Advance(250 * time.Millisecond)
	if !a.atTop || !a.controlsCollapsed {
		t.Fatalf("expected atTop=true and controls still collapsed right after reaching top, got atTop=%v collapsed=%v", a.atTop, a.controlsCollapsed)
	}

	// 在顶部再多滚一下向上：搜索栏展开
	tt.Wheel(panelWidth/2, panelHeight/2, 0, -40)
	tt.Advance(250 * time.Millisecond)
	if a.controlsCollapsed || a.listScroll.Y != 0 {
		t.Fatalf("expected controls expanded and list at Y=0, got collapsed=%v scrollY=%v", a.controlsCollapsed, a.listScroll.Y)
	}
}

func TestRunWithTimeout(t *testing.T) {
	if err := runWithTimeout(50*time.Millisecond, func() error { return nil }); err != nil {
		t.Fatalf("expected nil error on fast task, got %v", err)
	}
	if err := runWithTimeout(20*time.Millisecond, func() error {
		time.Sleep(80 * time.Millisecond)
		return nil
	}); err == nil {
		t.Fatal("expected timeout error on slow task")
	}
}

func TestNewItemAppearsAtTop(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for i := 0; i < 12; i++ {
		if _, err := st.SaveText(
			string(rune('A'+i))+" existing entry",
			store.SaveOptions{Deduplicate: false},
		); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	a := newClipApp(st, nil)
	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	if _, err := st.SaveText("brand new top entry", store.SaveOptions{Deduplicate: false}); err != nil {
		t.Fatal(err)
	}
	a.reload()
	a.listScroll.Y = 0
	a.list.ScrollTo(0, ui.Start)
	tt.Frame()
	tt.Advance(250 * time.Millisecond)

	if a.listScroll.Y != 0 {
		t.Fatalf("expected listScroll.Y=0 after top insert, got %v", a.listScroll.Y)
	}
	if !tt.HasText("brand new top entry") {
		t.Fatalf("expected new top entry visible without scrolling, texts=%q", tt.Texts())
	}
}

func TestRichTextPreviewAndFilter(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if _, err := st.UpsertText("plain only item"); err != nil {
		t.Fatal(err)
	}
	cfHTML := "Version:0.9\r\nStartHTML:000000\r\n<!--StartFragment--><style>.x{color:pink}</style><b>Bold</b> <span style=\"color:#dc2626\">Red</span> <u>Line</u><!--EndFragment-->"
	if _, err := st.SaveText("Bold Red Line", store.SaveOptions{
		Deduplicate: true,
		HTML:        cfHTML,
	}); err != nil {
		t.Fatal(err)
	}

	spans := parseHTMLPreviewSpans(cfHTML, myTiezLightTheme)
	if len(spans) < 3 || spans[0].Weight != 700 || spans[0].Text != "Bold" {
		t.Fatalf("unexpected parsed rich spans: %+v", spans)
	}

	// Verify UTF-16LE clipboard HTML (e.g. Firefox/Zen text/html) decodes cleanly.
	utf16HTML := `<div class="title"><h1><b>MyGO</b> 标题</h1></div>`
	utf16Bytes := make([]byte, 0, len(utf16HTML)*2)
	for _, r := range utf16HTML {
		utf16Bytes = append(utf16Bytes, byte(r), byte(r>>8))
	}
	if decoded := decodeClipboardHTML(utf16Bytes); decoded != utf16HTML {
		t.Fatalf("expected decoded UTF-16LE HTML %q, got %q", utf16HTML, decoded)
	}
	uSpans := parseHTMLPreviewSpans(string(utf16Bytes), myTiezLightTheme)
	if len(uSpans) != 1 || uSpans[0].Weight != 700 || uSpans[0].Text != "MyGO 标题" {
		t.Fatalf("unexpected UTF-16LE rich spans: %+v", uSpans)
	}

	a := newClipApp(st, nil)
	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	if err := tt.Click("富文本"); err != nil {
		t.Fatal(err)
	}
	if a.currentKind() != store.KindRichText {
		t.Fatalf("expected KindRichText filter, got %q", a.currentKind())
	}
	if tt.HasText("plain only item") || !tt.HasText("Bold") {
		t.Fatalf("expected only rich_text item visible, texts=%q", tt.Texts())
	}
}

func TestOpenHelpers(t *testing.T) {
	if !isHTTPURL("https://example.com/a?b=1") || !isHTTPURL("http://localhost:8080") {
		t.Fatal("expected http/https URLs to be recognized")
	}
	if isHTTPURL("not a url") || isHTTPURL("https://example.com\nsecond") || isHTTPURL("ftp://example.com") {
		t.Fatal("expected non-http or multiline text not to be recognized as http URL")
	}
	if got := openAppDisplayName(""); got != "系统默认" {
		t.Fatalf("expected empty app path to display 系统默认, got %q", got)
	}
	if got := openAppDisplayName(`C:\Program Files\Notepad++\notepad++.exe`); got != "notepad++" {
		t.Fatalf("expected exe suffix stripped, got %q", got)
	}
}
