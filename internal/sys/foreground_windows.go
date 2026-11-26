//go:build windows

package sys

import (
	"os"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

const (
	eventSystemForeground  uintptr = 0x0003
	wineventOutOfContext   uintptr = 0x0000
	wineventSkipOwnProcess uintptr = 0x0002
	objidWindow            int32   = 0
	childidSelf            int32   = 0
)

var (
	procIsWindow            = user32.NewProc("IsWindow")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procSetWinEventHook     = user32.NewProc("SetWinEventHook")
	procUnhookWinEvent      = user32.NewProc("UnhookWinEvent")

	lastExternalHWND   atomic.Uintptr
	foregroundHookOnce sync.Once
	foregroundHookCB   uintptr
)

// TrackForegroundWindow records the current foreground window if it belongs to
// another process (ignoring system shell chrome like the taskbar).
func TrackForegroundWindow() {
	hwnd, _, _ := procGetForegroundWindow.Call()
	recordExternalForeground(hwnd)
}

func recordExternalForeground(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	var pid uint32
	_, _, _ = procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 || pid == uint32(os.Getpid()) {
		return
	}
	if isSystemShellClass(hwnd) {
		return
	}
	lastExternalHWND.Store(hwnd)
}

// StartForegroundWatcher installs an out-of-context EVENT_SYSTEM_FOREGROUND hook
// so external foreground window changes are tracked without periodic polling.
func StartForegroundWatcher() (func(), bool) {
	TrackForegroundWindow()
	foregroundHookOnce.Do(func() {
		foregroundHookCB = syscall.NewCallback(foregroundWinEventProc)
	})
	return startMessagePump(
		func() uintptr {
			h, _, _ := procSetWinEventHook.Call(
				eventSystemForeground,
				eventSystemForeground,
				0,
				foregroundHookCB,
				0,
				0,
				wineventOutOfContext|wineventSkipOwnProcess,
			)
			return h
		},
		func(h uintptr) {
			procUnhookWinEvent.Call(h)
		},
		nil,
	)
}

func foregroundWinEventProc(hWinEventHook, event, hwnd, idObject, idChild, idEventThread, dwmsEventTime uintptr) uintptr {
	if event == eventSystemForeground && int32(idObject) == objidWindow && int32(idChild) == childidSelf {
		recordExternalForeground(hwnd)
	}
	return 0
}

// RestoreForegroundWindow activates the most recently tracked external window
// so that a subsequent SendPaste targets it even when our own panel stays open.
func RestoreForegroundWindow() bool {
	hwnd := lastExternalHWND.Load()
	if hwnd == 0 {
		return false
	}
	if valid, _, _ := procIsWindow.Call(hwnd); valid == 0 {
		lastExternalHWND.CompareAndSwap(hwnd, 0)
		return false
	}
	if vis, _, _ := procIsWindowVisible.Call(hwnd); vis == 0 {
		return false
	}
	ok, _, _ := procSetForegroundWindow.Call(hwnd)
	return ok != 0
}

// IsLeftMouseButtonDown reports whether the primary mouse button is currently pressed.
func IsLeftMouseButtonDown() bool {
	r, _, _ := procGetAsyncKeyState.Call(0x01) // VK_LBUTTON
	return (uint16(r) & 0x8000) != 0
}
