//go:build windows

package sys

import (
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

var (
	procGetWindowLongPtrW     = user32.NewProc("GetWindowLongPtrW")
	procSetWindowLongPtrW     = user32.NewProc("SetWindowLongPtrW")
	procCallWindowProcW       = user32.NewProc("CallWindowProcW")
	procDefWindowProcW        = user32.NewProc("DefWindowProcW")
	procGetWindowRect         = user32.NewProc("GetWindowRect")
	procGetCursorPos          = user32.NewProc("GetCursorPos")
	procSetWindowsHookExW     = user32.NewProc("SetWindowsHookExW")
	procUnhookWindowsHookEx   = user32.NewProc("UnhookWindowsHookEx")
	procCallNextHookEx        = user32.NewProc("CallNextHookEx")
	procGetMessageW           = user32.NewProc("GetMessageW")
	procPeekMessageW          = user32.NewProc("PeekMessageW")
	procTranslateMessage      = user32.NewProc("TranslateMessage")
	procDispatchMessageW      = user32.NewProc("DispatchMessageW")
	procPostThreadMessageW    = user32.NewProc("PostThreadMessageW")
)

const (
	gwlExStyle  int32 = -20
	gwlpWndProc int32 = -4

	wsExNoActivate uintptr = 0x08000000

	wmNcDestroy       uint32 = 0x0082
	wmNcActivate      uint32 = 0x0086
	wmMouseActivate   uint32 = 0x0021
	wmPointerActivate uint32 = 0x024B
	wmNcLButtonDown   uint32 = 0x00A1
	wmSysCommand      uint32 = 0x0112
	wmQuit            uint32 = 0x0012
	wmLButtonDown     uintptr = 0x0201
	wmRButtonDown     uintptr = 0x0204
	wmMButtonDown     uintptr = 0x0207

	maNoActivate uintptr = 3
	htCaption    uintptr = 2
	scMove       uintptr = 0xF010
	pmNoRemove   uintptr = 0
	whMouseLL    uintptr = 14
)

type winPoint struct {
	X int32
	Y int32
}

type winRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

type winMsg struct {
	hwnd     uintptr
	message  uint32
	wParam   uintptr
	lParam   uintptr
	time     uint32
	pt       winPoint
	lPrivate uint32
}

type msllHookStruct struct {
	pt          winPoint
	mouseData   uint32
	flags       uint32
	time        uint32
	dwExtraInfo uintptr
}

type noActivateEntry struct {
	prevProc   uintptr
	noActivate bool
}

var (
	noActivateMu   sync.RWMutex
	noActivateMap  = map[uintptr]*noActivateEntry{}
	noActivateOnce sync.Once
	noActivateCB   uintptr

	mouseHookOnce    sync.Once
	mouseHookCB      uintptr
	activeMouseWatch atomic.Pointer[mouseWatchState]
)

type mouseWatchState struct {
	hwnd   uintptr
	notify chan struct{}
	stopCh chan struct{}
}

func getWindowLong(hwnd uintptr, idx int32) uintptr {
	r, _, _ := procGetWindowLongPtrW.Call(hwnd, uintptr(idx))
	return r
}

func setWindowLong(hwnd uintptr, idx int32, val uintptr) uintptr {
	r, _, _ := procSetWindowLongPtrW.Call(hwnd, uintptr(idx), val)
	return r
}

// SetWindowNoActivate toggles WS_EX_NOACTIVATE and subclasses the window so
// mouse clicks, pointer presses, and caption drags do not steal foreground focus.
func SetWindowNoActivate(hwnd uintptr, noActivate bool) {
	if hwnd == 0 {
		return
	}
	noActivateOnce.Do(func() {
		noActivateCB = syscall.NewCallback(noActivateWndProc)
	})

	noActivateMu.Lock()
	entry := noActivateMap[hwnd]
	if entry == nil {
		prev := setWindowLong(hwnd, gwlpWndProc, noActivateCB)
		if prev != 0 {
			entry = &noActivateEntry{prevProc: prev, noActivate: noActivate}
			noActivateMap[hwnd] = entry
		}
	} else {
		entry.noActivate = noActivate
	}
	noActivateMu.Unlock()

	ex := getWindowLong(hwnd, gwlExStyle)
	var next uintptr
	if noActivate {
		next = ex | wsExNoActivate
	} else {
		next = ex &^ wsExNoActivate
	}
	if next != ex {
		setWindowLong(hwnd, gwlExStyle, next)
	}
}

func noActivateWndProc(hwnd uintptr, msg uint32, wp, lp uintptr) uintptr {
	noActivateMu.RLock()
	entry := noActivateMap[hwnd]
	var prevProc uintptr
	var noAct bool
	if entry != nil {
		prevProc = entry.prevProc
		noAct = entry.noActivate
	}
	noActivateMu.RUnlock()

	if prevProc == 0 {
		r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wp, lp)
		return r
	}

	if msg == wmNcDestroy {
		noActivateMu.Lock()
		delete(noActivateMap, hwnd)
		noActivateMu.Unlock()
		setWindowLong(hwnd, gwlpWndProc, prevProc)
		r, _, _ := procCallWindowProcW.Call(prevProc, hwnd, uintptr(msg), wp, lp)
		return r
	}

	if noAct {
		switch msg {
		case wmMouseActivate, wmPointerActivate:
			return maNoActivate
		case wmNcLButtonDown:
			if wp == htCaption {
				dragLP := lp
				if dragLP == 0 {
					var pt winPoint
					if ok, _, _ := procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt))); ok != 0 {
						dragLP = uintptr(uint16(pt.X)) | (uintptr(uint16(pt.Y)) << 16)
					}
				}
				r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(wmSysCommand), scMove|htCaption, dragLP)
				return r
			}
		case wmNcActivate:
			r, _, _ := procCallWindowProcW.Call(prevProc, hwnd, uintptr(msg), 1, lp)
			if wp == 0 {
				return 1
			}
			return r
		}
	}

	r, _, _ := procCallWindowProcW.Call(prevProc, hwnd, uintptr(msg), wp, lp)
	return r
}

