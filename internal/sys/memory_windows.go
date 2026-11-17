//go:build windows

package sys

import (
	"syscall"
	"unsafe"
)

var (
	kernel32                 = syscall.NewLazyDLL("kernel32.dll")
	procGetCurrentProcess    = kernel32.NewProc("GetCurrentProcess")
	procGetProcessMemoryInfo = kernel32.NewProc("K32GetProcessMemoryInfo")
	procSetProcessWorkingSet = kernel32.NewProc("SetProcessWorkingSetSize")
)

type processMemoryCounters struct {
	cb                         uint32
	pageFaultCount             uint32
	peakWorkingSetSize         uintptr
	workingSetSize             uintptr
	quotaPeakPagedPoolUsage    uintptr
	quotaPagedPoolUsage        uintptr
	quotaPeakNonPagedPoolUsage uintptr
	quotaNonPagedPoolUsage     uintptr
	pagefileUsage              uintptr
	peakPagefileUsage          uintptr
}

// TrimMemory gently trims excess physical pages when the working set exceeds 40 MB,
// keeping a 20-32 MB soft working set target to avoid aggressive page-out thrashing.
func TrimMemory() {
	proc, _, _ := procGetCurrentProcess.Call()
	if proc == 0 {
		return
	}
	const (
		trimThreshold = 40 << 20
		softMin       = 20 << 20
		softMax       = 32 << 20
	)
	var pmc processMemoryCounters
	pmc.cb = uint32(unsafe.Sizeof(pmc))
	ok, _, _ := procGetProcessMemoryInfo.Call(proc, uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.cb))
	if ok == 0 || pmc.workingSetSize <= trimThreshold {
		return
	}
	_, _, _ = procSetProcessWorkingSet.Call(proc, softMin, softMax)
}
