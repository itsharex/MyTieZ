//go:build !windows

package sys

func CancelGameModePaste() {}

func SendPasteWith(method, text string, isTextKind bool) error {
	return nil
}
