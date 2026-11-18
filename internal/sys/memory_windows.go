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

const (
	// MemoryTrimThresholdBytes is the working set size threshold (50 MB) below which
	// the process leaves caches and physical pages untouched.
	MemoryTrimThresholdBytes = 50 << 20
	memorySoftMinBytes       = 20 << 20
	memorySoftMaxBytes       = 36 << 20
)

// WorkingSetBytes returns the current physical working set size of the process in bytes.
func WorkingSetBytes() uintptr {
	proc, _, _ := procGetCurrentProcess.Call()
	if proc == 0 {
		return 0
	}
	var pmc processMemoryCounters
	pmc.cb = uint32(unsafe.Sizeof(pmc))
	ok, _, _ := procGetProcessMemoryInfo.Call(proc, uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.cb))
	if ok == 0 {
		return 0
	}
	return pmc.workingSetSize
}

// TrimMemory gently trims excess physical pages when the working set exceeds 50 MB,
// keeping a 20-36 MB soft working set target to avoid aggressive page-out thrashing.
func TrimMemory() bool {
	proc, _, _ := procGetCurrentProcess.Call()
	if proc == 0 {
		return false
	}
	var pmc processMemoryCounters
	pmc.cb = uint32(unsafe.Sizeof(pmc))
	ok, _, _ := procGetProcessMemoryInfo.Call(proc, uintptr(unsafe.Pointer(&pmc)), uintptr(pmc.cb))
	if ok == 0 || pmc.workingSetSize <= MemoryTrimThresholdBytes {
		return false
	}
	_, _, _ = procSetProcessWorkingSet.Call(proc, memorySoftMinBytes, memorySoftMaxBytes)
	return true
}
