//go:build !windows

package sys

func TrackForegroundWindow() {}

func RestoreForegroundWindow() bool { return false }

func IsLeftMouseButtonDown() bool { return false }
