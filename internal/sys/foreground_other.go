//go:build !windows

package sys

func TrackForegroundWindow() {}

func StartForegroundWatcher() (func(), bool) { return func() {}, false }

func RestoreForegroundWindow() bool { return false }

func IsLeftMouseButtonDown() bool { return false }
