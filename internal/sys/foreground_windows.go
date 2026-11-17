//go:build windows

package sys

import (
	"os"
	"sync/atomic"
	"unsafe"
)

var (
	procIsWindow            = user32.NewProc("IsWindow")
	procIsWindowVisible     = user32.NewProc("IsWindowVisible")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")

	lastExternalHWND atomic.Uintptr
)

// TrackForegroundWindow records the current foreground window if it belongs to
// another process (ignoring system shell chrome like the taskbar).
func TrackForegroundWindow() {
	hwnd, _, _ := procGetForegroundWindow.Call()
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
