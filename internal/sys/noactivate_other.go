//go:build !windows

package sys

func SetWindowNoActivate(uintptr, bool) {}

func StartOutsideClickWatch(uintptr, func()) func() { return func() {} }
