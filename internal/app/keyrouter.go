package app

import (
	"strings"
	"unicode/utf8"

	"mytiez/internal/sys"

	"github.com/egoist/mygo"
)

const (
	vkBack   uint32 = 0x08
	vkTab    uint32 = 0x09
	vkReturn uint32 = 0x0D
	vkEscape uint32 = 0x1B
	vkUp     uint32 = 0x26
	vkDown   uint32 = 0x28
	vkDelete uint32 = 0x2E
)

func (a *clipApp) shouldRouteKeys() bool {
	return a.panel != nil && mygo.App.IsReady() && a.panel.IsVisible() &&
		a.settings.NoFocusMode && a.panelNoActivate && !a.showSettings &&
		a.editingTagsID == 0 && !a.recordingHotkey && !a.confirmClearOpen && !a.pickingApp
}

func (a *clipApp) syncKeyRouter() {
	if !a.shouldRouteKeys() {
		a.stopKeyRouting()
		return
	}
	if a.stopKeyRouter != nil {
		return
	}
	if !sys.CanRouteForeground() {
		return
	}
	stop, ok := sys.StartKeyRouter(func(ev sys.RoutedKey) {
		if a.panel == nil {
			return
		}
		a.panel.Update(func() {
			a.handleRoutedKey(ev)
		})
	})
	if ok {
		a.stopKeyRouter = stop
	}
}

func (a *clipApp) stopKeyRouting() {
	a.routedSearchActive = false
	a.routedCaretPending = false
	if a.stopKeyRouter != nil {
		stop := a.stopKeyRouter
		a.stopKeyRouter = nil
		stop()
	}
}

func (a *clipApp) onSearchBoxPointerDown() {
	if !a.panelNoActivate {
		return
	}
	if a.stopKeyRouter == nil {
		a.syncKeyRouter()
	}
	if a.stopKeyRouter != nil {
		a.routedSearchActive = true
		a.routedCaretPending = true
		return
	}
	// 前景窗口为更高权限进程（UIPI）或钩子不可用时，回退为激活窗口以接收输入
	a.activatePanelForInput()
}

func (a *clipApp) handleRoutedKey(ev sys.RoutedKey) {
	if !a.shouldRouteKeys() {
		return
	}
	a.applyRoutedKey(ev)
}

func (a *clipApp) applyRoutedKey(ev sys.RoutedKey) {
	if a.hasOpenFileMenu() {
		if ev.VK == vkEscape {
			a.closeFileMenus()
		}
		return
	}
	if a.matchesPlainPasteRoutedKey(ev) {
		if len(a.items) > 0 {
			a.pasteSelectedPlainText()
		}
		return
	}
	if ev.Alt {
		return
	}

	if ev.Ctrl {
		if ev.VK >= '1' && ev.VK <= '9' && a.settings.QuickPasteModifier == "ctrl" {
			idx := int(ev.VK - '1')
			if idx < len(a.items) {
				a.selected = idx
				a.pasteSelected()
				a.selected = -1
			}
			return
		}
		switch ev.VK {
		case 'V':
			if mygo.App.IsReady() {
				if text := mygo.Clipboard.ReadText(); text != "" {
					clean := strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ").Replace(text)
					a.appendRoutedSearchText(clean)
				}
			}
		case vkBack:
			if a.query != "" {
				a.query = ""
				a.routedSearchActive = true
				a.routedCaretPending = true
				a.scheduleSearchReload()
			}
		}
		return
	}

	switch ev.VK {
	case vkEscape:
		if a.query != "" {
			a.clearSearch(nil)
			a.routedSearchActive = false
		} else if a.selected >= 0 {
			a.selected = -1
		} else {
			a.hidePanel()
		}
	case vkUp:
		a.moveSelection(-1)
	case vkDown:
		a.moveSelection(1)
	case vkReturn:
		if a.selected >= 0 && a.selected < len(a.items) {
			a.pasteSelected()
		} else if len(a.items) > 0 {
			a.selected = 0
			a.pasteSelected()
		}
	case vkBack, vkDelete:
		if a.query != "" {
			_, size := utf8.DecodeLastRuneInString(a.query)
			if size > 0 {
				a.query = a.query[:len(a.query)-size]
				a.routedSearchActive = true
				a.routedCaretPending = true
				a.scheduleSearchReload()
			}
		}
	case vkTab:
		a.routedSearchActive = !a.routedSearchActive
	default:
		if ev.Text != "" {
			a.appendRoutedSearchText(ev.Text)
		}
	}
}

func (a *clipApp) appendRoutedSearchText(text string) {
	if text == "" {
		return
	}
	a.routedSearchActive = true
	a.routedCaretPending = true
	a.controlsCollapsed = false
	a.query += text
	a.scheduleSearchReload()
}
