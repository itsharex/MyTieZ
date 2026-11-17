//go:build !windows

package sys

// TrimMemory is a no-op on non-Windows platforms.
func TrimMemory() {}
