package app

import (
	"log"
	"os"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"mytiez/internal/store"
	"mytiez/internal/sys"

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
	tempFiles         map[string]struct{}
	fileMenuOpen      map[int64]*bool
	fileMenuStatID    int64
	fileMenuStatAt    time.Time
	fileMenuStatPaths []string
	fileMenuExists    map[string]bool

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
	persistTimer     *time.Timer
	persistPending   bool
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
	recordingHotkey        bool
	recordingTarget        string // "" | "global" | "plain_paste"
	hotkeyFocusPending     bool
	hotkeyMods             ui.Modifiers
	hotkeyHandle           ui.Handle
	plainShortcutHandle    ui.Handle
	pendingToast       string
	memTrimming        atomic.Bool
	saveMu             sync.Mutex
	saveQueue          []func() (string, error)
	saveRunning        bool
}

func newClipApp(st *store.Store, iconPNG []byte) *clipApp {
	cfg := store.DefaultSettings()
	if st != nil {
		if loaded, err := st.LoadSettings(); err == nil {
			cfg = loaded
		}
	}
	a := &clipApp{
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
	a.syncPlainPasteShortcut()
	return a
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
		log.Printf("run app: %v", err)
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
	if mygo.App.IsReady() && a.memTrimming.CompareAndSwap(false, true) {
		go func() {
			defer a.memTrimming.Store(false)
			if sys.WorkingSetBytes() > sys.MemoryTrimThresholdBytes {
				debug.FreeOSMemory()
				sys.TrimMemory()
			}
		}()
	}
}

func (a *clipApp) panelWidth() float32 {
	winW := float32(a.settings.WindowWidth)
	if a.panel != nil {
		if w, _ := a.panel.Size(); w > 0 {
			winW = float32(w)
		}
	}
	if winW <= 0 {
		winW = panelWidth
	}
	return winW
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
		if err := mygo.GlobalShortcut.Register(s, a.togglePanel); err == nil {
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

	if !mygo.App.WasOpenedAtLogin() {
		a.showPanel()
	}
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
