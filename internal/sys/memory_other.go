//go:build !windows

package sys

const MemoryTrimThresholdBytes = 75 << 20

// WorkingSetBytes returns 0 on non-Windows platforms.
func WorkingSetBytes() uintptr { return 0 }

// TrimMemory is a no-op on non-Windows platforms.
func TrimMemory() bool { return false }

// TrimIdleMemory is a no-op on non-Windows platforms.
func TrimIdleMemory() bool { return false }
