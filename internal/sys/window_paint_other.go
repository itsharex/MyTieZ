//go:build !windows

package sys

func SetWindowCloaked(uintptr, bool) bool { return false }

func SyncWindowPaint(uintptr) {}
