//go:build windows

package sys

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	elevatedWaitFlagPrefix = "--wait-pid="
	elevatedWaitTimeout    = 10 * time.Second
)

var (
	shell32                      = syscall.NewLazyDLL("shell32.dll")
	procShellExecuteW            = shell32.NewProc("ShellExecuteW")
	procAllowSetForegroundWindow = user32.NewProc("AllowSetForegroundWindow")
)

// IsAdmin reports whether the current process is running with elevated privileges.
func IsAdmin() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// PrepareElevatedRestart waits for the previous instance to exit before
// acquiring the single-instance lock so an elevated restart never forwards
// to a shutting-down instance and exits.
func PrepareElevatedRestart() {
	waitPID, cleaned := extractWaitPID(os.Args)
	os.Args = cleaned
	if waitPID > 0 && waitPID != os.Getpid() {
		waitForProcessExit(waitPID, elevatedWaitTimeout)
	}
}

func waitForProcessExit(pid int, timeout time.Duration) {
	if pid <= 0 || timeout <= 0 {
		return
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return
	}
	defer windows.CloseHandle(h)
	_, _ = windows.WaitForSingleObject(h, uint32(timeout.Milliseconds()))
}

func buildRestartAdminArgs(pid int, origArgs []string) []string {
	args := make([]string, 0, len(origArgs)+1)
	skipNext := false
	for _, a := range origArgs {
		if skipNext {
			skipNext = false
			continue
		}
		if a == "--wait-pid" {
			skipNext = true
			continue
		}
		if strings.HasPrefix(a, elevatedWaitFlagPrefix) {
			continue
		}
		args = append(args, a)
	}
	args = append(args, elevatedWaitFlagPrefix+strconv.Itoa(pid))
	return args
}

func extractWaitPID(args []string) (int, []string) {
	if len(args) == 0 {
		return 0, args
	}
	cleaned := make([]string, 0, len(args))
	cleaned = append(cleaned, args[0])
	waitPID := 0
	for i := 1; i < len(args); i++ {
		arg := args[i]
		if arg == "--wait-pid" {
			if i+1 < len(args) {
				if pid, err := strconv.Atoi(args[i+1]); err == nil && pid > 0 {
					waitPID = pid
				}
				i++
			}
			continue
		}
		if raw, ok := strings.CutPrefix(arg, elevatedWaitFlagPrefix); ok {
			if pid, err := strconv.Atoi(raw); err == nil && pid > 0 {
				waitPID = pid
			}
			continue
		}
		cleaned = append(cleaned, arg)
	}
	return waitPID, cleaned
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
	params := windows.ComposeCommandLine(buildRestartAdminArgs(os.Getpid(), os.Args[1:]))
	paramsPtr, err := syscall.UTF16PtrFromString(params)
	if err != nil {
		return err
	}
	dirPtr, err := syscall.UTF16PtrFromString(filepath.Dir(exe))
	if err != nil {
		return err
	}
	// Pass foreground activation rights before launching so the elevated instance can raise its panel.
	procAllowSetForegroundWindow.Call(^uintptr(0))
	const swShowNormal = 1
	r, _, callErr := procShellExecuteW.Call(
		0,
		uintptr(unsafe.Pointer(verbPtr)),
		uintptr(unsafe.Pointer(filePtr)),
		uintptr(unsafe.Pointer(paramsPtr)),
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
