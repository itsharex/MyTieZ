package app

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mytiez/internal/store"
	"mytiez/internal/sys"
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

	tt.Key(0, ui.KeyDown)
	if a.selected != 0 {
		t.Fatalf("expected selected=0 after KeyDown, got %d", a.selected)
	}
	tt.Key(0, ui.KeyEscape)
	if a.selected != -1 {
		t.Fatalf("expected Escape to clear selection, got %d", a.selected)
	}

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

func TestSettingsAndQuickPaste(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for i := 0; i < 15; i++ {
		if _, err := st.SaveText(
			string(rune('A'+i))+" setting entry",
			store.SaveOptions{Deduplicate: false},
		); err != nil {
			t.Fatal(err)
		}
	}

	a := newClipApp(st, nil)
	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if !a.showSettings || !tt.HasText("常规设置") || !tt.HasText("剪贴板设置") {
		t.Fatalf("expected settings groups visible, texts=%q", tt.Texts())
	}
	if a.expanded.general || tt.HasText("开机自启动") {
		t.Fatalf("expected groups collapsed by default, texts=%q", tt.Texts())
	}

	// 常规设置：失焦自动隐藏与固定搜索栏
	if err := tt.Click("常规设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("切换失焦自动隐藏"); err != nil {
		t.Fatal(err)
	}
	if a.settings.AutoHideOnBlur || !a.winPinned {
		t.Fatalf("expected AutoHideOnBlur=false and winPinned=true, got autoHide=%v winPinned=%v", a.settings.AutoHideOnBlur, a.winPinned)
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

	// 剪贴板设置：去重、上限、粘贴后删除、粘贴方式与快捷粘贴
	if err := tt.Click("剪贴板设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("切换自动去重"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("上限200"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("切换粘贴后删除"); err != nil {
		t.Fatal(err)
	}
	tt.Wheel(panelWidth/2, panelHeight/2, 0, 320)
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("Ctrl+V"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("游戏模式"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("快捷粘贴Alt"); err != nil {
		t.Fatal(err)
	}
	if a.settings.Deduplicate || a.settings.MaxHistory != 200 || !a.settings.DeleteAfterPaste ||
		a.settings.PasteMethod != "game_mode" || a.settings.QuickPasteModifier != "alt" {
		t.Fatalf("unexpected clipboard settings: %+v", a.settings)
	}
	tt.Wheel(panelWidth/2, panelHeight/2, 0, -320)
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("剪贴板设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)

	// 界面设置：窗口材质下拉与 Vibrancy 映射
	tt.SetVibrancy(true)
	if err := tt.Click("界面设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	for _, tc := range []struct {
		itemLabel string
		want      string
	}{
		{"材质 Mica Alt", "mica_alt"},
		{"材质 Acrylic", "acrylic_base"},
		{"材质 Acrylic Thin", "acrylic"},
	} {
		if err := tt.Click(tc.itemLabel); err != nil {
			t.Fatal(err)
		}
		if a.settings.BackdropMode != tc.want {
			t.Fatalf("expected BackdropMode=%s, got %q", tc.want, a.settings.BackdropMode)
		}
		if loaded, err := a.store.LoadSettings(); err != nil || loaded.BackdropMode != tc.want {
			t.Fatalf("expected persisted BackdropMode=%s, got %q err=%v", tc.want, loaded.BackdropMode, err)
		}
	}
	if backdropVibrancy("mica_alt") != mygo.VibrancyTabbed || backdropVibrancy("acrylic") != mygo.VibrancyAcrylic {
		t.Fatalf("unexpected vibrancy mapping")
	}
	if err := tt.Click("界面设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)

	// 默认打开程序：恢复默认
	a.settings.OpenAppText = `C:\Windows\System32\notepad.exe`
	if err := tt.Click("默认打开程序"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("恢复文本默认打开方式"); err != nil {
		t.Fatal(err)
	}
	if a.settings.OpenAppText != "" {
		t.Fatalf("expected OpenAppText cleared after reset, got %q", a.settings.OpenAppText)
	}

	// 返回主列表：验证 PinSearchBox、回到顶部按钮与 Alt+2 快捷粘贴删除
	if err := tt.Click("返回"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	tt.Wheel(panelWidth/2, panelHeight/2, 0, 260)
	tt.Advance(250 * time.Millisecond)
	if a.controlsCollapsed || !tt.HasText("回到顶部") {
		t.Fatalf("expected pinned controls and scroll-top button visible, collapsed=%v", a.controlsCollapsed)
	}
	if err := tt.Click("回到顶部"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.listScroll.Y != 0 || !a.atTop {
		t.Fatalf("expected scrolled back to top, scrollY=%v atTop=%v", a.listScroll.Y, a.atTop)
	}

	secondText := a.items[1].Text
	tt.Key(ui.Alt, ui.Key2)
	if tt.HasText(secondText) {
		t.Fatalf("expected second item %q deleted after Alt+2 quick paste", secondText)
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

	// 首次在顶部向下滚动：仅收起搜索栏并拦截本次列表滚动
	tt.Wheel(panelWidth/2, panelHeight/2, 0, 200)
	tt.Advance(250 * time.Millisecond)
	if !a.controlsCollapsed || a.listScroll.Y != 0 {
		t.Fatalf("expected controls collapsed at Y=0, got collapsed=%v scrollY=%v", a.controlsCollapsed, a.listScroll.Y)
	}

	// 继续向下滚动离开顶部
	tt.Wheel(panelWidth/2, panelHeight/2, 0, 200)
	tt.Advance(250 * time.Millisecond)
	if a.atTop || a.listScroll.Y <= hideScrollOffset {
		t.Fatalf("expected list scrolled away from top, got atTop=%v scrollY=%v", a.atTop, a.listScroll.Y)
	}

	// 滑回顶部后进入冷却期不立即展开，再次向上滚动才展开
	tt.Wheel(panelWidth/2, panelHeight/2, 0, -400)
	tt.Advance(250 * time.Millisecond)
	if !a.atTop || !a.controlsCollapsed {
		t.Fatalf("expected atTop=true and controls still collapsed right after reaching top, got atTop=%v collapsed=%v", a.atTop, a.controlsCollapsed)
	}
	tt.Wheel(panelWidth/2, panelHeight/2, 0, -40)
	tt.Advance(250 * time.Millisecond)
	if a.controlsCollapsed || a.listScroll.Y != 0 {
		t.Fatalf("expected controls expanded at Y=0, got collapsed=%v scrollY=%v", a.controlsCollapsed, a.listScroll.Y)
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

	if a.listScroll.Y != 0 || !tt.HasText("brand new top entry") {
		t.Fatalf("expected new top entry visible at Y=0, scrollY=%v texts=%q", a.listScroll.Y, tt.Texts())
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

	// Verify UTF-16LE clipboard HTML decodes cleanly.
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

	// Verify leading plain text before an inline tag survives CF_HTML header stripping.
	prefixedHTML := `感源自 <a href="https://github.com/jimuzhe/tiez-clipboard">tiez-clipboard</a>。`
	cfPrefixSpans := parseHTMLPreviewSpans("Version:0.9\r\nStartHTML:000080\r\nEndHTML:000160\r\n"+prefixedHTML, myTiezLightTheme)
	if len(cfPrefixSpans) != 3 || cfPrefixSpans[0].Text != "感源自 " || cfPrefixSpans[1].Text != "tiez-clipboard" || !cfPrefixSpans[1].Underline || cfPrefixSpans[2].Text != "。" {
		t.Fatalf("expected CF_HTML headers stripped without losing leading text, got %+v", cfPrefixSpans)
	}
	if _, err := st.SaveText("感源自 tiez-clipboard。", store.SaveOptions{
		Deduplicate: true,
		HTML:        prefixedHTML,
	}); err != nil {
		t.Fatal(err)
	}

	a := newClipApp(st, nil)
	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	if err := tt.Click("富文本"); err != nil {
		t.Fatal(err)
	}
	if a.currentKind() != store.KindRichText || tt.HasText("plain only item") || !tt.HasText("Bold") || !tt.HasText("tiez-clipboard") {
		t.Fatalf("expected rich_text filter result, kind=%q texts=%q", a.currentKind(), tt.Texts())
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

	if err := tt.Click("完成标签编辑"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("#工作"); err != nil {
		t.Fatal(err)
	}
	if a.query != "#工作" || !tt.HasText("second plain entry") || tt.HasText("first tagged entry") {
		t.Fatalf("expected tag filter #工作, query=%q texts=%q", a.query, tt.Texts())
	}

	// 多标签折叠为 +N 并通过搜索标签浮层筛选
	if err := tt.Click("#工作"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpdateTags(a.items[0].ID, []string{"工作", "一个非常非常长的标签名称用于测试预算", "第三个标签", "第四个标签", "第五个标签"}); err != nil {
		t.Fatal(err)
	}
	a.reload()
	tt.Frame()
	if err := tt.Click("展开全部标签"); err != nil {
		t.Fatal(err)
	}
	if a.editingTagsID != a.items[0].ID || !tt.HasText("第五个标签") {
		t.Fatalf("expected clicking +N to open full tag editor, editingTagsID=%d texts=%q", a.editingTagsID, tt.Texts())
	}
	if err := tt.Click("完成标签编辑"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("搜索剪贴板"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("筛选标签 第五个标签"); err != nil {
		t.Fatal(err)
	}
	if a.query != "#第五个标签" || !tt.HasText("second plain entry") || tt.HasText("first tagged entry") {
		t.Fatalf("expected popover tag filter #第五个标签, query=%q texts=%q", a.query, tt.Texts())
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

	if err := tt.Click("录制呼出快捷键"); err != nil {
		t.Fatal(err)
	}
	if !a.recordingHotkey || !tt.HasText("请按下快捷键...") {
		t.Fatalf("expected recording state active, recording=%v texts=%q", a.recordingHotkey, tt.Texts())
	}

	// 无修饰键普通按键应被拒绝
	tt.Key(0, ui.KeyA)
	if !a.recordingHotkey || a.settings.Hotkey != "Alt+V" || !tt.HasText(locales.SettingsHotkeyNeedMod()) {
		t.Fatalf("expected plain key A rejected with toast, recording=%v hotkey=%q", a.recordingHotkey, a.settings.Hotkey)
	}

	tt.Key(0, ui.KeyEscape)
	if a.recordingHotkey {
		t.Fatal("expected Esc to cancel recording")
	}

	// 录制组合键 Ctrl+Shift+K 与功能键 F8，随后恢复默认
	if err := tt.Click("录制呼出快捷键"); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Ctrl|ui.Shift, ui.KeyK)
	if a.recordingHotkey || a.settings.Hotkey != "Ctrl+Shift+K" || a.shortcut != "Ctrl+Shift+K" {
		t.Fatalf("expected recorded Ctrl+Shift+K, hotkey=%q shortcut=%q", a.settings.Hotkey, a.shortcut)
	}
	if err := tt.Click("录制呼出快捷键"); err != nil {
		t.Fatal(err)
	}
	tt.Key(0, ui.KeyF8)
	if a.recordingHotkey || a.settings.Hotkey != "F8" {
		t.Fatalf("expected recorded F8, hotkey=%q", a.settings.Hotkey)
	}
	if err := tt.Click("恢复默认快捷键"); err != nil {
		t.Fatal(err)
	}
	if a.settings.Hotkey != "Alt+V" || a.shortcut != "Alt+V" {
		t.Fatalf("expected hotkey reset to Alt+V, hotkey=%q shortcut=%q", a.settings.Hotkey, a.shortcut)
	}
}

func TestFollowMouseTarget(t *testing.T) {
	workArea := mygo.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1040}
	w, h := 400, 560

	if x, y := followMouseTarget(mygo.Point{X: 960, Y: 200}, w, h, workArea); x != 960 || y != 200 {
		t.Fatalf("expected (960, 200), got (%d, %d)", x, y)
	}
	if x, y := followMouseTarget(mygo.Point{X: 960, Y: 900}, w, h, workArea); x != 960 || y != 900-h {
		t.Fatalf("expected flipped (960, %d), got (%d, %d)", 900-h, x, y)
	}
	if x, _ := followMouseTarget(mygo.Point{X: 2, Y: 200}, w, h, workArea); x != 8 {
		t.Fatalf("expected clamped left 8, got %d", x)
	}
	if x, y := followMouseTarget(mygo.Point{X: 1700, Y: 900}, w, h, workArea); x != 1920-w-8 || y != 900-h {
		t.Fatalf("expected (%d, %d), got (%d, %d)", 1920-w-8, 900-h, x, y)
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

	tt.SetLocale("zh-Hans")
	if !tt.HasText("暂无剪贴板记录") || !tt.HasText("富文本") {
		t.Fatalf("expected zh-Hans texts, got %q", tt.Texts())
	}
	tt.SetLocale("zh-Hant")
	if !tt.HasText("暫無剪貼簿紀錄") || !tt.HasText("程式碼") {
		t.Fatalf("expected zh-Hant texts, got %q", tt.Texts())
	}
	tt.SetLocale("en")
	if !tt.HasText("No clipboard history") || !tt.HasText("Rich Text") {
		t.Fatalf("expected en texts, got %q", tt.Texts())
	}
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
		t.Fatalf("expected modal closed and items preserved, open=%v len=%d", a.confirmClearOpen, len(a.items))
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

	if a.filterIdx != 0 || tt.HasText("筛选标签 工作") {
		t.Fatalf("expected initial filterIdx=0 and tag popover closed, filterIdx=%d", a.filterIdx)
	}

	// 选中“文本”再聚焦搜索框弹出标签浮层，输入关键词后跨 Tab 切换即时过滤
	if err := tt.Click("文本"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("搜索剪贴板"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("筛选标签 工作") {
		t.Fatalf("expected tag popover visible on empty focused search, texts=%q", tt.Texts())
	}
	tt.Type("demo")
	if tt.HasText("筛选标签 工作") || !tt.HasText("alpha demo note") || tt.HasText("https://example.com/demo") {
		t.Fatalf("expected text tab to show text hit and hide popover, texts=%q", tt.Texts())
	}
	if err := tt.Click("链接"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("https://example.com/demo") || tt.HasText("alpha demo note") {
		t.Fatalf("expected link tab hit, texts=%q", tt.Texts())
	}
	if err := tt.Click("代码"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("const demo = 1; return demo;") || tt.HasText("https://example.com/demo") {
		t.Fatalf("expected code tab hit, texts=%q", tt.Texts())
	}
	// 再次点击“代码”取消选中回到全部
	if err := tt.Click("代码"); err != nil {
		t.Fatal(err)
	}
	if a.filterIdx != 0 || !tt.HasText("alpha demo note") || !tt.HasText("https://example.com/demo") {
		t.Fatalf("expected all search hits after deselecting tab, filterIdx=%d", a.filterIdx)
	}

	// 滚动后点击清除搜索应回到顶部并收起标签浮层
	tt.Wheel(panelWidth/2, panelHeight/2, 0, 120)
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("清除搜索"); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if a.query != "" || a.listScroll.Y != 0 || !a.atTop || a.controlsCollapsed || tt.HasText("筛选标签 工作") {
		t.Fatalf("expected search cleared at top with popover closed, query=%q scrollY=%v atTop=%v collapsed=%v", a.query, a.listScroll.Y, a.atTop, a.controlsCollapsed)
	}
}

func TestNoFocusModeAndRoutedSearch(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if _, err := st.UpsertText("微信聊天记录"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.UpsertText("second item"); err != nil {
		t.Fatal(err)
	}

	a := newClipApp(st, nil)
	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	// 1. 设置页开启不抢焦点并验证输入场景自动恢复激活
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("常规设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("切换不抢焦点 (实验性)"); err != nil {
		t.Fatal(err)
	}
	if !a.settings.NoFocusMode || !a.panelNoActivate {
		t.Fatalf("expected NoFocusMode and panelNoActivate true")
	}
	if err := tt.Click("返回"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)

	// 2. 键盘路由激活时点击搜索框与拼音输入全程保持 no-activate
	a.stopKeyRouter = func() {}
	a.onSearchBoxPointerDown()
	if !a.panelNoActivate || !a.routedSearchActive {
		t.Fatalf("expected routed search without activating panel")
	}
	a.applyRoutedKey(sys.RoutedKey{VK: 'W', Text: "w"})
	a.applyRoutedKey(sys.RoutedKey{VK: 'X', Text: "x"})
	a.applyRoutedKey(sys.RoutedKey{VK: 'A', Text: "a"})
	a.applyRoutedKey(sys.RoutedKey{VK: vkBack})
	a.stopSearchTimer()
	a.reload()
	if a.query != "wx" || len(a.items) != 1 || a.items[0].Text != "微信聊天记录" {
		t.Fatalf("expected pinyin routed search hit, query=%q items=%+v", a.query, a.items)
	}
	a.applyRoutedKey(sys.RoutedKey{VK: vkDown})
	if a.selected != 0 {
		t.Fatalf("expected selected=0 after vkDown, got %d", a.selected)
	}
	a.applyRoutedKey(sys.RoutedKey{VK: vkEscape})
	if a.query != "" || a.routedSearchActive || !a.panelNoActivate {
		t.Fatalf("expected query cleared and no-focus preserved")
	}

	// 3. 无键盘路由时点击搜索框或打开标签编辑应回退为激活窗口
	a.stopKeyRouting()
	a.onSearchBoxPointerDown()
	if a.panelNoActivate {
		t.Fatal("expected fallback to activatePanelForInput when key router is inactive")
	}
	a.setPanelNoActivate(true)
	a.toggleTagEditor(a.items[0].ID)
	if a.panelNoActivate {
		t.Fatal("expected opening tag editor to clear panelNoActivate")
	}
}

func TestPrivacyProtectionUI(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a := newClipApp(st, nil)
	it, err := a.saveTextWithPrivacy("13800138000", store.SaveOptions{Deduplicate: true})
	if err != nil || !store.HasSensitiveTag(it.Tags) {
		t.Fatalf("expected sensitive tag on phone item, got %+v err=%v", it.Tags, err)
	}

	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)
	if tt.HasText("13800138000") || !tt.HasText("138••••••000") {
		t.Fatalf("expected masked text '138••••••000', texts=%q", tt.Texts())
	}
	if err := tt.Click("显示敏感内容"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("13800138000") {
		t.Fatalf("expected revealed text after clicking eye button, texts=%q", tt.Texts())
	}
	if err := tt.Click("隐藏窗口"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("13800138000") {
		t.Fatalf("expected sensitive item re-masked after hidePanel, texts=%q", tt.Texts())
	}

	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("剪贴板设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if !tt.HasText("保护种类") || !tt.HasText("自定义规则") || !tt.HasText("脱敏预览设置") || tt.HasText("密钥/Token") {
		t.Fatalf("expected collapsed privacy subgroups, texts=%q", tt.Texts())
	}
	tt.Wheel(panelWidth/2, panelHeight/2, 0, 200)
	tt.Advance(250 * time.Millisecond)
	if err := tt.Click("保护种类"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if !tt.HasText("密钥/Token") {
		t.Fatalf("expected privacyKinds expanded after click, texts=%q", tt.Texts())
	}
}

func TestHiddenMemoryAndCacheRetention(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a := newClipApp(st, nil)
	a.richSpans = map[int64][]ui.Span{1: {{Text: "cached span"}}}
	for i := 0; i < warmThumbCacheLimit; i++ {
		a.thumbs.put(string(rune('a'+i)), &ui.Bitmap{})
	}
	a.thumbs.setActiveKeys(map[string]struct{}{"queued-key": {}})

	// 低于 75MB 阈值时，隐藏保留热缓存上限 warmThumbCacheLimit 与 richSpans，同时收窄 activeKeys 取消排队解码
	a.hidePanel()
	if a.thumbs.len() != warmThumbCacheLimit || len(a.richSpans) != 1 {
		t.Fatalf("expected warm thumb cache (%d) and richSpans preserved, got %d / %d", warmThumbCacheLimit, a.thumbs.len(), len(a.richSpans))
	}
	if a.thumbs.isActive("queued-key") {
		t.Fatalf("expected pending decode keys deactivated on hide")
	}

	a.thumbs.put("overflow", &ui.Bitmap{})
	a.hidePanel()
	if a.thumbs.len() != warmThumbCacheLimit || len(a.richSpans) != 1 {
		t.Fatalf("expected oversized thumb cache pruned to %d, got %d", warmThumbCacheLimit, a.thumbs.len())
	}
}

func TestEdgeDockingAndImageGeometry(t *testing.T) {
	workArea := mygo.Rectangle{X: 0, Y: 0, Width: 1920, Height: 1040}
	if got := detectEdgeDock(mygo.Rectangle{X: 300, Y: 200, Width: 400, Height: 660}, workArea, edgeDockThreshold); got != dockNone {
		t.Fatalf("expected center window dockNone, got %v", got)
	}

	topWin := mygo.Rectangle{X: 300, Y: 3, Width: 400, Height: 660}
	if got := detectEdgeDock(topWin, workArea, edgeDockThreshold); got != dockTop {
		t.Fatalf("expected dockTop, got %v", got)
	}
	dx, dy := dockedWindowPosition(dockTop, topWin, workArea, edgeDockHideSize)
	dockedTopRect := mygo.Rectangle{X: dx, Y: dy, Width: 400, Height: 660}
	if !isCursorInDockedSliver(dockTop, mygo.Point{X: 450, Y: 2}, dockedTopRect, workArea, edgeDockHoverSliver) {
		t.Fatalf("expected cursor inside top docked sliver")
	}
	if ux, uy := undockedWindowPosition(dockTop, dockedTopRect, workArea); ux != 300 || uy != 0 {
		t.Fatalf("unexpected undocked top pos (%d, %d)", ux, uy)
	}
	if got := detectEdgeDock(mygo.Rectangle{X: 4, Y: 120, Width: 400, Height: 660}, workArea, edgeDockThreshold); got != dockLeft {
		t.Fatalf("expected dockLeft, got %v", got)
	}
	if got := detectEdgeDock(mygo.Rectangle{X: 1516, Y: 120, Width: 400, Height: 660}, workArea, edgeDockThreshold); got != dockRight {
		t.Fatalf("expected dockRight, got %v", got)
	}

	a := newClipApp(nil, nil)
	wideBmp := ui.NewBitmap(image.NewRGBA(image.Rect(0, 0, 1200, 200)))
	for _, winW := range []int{minPanelWidth, panelWidth, 520} {
		a.settings.WindowWidth = winW
		maxW := a.cardContentWidth()
		w, h := fitImageSize(wideBmp, maxW, 100)
		if w > maxW || h <= 0 || h > 100 {
			t.Fatalf("winW=%d: unexpected fitted size (%.1f, %.1f) for maxW=%.1f", winW, w, h, maxW)
		}
	}
}

func TestSaveAsyncSerializesAndDeduplicates(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a := newClipApp(st, nil)
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	var active atomic.Int32
	var maxActive atomic.Int32
	done := make(chan struct{})

	recordActive := func() func() {
		cur := active.Add(1)
		for {
			prev := maxActive.Load()
			if cur <= prev || maxActive.CompareAndSwap(prev, cur) {
				break
			}
		}
		return func() { active.Add(-1) }
	}

	a.saveAsync(func() (string, error) {
		defer recordActive()()
		close(firstStarted)
		<-releaseFirst
		it, err := st.SaveText("first-item", store.SaveOptions{Deduplicate: true})
		return it.Hash, err
	})
	<-firstStarted

	for i := 0; i < 4; i++ {
		a.saveAsync(func() (string, error) {
			defer recordActive()()
			it, err := st.SaveText("first-item", store.SaveOptions{Deduplicate: true})
			return it.Hash, err
		})
	}
	for i := 0; i < 4; i++ {
		idx := i
		a.saveAsync(func() (string, error) {
			defer recordActive()()
			it, err := st.SaveText("second-item", store.SaveOptions{Deduplicate: true})
			if idx == 3 {
				close(done)
			}
			return it.Hash, err
		})
	}
	close(releaseFirst)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for saveAsync queue to drain")
	}

	if got := maxActive.Load(); got != 1 {
		t.Fatalf("expected saveAsync tasks to execute serially (maxActive=1), got %d", got)
	}

	items, err := st.List("", "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 || items[0].Text != "second-item" || items[1].Text != "first-item" {
		t.Fatalf("expected FIFO deduplicated [second-item, first-item], got %+v", items)
	}
}

func TestAppHelpersAndSecurity(t *testing.T) {
	if err := runWithTimeout(50*time.Millisecond, func() error { return nil }); err != nil {
		t.Fatalf("expected nil error on fast task, got %v", err)
	}
	if err := runWithTimeout(20*time.Millisecond, func() error {
		time.Sleep(80 * time.Millisecond)
		return nil
	}); err == nil {
		t.Fatal("expected timeout error on slow task")
	}

	if !isHTTPURL("https://example.com/a?b=1") || !isHTTPURL("http://localhost:8080") {
		t.Fatal("expected http/https URLs recognized")
	}
	if isHTTPURL("ftp://example.com") || isHTTPURL("file:///C:/Windows/System32/cmd.exe") || isBareWWWURL("ms-msdt:/id") {
		t.Fatal("expected non-http URI schemes rejected")
	}
	if !isBareWWWURL("www.example.com/docs") {
		t.Fatal("expected www. prefix recognized")
	}
	if got := openAppDisplayName(locales.In(""), ""); got != "系统默认" {
		t.Fatalf("expected empty app path to display 系统默认, got %q", got)
	}
	if got := openAppDisplayName(locales.In(""), `C:\Program Files\Notepad++\notepad++.exe`); got != "notepad++" {
		t.Fatalf("expected exe suffix stripped, got %q", got)
	}

	rawHTML := `<div onclick="alert(1)"><script>evil()</script><iframe src="https://x.test"></iframe><a href="javascript:alert(2)">bad</a><a href="https://example.com">ok</a></div>`
	clean := sanitizeHTMLForExternalOpen(rawHTML)
	for _, forbidden := range []string{"<script", "<iframe", "onclick", "javascript:"} {
		if strings.Contains(strings.ToLower(clean), forbidden) {
			t.Fatalf("expected %q stripped from sanitized HTML, got %q", forbidden, clean)
		}
	}
	if !strings.Contains(clean, "https://example.com") {
		t.Fatalf("expected safe link preserved, got %q", clean)
	}
	if got := truncateUTF8Bytes("中文测试ABC", 5); got != "中" {
		t.Fatalf("expected UTF-8 safe truncation to %q, got %q", "中", got)
	}

	a := newClipApp(nil, nil)
	p := filepath.Join(t.TempDir(), "MyTieZ_entry_42.txt")
	if err := os.WriteFile(p, []byte("tmp"), 0o600); err != nil {
		t.Fatal(err)
	}
	a.trackTempFile(p)
	a.cleanupTempFiles()
	if _, err := os.Stat(p); !os.IsNotExist(err) {
		t.Fatalf("expected tracked temp file removed on cleanup, err=%v", err)
	}
}

func TestMicaPresets(t *testing.T) {
	mica, ok := backdropPresetFor("mica")
	if !ok || !mica.isMica || mica.vibrancy != mygo.VibrancyMica {
		t.Fatalf("unexpected mica preset: %+v ok=%v", mica, ok)
	}
	micaAlt, ok := backdropPresetFor("mica_alt")
	if !ok || !micaAlt.isMica || micaAlt.vibrancy != mygo.VibrancyTabbed {
		t.Fatalf("unexpected mica_alt preset: %+v ok=%v", micaAlt, ok)
	}
	acrylicBase, ok := backdropPresetFor("acrylic_base")
	if !ok || acrylicBase.isMica || acrylicBase.vibrancy != mygo.VibrancyAcrylic {
		t.Fatalf("unexpected acrylic_base preset: %+v ok=%v", acrylicBase, ok)
	}
	acrylic, ok := backdropPresetFor("acrylic")
	if !ok || acrylic.isMica || acrylic.vibrancy != mygo.VibrancyAcrylic {
		t.Fatalf("unexpected acrylic preset: %+v ok=%v", acrylic, ok)
	}

	// WinUI 3 标准：Mica/Mica Alt 根背景透明由 DWM 原生绘制；Acrylic 叠加高白度/深灰面纱且标准 > 轻薄。
	if mica.rootOverlayDark != 0 || mica.rootOverlayLight != 0 || micaAlt.rootOverlayDark != 0 || micaAlt.rootOverlayLight != 0 {
		t.Fatalf("expected mica/mica_alt transparent root overlays, got mica=(%.2f,%.2f) mica_alt=(%.2f,%.2f)",
			mica.rootOverlayDark, mica.rootOverlayLight, micaAlt.rootOverlayDark, micaAlt.rootOverlayLight)
	}
	if acrylicBase.rootOverlayLight <= acrylic.rootOverlayLight || acrylicBase.rootOverlayDark <= acrylic.rootOverlayDark {
		t.Fatalf("expected acrylic_base tint opacity > acrylic, got light=%.2f/%.2f dark=%.2f/%.2f",
			acrylicBase.rootOverlayLight, acrylic.rootOverlayLight, acrylicBase.rootOverlayDark, acrylic.rootOverlayDark)
	}
}


func TestPinnedBadgeRemovedAndDragReorder(t *testing.T) {
	st, a := newTestAppWithStore(t)
	i1, _ := st.SaveText("pinned one", store.SaveOptions{})
	i2, _ := st.SaveText("pinned two", store.SaveOptions{})
	i3, _ := st.SaveText("unpinned three", store.SaveOptions{})
	_, _ = st.TogglePin(i1.ID)
	_, _ = st.TogglePin(i2.ID)
	a.reload()

	tt := ui.NewTester(a.view, panelWidth, panelHeight)
	msgs := locales.In("zh-Hans")
	for _, info := range tt.AccessAll() {
		if info.Role == ui.RoleText && info.Name == msgs.CardPinned() {
			t.Fatalf("expected %q text badge removed from pinned cards, found %+v", msgs.CardPinned(), info)
		}
	}
	if a.list.Reorder != nil {
		t.Fatalf("expected list.Reorder nil so unpinned cards are never draggable")
	}

	if !a.list.KeepOffset {
		t.Fatalf("expected list.KeepOffset=true so pinned reorder keeps scroll position")
	}

	// Pointer-drag on unpinned card must not initiate drag or reorder.
	unpinnedRect, ok := tt.Find("unpinned three")
	if !ok {
		t.Fatalf("expected unpinned three in view")
	}
	pinnedTwoRect, ok := tt.Find("pinned two")
	if !ok {
		t.Fatalf("expected pinned two in view")
	}
	tt.Press(unpinnedRect.X+20, unpinnedRect.Y+unpinnedRect.H/2)
	tt.Move(pinnedTwoRect.X+20, pinnedTwoRect.Y+2)
	tt.Release(pinnedTwoRect.X+20, pinnedTwoRect.Y+2)
	if a.items[0].ID != i2.ID || a.items[1].ID != i1.ID || a.items[2].ID != i3.ID {
		t.Fatalf("expected dragging unpinned card ignored, got [%d,%d,%d]", a.items[0].ID, a.items[1].ID, a.items[2].ID)
	}

	// Pointer-drag pinned row 0 (pinned two) onto pinned row 1 (pinned one) -> [i1, i2, i3] and keeps scroll at top.
	pinnedOneRect, ok := tt.Find("pinned one")
	if !ok {
		t.Fatalf("expected pinned one in view")
	}
	tt.Press(pinnedTwoRect.X+20, pinnedTwoRect.Y+pinnedTwoRect.H/2)
	tt.Move(pinnedOneRect.X+20, pinnedOneRect.Y+pinnedOneRect.H/2)
	tt.Release(pinnedOneRect.X+20, pinnedOneRect.Y+pinnedOneRect.H/2)
	tt.Frame()
	if len(a.items) != 3 || a.items[0].ID != i1.ID || a.items[1].ID != i2.ID || a.items[2].ID != i3.ID {
		t.Fatalf("expected pinned reorder via drag -> [%d,%d,%d], got [%d,%d,%d]", i1.ID, i2.ID, i3.ID, a.items[0].ID, a.items[1].ID, a.items[2].ID)
	}
	if a.listScroll.Y != 0 {
		t.Fatalf("expected scroll Y to stay at 0 after reordering top pinned item, got %.1f", a.listScroll.Y)
	}

	// Left-clicking both unpinned and pinned cards must trigger paste.
	prevUnpinnedUpdated := a.items[2].UpdatedAt
	time.Sleep(5 * time.Millisecond)
	a.settings.MoveToTopOnPaste = true
	tt.Press(unpinnedRect.X+20, unpinnedRect.Y+unpinnedRect.H/2)
	tt.Release(unpinnedRect.X+20, unpinnedRect.Y+unpinnedRect.H/2)
	tt.Frame()
	if !a.items[2].UpdatedAt.After(prevUnpinnedUpdated) {
		t.Fatalf("expected left-clicking unpinned card to trigger paste (%v <= %v)", a.items[2].UpdatedAt, prevUnpinnedUpdated)
	}

	pinnedRectNow, ok := tt.Find("pinned one")
	if !ok {
		t.Fatalf("expected pinned one in view after reorder")
	}
	prevPinnedUpdated := a.items[0].UpdatedAt
	time.Sleep(5 * time.Millisecond)
	tt.Press(pinnedRectNow.X+20, pinnedRectNow.Y+pinnedRectNow.H/2)
	tt.Release(pinnedRectNow.X+20, pinnedRectNow.Y+pinnedRectNow.H/2)
	tt.Frame()
	if !a.items[0].UpdatedAt.After(prevPinnedUpdated) {
		t.Fatalf("expected left-clicking pinned card to trigger paste (%v <= %v)", a.items[0].UpdatedAt, prevPinnedUpdated)
	}
}

func TestThumbWindowRetentionAndEviction(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a := newClipApp(st, nil)
	items := make([]store.ClipItem, 50)
	for i := range items {
		path := fmt.Sprintf("img-%02d.png", i)
		items[i] = store.ClipItem{
			ID:        int64(i + 1),
			Kind:      store.KindImage,
			ImagePath: path,
		}
		a.thumbs.put(path, &ui.Bitmap{})
	}
	// Mark index 10 as sensitive and unrevealed; syncThumbWindow should evict it immediately.
	items[10].Tags = []string{store.SensitiveTag}

	// Viewport [8, 12] touches [8..12], evicts sensitive index 10, and prunes total cache to maxThumbCacheCount.
	a.syncThumbWindow(items, 8, 12)
	if a.thumbs.len() != maxThumbCacheCount {
		t.Fatalf("expected cache pruned to %d, got %d", maxThumbCacheCount, a.thumbs.len())
	}
	if _, exists := a.thumbs.items[items[10].ImagePath]; exists {
		t.Fatalf("expected sensitive unrevealed thumb %s evicted", items[10].ImagePath)
	}
	for i := 8; i <= 12; i++ {
		if i == 10 {
			continue
		}
		if _, exists := a.thumbs.items[items[i].ImagePath]; !exists {
			t.Fatalf("expected visible thumb %d (%s) retained", i, items[i].ImagePath)
		}
	}
	// Recently used top entry (when total <= maxThumbCacheCount) stays cached on scroll down.
	if _, exists := a.thumbs.items[items[49].ImagePath]; !exists {
		t.Fatalf("expected recent entry %s retained within LRU capacity", items[49].ImagePath)
	}
	if _, exists := a.thumbs.items[items[0].ImagePath]; exists {
		t.Fatalf("expected oldest LRU entry %s evicted", items[0].ImagePath)
	}

	// Empty / invalid range does not drop existing warm cache, and empty items clears activeKeys.
	before := a.thumbs.len()
	a.syncThumbWindow(items, 0, -1)
	if a.thumbs.len() != before {
		t.Fatalf("expected warm cache preserved (%d) on invalid range, got %d", before, a.thumbs.len())
	}
	a.syncThumbWindow(nil, 0, -1)
	if a.thumbs.len() != before {
		t.Fatalf("expected warm cache preserved (%d) on empty items, got %d", before, a.thumbs.len())
	}
	if a.thumbs.isActive(items[8].ImagePath) {
		t.Fatalf("expected empty items to clear activeKeys")
	}
}

func TestListViewportThumbEvictionOnScroll(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	a := newClipApp(st, nil)
	const total = 32
	items := make([]store.ClipItem, total)
	for i := range items {
		path := fmt.Sprintf("scroll-img-%02d.png", i)
		items[i] = store.ClipItem{
			ID:        int64(i + 1),
			Kind:      store.KindImage,
			ImagePath: path,
			Preview:   fmt.Sprintf("100 × 100 · #%02d", i),
		}
	}
	// Seed cache in ascending order so [0..7] are oldest, then render top viewport so [0..firstLast] become MRU.
	for _, it := range items {
		a.thumbs.put(it.ImagePath, &ui.Bitmap{})
	}
	a.items = items

	tt := ui.NewTester(a.view, panelWidth, panelHeight)
	tt.Frame()

	first0, last0 := a.list.Visible()
	if last0 < first0 {
		t.Fatalf("expected visible rows after initial frame, got (%d, %d)", first0, last0)
	}
	if a.thumbs.len() > maxThumbCacheCount {
		t.Fatalf("expected cache bounded to %d, got %d", maxThumbCacheCount, a.thumbs.len())
	}
	if _, ok := a.thumbs.items[items[0].ImagePath]; !ok {
		t.Fatalf("expected top visible image retained at top")
	}

	// Scroll to bottom: visible bottom rows are touched, and because top rows were recently viewed,
	// top row 0 remains cached within the 24-item LRU budget (avoiding decode spikes on next open).
	a.list.ScrollToEnd()
	tt.Frame()
	tt.Frame()

	first1, last1 := a.list.Visible()
	if last1 < first1 || last1 < total-2 {
		t.Fatalf("expected list scrolled near bottom, got visible (%d, %d)", first1, last1)
	}
	if a.thumbs.len() > maxThumbCacheCount {
		t.Fatalf("expected cache bounded to %d after scroll, got %d", maxThumbCacheCount, a.thumbs.len())
	}
	if _, ok := a.thumbs.items[items[total-1].ImagePath]; !ok {
		t.Fatalf("expected bottom visible image retained after scrolling to bottom")
	}
	if _, ok := a.thumbs.items[items[0].ImagePath]; !ok {
		t.Fatalf("expected recently viewed top image retained in LRU after scrolling to bottom")
	}
}

func TestUnloadHiddenIdleStateAndReload(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if _, err := st.SaveText("idle-unload-item", store.SaveOptions{Deduplicate: true, MaxHistory: 50}); err != nil {
		t.Fatal(err)
	}

	a := newClipApp(st, nil)
	a.reload()
	if len(a.items) != 1 {
		t.Fatalf("expected 1 loaded item, got %d", len(a.items))
	}
	a.thumbs.put("t1", &ui.Bitmap{})
	a.appIcons.put("i1", &ui.Bitmap{})
	a.richSpans = map[int64][]ui.Span{1: {{Text: "span"}}}
	a.searchHits = append([]store.ClipItem(nil), a.items...)
	a.selected = 0

	// Stale timer generation callback must not unload active state.
	staleGen := a.idleUnloadGen
	a.stopIdleUnloadTimer()
	a.unloadHiddenIdleStateForGen(staleGen)
	if len(a.items) != 1 || a.thumbs.len() != 1 {
		t.Fatalf("expected stale idle unload gen ignored, got items=%d thumbs=%d", len(a.items), a.thumbs.len())
	}

	a.unloadHiddenIdleState()

	if a.thumbs.len() != 0 || a.appIcons.len() != 0 || len(a.richSpans) != 0 {
		t.Fatalf("expected all caches cleared on idle unload, got thumbs=%d icons=%d spans=%d", a.thumbs.len(), a.appIcons.len(), len(a.richSpans))
	}
	if len(a.items) != 0 || len(a.searchHits) != 0 || a.selected != -1 || a.loadedRevision != 0 {
		t.Fatalf("expected list state unloaded, got items=%d searchHits=%d selected=%d rev=%d", len(a.items), len(a.searchHits), a.selected, a.loadedRevision)
	}

	// Next wake-up reloads transparently via reloadIfStale.
	a.reloadIfStale()
	if len(a.items) != 1 || a.items[0].Text != "idle-unload-item" {
		t.Fatalf("expected item reloaded after idle unload, got %+v", a.items)
	}
}

func TestHiddenPanelSettlesListAndFirstFrameWithoutJump(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	for i := 0; i < 15; i++ {
		if _, err := st.SaveText(
			fmt.Sprintf("entry-%02d clipboard item", i),
			store.SaveOptions{Deduplicate: false},
		); err != nil {
			t.Fatal(err)
		}
		time.Sleep(2 * time.Millisecond)
	}

	a := newClipApp(st, nil)
	a.settings.MoveToTopOnPaste = true
	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	// Scroll down so controls collapse and scroll Y leaves the top.
	tt.Wheel(panelWidth/2, panelHeight/2, 0, 200)
	tt.Advance(250 * time.Millisecond)
	tt.Wheel(panelWidth/2, panelHeight/2, 0, 300)
	tt.Advance(250 * time.Millisecond)
	if !a.controlsCollapsed || a.listScroll.Y <= hideScrollOffset || a.atTop {
		t.Fatalf("expected scrolled away with controls collapsed, collapsed=%v scrollY=%v atTop=%v", a.controlsCollapsed, a.listScroll.Y, a.atTop)
	}

	// Switch to text tab, then paste an entry from down the list and hide the panel.
	a.filterIdx = 1
	a.onTabChanged()
	tt.Frame()
	tt.Wheel(panelWidth/2, panelHeight/2, 0, 200)
	tt.Advance(250 * time.Millisecond)
	tt.Wheel(panelWidth/2, panelHeight/2, 0, 300)
	tt.Advance(250 * time.Millisecond)

	target := a.items[len(a.items)-2]
	time.Sleep(5 * time.Millisecond)
	tt.Move(1, 1)
	tt.Advance(300 * time.Millisecond)
	a.pasteItem(target, false)
	a.hidePanel()

	// Before any new UI frame renders, list order, active tab, and scroll state must already be settled.
	if len(a.items) == 0 || a.items[0].ID != target.ID {
		t.Fatalf("expected pasted item %d settled at top while hidden, got %+v", target.ID, a.items[0])
	}
	if a.filterIdx != 0 || a.listScroll.Y != 0 || !a.atTop || a.controlsCollapsed || a.selected != -1 {
		t.Fatalf("expected hidden state reset to top/all tab, filter=%d scrollY=%v atTop=%v collapsed=%v selected=%d",
			a.filterIdx, a.listScroll.Y, a.atTop, a.controlsCollapsed, a.selected)
	}

	// Simulate another clipboard item arriving while the panel remains hidden.
	time.Sleep(5 * time.Millisecond)
	bgItem, err := st.SaveText("background-copied-while-hidden", store.SaveOptions{Deduplicate: false})
	if err != nil {
		t.Fatal(err)
	}
	a.settleHiddenState()
	if len(a.items) == 0 || a.items[0].ID != bgItem.ID || a.items[1].ID != target.ID {
		t.Fatalf("expected background item %d above pasted item %d while hidden, got [%d, %d]",
			bgItem.ID, target.ID, a.items[0].ID, a.items[1].ID)
	}

	// First frame on reopen must already be in its final settled state with no pending animations or position jumps.
	tt.Frame()
	if a.listScroll.Y != 0 || !a.atTop || a.controlsCollapsed || a.snapNextFrame || a.listResetPending {
		t.Fatalf("expected first frame settled at top, scrollY=%v atTop=%v collapsed=%v snap=%v reset=%v",
			a.listScroll.Y, a.atTop, a.controlsCollapsed, a.snapNextFrame, a.listResetPending)
	}
	if tt.FramePending() {
		t.Fatalf("expected zero pending animations on first frame after reopen")
	}
	bgFirstRect, ok := tt.Find(bgItem.Text)
	if !ok {
		t.Fatalf("expected background item visible on first frame, texts=%q", tt.Texts())
	}
	targetFirstRect, ok := tt.Find(target.Text)
	if !ok {
		t.Fatalf("expected pasted item visible on first frame, texts=%q", tt.Texts())
	}

	tt.Advance(300 * time.Millisecond)
	bgSettledRect, ok := tt.Find(bgItem.Text)
	if !ok {
		t.Fatalf("expected background item still visible after advancing time")
	}
	targetSettledRect, ok := tt.Find(target.Text)
	if !ok {
		t.Fatalf("expected pasted item still visible after advancing time")
	}
	if bgFirstRect != bgSettledRect || targetFirstRect != targetSettledRect {
		t.Fatalf("expected zero visible jump after reopen: bg %+v -> %+v, target %+v -> %+v",
			bgFirstRect, bgSettledRect, targetFirstRect, targetSettledRect)
	}
}

func TestAccentColorSettings(t *testing.T) {
	if got := store.NormalizeHexColor(" #8B5CF6 "); got != "#8b5cf6" {
		t.Fatalf("expected normalized #8b5cf6, got %q", got)
	}
	if got := store.NormalizeHexColor("invalid"); got != "" {
		t.Fatalf("expected empty for invalid hex, got %q", got)
	}

	light := *myTiezLightTheme
	applyAccentColor(&light, ui.Hex("#fde047"), false)
	if light.AccentText != ui.Hex("#182235") {
		t.Fatalf("expected dark AccentText on bright yellow, got %+v", light.AccentText)
	}

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
	if err := tt.Click("界面设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)

	if err := tt.Click("选择强调色 #8b5cf6"); err != nil {
		t.Fatal(err)
	}
	if a.settings.AccentColor != "#8b5cf6" {
		t.Fatalf("expected AccentColor=#8b5cf6, got %q", a.settings.AccentColor)
	}
	if loaded, err := a.store.LoadSettings(); err != nil || loaded.AccentColor != "#8b5cf6" {
		t.Fatalf("expected persisted AccentColor=#8b5cf6, got %q err=%v", loaded.AccentColor, err)
	}

	if err := tt.Click("取色盘"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if !a.expanded.accentPicker {
		t.Fatalf("expected accentPicker expanded after clicking 取色盘")
	}

	if err := tt.Click("恢复默认"); err != nil {
		t.Fatal(err)
	}
	if a.settings.AccentColor != "" {
		t.Fatalf("expected AccentColor cleared after reset, got %q", a.settings.AccentColor)
	}
	if loaded, err := a.store.LoadSettings(); err != nil || loaded.AccentColor != "" {
		t.Fatalf("expected persisted AccentColor empty after reset, got %q err=%v", loaded.AccentColor, err)
	}
}

func TestOverrideWinVSetting(t *testing.T) {
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
	if err := tt.Click("剪贴板设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)

	if !tt.HasText("呼出快捷键") {
		t.Fatalf("expected custom hotkey row visible when OverrideWinV=false")
	}
	if err := tt.Click("切换使用 Win+V 呼出"); err != nil {
		t.Fatal(err)
	}
	if !a.settings.OverrideWinV {
		t.Fatalf("expected OverrideWinV=true after toggle")
	}
	if loaded, err := a.store.LoadSettings(); err != nil || !loaded.OverrideWinV {
		t.Fatalf("expected persisted OverrideWinV=true, got %+v err=%v", loaded, err)
	}
	if !tt.HasText("呼出快捷键") {
		t.Fatalf("expected custom hotkey row always visible")
	}

	if err := tt.Click("切换使用 Win+V 呼出"); err != nil {
		t.Fatal(err)
	}
	if a.settings.OverrideWinV {
		t.Fatalf("expected OverrideWinV=false after second toggle")
	}
	if !tt.HasText("呼出快捷键") {
		t.Fatalf("expected custom hotkey row visible when OverrideWinV=false")
	}
}

func TestFontPickerLazyAndPersistence(t *testing.T) {
	st, err := store.OpenStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	a := newClipApp(st, nil)
	a.reload()
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	if a.systemFontsLoaded {
		t.Fatalf("expected systemFontsLoaded=false on startup (lazy load)")
	}
	if err := tt.Click("设置"); err != nil {
		t.Fatal(err)
	}
	if err := tt.Click("界面设置"); err != nil {
		t.Fatal(err)
	}
	tt.Advance(250 * time.Millisecond)
	if a.systemFontsLoaded {
		t.Fatalf("expected systemFontsLoaded=false before opening font picker")
	}

	// 预先注入测试字体列表或直接触发点击展开验证懒加载触发
	if err := tt.Click("选择界面字体"); err != nil {
		t.Fatal(err)
	}
	if !a.systemFontsLoaded {
		t.Fatalf("expected systemFontsLoaded=true after opening font picker")
	}
	// 注入确定字体集合用于测试选择与搜索
	a.systemFonts = []ui.FontFamily{
		{Name: "HarmonyOS Sans SC"},
		{Name: "Consolas", Monospaced: true},
	}
	tt.Advance(20 * time.Millisecond)

	if err := tt.Click("HarmonyOS Sans SC"); err != nil {
		t.Fatal(err)
	}
	if a.settings.FontFamily != "HarmonyOS Sans SC" {
		t.Fatalf("expected FontFamily=HarmonyOS Sans SC, got %q", a.settings.FontFamily)
	}
	if loaded, err := a.store.LoadSettings(); err != nil || loaded.FontFamily != "HarmonyOS Sans SC" {
		t.Fatalf("expected persisted FontFamily=HarmonyOS Sans SC, got %q err=%v", loaded.FontFamily, err)
	}

	if err := tt.Click("恢复默认"); err != nil {
		t.Fatal(err)
	}
	if a.settings.FontFamily != "" {
		t.Fatalf("expected FontFamily cleared after reset, got %q", a.settings.FontFamily)
	}
}

