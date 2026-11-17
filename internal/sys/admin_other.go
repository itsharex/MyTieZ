//go:build !windows

package sys

import "fmt"

// RestartAsAdmin is only supported on Windows.
func RestartAsAdmin() error {
	return fmt.Errorf("restart as admin is only supported on Windows")
}
