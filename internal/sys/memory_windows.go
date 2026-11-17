//go:build windows

package sys

import "syscall"

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentProcess    = kernel32.NewProc("GetCurrentProcess")
	procSetProcessWorkingSet = kernel32.NewProc("SetProcessWorkingSetSize")
)

// TrimMemory asks Windows to trim unused physical pages from the process working set.
func TrimMemory() {
	proc, _, _ := procGetCurrentProcess.Call()
	if proc == 0 {
		return
	}
	const sizeMax = ^uintptr(0)
	_, _, _ = procSetProcessWorkingSet.Call(proc, sizeMax, sizeMax)
}
