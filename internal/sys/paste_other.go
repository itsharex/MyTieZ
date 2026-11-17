//go:build !windows

package sys

func SendPaste() error {
	return nil
}

func SendPasteWith(method, text string, isTextKind bool) error {
	return nil
}
