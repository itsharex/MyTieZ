//go:build !windows

package sys

func CaptureClipboardSource() (string, string) {
	return "Unknown", ""
}
