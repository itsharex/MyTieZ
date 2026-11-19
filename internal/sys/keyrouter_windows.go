//go:build windows

package sys

import (
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// RoutedKey represents a keyboard event intercepted while the panel is in no-activate mode.
type RoutedKey struct {
	VK    uint32
	Text  string
	Ctrl  bool
	Shift bool
	Alt   bool
}

var (
	procGetAsyncKeyState  = user32.NewProc("GetAsyncKeyState")
	procGetKeyState       = user32.NewProc("GetKeyState")
	procGetKeyboardLayout = user32.NewProc("GetKeyboardLayout")
	procToUnicodeEx       = user32.NewProc("ToUnicodeEx")

	keyHookOnce    sync.Once
	keyHookCB      uintptr
	activeKeyWatch atomic.Pointer[keyRouterState]
)

const (
	whKeyboardLL uintptr = 13

	wmKeyDown    uintptr = 0x0100
	wmKeyUp      uintptr = 0x0101
	wmSysKeyDown uintptr = 0x0104
	wmSysKeyUp   uintptr = 0x0105

	llkhfInjected uint32 = 0x10

	vkBack     uint32 = 0x08
	vkTab      uint32 = 0x09
	vkCapital  uint32 = 0x14
	vkEscape   uint32 = 0x1B
	vkSpace    uint32 = 0x20
	vkEnd      uint32 = 0x23
	vkHome     uint32 = 0x24
	vkUp       uint32 = 0x26
	vkDown     uint32 = 0x28
	vkDelete   uint32 = 0x2E
	vkLShift   uint32 = 0xA0
	vkRShift   uint32 = 0xA1
	vkLControl uint32 = 0xA2
	vkRControl uint32 = 0xA3
	vkLMenu    uint32 = 0xA4
	vkRMenu    uint32 = 0xA5
)

type kbdllHookStruct struct {
	vkCode      uint32
	scanCode    uint32
	flags       uint32
	time        uint32
	dwExtraInfo uintptr
}

type keyRouterState struct {
	events  chan RoutedKey
	stopCh  chan struct{}
	pressed [256]bool
}

// CanRouteForeground reports whether the current foreground window can have
// its keyboard input intercepted by a low-level hook from this process (UIPI check).
func CanRouteForeground() bool {
	if IsAdmin() {
		return true
	}
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return true
	}
	var pid uint32
	_, _, _ = procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 || pid == uint32(os.Getpid()) {
		return true
	}
	hProc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return false
	}
	defer windows.CloseHandle(hProc)

	var tok windows.Token
	if err := windows.OpenProcessToken(hProc, windows.TOKEN_QUERY, &tok); err != nil {
		return false
	}
	defer tok.Close()
	return !tok.IsElevated()
}

// StartKeyRouter installs a low-level keyboard hook while the panel is in no-activate mode
// and forwards panel-owned keys without stealing foreground focus.
func StartKeyRouter(onKey func(RoutedKey)) (func(), bool) {
	if onKey == nil {
		return func() {}, false
	}
	keyHookOnce.Do(func() {
		keyHookCB = syscall.NewCallback(lowLevelKeyboardProc)
	})

	st := &keyRouterState{
		events: make(chan RoutedKey, 64),
		stopCh: make(chan struct{}),
	}
	activeKeyWatch.Store(st)

	tidReady := make(chan uint32, 1)
	hookOK := make(chan bool, 1)
	pumpDone := make(chan struct{})

	go func() {
		for {
			select {
			case ev := <-st.events:
				onKey(ev)
			case <-st.stopCh:
				return
			}
		}
	}()

	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		defer close(pumpDone)

		var m winMsg
		procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0, pmNoRemove)
		tid := windows.GetCurrentThreadId()
		hhook, _, _ := procSetWindowsHookExW.Call(whKeyboardLL, keyHookCB, 0, 0)
		tidReady <- tid
		hookOK <- (hhook != 0)
		if hhook == 0 {
			return
		}
		defer procUnhookWindowsHookEx.Call(hhook)

		for {
			r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
			if int32(r) <= 0 {
				break
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
		}
	}()

	tid := <-tidReady
	ok := <-hookOK
	if !ok {
		activeKeyWatch.CompareAndSwap(st, nil)
		close(st.stopCh)
		<-pumpDone
		return func() {}, false
	}

	var once sync.Once
	return func() {
		once.Do(func() {
			activeKeyWatch.CompareAndSwap(st, nil)
			close(st.stopCh)
			if tid != 0 {
				procPostThreadMessageW.Call(uintptr(tid), uintptr(wmQuit), 0, 0)
			}
			<-pumpDone
		})
	}, true
}

