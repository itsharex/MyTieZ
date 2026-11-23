//go:build windows

package sys

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	dwmapi                    = windows.NewLazySystemDLL("dwmapi.dll")
	procDwmSetWindowAttribute = dwmapi.NewProc("DwmSetWindowAttribute")
	procRedrawWindow          = user32.NewProc("RedrawWindow")
)

const (
	dwmwaCloak     uintptr = 13
	rdwInvalidate  uintptr = 0x0001
	rdwAllChildren uintptr = 0x0080
	rdwUpdateNow   uintptr = 0x0100
)

// SetWindowCloaked toggles DWMWA_CLOAK so a shown window remains invisible until
// its settled frame has been painted into the swap chain.
func SetWindowCloaked(hwnd uintptr, cloaked bool) bool {
	if hwnd == 0 {
		return false
	}
	var val int32
	if cloaked {
		val = 1
	}
	hr, _, _ := procDwmSetWindowAttribute.Call(hwnd, dwmwaCloak, uintptr(unsafe.Pointer(&val)), unsafe.Sizeof(val))
	return hr == 0
}

// SyncWindowPaint forces immediate synchronous WM_PAINT delivery for hwnd and
// its surface child window so the first revealed frame reflects current state.
func SyncWindowPaint(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	procRedrawWindow.Call(hwnd, 0, 0, rdwInvalidate|rdwUpdateNow|rdwAllChildren)
}
