package app

import (
	"fmt"
	"strings"

	"mytiez/locales"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *clipApp) startHotkeyRecording() {
	if a.recordingHotkey {
		a.hotkeyFocusPending = true
		return
	}
	if mygo.App.IsReady() && a.shortcut != "" {
		mygo.GlobalShortcut.Unregister(a.shortcut)
	}
	a.recordingHotkey = true
	a.hotkeyFocusPending = true
	a.hotkeyMods = 0
	a.confirmAction = ""
}

func (a *clipApp) cancelHotkeyRecording() {
	if !a.recordingHotkey {
		return
	}
	a.recordingHotkey = false
	a.hotkeyFocusPending = false
	a.hotkeyMods = 0
	if mygo.App.IsReady() && a.shortcut != "" && !mygo.GlobalShortcut.IsRegistered(a.shortcut) {
		_ = mygo.GlobalShortcut.Register(a.shortcut, a.togglePanel)
	}
}

func (a *clipApp) finishHotkeyRecording(combo string) {
	a.recordingHotkey = false
	a.hotkeyFocusPending = false
	a.hotkeyMods = 0
	a.applyHotkey(combo)
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
		if ev.Key == ui.KeyUnknown {
			return true
		}
		keyName, ok := keyToAcceleratorName(ev.Key)
		if !ok {
			a.setStatus(locales.SettingsHotkeyUnsupported())
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
