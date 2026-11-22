package app

import (
	"strings"
	"testing"

	"mytiez/internal/store"
	"mytiez/internal/sys"
	"mytiez/locales"

	"github.com/egoist/mygo/ui"
)

func TestRichTextContextMenuPlainPasteAndCopyOnly(t *testing.T) {
	writes := stubWriteClipItem(t)
	msgs := locales.In("zh-Hans")

	const (
		richText = "Hello Rich World"
		richHTML = "<p><strong>Hello</strong> Rich World</p>"
	)
	a := newTestAppWithRichText(t, richText, richHTML)
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	rect, ok := tt.Find(richText)
	if !ok {
		t.Fatalf("expected rich text preview in view, texts=%q", tt.Texts())
	}
	tt.RightClickAt(rect.X+12, rect.Y+rect.H/2)

	plainInfo, ok := tt.AccessByRole(ui.RoleMenuItem, msgs.RichMenuPastePlain())
	if !ok {
		t.Fatalf("expected %q in rich text context menu, menu=%q", msgs.RichMenuPastePlain(), tt.Menu())
	}
	if strings.Contains(plainInfo.Description, "Shift+Enter") {
		t.Fatalf("did not expect plain paste item description to include shortcut hint, got %+v", plainInfo)
	}
	clickCenter(tt, plainInfo.Bounds)

	if len(*writes) != 1 || !(*writes)[0].plainText || (*writes)[0].item.Text != richText {
		t.Fatalf("expected single plain-text write of %q, got %+v", richText, *writes)
	}
	wantPlainHash := store.TextContentHash(richText, "")
	if a.ignoreHash != wantPlainHash {
		t.Fatalf("expected plain-text ignoreHash %q to prevent loop, got %q", wantPlainHash, a.ignoreHash)
	}

	// ClickCopyOnly switches menu label to "Copy as Plain Text".
	a.settings.ClickCopyOnly = true
	tt.RightClickAt(rect.X+12, rect.Y+rect.H/2)
	copyPlainInfo, ok := tt.AccessByRole(ui.RoleMenuItem, msgs.RichMenuCopyPlain())
	if !ok {
		t.Fatalf("expected %q under ClickCopyOnly, menu=%q", msgs.RichMenuCopyPlain(), tt.Menu())
	}
	clickCenter(tt, copyPlainInfo.Bounds)
	if len(*writes) != 2 || !(*writes)[1].plainText {
		t.Fatalf("expected second plain-text copy write, got %+v", *writes)
	}
}

func TestPlainPasteShortcutFocusedAndNoFocus(t *testing.T) {
	writes := stubWriteClipItem(t)
	a := newTestAppWithRichText(t, "Shortcut Rich", "<em>Shortcut Rich</em>")
	tt := ui.NewTester(a.view, panelWidth, panelHeight)

	// 1. Default Shift+Enter in focused mode.
	tt.Key(ui.Shift, ui.KeyEnter)
	if len(*writes) != 1 || !(*writes)[0].plainText {
		t.Fatalf("expected Shift+Enter to trigger plain paste in focused mode, got %+v", *writes)
	}

	// 2. Customize shortcut to Alt+P and test both focused mode and no-focus routed key.
	a.applyPlainPasteShortcut("Alt+P")
	tt.Frame()
	tt.Key(ui.Alt, ui.KeyP)
	if len(*writes) != 2 || !(*writes)[1].plainText {
		t.Fatalf("expected Alt+P to trigger plain paste in focused mode, got %+v", *writes)
	}

	a.applyRoutedKey(sys.RoutedKey{VK: 'P', Alt: true})
	if len(*writes) != 3 || !(*writes)[2].plainText {
		t.Fatalf("expected Alt+P to trigger plain paste in no-focus key router, got %+v", *writes)
	}

	// Disabled shortcut must not trigger plain paste.
	a.applyPlainPasteShortcut("")
	a.applyRoutedKey(sys.RoutedKey{VK: 'P', Alt: true})
	if len(*writes) != 3 {
		t.Fatalf("expected disabled shortcut not to paste, got %+v", *writes)
	}
}

func TestPlainPasteShortcutRecordingAndValidation(t *testing.T) {
	a := newTestAppWithRichText(t, "Record Test", "<i>Record Test</i>")
	a.showSettings = true
	a.expanded.clipboard = true
	tt := ui.NewTester(a.view, panelWidth, 1200)

	msgs := locales.In("zh-Hans")
	if err := tt.Click(msgs.SettingsPlainPasteRecA11y()); err != nil {
		t.Fatalf("click plain paste recorder: %v", err)
	}
	if !a.recordingHotkey || a.recordingTarget != recordTargetPlainPaste {
		t.Fatalf("expected plain paste recording active, got recording=%v target=%q", a.recordingHotkey, a.recordingTarget)
	}

	// Shift+A (printable) must be rejected.
	a.handleHotkeyInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyA, Mods: ui.Shift})
	if !a.recordingHotkey || a.settings.PlainPasteShortcut != "Shift+Enter" {
		t.Fatalf("expected Shift+A rejected, recording=%v shortcut=%q", a.recordingHotkey, a.settings.PlainPasteShortcut)
	}

	// Ctrl+Enter must be accepted and persisted.
	a.handleHotkeyInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyEnter, Mods: ui.Ctrl})
	if a.recordingHotkey || a.settings.PlainPasteShortcut != "Ctrl+Enter" {
		t.Fatalf("expected Ctrl+Enter saved, recording=%v shortcut=%q", a.recordingHotkey, a.settings.PlainPasteShortcut)
	}

	// Re-enter recording and press Backspace to disable.
	a.startPlainShortcutRecording()
	a.handleHotkeyInput(ui.InputEvent{Kind: ui.InputKeyDown, Key: ui.KeyBackspace})
	if a.recordingHotkey || a.settings.PlainPasteShortcut != "" {
		t.Fatalf("expected Backspace to disable shortcut, recording=%v shortcut=%q", a.recordingHotkey, a.settings.PlainPasteShortcut)
	}

	// Reset button should restore default Shift+Enter.
	tt.Frame()
	if err := tt.Click(msgs.SettingsPlainPasteResetA11y()); err != nil {
		t.Fatalf("click reset plain paste shortcut: %v", err)
	}
	if a.settings.PlainPasteShortcut != "Shift+Enter" {
		t.Fatalf("expected reset to restore Shift+Enter, got %q", a.settings.PlainPasteShortcut)
	}
}
