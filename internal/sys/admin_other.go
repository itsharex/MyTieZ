//go:build !windows

package sys

import "fmt"

// IsAdmin reports whether the current process is running with elevated privileges.
func IsAdmin() bool { return false }

// PrepareElevatedRestart is a no-op outside Windows.
func PrepareElevatedRestart() {}

// RestartAsAdmin is only supported on Windows.
func RestartAsAdmin() error {
	return fmt.Errorf("restart as admin is only supported on Windows")
}
