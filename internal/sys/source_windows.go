//go:build windows

package sys

import (
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	procGetClipboardOwner        = user32.NewProc("GetClipboardOwner")
	procGetForegroundWindow      = user32.NewProc("GetForegroundWindow")
	procGetWindowThreadProcessId = user32.NewProc("GetWindowThreadProcessId")
	procGetClassNameW            = user32.NewProc("GetClassNameW")
)

// CaptureClipboardSource returns the executable name and full path of the
// process that most recently wrote to or owns the clipboard.
func CaptureClipboardSource() (string, string) {
	ownPID := uint32(os.Getpid())

	if hwnd, _, _ := procGetClipboardOwner.Call(); hwnd != 0 {
		if name, path := resolveWindowProcess(hwnd, ownPID, false); path != "" {
			return name, path
		}
	}
	if hwnd, _, _ := procGetForegroundWindow.Call(); hwnd != 0 {
		if name, path := resolveWindowProcess(hwnd, ownPID, true); path != "" {
			return name, path
		}
	}
	return "Unknown", ""
}

func resolveWindowProcess(hwnd uintptr, ownPID uint32, skipSystemShell bool) (string, string) {
	var pid uint32
	_, _, _ = procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 || pid == ownPID {
		return "", ""
	}
	if skipSystemShell && isSystemShellClass(hwnd) {
		return "", ""
	}

	hProc, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pid)
	if err != nil {
		return "", ""
	}
	defer windows.CloseHandle(hProc)

	buf := make([]uint16, 1024)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(hProc, 0, &buf[0], &size); err != nil || size == 0 {
		return "", ""
	}
	fullPath := windows.UTF16ToString(buf[:size])
	if fullPath == "" {
		return "", ""
	}
	return filepath.Base(fullPath), fullPath
}

func isSystemShellClass(hwnd uintptr) bool {
	buf := make([]uint16, 128)
	n, _, _ := procGetClassNameW.Call(hwnd, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n == 0 {
		return false
	}
	cls := windows.UTF16ToString(buf[:n])
	switch cls {
	case "Shell_TrayWnd", "NotifyIconOverflowWindow", "TopLevelWindowForOverflowXamlIsland", "Progman", "WorkerW":
		return true
	default:
		return false
	}
}
