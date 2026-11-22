package app

import (
	"log"
	"time"

	"mytiez/internal/sys"
	"mytiez/locales"

	"github.com/egoist/mygo"
)

func (a *clipApp) switchHotkey(newKey string) error {
	if newKey == "" {
		return nil
	}
	if !mygo.App.IsReady() {
		a.shortcut = newKey
		return nil
	}
	if newKey == a.shortcut && mygo.GlobalShortcut.IsRegistered(newKey) {
		return nil
	}
	oldKey := a.shortcut
	if oldKey != "" {
		mygo.GlobalShortcut.Unregister(oldKey)
	}
	if err := mygo.GlobalShortcut.Register(newKey, a.togglePanel); err != nil {
		if oldKey != "" && !mygo.GlobalShortcut.IsRegistered(oldKey) {
			_ = mygo.GlobalShortcut.Register(oldKey, a.togglePanel)
		}
		return err
	}
	a.shortcut = newKey
	if a.trayOpenItem != nil {
		a.trayOpenItem.SetAccelerator(newKey)
	}
	return nil
}

func (a *clipApp) setupTray() {
	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon:    a.iconPNG,
		ToolTip: locales.TrayTooltip(),
	})
	if err != nil {
		log.Printf("create tray: %v", err)
		return
	}
	a.tray = tray
	a.updateTrayMenu()
	a.tray.OnClick(func() {
		if time.Since(a.lastOutsideHide) < 300*time.Millisecond {
			return
		}
		a.togglePanel()
	})
	mygo.App.OnLocaleChanged(func(string) {
		a.updateTrayMenu()
	})
	mygo.App.OnBeforeQuit(func(*mygo.QuitEvent) {
		a.quitting = true
	})
	mygo.App.OnQuitCanceled(func() {
		a.quitting = false
	})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		sys.CancelGameModePaste()
		a.stopOutsideClickWatch()
		a.stopKeyRouting()
		a.stopEdgeDocking()
		if a.resizeTimer != nil {
			a.resizeTimer.Stop()
		}
		a.stopSearchTimer()
		a.stopHeapReleaseTimer()
		a.stopIdleUnloadTimer()
		a.flushPendingSettingsPersist()
		if a.updateTimer != nil {
			a.updateTimer.Stop()
		}
		a.saveWindowSize()
		if a.stopWatch != nil {
			a.stopWatch()
		}
		if a.stopTrack != nil {
			close(a.stopTrack)
			a.stopTrack = nil
		}
		a.cleanupTempFiles()
		if a.tray != nil {
			a.tray.Destroy()
		}
	})
}

func (a *clipApp) updateTrayMenu() {
	if a.tray == nil {
		return
	}
	a.trayOpenItem = &mygo.MenuItem{
		Label:       locales.TrayOpen(),
		Accelerator: a.shortcut,
		Click:       func(*mygo.MenuItem, *mygo.Window) { a.showPanel() },
	}
	menu := mygo.NewMenu([]*mygo.MenuItem{
		a.trayOpenItem,
		mygo.Separator(),
		{Role: mygo.RoleQuit, Label: locales.TrayQuit()},
	})
	a.tray.SetMenu(menu)
}
