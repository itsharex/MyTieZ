//go:build windows

package sys

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	shell32           = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteW = shell32.NewProc("ShellExecuteW")
)

// IsAdmin reports whether the current process is running with elevated privileges.
func IsAdmin() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// RestartAsAdmin launches a new elevated instance of the current executable via UAC.
func RestartAsAdmin() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	verbPtr, err := syscall.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	filePtr, err := syscall.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	dirPtr, err := syscall.UTF16PtrFromString(filepath.Dir(exe))
	if err != nil {
		return err
	}
	const swShowNormal = 1
	r, _, callErr := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(filePtr)),
		0,
		uintptr(unsafe.Pointer(dirPtr)),
		swShowNormal,
	)
	if r <= 32 {
		if callErr != nil && callErr != syscall.Errno(0) {
			return callErr
		}
		return fmt.Errorf("ShellExecuteW failed with code %d", r)
	}
	return nil
}
