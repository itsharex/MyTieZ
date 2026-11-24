package app

import (
	"fmt"
	"strings"

	"mytiez/internal/sys"
	"mytiez/locales"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	recordTargetGlobal     = "global"
	recordTargetPlainPaste = "plain_paste"
)

func (a *clipApp) startHotkeyRecording() {
	a.activatePanelForInput()
	if a.recordingHotkey && a.recordingTarget == recordTargetGlobal {
		a.hotkeyFocusPending = true
		return
	}
	a.cancelHotkeyRecording()
	if mygo.App.IsReady() && a.shortcut != "" {
		mygo.GlobalShortcut.Unregister(a.shortcut)
	}
	a.recordingHotkey = true
	a.recordingTarget = recordTargetGlobal
	a.hotkeyFocusPending = true
	a.hotkeyMods = 0
	a.confirmAction = ""
}

func (a *clipApp) startPlainShortcutRecording() {
	a.activatePanelForInput()
	if a.recordingHotkey && a.recordingTarget == recordTargetPlainPaste {
		a.hotkeyFocusPending = true
		return
	}
	a.cancelHotkeyRecording()
	a.recordingHotkey = true
	a.recordingTarget = recordTargetPlainPaste
	a.hotkeyFocusPending = true
	a.hotkeyMods = 0
	a.confirmAction = ""
}

func (a *clipApp) cancelHotkeyRecording() {
	if !a.recordingHotkey {
		return
	}
	wasGlobal := a.recordingTarget == recordTargetGlobal
	a.recordingHotkey = false
	a.recordingTarget = ""
	a.hotkeyFocusPending = false
	a.hotkeyMods = 0
	if wasGlobal && !a.settings.OverrideWinV && mygo.App.IsReady() && a.shortcut != "" && !mygo.GlobalShortcut.IsRegistered(a.shortcut) {
		_ = mygo.GlobalShortcut.Register(a.shortcut, a.togglePanel)
	}
}

func (a *clipApp) finishHotkeyRecording(combo string) {
	target := a.recordingTarget
	a.recordingHotkey = false
	a.recordingTarget = ""
	a.hotkeyFocusPending = false
	a.hotkeyMods = 0
	if target == recordTargetPlainPaste {
		a.applyPlainPasteShortcut(combo)
		return
	}
	a.applyHotkey(combo)
}

func (a *clipApp) applyPlainPasteShortcut(combo string) {
	a.settings.PlainPasteShortcut = combo
	a.syncPlainPasteShortcut()
	a.persistSettings()
}

func (a *clipApp) syncPlainPasteShortcut() {
	mods, _, vk, ok := parsePanelShortcut(a.settings.PlainPasteShortcut)
	if !ok {
		sys.SetExtraRoutedShortcut(0, false, false, false)
		return
	}
	sys.SetExtraRoutedShortcut(vk, mods&ui.Ctrl != 0, mods&ui.Shift != 0, mods&ui.Alt != 0)
}

func (a *clipApp) matchesPlainPasteRoutedKey(ev sys.RoutedKey) bool {
	mods, _, vk, ok := parsePanelShortcut(a.settings.PlainPasteShortcut)
	if !ok {
		return false
	}
	return ev.VK == vk &&
		ev.Ctrl == (mods&ui.Ctrl != 0) &&
		ev.Shift == (mods&ui.Shift != 0) &&
		ev.Alt == (mods&ui.Alt != 0)
}

func (a *clipApp) applyHotkey(combo string) {
	if err := a.switchHotkey(combo); err != nil {
		if a.shortcut != "" {
			a.settings.Hotkey = a.shortcut
			if mygo.App.IsReady() && !mygo.GlobalShortcut.IsRegistered(a.shortcut) {
				_ = mygo.GlobalShortcut.Register(a.shortcut, a.togglePanel)
			}
		}
		a.setStatus(locales.SettingsHotkeyOccupied(combo))
		return
	}
	a.settings.Hotkey = combo
	a.persistSettings()
}