// StartOutsideClickWatch installs a low-level mouse hook while a non-activating
// window is open and invokes onOutside when a mouse click occurs outside hwnd.
func StartOutsideClickWatch(hwnd uintptr, onOutside func()) (func(), bool) {
	if hwnd == 0 || onOutside == nil {
		return func() {}, false
	}
	mouseHookOnce.Do(func() {
		mouseHookCB = syscall.NewCallback(lowLevelMouseProc)
	})

	st := &mouseWatchState{
		hwnd:   hwnd,
		notify: make(chan struct{}, 1),
		stopCh: make(chan struct{}),
	}
	activeMouseWatch.Store(st)

	go func() {
		for {
			select {
			case <-st.notify:
				onOutside()
			case <-st.stopCh:
				return
			}
		}
	}()

	return startLLHook(whMouseLL, mouseHookCB, func() {
		activeMouseWatch.CompareAndSwap(st, nil)
		close(st.stopCh)
	})
}

// nativePtr converts a Windows hook lParam (pointing to C-allocated hook struct
// valid for the duration of the callback) into an unsafe.Pointer.
func nativePtr(addr uintptr) unsafe.Pointer {
	return *(*unsafe.Pointer)(unsafe.Pointer(&addr))
}

func lowLevelMouseProc(nCode int32, wp, lp uintptr) uintptr {
	if nCode >= 0 && (wp == wmLButtonDown || wp == wmRButtonDown || wp == wmMButtonDown) {
		if st := activeMouseWatch.Load(); st != nil && lp != 0 {
			if vis, _, _ := procIsWindowVisible.Call(st.hwnd); vis != 0 {
				info := (*msllHookStruct)(nativePtr(lp))
				var rc winRect
				if ok, _, _ := procGetWindowRect.Call(st.hwnd, uintptr(unsafe.Pointer(&rc))); ok != 0 {
					pt := info.pt
					if pt.X < rc.Left || pt.X >= rc.Right || pt.Y < rc.Top || pt.Y >= rc.Bottom {
						select {
						case st.notify <- struct{}{}:
						default:
						}
					}
				}
			}
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wp, lp)
	return r
}
