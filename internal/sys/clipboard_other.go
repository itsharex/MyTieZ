//go:build !windows

package sys

import "errors"

// ErrClipboardUnsupported is returned on non-Windows platforms so callers fall
// back to MyGo's clipboard API.
var ErrClipboardUnsupported = errors.New("clipboard image write unsupported on this platform")

// ClipboardSequenceNumber returns 0 on non-Windows platforms.
func ClipboardSequenceNumber() uint32 { return 0 }

// WriteClipboardImageFile is unsupported on non-Windows platforms.
func WriteClipboardImageFile(path string) error { return ErrClipboardUnsupported }
