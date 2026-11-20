package app

import (
	"time"

	"mytiez/internal/sys"

	"github.com/egoist/mygo"
)

func (a *clipApp) applyNoFocusMode(on bool) {
	a.settings.NoFocusMode = on
	a.setPanelNoActivate(on)
	if a.panel != nil && mygo.App.IsReady() {
		a.panel.SetAlwaysOnTop(a.winPinned || a.panelNoActivate)
	}
	a.syncOutsideClickWatch()
}

func (a *clipApp) setPanelNoActivate(on bool) {
	a.panelNoActivate = on
	if !on {
		a.routedSearchActive = false
	}
	if a.panel != nil && mygo.App.IsReady() {
		sys.SetWindowNoActivate(a.panel.NativeHandle(), on)
	}
	a.syncOutsideClickWatch()
	a.syncKeyRouter()
}

func (a *clipApp) syncOutsideClickWatch() {
	shouldWatch := a.panel != nil && mygo.App.IsReady() && a.panel.IsVisible() &&
		a.settings.NoFocusMode && a.panelNoActivate && !a.winPinned && !a.pickingApp
	if !shouldWatch {
		a.stopOutsideClickWatch()
		return
	}
	if a.stopOutsideWatch != nil {
		return
	}
	hwnd := a.panel.NativeHandle()
	if hwnd == 0 {
		return
	}
	stop, ok := sys.StartOutsideClickWatch(hwnd, func() {
		if a.panel == nil {
			return
		}
		a.panel.Update(func() {
			if a.panel == nil || !a.panel.IsVisible() {
				return
			}
			if !a.settings.NoFocusMode || !a.panelNoActivate || a.winPinned || a.pickingApp {
				return
			}
			a.lastOutsideHide = time.Now()
			a.hidePanel()
		})
	})
	if ok {
		a.stopOutsideWatch = stop
	}
}

func (a *clipApp) stopOutsideClickWatch() {
	if a.stopOutsideWatch != nil {
		stop := a.stopOutsideWatch
		a.stopOutsideWatch = nil
		stop()
	}
}

func (a *clipApp) activatePanelForInput() {
	if !a.panelNoActivate {
		return
	}
	a.setPanelNoActivate(false)
	if a.panel != nil && mygo.App.IsReady() {
		a.panel.SetAlwaysOnTop(a.winPinned)
		a.panel.Show()
		a.panel.Focus()
	}
}
