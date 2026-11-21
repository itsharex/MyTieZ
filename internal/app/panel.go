package app

import (
	"time"

	"mytiez/internal/sys"

	"github.com/egoist/mygo"
)

func (a *clipApp) showPanel() {
	a.showPanelWithFocus(!a.settings.NoFocusMode)
}

func (a *clipApp) showPanelWithFocus(focus bool) {
	if a.panel == nil {
		return
	}
	sys.TrackForegroundWindow()
	wasDocked := a.edgeDock != dockNone
	if wasDocked {
		a.undockWindow(false)
	}
	if !a.panel.IsVisible() {
		// 非固定窗口每次重新呼出从「全部」Tab 开始，避免遗忘停留在旧子分类中
		a.filterIdx = 0
		cursor := mygo.Screen.CursorScreenPoint()
		d := mygo.Screen.DisplayNearestPoint(cursor)
		w, h := a.panel.Size()
		if w <= 0 {
			w = max(minPanelWidth, a.settings.WindowWidth)
		}
		if h <= 0 {
			h = max(minPanelHeight, a.settings.WindowHeight)
		}
		var x, y int
		if a.settings.FollowMouse {
			x, y = followMouseTarget(cursor, w, h, d.WorkArea)
		} else {
			x, y = centeredPanelTarget(w, h, d.WorkArea)
		}
		a.panel.SetPosition(x, y)
	}
	a.stopSearchTimer()
	a.closeFileMenus()
	a.selected = -1
	a.confirmClearOpen = false
	a.resetScrollToTop()
	a.reloadIfStale()
	a.scrollListToTop()
	a.setPanelNoActivate(!focus)
	// 不抢焦点呼出时必须置顶，否则非置顶窗口执行 ShowInactive 会被压在当前前台窗口后面
	a.panel.SetAlwaysOnTop(a.winPinned || !focus)
	a.lastShowAt = time.Now()
	if focus {
		a.panel.Show()
		a.panel.Focus()
	} else {
		a.panel.ShowInactive()
		a.syncOutsideClickWatch()
		a.syncKeyRouter()
	}
}

func (a *clipApp) hidePanel() {
	a.flushPendingSettingsPersist()
	a.closeFileMenus()
	a.stopOutsideClickWatch()
	a.stopKeyRouting()
	a.edgeDock = dockNone
	a.routedSearchActive = false
	a.revealedSensitive = nil
	a.confirmClearOpen = false
	if a.panel != nil {
		a.panel.Hide()
	}
	if a.settings.NoFocusMode {
		a.setPanelNoActivate(true)
	}
	a.releaseHiddenMemory()
}

func (a *clipApp) togglePanel() {
	a.togglePanelWithFocus(!a.settings.NoFocusMode)
}

func (a *clipApp) togglePanelWithFocus(focus bool) {
	if a.panel == nil {
		return
	}
	if a.panel.IsVisible() && a.edgeDock == dockNone {
		a.hidePanel()
	} else {
		a.showPanelWithFocus(focus)
	}
}

func followMouseTarget(cursor mygo.Point, w, h int, workArea mygo.Rectangle) (int, int) {
	const edgePad = 8
	x := cursor.X
	y := cursor.Y

	minX := workArea.X + edgePad
	maxX := workArea.X + workArea.Width - w - edgePad
	if maxX < minX {
		x = workArea.X
	} else {
		x = max(minX, min(x, maxX))
	}

	minY := workArea.Y + edgePad
	maxY := workArea.Y + workArea.Height - h - edgePad
	if maxY < minY {
		return x, workArea.Y
	}
	if y > maxY {
		aboveY := cursor.Y - h
		if aboveY >= minY {
			y = aboveY
		} else {
			y = maxY
		}
	}
	if y < minY {
		y = minY
	}
	return x, y
}

func centeredPanelTarget(w, h int, workArea mygo.Rectangle) (int, int) {
	x := workArea.X + (workArea.Width-w)/2
	y := workArea.Y + (workArea.Height-h)/3
	if workArea.Width > w {
		x = max(workArea.X, min(x, workArea.X+workArea.Width-w))
	} else {
		x = workArea.X
	}
	if workArea.Height > h {
		y = max(workArea.Y, min(y, workArea.Y+workArea.Height-h))
	} else {
		y = workArea.Y
	}
	return x, y
}
