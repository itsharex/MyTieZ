package app

import (
	"log"
	"os"
	"runtime/debug"
	"time"

	"mytiez/internal/store"
	"mytiez/internal/sys"
	"mytiez/locales"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var globalShortcuts = []string{"Alt+V", "Ctrl+Shift+V", "Alt+Shift+V"}

const (
	// warmThumbCacheLimit keeps up to 8 recently viewed thumbnails (~1 MB) intact
	// on panel hide when process memory is healthy.
	warmThumbCacheLimit = 8
)

type clipApp struct {
	iconPNG         []byte
	store           *store.Store
	settings        store.Settings
	panel           *mygo.Window
	tray            *mygo.Tray
	trayOpenItem    *mygo.MenuItem
	shortcut        string
	stopWatch       func()
	stopTrack       chan struct{}
	items           []store.ClipItem
	searchHits      []store.ClipItem
	query           string
	filterIdx       int // 0: 全部, 1: 文本, 2: 图片, 3: 文件, 4: 链接, 5: 代码, 6: 富文本
	selected        int
	list            ui.ListState
	listHandle      ui.Handle
	listScroll      ui.ScrollState
	searchHandle    ui.Handle
	editingTagsID   int64
	tagInput        string
	tagInputHandle  ui.Handle
	tagFocusPending bool
	allTags           []string
	editingTagOrder   []string
	loadedRevision    uint64
	loadedTagRevision uint64
	loadedQuery       string
	loadedKind        store.ClipKind
	loadedLimit       int

	thumbs            *bitmapCache
	appIcons          *bitmapCache
	richSpans         map[int64][]ui.Span
	richSpansDark     bool
	revealedSensitive map[int64]bool

	ignoreHash       string
	ignoreUntil      time.Time
	winPinned          bool
	panelNoActivate    bool
	routedSearchActive bool
	routedCaretPending bool
	stopOutsideWatch   func()
	stopKeyRouter      func()
	edgeDock           dockPosition
	edgeDockStop       chan struct{}
	lastShowAt         time.Time
	lastOutsideHide    time.Time
	pickingApp       bool
	quitting         bool
	resizeTimer      *time.Timer
	searchTimer      *time.Timer
	updateTimer      *time.Timer
	updateChecking   bool
	updateInstalling bool
	updateProgress   int
	updateReady      bool
	availableUpdate  *mygo.Update
	updateStatus     string

	showSettings       bool
	confirmClearOpen   bool
	controlsCollapsed  bool
	atTop              bool
	topReachedAt       time.Time
	upScrollAccum      float32
	expanded           settingsGroupExpanded
	confirmAction      string
	recordingHotkey    bool
	hotkeyFocusPending bool
	hotkeyMods         ui.Modifiers
	hotkeyHandle       ui.Handle
	pendingToast       string
}

func newClipApp(st *store.Store, iconPNG []byte) *clipApp {
	cfg := store.DefaultSettings()
	if st != nil {
		if loaded, err := st.LoadSettings(); err == nil {
			cfg = loaded
		}
	}
	return &clipApp{
		iconPNG:         iconPNG,
		store:           st,
		settings:        cfg,
		selected:        -1,
		atTop:           true,
		winPinned:       !cfg.AutoHideOnBlur,
		panelNoActivate: cfg.NoFocusMode,
		thumbs:          newBitmapCache(),
		appIcons:        newBitmapCache(),
	}
}

// Run starts the clipboard manager application.
func Run(iconPNG []byte) {
	if os.Getenv("GOGC") == "" {
		debug.SetGCPercent(50)
	}
	if !mygo.App.RequestSingleInstanceLock() {
		return
	}

	st, err := store.OpenStore("")
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	app := newClipApp(st, iconPNG)
	mygo.App.OnSecondInstance(func([]string, string) {
		app.showPanel()
	})
	mygo.App.WhenReady(app.ready)

	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}

func (a *clipApp) toggleWindowPin() {
	a.applyAutoHide(a.winPinned)
	a.persistSettings()
}

func (a *clipApp) applyAutoHide(autoHide bool) {
	a.settings.AutoHideOnBlur = autoHide
	a.winPinned = !autoHide
	if a.panel != nil && mygo.App.IsReady() {
		a.panel.SetAlwaysOnTop(a.winPinned || a.panelNoActivate)
	}
	a.syncOutsideClickWatch()
}

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
	a.stopOutsideWatch = sys.StartOutsideClickWatch(hwnd, func() {
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
	if err := mygo.GlobalShortcut.Register(newKey, a.togglePanelFromHotkey); err != nil {
		if oldKey != "" && !mygo.GlobalShortcut.IsRegistered(oldKey) {
			_ = mygo.GlobalShortcut.Register(oldKey, a.togglePanelFromHotkey)
		}
		return err
	}
	a.shortcut = newKey
	if a.trayOpenItem != nil {
		a.trayOpenItem.SetAccelerator(newKey)
	}
	return nil
}

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

func followMouseTarget(cursor mygo.Point, w, h int, workArea mygo.Rectangle) (int, int) {
	const (
		cursorGap = 12
		edgePad   = 8
	)
	x := cursor.X - w/2
	y := cursor.Y + cursorGap

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
		aboveY := cursor.Y - h - cursorGap
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

// releaseHiddenMemory prunes caches and trims OS working set pages as needed.
// Under the 75 MB working set threshold, up to warmThumbCacheLimit (8) recently
// viewed thumbnails and richSpans are preserved on hide to avoid wake-up CPU spikes.
func (a *clipApp) releaseHiddenMemory() {
	overMemory := sys.WorkingSetBytes() > sys.MemoryTrimThresholdBytes

	if a.thumbs != nil {
		if overMemory {
			a.thumbs.clear()
		} else if a.thumbs.len() > warmThumbCacheLimit {
			a.thumbs.pruneTo(warmThumbCacheLimit)
		}
	}
	if overMemory && len(a.richSpans) > 0 {
		a.richSpans = nil
	}

	if !overMemory {
		return
	}
	if mygo.App.IsReady() {
		go func() {
			if sys.WorkingSetBytes() > sys.MemoryTrimThresholdBytes {
				debug.FreeOSMemory()
				sys.TrimMemory()
			}
		}()
	}
}

func (a *clipApp) hidePanel() {
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

func (a *clipApp) togglePanelFromHotkey() {
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

func (a *clipApp) saveWindowSize() {
	if a.panel == nil {
		return
	}
	w, h := a.panel.Size()
	if w < minPanelWidth || h < minPanelHeight {
		return
	}
	if a.settings.WindowWidth == w && a.settings.WindowHeight == h {
		return
	}
	a.settings.WindowWidth = w
	a.settings.WindowHeight = h
	a.persistSettings()
}

func (a *clipApp) ready() {
	a.settings.AutoStart = mygo.App.OpenAtLogin()
	winW := max(minPanelWidth, a.settings.WindowWidth)
	winH := max(minPanelHeight, a.settings.WindowHeight)

	a.panel = mygo.NewWindow(mygo.WindowOptions{
		Title:           "MyTieZ",
		Panel:           true,
		AlwaysOnTop:     a.winPinned,
		Hidden:          true,
		Frameless:       true,
		Transparent:     true,
		BackgroundColor: "transparent",
		Vibrancy:        backdropVibrancy(a.settings.BackdropMode),
		CornerRadius:    22,
		Width:           winW,
		Height:          winH,
		MinWidth:        minPanelWidth,
		MinHeight:       minPanelHeight,
		StateKey:        "main-panel",
		Content:         ui.View(a.view),
	})

	a.panel.OnResize(func() {
		if a.resizeTimer != nil {
			a.resizeTimer.Reset(300 * time.Millisecond)
			return
		}
		a.resizeTimer = time.AfterFunc(300*time.Millisecond, func() {
			if a.panel != nil {
				a.panel.Update(a.saveWindowSize)
			}
		})
	})
	a.panel.OnClose(func(e *mygo.CloseEvent) {
		a.cancelHotkeyRecording()
		a.saveWindowSize()
		if a.quitting || !a.panel.IsVisible() {
			return
		}
		e.PreventDefault()
		a.hidePanel()
	})
	a.panel.OnBlur(func() {
		a.cancelHotkeyRecording()
		if !a.winPinned && !a.pickingApp && !(a.settings.EdgeDocking && a.isNearDockEdge()) {
			a.saveWindowSize()
			a.hidePanel()
		}
	})

	candidates := append([]string{a.settings.Hotkey}, globalShortcuts...)
	for _, s := range candidates {
		if s == "" {
			continue
		}
		if err := mygo.GlobalShortcut.Register(s, a.togglePanelFromHotkey); err == nil {
			a.shortcut = s
			a.settings.Hotkey = s
			break
		} else {
			log.Printf("register shortcut %s: %v", s, err)
		}
	}

	a.setupTray()
	a.stopWatch = mygo.Clipboard.OnChanged(a.onClipboardChanged)
	a.startForegroundTracker()
	a.syncEdgeDocking()
	a.reload()
	a.updateTimer = time.AfterFunc(5*time.Second, func() {
		if a.panel != nil {
			a.panel.Update(func() { a.checkForUpdates(false) })
		}
	})

	if !mygo.App.WasOpenedAtLogin() && !a.settings.SilentStart {
		a.showPanel()
	}
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
		a.stopOutsideClickWatch()
		a.stopKeyRouting()
		a.stopEdgeDocking()
		if a.resizeTimer != nil {
			a.resizeTimer.Stop()
		}
		a.stopSearchTimer()
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

func (a *clipApp) startForegroundTracker() {
	sys.TrackForegroundWindow()
	stop := make(chan struct{})
	a.stopTrack = stop
	go func() {
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				sys.TrackForegroundWindow()
			case <-stop:
				return
			}
		}
	}()
}
