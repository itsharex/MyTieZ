//go:build !windows

package sys

// RoutedKey represents a keyboard event intercepted while the panel is in no-activate mode.
type RoutedKey struct {
	VK    uint32
	Text  string
	Ctrl  bool
	Shift bool
	Alt   bool
}

// CanRouteForeground reports whether the current foreground window can have
// its keyboard input intercepted by a low-level hook from this process.
func CanRouteForeground() bool {
	return false
}

// StartKeyRouter installs a low-level keyboard hook and forwards panel-owned keys.
func StartKeyRouter(func(RoutedKey)) (func(), bool) {
	return func() {}, false
}

// SetExtraRoutedShortcut is a no-op on non-Windows platforms.
func SetExtraRoutedShortcut(vk uint32, ctrl, shift, alt bool) {}
