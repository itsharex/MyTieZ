//go:build !windows

package sys

// StartGlobalHotkeyHook is a no-op on non-Windows platforms.
func StartGlobalHotkeyHook(uint32, bool, bool, bool, bool, func()) (func(), bool) {
	return func() {}, false
}

// StopGlobalHotkeyHook is a no-op on non-Windows platforms.
func StopGlobalHotkeyHook() {}
