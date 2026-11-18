package app

import (
	"path/filepath"
	"testing"
	"time"

	"mytiez/internal/store"
	"mytiez/locales"

	"github.com/egoist/mygo"
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

	// 再次点击“文本”取消选中回到全部（无状态），并用搜索过滤
	if err := tt.Click("文本"); err != nil {
		t.Fatal(err)
	}
	if a.filterIdx != 0 || !tt.HasText("report.pdf") {
		t.Fatalf("expected clicking active tab again to reset to all (0), filterIdx=%d texts=%q", a.filterIdx, tt.Texts())
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

	// 进入设置页，验证弹性入场动画期间与结束后设置页正常显示且所有分组默认收起
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(180 * time.Millisecond)
	tt.Advance(250 * time.Millisecond)
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

	// 展开“默认打开程序”，验证默认显示与恢复默认交互
	a.settings.OpenAppText = `C:\Windows\System32\notepad.exe`
	if err := tt.Click("默认打开程序"); err != nil {
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
	if err := tt.Click("默认打开程序"); err != nil {
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

	// 返回主列表（无退场动画，立即恢复主列表显示）
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
	if got := openAppDisplayName(locales.In(""), ""); got != "系统默认" {
		t.Fatalf("expected empty app path to display 系统默认, got %q", got)
	}
	if got := openAppDisplayName(locales.In(""), `C:\Program Files\Notepad++\notepad++.exe`); got != "notepad++" {
		t.Fatalf("expected exe suffix stripped, got %q", got)
	}
}

func TestCardTagsInteractions(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if _, err := st.UpsertText("first tagged entry"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertText("second plain entry"); err != nil {
		t.Fatal(err)
	}

	a := newClipApp(st, nil)
	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	// 打开第 0 张卡片（second plain entry）的标签编辑器并添加标签
	if err := tt.Click("标签"); err != nil {
		t.Fatal(err)
	}
	if a.editingTagsID == 0 || !tt.HasText("添加") {
		t.Fatalf("expected inline tag editor visible, editingTagsID=%d texts=%q", a.editingTagsID, tt.Texts())
	}
	if err := tt.Click("标签输入框"); err != nil {
		t.Fatal(err)
	}
	tt.Type("工作")
	tt.Key(0, ui.KeyEnter)
	if len(a.items[0].Tags) != 1 || a.items[0].Tags[0] != "工作" {
		t.Fatalf("expected tag 工作 added on Enter, got %#v", a.items[0].Tags)
	}

	// 完成编辑后点击标签胶囊按 #工作 筛选
	if err := tt.Click("完成标签编辑"); err != nil {
		t.Fatal(err)
	}
	if a.editingTagsID != 0 {
		t.Fatalf("expected editingTagsID=0 after done, got %d", a.editingTagsID)
	}
	if err := tt.Click("#工作"); err != nil {
		t.Fatal(err)
	}
	if a.query != "#工作" || !tt.HasText("second plain entry") || tt.HasText("first tagged entry") {
		t.Fatalf("expected tag filter #工作 to show only tagged item, query=%q texts=%q", a.query, tt.Texts())
	}

	// 取消筛选后测试多标签折叠为 +N
	if err := tt.Click("#工作"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateTags(a.items[0].ID, []string{"工作", "一个非常非常长的标签名称用于测试预算", "第三个标签", "第四个标签", "第五个标签"}); err != nil {
		t.Fatal(err)
	}
	a.reload()
	tt.Frame()
	if !tt.HasText("展开全部标签") {
		t.Fatalf("expected overflow tags collapsed with +N button, texts=%q", tt.Texts())
	}
	if err := tt.Click("展开全部标签"); err != nil {
		t.Fatal(err)
	}
	if a.editingTagsID != a.items[0].ID || !tt.HasText("第五个标签") {
		t.Fatalf("expected clicking +N to open full tag editor, editingTagsID=%d texts=%q", a.editingTagsID, tt.Texts())
	}

	// 退出编辑后聚焦搜索框，弹出标签选择浮层并点击筛选
	if err := tt.Click("完成标签编辑"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("搜索剪贴板"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("标签") {
		t.Fatalf("expected search tag popover visible when search focused, texts=%q", tt.Texts())
	}
	if err := tt.Click("筛选标签 第五个标签"); err != nil {
		t.Fatal(err)
	}
	if a.query != "#第五个标签" || !tt.HasText("second plain entry") || tt.HasText("first tagged entry") {
		t.Fatalf("expected clicking popover tag to filter by #第五个标签, query=%q texts=%q", a.query, tt.Texts())
	}
}

func TestHotkeyRecorder(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a := newClipApp(st, nil)
	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("剪贴板设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)

	if !tt.HasText("Alt+V") {
		t.Fatalf("expected default hotkey Alt+V shown, texts=%q", tt.Texts())
	}

	// 点击进入录制状态
	if err := tt.Click("录制呼出快捷键"); err != nil {
		t.Fatal(err)
	}
	if !a.recordingHotkey || !tt.HasText("请按下快捷键...") {
		t.Fatalf("expected recording state active, recording=%v texts=%q", a.recordingHotkey, tt.Texts())
	}

	// 无修饰键的普通字母按键应被拒绝并通过 Toast 提示
	tt.Key(0, ui.KeyA)
	if !a.recordingHotkey || a.settings.Hotkey != "Alt+V" || !tt.HasText(locales.SettingsHotkeyNeedMod()) {
		t.Fatalf("expected plain key A rejected with toast, recording=%v hotkey=%q texts=%q", a.recordingHotkey, a.settings.Hotkey, tt.Texts())
	}

	// 按 Esc 取消录制
	tt.Key(0, ui.KeyEscape)
	if a.recordingHotkey || a.settings.Hotkey != "Alt+V" {
		t.Fatalf("expected Esc to cancel recording, recording=%v hotkey=%q", a.recordingHotkey, a.settings.Hotkey)
	}

	// 录制组合键 Ctrl+Shift+K
	if err := tt.Click("录制呼出快捷键"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl|ui.Shift, ui.KeyK)
	if a.recordingHotkey || a.settings.Hotkey != "Ctrl+Shift+K" || a.shortcut != "Ctrl+Shift+K" {
		t.Fatalf("expected recorded Ctrl+Shift+K, recording=%v hotkey=%q shortcut=%q", a.recordingHotkey, a.settings.Hotkey, a.shortcut)
	}
	loaded, err := st.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Hotkey != "Ctrl+Shift+K" {
		t.Fatalf("expected persisted hotkey Ctrl+Shift+K, got %q", loaded.Hotkey)
	}

	// 录制功能键 F8
	if err := tt.Click("录制呼出快捷键"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyF8)
	if a.recordingHotkey || a.settings.Hotkey != "F8" {
		t.Fatalf("expected recorded F8, recording=%v hotkey=%q", a.recordingHotkey, a.settings.Hotkey)
	}

	// 点击恢复默认
	if err := tt.Click("恢复默认快捷键"); err != nil {
		t.Fatal(err)
	}
	if a.settings.Hotkey != "Alt+V" || a.shortcut != "Alt+V" {
		t.Fatalf("expected hotkey reset to Alt+V, hotkey=%q shortcut=%q", a.settings.Hotkey, a.shortcut)
	}
}

func TestClearSearchResetsTopAndClosesSearch(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for i := 0; i < 12; i++ {
		item, err := st.SaveText(
			string(rune('A'+i))+" searchable clipboard entry",
			store.SaveOptions{Deduplicate: false},
		)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			if _, err := st.UpdateTags(item.ID, []string{"工作"}); err != nil {
				t.Fatal(err)
			}
		}
	}

	a := newClipApp(st, nil)
	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	// 初始打开不自动聚焦搜索框，不弹出标签浮层
	if tt.HasText("筛选标签 工作") {
		t.Fatalf("expected search tag popover closed initially, texts=%q", tt.Texts())
	}

	// 点击搜索框并输入查询、向下滚动
	if err := tt.Click("搜索剪贴板"); err != nil {
		t.Fatal(err)
	}
	tt.Type("searchable")
	if a.query != "searchable" {
		t.Fatalf("expected query=searchable, got %q", a.query)
	}
	tt.Wheel(panelWidth/2, panelHeight/2, 0, 120)
	tt.Advance(250 * time.Millisecond)

	// 点击叉号清除搜索：应回到顶部且关闭搜索（不再聚焦、关闭标签浮层）
	if err := tt.Click("清除搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.query != "" || a.listScroll.Y != 0 || !a.atTop || a.controlsCollapsed {
		t.Fatalf("expected search cleared at top, query=%q scrollY=%v atTop=%v collapsed=%v", a.query, a.listScroll.Y, a.atTop, a.controlsCollapsed)
	}
	if tt.HasText("筛选标签 工作") {
		t.Fatalf("expected search tag popover closed after clearing search, texts=%q", tt.Texts())
	}
}

func TestFollowMouseTarget(t *testing.T) {
	workArea := mygo.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1040}
	w, h := 400, 560

	// 1. 光标在中上部：水平居中，位于光标下方 +12
	x, y := followMouseTarget(mygo.Point{X: 960, Y: 200}, w, h, workArea)
	if x != 760 || y != 212 {
		t.Fatalf("expected (760, 212) below cursor, got (%d, %d)", x, y)
	}

	// 2. 光标靠近底部：下方放不下时自动翻转到光标上方
	x, y = followMouseTarget(mygo.Point{X: 960, Y: 900}, w, h, workArea)
	if x != 760 || y != 900-h-12 {
		t.Fatalf("expected flipped above cursor to (760, %d), got (%d, %d)", 900-h-12, x, y)
	}

	// 3. 光标靠近左右边缘：保留 8px 安全边距
	x, _ = followMouseTarget(mygo.Point{X: 30, Y: 200}, w, h, workArea)
	if x != 8 {
		t.Fatalf("expected clamped left margin 8, got %d", x)
	}
	x, _ = followMouseTarget(mygo.Point{X: 1900, Y: 200}, w, h, workArea)
	if x != 1920-w-8 {
		t.Fatalf("expected clamped right margin %d, got %d", 1920-w-8, x)
	}

	// 4. 上下都放不下时夹紧在工作区底边
	x, y = followMouseTarget(mygo.Point{X: 960, Y: 520}, w, h, workArea)
	if x != 760 || y != 1040-h-8 {
		t.Fatalf("expected clamped bottom %d, got (%d, %d)", 1040-h-8, x, y)
	}
}

func TestURLAndCodeFilter(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if _, err := st.UpsertText("plain note entry"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertText("https://example.com/path"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertText("const x = 1; return x;"); err != nil {
		t.Fatal(err)
	}

	a := newClipApp(st, nil)
	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	if err := tt.Click("链接"); err != nil {
		t.Fatal(err)
	}
	if a.currentKind() != store.KindURL || !tt.HasText("https://example.com/path") || tt.HasText("plain note entry") {
		t.Fatalf("expected 链接 filter to show URL only, kind=%q texts=%q", a.currentKind(), tt.Texts())
	}

	if err := tt.Click("代码"); err != nil {
		t.Fatal(err)
	}
	if a.currentKind() != store.KindCode || !tt.HasText("const x = 1; return x;") || tt.HasText("https://example.com/path") {
		t.Fatalf("expected 代码 filter to show code only, kind=%q texts=%q", a.currentKind(), tt.Texts())
	}
}

func TestLightweightSettings(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for i := 0; i < 15; i++ {
		if _, err := st.SaveText(
			string(rune('A'+i))+" lightweight setting entry",
			store.SaveOptions{Deduplicate: false},
		); err != nil {
			t.Fatal(err)
		}
	}

	a := newClipApp(st, nil)
	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	// 1. 设置页开关与选项验证
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("常规设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if !tt.HasText("以管理员身份重启") {
		t.Fatalf("expected restart as admin row in general settings, texts=%q", tt.Texts())
	}
	if err := tt.Click("切换固定显示搜索栏"); err != nil {
		t.Fatal(err)
	}
	if !a.settings.PinSearchBox {
		t.Fatal("expected PinSearchBox toggled to true")
	}
	if err := tt.Click("常规设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)

	if err := tt.Click("剪贴板设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("切换粘贴后删除"); err != nil {
		t.Fatal(err)
	}
	tt.Wheel(panelWidth/2, panelHeight/2, 0, 200)
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("Ctrl+V"); err != nil {
		t.Fatal(err)
	}
	if a.settings.PasteMethod != "ctrl_v" {
		t.Fatalf("expected PasteMethod=ctrl_v, got %q", a.settings.PasteMethod)
	}
	if err := tt.Click("游戏模式"); err != nil {
		t.Fatal(err)
	}
	if a.settings.PasteMethod != "game_mode" {
		t.Fatalf("expected PasteMethod=game_mode, got %q", a.settings.PasteMethod)
	}
	if err := tt.Click("快捷粘贴Alt"); err != nil {
		t.Fatal(err)
	}
	if !a.settings.DeleteAfterPaste || a.settings.QuickPasteModifier != "alt" {
		t.Fatalf("expected DeleteAfterPaste=true and QuickPasteModifier=alt, got %+v", a.settings)
	}
	tt.Wheel(panelWidth/2, panelHeight/2, 0, -200)
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("剪贴板设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)

	if err := tt.Click("界面设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("切换显示来源应用图标"); err != nil {
		t.Fatal(err)
	}
	if a.settings.ShowSourceIcon {
		t.Fatal("expected ShowSourceIcon toggled to false")
	}

	if err := tt.Click("返回"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)

	// 2. PinSearchBox=true 时滚动不收起搜索栏，且出现“回到顶部”按钮
	tt.Wheel(panelWidth/2, panelHeight/2, 0, 260)
	tt.Advance(250 * time.Millisecond)
	if a.controlsCollapsed {
		t.Fatal("expected controls not collapsed when PinSearchBox=true")
	}
	if !tt.HasText("回到顶部") {
		t.Fatalf("expected scroll-top button visible after scrolling down, texts=%q", tt.Texts())
	}
	if err := tt.Click("回到顶部"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.listScroll.Y != 0 || !a.atTop {
		t.Fatalf("expected scrolled back to top, scrollY=%v atTop=%v", a.listScroll.Y, a.atTop)
	}

	// 3. Alt+2 快捷粘贴第 2 条并触发 DeleteAfterPaste 删除
	secondText := a.items[1].Text
	tt.Key(ui.Alt, ui.Key2)
	if tt.HasText(secondText) {
		t.Fatalf("expected second item %q deleted after Alt+2 quick paste", secondText)
	}
}

func TestI18nLocales(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a := newClipApp(st, nil)
	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	// 默认简体中文
	tt.SetLocale("zh-Hans")
	if !tt.HasText("暂无剪贴板记录") || !tt.HasText("富文本") {
		t.Fatalf("expected zh-Hans texts, got %q", tt.Texts())
	}

	// 切换繁體中文
	tt.SetLocale("zh-Hant")
	if !tt.HasText("暫無剪貼簿紀錄") || !tt.HasText("程式碼") {
		t.Fatalf("expected zh-Hant texts, got %q", tt.Texts())
	}

	// 切换英文
	tt.SetLocale("en")
	if !tt.HasText("No clipboard history") || !tt.HasText("Rich Text") {
		t.Fatalf("expected en texts, got %q", tt.Texts())
	}

	// 设置页英文显示
	if err := tt.Click("Settings"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if !tt.HasText("General") || !tt.HasText("Appearance") || !tt.HasText("Default Apps") {
		t.Fatalf("expected en settings groups, got %q", tt.Texts())
	}
}

func TestClearUnpinnedModalConfirm(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	pinned, err := st.UpsertText("pinned keep")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.TogglePin(pinned.ID); err != nil {
		t.Fatal(err)
	}
	tagged, err := st.UpsertText("tagged keep")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateTags(tagged.ID, []string{"保留"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertText("unpinned drop"); err != nil {
		t.Fatal(err)
	}

	a := newClipApp(st, nil)
	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)
	tt.SetLocale("zh-Hans")

	if err := tt.Click("清空记录"); err != nil {
		t.Fatal(err)
	}
	if !a.confirmClearOpen || len(a.items) != 3 {
		t.Fatalf("expected modal open without clearing yet, open=%v len=%d", a.confirmClearOpen, len(a.items))
	}
	if err := tt.Click("取消"); err != nil {
		t.Fatal(err)
	}
	if a.confirmClearOpen || len(a.items) != 3 {
		t.Fatalf("expected modal closed and items preserved after cancel, open=%v len=%d", a.confirmClearOpen, len(a.items))
	}

	if err := tt.Click("清空记录"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("清空"); err != nil {
		t.Fatal(err)
	}
	if a.confirmClearOpen || len(a.items) != 2 {
		t.Fatalf("expected pinned and tagged items kept after confirm, open=%v items=%+v", a.confirmClearOpen, a.items)
	}
}

func TestCategoryToggleResetsTop(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// 先插入一条链接记录（排在底部），再插入 12 条更新的文本记录（排在顶部）
	if _, err := st.SaveText("https://example.com/bottom-link", store.SaveOptions{Deduplicate: false}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(2 * time.Millisecond)
	for i := 0; i < 12; i++ {
		if _, err := st.SaveText(
			string(rune('A'+i))+" newer text item",
			store.SaveOptions{Deduplicate: false},
		); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	a := newClipApp(st, nil)
	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)
	tt.SetLocale("zh-Hans")

	if !tt.HasText("L newer text item") {
		t.Fatalf("expected newest text item at top initially, texts=%q", tt.Texts())
	}

	// 点击“链接”分类
	if err := tt.Click("链接"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("https://example.com/bottom-link") || tt.HasText("L newer text item") {
		t.Fatalf("expected only link item after selecting category, texts=%q", tt.Texts())
	}

	// 再次点击“链接”取消选中回到全部（无状态）：应回到顶部且保留顶部控制栏展开，而不是停留在底部链接卡片
	if err := tt.Click("链接"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.filterIdx != 0 || a.listScroll.Y != 0 || !a.atTop || a.controlsCollapsed {
		t.Fatalf("expected top reset after deselecting category, filterIdx=%d scrollY=%v atTop=%v collapsed=%v", a.filterIdx, a.listScroll.Y, a.atTop, a.controlsCollapsed)
	}
	if !tt.HasText("L newer text item") {
		t.Fatalf("expected newest text item visible at top after deselecting category, texts=%q", tt.Texts())
	}
}

func TestTabsAndSearchBehavior(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	tagged, err := st.UpsertText("alpha demo note")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateTags(tagged.ID, []string{"工作"}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertText("https://example.com/demo"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertText("const demo = 1; return demo;"); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if _, err := st.SaveText(
			string(rune('A'+i))+" extra plain row",
			store.SaveOptions{Deduplicate: false},
		); err != nil {
			t.Fatal(err)
		}
	}

	a := newClipApp(st, nil)
	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)
	tt.SetLocale("zh-Hans")

	if a.filterIdx != 0 {
		t.Fatalf("expected initial state to be 0 (unselected = all), got %d", a.filterIdx)
	}

	// 先选中“文本”类别，再聚焦空搜索框：标签浮层弹出；输入 demo 后浮层自动收起并拉取全类别命中结果
	if err := tt.Click("文本"); err != nil {
		t.Fatal(err)
	}
	if a.filterIdx != 1 || a.currentKind() != store.KindText {
		t.Fatalf("expected text tab selected, filterIdx=%d kind=%q", a.filterIdx, a.currentKind())
	}
	if err := tt.Click("搜索剪贴板"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("筛选标签 工作") {
		t.Fatalf("expected tag popover visible on empty focused search, texts=%q", tt.Texts())
	}
	tt.Type("demo")
	if tt.HasText("筛选标签 工作") {
		t.Fatalf("expected tag popover hidden once query is non-empty, texts=%q", tt.Texts())
	}
	if !tt.HasText("alpha demo note") || tt.HasText("https://example.com/demo") || tt.HasText("const demo = 1") {
		t.Fatalf("expected text tab to show text search hit only, texts=%q", tt.Texts())
	}

	if err := tt.Click("链接"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("https://example.com/demo") || tt.HasText("alpha demo note") {
		t.Fatalf("expected link tab to immediately show link search hit, texts=%q", tt.Texts())
	}

	if err := tt.Click("代码"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("const demo = 1; return demo;") || tt.HasText("https://example.com/demo") {
		t.Fatalf("expected code tab to immediately show code search hit, texts=%q", tt.Texts())
	}

	// 再次点击“代码”取消选中回到无状态（全部），看到全部搜索命中结果
	if err := tt.Click("代码"); err != nil {
		t.Fatal(err)
	}
	if a.filterIdx != 0 || !tt.HasText("alpha demo note") || !tt.HasText("https://example.com/demo") || !tt.HasText("const demo = 1; return demo;") {
		t.Fatalf("expected deselected (all) state to show all search hits, filterIdx=%d texts=%q", a.filterIdx, tt.Texts())
	}

	// 清空搜索后切换到“文本”Tab，向下滚动同样支持自动收起顶栏
	if err := tt.Click("清除搜索"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("文本"); err != nil {
		t.Fatal(err)
	}
	tt.Wheel(panelWidth/2, panelHeight/2, 0, 180)
	tt.Advance(250 * time.Millisecond)
	if !a.controlsCollapsed {
		t.Fatal("expected header controls to auto-collapse on scroll in category tab as well")
	}
}

func TestNoFocusModeSetting(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if _, err := st.UpsertText("sample item"); err != nil {
		t.Fatal(err)
	}

	a := newClipApp(st, nil)
	a.reload()
	if a.settings.NoFocusMode || a.panelNoActivate {
		t.Fatalf("expected NoFocusMode and panelNoActivate to default to false, got settings=%v noAct=%v", a.settings.NoFocusMode, a.panelNoActivate)
	}

	tt := ui.NewTester(a.view, panelWidth, panelHeight)
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("常规设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)

	if !tt.HasText("不抢焦点 (实验性)") {
		t.Fatalf("expected NoFocusMode row in general settings, texts=%q", tt.Texts())
	}
	if err := tt.Click("切换不抢焦点 (实验性)"); err != nil {
		t.Fatal(err)
	}
	if !a.settings.NoFocusMode || !a.panelNoActivate {
		t.Fatalf("expected NoFocusMode and panelNoActivate toggled to true, got settings=%v noAct=%v", a.settings.NoFocusMode, a.panelNoActivate)
	}
	loaded, err := st.LoadSettings()
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.NoFocusMode {
		t.Fatal("expected NoFocusMode=true persisted to store")
	}

	tt.SetLocale("zh-Hant")
	if !tt.HasText("不搶焦點 (實驗性)") {
		t.Fatalf("expected zh-Hant NoFocusMode label, texts=%q", tt.Texts())
	}
	tt.SetLocale("en")
	if !tt.HasText("No Focus Mode (Experimental)") {
		t.Fatalf("expected en NoFocusMode label, texts=%q", tt.Texts())
	}
	tt.SetLocale("zh-Hans")

	// 录制快捷键会临时退出无激活状态以接收按键
	if err := tt.Click("剪贴板设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("录制呼出快捷键"); err != nil {
		t.Fatal(err)
	}
	if a.panelNoActivate {
		t.Fatal("expected panelNoActivate=false while recording hotkey")
	}
	tt.Key(0, ui.KeyEscape)

	// 返回主列表后，点击搜索框与标签编辑均自动恢复激活以接收输入
	if err := tt.Click("返回"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	a.setPanelNoActivate(true)
	if err := tt.Click("搜索剪贴板"); err != nil {
		t.Fatal(err)
	}
	if a.panelNoActivate {
		t.Fatal("expected clicking search box to clear panelNoActivate for text input")
	}

	a.setPanelNoActivate(true)
	a.toggleTagEditor(a.items[0].ID)
	if a.panelNoActivate {
		t.Fatal("expected opening tag editor to clear panelNoActivate for text input")
	}
}
