//go:build !windows

package sys

func SendPaste() error {
	return nil
}
