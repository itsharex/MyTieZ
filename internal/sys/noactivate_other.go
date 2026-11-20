//go:build !windows

package sys

func SetWindowNoActivate(uintptr, bool) {}

func StartOutsideClickWatch(uintptr, func()) (func(), bool) { return func() {}, false }