func (a *clipApp) handleHotkeyInput(ev ui.InputEvent) bool {
	if !a.recordingHotkey {
		return false
	}
	switch ev.Kind {
	case ui.InputKeyUp:
		a.hotkeyMods = ev.Mods
		return true
	case ui.InputText:
		return true
	case ui.InputKeyDown:
		if ev.Repeat {
			return true
		}
		a.hotkeyMods = ev.Mods
		if ev.Mods == 0 && ev.Key == ui.KeyEscape {
			a.cancelHotkeyRecording()
			return true
		}
		if a.recordingTarget == recordTargetPlainPaste && ev.Mods == 0 && (ev.Key == ui.KeyBackspace || ev.Key == ui.KeyDelete) {
			a.finishHotkeyRecording("")
			return true
		}
		if ev.Key == ui.KeyUnknown {
			return true
		}
		keyName, ok := keyToAcceleratorName(ev.Key)
		if !ok {
			a.setStatus(locales.SettingsHotkeyUnsupported())
			return true
		}
		if a.recordingTarget == recordTargetPlainPaste {
			if ev.Mods&ui.Super != 0 {
				a.setStatus(locales.SettingsHotkeyUnsupported())
				return true
			}
			hasPrimaryMod := ev.Mods&(ui.Ctrl|ui.Alt) != 0
			shiftNonPrintable := ev.Mods&ui.Shift != 0 && isNonPrintableShortcutKey(ev.Key)
			if !hasPrimaryMod && !shiftNonPrintable && !isFunctionKey(ev.Key) {
				a.setStatus(locales.SettingsPlainPasteNeedMod())
				return true
			}
			combo := formatHotkey(ev.Mods, keyName)
			a.finishHotkeyRecording(combo)
			return true
		}
		hasPrimaryMod := ev.Mods&(ui.Ctrl|ui.Alt|ui.Super) != 0
		if !hasPrimaryMod && !isFunctionKey(ev.Key) {
			a.setStatus(locales.SettingsHotkeyNeedMod())
			return true
		}
		combo := formatHotkey(ev.Mods, keyName)
		a.finishHotkeyRecording(combo)
		return true
	default:
		return false
	}
}

func recordingHotkeyLabel(msgs locales.Messages, mods ui.Modifiers) string {
	parts := modifierParts(mods)
	if len(parts) == 0 {
		return msgs.SettingsHotkeyPrompt()
	}
	return strings.Join(parts, "+") + "+..."
}

func formatHotkey(mods ui.Modifiers, keyName string) string {
	parts := append(modifierParts(mods), keyName)
	return strings.Join(parts, "+")
}

func modifierParts(mods ui.Modifiers) []string {
	var parts []string
	if mods&ui.Ctrl != 0 {
		parts = append(parts, "Ctrl")
	}
	if mods&ui.Alt != 0 {
		parts = append(parts, "Alt")
	}
	if mods&ui.Shift != 0 {
		parts = append(parts, "Shift")
	}
	if mods&ui.Super != 0 {
		parts = append(parts, "Win")
	}
	return parts
}

func isFunctionKey(k ui.Key) bool {
	return k >= ui.KeyF1 && k <= ui.KeyF12
}

func isNonPrintableShortcutKey(k ui.Key) bool {
	switch k {
	case ui.KeyEnter, ui.KeyTab, ui.KeyBackspace, ui.KeyDelete, ui.KeyInsert,
		ui.KeyHome, ui.KeyEnd, ui.KeyPageUp, ui.KeyPageDown,
		ui.KeyUp, ui.KeyDown, ui.KeyLeft, ui.KeyRight:
		return true
	}
	return false
}

func parsePanelShortcut(combo string) (mods ui.Modifiers, key ui.Key, vk uint32, ok bool) {
	combo = strings.TrimSpace(combo)
	if combo == "" {
		return 0, ui.KeyUnknown, 0, false
	}
	parts := strings.Split(combo, "+")
	if len(parts) == 0 {
		return 0, ui.KeyUnknown, 0, false
	}
	for _, p := range parts[:len(parts)-1] {
		switch strings.ToLower(strings.TrimSpace(p)) {
		case "ctrl", "control":
			mods |= ui.Ctrl
		case "alt":
			mods |= ui.Alt
		case "shift":
			mods |= ui.Shift
		default:
			return 0, ui.KeyUnknown, 0, false
		}
	}
	key, vk, ok = acceleratorNameToKey(strings.TrimSpace(parts[len(parts)-1]))
	if !ok {
		return 0, ui.KeyUnknown, 0, false
	}
	hasPrimaryMod := mods&(ui.Ctrl|ui.Alt) != 0
	shiftNonPrintable := mods&ui.Shift != 0 && isNonPrintableShortcutKey(key)
	if !hasPrimaryMod && !shiftNonPrintable && !isFunctionKey(key) {
		return 0, ui.KeyUnknown, 0, false
	}
	return mods, key, vk, true
}

