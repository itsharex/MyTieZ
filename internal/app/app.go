package app

import (
	"log"
	"time"

	"mytiez/internal/store"
	"mytiez/internal/sys"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

var globalShortcuts = []string{"Alt+V", "Ctrl+Shift+V", "Alt+Shift+V"}

type clipApp struct {
	iconPNG      []byte
	store        *store.Store
	settings     store.Settings
	panel        *mygo.Window
	tray         *mygo.Tray
	shortcut     string
	stopWatch    func()
	stopTrack    chan struct{}
	items        []store.ClipItem
	query        string
	filterIdx    int // 0: 全部, 1: 文本, 2: 图片, 3: 文件
	selected     int
	list         ui.ListState
	listScroll   ui.ScrollState
	searchHandle ui.Handle

	thumbs   *bitmapCache
	appIcons *bitmapCache

	ignoreHash  string
	ignoreUntil time.Time
	winPinned   bool
	resizeTimer *time.Timer

	showSettings      bool
	controlsCollapsed bool
	atTop             bool
	topReachedAt      time.Time
	upScrollAccum     float32
	expanded          settingsGroupExpanded
	confirmAction     string
	settingsNotice    string
	statusMsg         string
	statusUntil       time.Time
}

func newClipApp(st *store.Store, iconPNG []byte) *clipApp {
	cfg := store.DefaultSettings()
	if st != nil {
		if loaded, err := st.LoadSettings(); err == nil {
			cfg = loaded
		}
	}
	return &clipApp{
		iconPNG:   iconPNG,
		store:     st,
		settings:  cfg,
		selected:  -1,
		atTop:     true,
		winPinned: !cfg.AutoHideOnBlur,
		thumbs:    newBitmapCache(),
		appIcons:  newBitmapCache(),
	}
}

// Run starts the clipboard manager application.
func Run(iconPNG []byte) {
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
		a.panel.SetAlwaysOnTop(a.winPinned)
	}
}

func (a *clipApp) switchHotkey(newKey string) error {
	if newKey == "" || newKey == a.shortcut {
		return nil
	}
	if !mygo.App.IsReady() {
		a.shortcut = newKey
		return nil
	}
	oldKey := a.shortcut
	if oldKey != "" {
		mygo.GlobalShortcut.Unregister(oldKey)
	}
	if err := mygo.GlobalShortcut.Register(newKey, a.togglePanel); err != nil {
		if oldKey != "" {
			_ = mygo.GlobalShortcut.Register(oldKey, a.togglePanel)
		}
		return err
	}
	a.shortcut = newKey
	return nil
}

func (a *clipApp) showPanel() {
	if a.panel == nil {
		return
	}
	sys.TrackForegroundWindow()
	if !a.panel.IsVisible() {
		cursor := mygo.Screen.CursorScreenPoint()
		d := mygo.Screen.DisplayNearestPoint(cursor)
		w, h := a.panel.Size()
		x := d.WorkArea.X + (d.WorkArea.Width-w)/2
		y := d.WorkArea.Y + (d.WorkArea.Height-h)/3
		if a.settings.FollowMouse {
			const margin = 12
			x = cursor.X - w/2
			y = cursor.Y + 16
			maxX := d.WorkArea.X + d.WorkArea.Width - w - margin
			maxY := d.WorkArea.Y + d.WorkArea.Height - h - margin
			x = max(d.WorkArea.X+margin, min(x, maxX))
			y = max(d.WorkArea.Y+margin, min(y, maxY))
		}
		a.panel.SetPosition(x, y)
	}
	a.selected = -1
	a.controlsCollapsed = false
	a.atTop = true
	a.upScrollAccum = 0
	a.listScroll.Y = 0
	a.reload()
	if len(a.items) > 0 {
		a.list.ScrollTo(0, ui.Start)
	}
	a.panel.SetAlwaysOnTop(a.winPinned)
	a.panel.Show()
	a.panel.Focus()
	if !a.showSettings {
		a.panel.Update(func() {
			a.searchHandle.Focus()
		})
	}
}

func (a *clipApp) togglePanel() {
	if a.panel == nil {
		return
	}
	if a.panel.IsVisible() {
		a.panel.Hide()
	} else {
		a.showPanel()
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
		e.PreventDefault()
		a.saveWindowSize()
		a.panel.Hide()
	})
	a.panel.OnBlur(func() {
		if !a.winPinned {
			a.saveWindowSize()
			a.panel.Hide()
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
	a.reload()

	if !mygo.App.WasOpenedAtLogin() && !a.settings.SilentStart {
		a.showPanel()
	}
}

func (a *clipApp) setupTray() {
	tray, err := mygo.NewTray(mygo.TrayOptions{
		Icon:    a.iconPNG,
		ToolTip: "MyTieZ 剪贴板",
	})
	if err != nil {
		log.Printf("create tray: %v", err)
		return
	}
	a.tray = tray
	menu := mygo.NewMenu([]*mygo.MenuItem{
		{
			Label:       "打开剪贴板",
			Accelerator: a.shortcut,
			Click:       func(*mygo.MenuItem, *mygo.Window) { a.showPanel() },
		},
		{
			Label: "清空未置顶记录",
			Click: func(*mygo.MenuItem, *mygo.Window) {
				if a.panel != nil {
					a.panel.Update(a.clearUnpinned)
				}
			},
		},
		mygo.Separator(),
		{
			Label:   "开机自启动",
			Type:    mygo.MenuItemCheckbox,
			Checked: mygo.App.OpenAtLogin(),
			Click: func(it *mygo.MenuItem, _ *mygo.Window) {
				if err := mygo.App.SetOpenAtLogin(it.Checked); err != nil {
					log.Printf("set open at login: %v", err)
				}
			},
		},
		mygo.Separator(),
		{Role: mygo.RoleQuit, Label: "退出"},
	})
	a.tray.SetMenu(menu)
	a.tray.OnClick(func() { a.togglePanel() })
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) {
		if a.resizeTimer != nil {
			a.resizeTimer.Stop()
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