func keyDownAsync(vk uint32) bool {
	r, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
	return (r & 0x8000) != 0
}

func isModifierVK(vk uint32) bool {
	switch vk {
	case vkShift, vkLShift, vkRShift,
		vkControl, vkLControl, vkRControl,
		vkMenu, vkLMenu, vkRMenu,
		vkLWin, vkRWin:
		return true
	}
	return false
}

func isPrintableVK(vk uint32) bool {
	if vk == vkSpace {
		return true
	}
	if vk >= '0' && vk <= '9' {
		return true
	}
	if vk >= 'A' && vk <= 'Z' {
		return true
	}
	if vk >= 0x60 && vk <= 0x6F { // Numpad
		return true
	}
	if (vk >= 0xBA && vk <= 0xC0) || (vk >= 0xDB && vk <= 0xDF) || vk == 0xE2 { // OEM punctuation
		return true
	}
	return false
}

func shouldRouteKey(vk uint32, ctrl, alt bool) bool {
	if alt {
		return false
	}
	if ctrl {
		switch vk {
		case 'A', 'V', vkBack:
			return true
		}
		return vk >= '1' && vk <= '9'
	}
	switch vk {
	case vkUp, vkDown, vkReturn, vkEscape, vkBack, vkDelete, vkTab, vkHome, vkEnd:
		return true
	}
	return isPrintableVK(vk)
}

func translateVKToText(vk, scanCode uint32, shift bool) string {
	var state [256]byte
	if shift {
		state[vkShift] = 0x80
	}
	capState, _, _ := procGetKeyState.Call(uintptr(vkCapital))
	if (capState & 0x01) != 0 {
		state[vkCapital] = 0x01
	}

	var hkl uintptr
	if fg, _, _ := procGetForegroundWindow.Call(); fg != 0 {
		tid, _, _ := procGetWindowThreadProcessId.Call(fg, 0)
		if tid != 0 {
			hkl, _, _ = procGetKeyboardLayout.Call(tid)
		}
	}
	var buf [8]uint16
	r, _, _ := procToUnicodeEx.Call(
		uintptr(vk),
		uintptr(scanCode),
		uintptr(unsafe.Pointer(&state[0])),
		uintptr(unsafe.Pointer(&buf[0])),
		uintptr(len(buf)),
		0,
		hkl,
	)
	n := int32(r)
	if n <= 0 {
		return ""
	}
	if int(n) > len(buf) {
		n = int32(len(buf))
	}
	s := windows.UTF16ToString(buf[:n])
	// Filter control characters (< 0x20)
	for _, ch := range s {
		if ch < 0x20 {
			return ""
		}
	}
	return s
}

func lowLevelKeyboardProc(nCode int32, wp, lp uintptr) uintptr {
	if nCode >= 0 && lp != 0 {
		if st := activeKeyWatch.Load(); st != nil {
			info := (*kbdllHookStruct)(nativePtr(lp))
			if (info.flags & llkhfInjected) != 0 {
				r, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wp, lp)
				return r
			}
			vk := info.vkCode
			if vk < 256 && !isModifierVK(vk) {
				switch wp {
				case wmKeyUp, wmSysKeyUp:
					if st.pressed[vk] {
						st.pressed[vk] = false
						return 1
					}
				case wmKeyDown, wmSysKeyDown:
					if !keyDownAsync(vkLWin) && !keyDownAsync(vkRWin) {
						ctrl := keyDownAsync(vkControl) || keyDownAsync(vkLControl) || keyDownAsync(vkRControl)
						shift := keyDownAsync(vkShift) || keyDownAsync(vkLShift) || keyDownAsync(vkRShift)
						alt := keyDownAsync(vkMenu) || keyDownAsync(vkLMenu) || keyDownAsync(vkRMenu)
						if shouldRouteKey(vk, ctrl, alt) {
							var text string
							if !ctrl && isPrintableVK(vk) {
								text = translateVKToText(vk, info.scanCode, shift)
							}
							st.pressed[vk] = true
							select {
							case st.events <- RoutedKey{
								VK:    vk,
								Text:  text,
								Ctrl:  ctrl,
								Shift: shift,
								Alt:   alt,
							}:
							default:
							}
							return 1
						}
					}
				}
			}
		}
	}
	r, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wp, lp)
	return r
}