func acceleratorNameToKey(name string) (ui.Key, uint32, bool) {
	if len(name) == 1 {
		ch := name[0]
		if ch >= 'a' && ch <= 'z' {
			ch -= 'a' - 'A'
		}
		if ch >= 'A' && ch <= 'Z' {
			return ui.KeyA + ui.Key(ch-'A'), uint32(ch), true
		}
		if ch >= '0' && ch <= '9' {
			return ui.Key0 + ui.Key(ch-'0'), uint32(ch), true
		}
	}
	upper := strings.ToUpper(name)
	if strings.HasPrefix(upper, "F") && len(upper) >= 2 && len(upper) <= 3 {
		var n int
		if _, err := fmt.Sscanf(upper[1:], "%d", &n); err == nil && n >= 1 && n <= 12 {
			return ui.KeyF1 + ui.Key(n-1), uint32(0x70 + n - 1), true
		}
	}
	switch upper {
	case "SPACE":
		return ui.KeySpace, 0x20, true
	case "ENTER", "RETURN":
		return ui.KeyEnter, 0x0D, true
	case "TAB":
		return ui.KeyTab, 0x09, true
	case "BACKSPACE":
		return ui.KeyBackspace, 0x08, true
	case "DELETE":
		return ui.KeyDelete, 0x2E, true
	case "INSERT":
		return ui.KeyInsert, 0x2D, true
	case "HOME":
		return ui.KeyHome, 0x24, true
	case "END":
		return ui.KeyEnd, 0x23, true
	case "PAGEUP":
		return ui.KeyPageUp, 0x21, true
	case "PAGEDOWN":
		return ui.KeyPageDown, 0x22, true
	case "UP":
		return ui.KeyUp, 0x26, true
	case "DOWN":
		return ui.KeyDown, 0x28, true
	case "LEFT":
		return ui.KeyLeft, 0x25, true
	case "RIGHT":
		return ui.KeyRight, 0x27, true
	case "-":
		return ui.KeyMinus, 0xBD, true
	case "=":
		return ui.KeyEqual, 0xBB, true
	case ",":
		return ui.KeyComma, 0xBC, true
	case ".":
		return ui.KeyPeriod, 0xBE, true
	case "/":
		return ui.KeySlash, 0xBF, true
	case ";":
		return ui.KeySemicolon, 0xBA, true
	case "'":
		return ui.KeyQuote, 0xDE, true
	case "[":
		return ui.KeyBracketLeft, 0xDB, true
	case "]":
		return ui.KeyBracketRight, 0xDD, true
	case `\`:
		return ui.KeyBackslash, 0xDC, true
	case "`":
		return ui.KeyBackquote, 0xC0, true
	default:
		return ui.KeyUnknown, 0, false
	}
}

func keyToAcceleratorName(k ui.Key) (string, bool) {
	if k >= ui.KeyA && k <= ui.KeyZ {
		return string(rune('A' + (k - ui.KeyA))), true
	}
	if k >= ui.Key0 && k <= ui.Key9 {
		return string(rune('0' + (k - ui.Key0))), true
	}
	if k >= ui.KeyF1 && k <= ui.KeyF12 {
		return fmt.Sprintf("F%d", int(k-ui.KeyF1)+1), true
	}
	switch k {
	case ui.KeySpace:
		return "Space", true
	case ui.KeyEnter:
		return "Enter", true
	case ui.KeyTab:
		return "Tab", true
	case ui.KeyBackspace:
		return "Backspace", true
	case ui.KeyDelete:
		return "Delete", true
	case ui.KeyInsert:
		return "Insert", true
	case ui.KeyHome:
		return "Home", true
	case ui.KeyEnd:
		return "End", true
	case ui.KeyPageUp:
		return "PageUp", true
	case ui.KeyPageDown:
		return "PageDown", true
	case ui.KeyUp:
		return "Up", true
	case ui.KeyDown:
		return "Down", true
	case ui.KeyLeft:
		return "Left", true
	case ui.KeyRight:
		return "Right", true
	case ui.KeyMinus:
		return "-", true
	case ui.KeyEqual:
		return "=", true
	case ui.KeyComma:
		return ",", true
	case ui.KeyPeriod:
		return ".", true
	case ui.KeySlash:
		return "/", true
	case ui.KeySemicolon:
		return ";", true
	case ui.KeyQuote:
		return "'", true
	case ui.KeyBracketLeft:
		return "[", true
	case ui.KeyBracketRight:
		return "]", true
	case ui.KeyBackslash:
		return `\`, true
	case ui.KeyBackquote:
		return "`", true
	default:
		return "", false
	}
}
