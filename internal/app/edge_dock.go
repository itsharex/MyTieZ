package app

import (
	"time"

	"mytiez/internal/sys"

	"github.com/egoist/mygo"
)

type dockPosition uint8

const (
	dockNone dockPosition = iota
	dockTop
	dockLeft
	dockRight
)

const (
	edgeDockThreshold    = 5
	edgeDockHideSize     = 4
	edgeDockHoverSliver  = 6
	edgeDockPollInterval = 150 * time.Millisecond
	edgeDockShowGrace    = 600 * time.Millisecond
)

func detectEdgeDock(win, workArea mygo.Rectangle, threshold int) dockPosition {
	switch {
	case win.Y <= workArea.Y+threshold:
		return dockTop
	case win.X <= workArea.X+threshold:
		return dockLeft
	case win.X+win.Width >= workArea.X+workArea.Width-threshold:
		return dockRight
	default:
		return dockNone
	}
}

func dockedWindowPosition(dock dockPosition, win, workArea mygo.Rectangle, hideSize int) (int, int) {
	switch dock {
	case dockTop:
		return win.X, workArea.Y - win.Height + hideSize
	case dockLeft:
		return workArea.X - win.Width + hideSize, win.Y
	case dockRight:
		return workArea.X + workArea.Width - hideSize, win.Y
	default:
		return win.X, win.Y
	}
}

func undockedWindowPosition(dock dockPosition, win, workArea mygo.Rectangle) (int, int) {
	switch dock {
	case dockTop:
		return win.X, workArea.Y
	case dockLeft:
		return workArea.X, win.Y
	case dockRight:
		return workArea.X + workArea.Width - win.Width, win.Y
	default:
		return win.X, win.Y
	}
}

func isCursorInRect(cursor mygo.Point, r mygo.Rectangle) bool {
	return cursor.X >= r.X && cursor.X <= r.X+r.Width &&
		cursor.Y >= r.Y && cursor.Y <= r.Y+r.Height
}

func isCursorInDockedSliver(dock dockPosition, cursor mygo.Point, win, workArea mygo.Rectangle, threshold int) bool {
	switch dock {
	case dockTop:
		return cursor.Y <= workArea.Y+threshold && cursor.X >= win.X && cursor.X <= win.X+win.Width
	case dockLeft:
		return cursor.X <= workArea.X+threshold && cursor.Y >= win.Y && cursor.Y <= win.Y+win.Height
	case dockRight:
		return cursor.X >= workArea.X+workArea.Width-threshold && cursor.Y >= win.Y && cursor.Y <= win.Y+win.Height
	default:
		return false
	}
}

func (a *clipApp) isNearDockEdge() bool {
	if a.edgeDock != dockNone {
		return true
	}
	if a.panel == nil {
		return false
	}
	win := a.panel.Bounds()
	center := mygo.Point{X: win.X + win.Width/2, Y: win.Y + win.Height/2}
	d := mygo.Screen.DisplayNearestPoint(center)
	return detectEdgeDock(win, d.WorkArea, edgeDockThreshold) != dockNone
}

func (a *clipApp) undockWindow(restorePin bool) {
	if a.edgeDock == dockNone {
		return
	}
	dock := a.edgeDock
	a.edgeDock = dockNone
	if a.panel == nil {
		return
	}
	win := a.panel.Bounds()
	center := mygo.Point{X: win.X + win.Width/2, Y: win.Y + win.Height/2}
	d := mygo.Screen.DisplayNearestPoint(center)
	ux, uy := undockedWindowPosition(dock, win, d.WorkArea)
	a.panel.SetPosition(ux, uy)
	if restorePin && !a.winPinned && !a.settings.NoFocusMode {
		a.panel.SetAlwaysOnTop(false)
	}
	a.lastShowAt = time.Now()
}

func (a *clipApp) syncEdgeDocking() {
	if !a.settings.EdgeDocking {
		a.undockWindow(true)
		a.stopEdgeDocking()
		return
	}
	if a.edgeDockStop != nil || a.panel == nil || !mygo.App.IsReady() {
		return
	}
	stop := make(chan struct{})
	a.edgeDockStop = stop
	go func() {
		ticker := time.NewTicker(edgeDockPollInterval)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				if a.panel == nil {
					return
				}
				a.panel.Update(a.checkEdgeDocking)
			}
		}
	}()
}

func (a *clipApp) stopEdgeDocking() {
	if a.edgeDockStop != nil {
		close(a.edgeDockStop)
		a.edgeDockStop = nil
	}
}

func (a *clipApp) checkEdgeDocking() {
	if !a.settings.EdgeDocking || a.panel == nil || !a.panel.IsVisible() || a.panel.IsMinimized() {
		return
	}
	win := a.panel.Bounds()
	if win.Width <= 0 || win.Height <= 0 {
		return
	}
	center := mygo.Point{X: win.X + win.Width/2, Y: win.Y + win.Height/2}
	d := mygo.Screen.DisplayNearestPoint(center)
	workArea := d.WorkArea
	cursor := mygo.Screen.CursorScreenPoint()

	if a.edgeDock != dockNone {
		if isCursorInDockedSliver(a.edgeDock, cursor, win, workArea, edgeDockHoverSliver) {
			a.undockWindow(true)
		}
		return
	}

	if !a.lastShowAt.IsZero() && time.Since(a.lastShowAt) < edgeDockShowGrace {
		return
	}
	if isCursorInRect(cursor, win) || sys.IsLeftMouseButtonDown() {
		return
	}

	dock := detectEdgeDock(win, workArea, edgeDockThreshold)
	if dock == dockNone {
		return
	}
	if !a.winPinned {
		a.panel.SetAlwaysOnTop(true)
	}
	dx, dy := dockedWindowPosition(dock, win, workArea, edgeDockHideSize)
	a.panel.SetPosition(dx, dy)
	a.edgeDock = dock
}
