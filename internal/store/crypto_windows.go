//go:build windows

package store

import (
	"encoding/base64"
	"fmt"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// EncryptPrefix marks DPAPI-protected fields persisted in SQLite.
const EncryptPrefix = "dpapi:"

func encryptString(plain string) (string, error) {
	if plain == "" || strings.HasPrefix(plain, EncryptPrefix) {
		return plain, nil
	}
	inBytes := []byte(plain)
	var inBlob windows.DataBlob
	inBlob.Size = uint32(len(inBytes))
	inBlob.Data = &inBytes[0]

	var outBlob windows.DataBlob
	if err := windows.CryptProtectData(
		&inBlob,
		nil,
		nil,
		0,
		nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN,
		&outBlob,
	); err != nil {
		return "", fmt.Errorf("dpapi encrypt: %w", err)
	}
	defer func() {
		if outBlob.Data != nil {
			_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(outBlob.Data)))
		}
	}()
	out := unsafe.Slice(outBlob.Data, outBlob.Size)
	return EncryptPrefix + base64.StdEncoding.EncodeToString(out), nil
}

func decryptString(value string) (string, error) {
	if !strings.HasPrefix(value, EncryptPrefix) {
		return value, nil
	}
	payload := strings.TrimPrefix(value, EncryptPrefix)
	raw, err := base64.StdEncoding.DecodeString(payload)
	if err != nil || len(raw) == 0 {
		return "", fmt.Errorf("dpapi decode: invalid payload")
	}
	var inBlob windows.DataBlob
	inBlob.Size = uint32(len(raw))
	inBlob.Data = &raw[0]

	var outBlob windows.DataBlob
	if err := windows.CryptUnprotectData(
		&inBlob,
		nil,
		nil,
		0,
		nil,
		windows.CRYPTPROTECT_UI_FORBIDDEN,
		&outBlob,
	); err != nil {
		return "", fmt.Errorf("dpapi decrypt: %w", err)
	}
	defer func() {
		if outBlob.Data != nil {
			_, _ = windows.LocalFree(windows.Handle(unsafe.Pointer(outBlob.Data)))
		}
	}()
	if outBlob.Size == 0 || outBlob.Data == nil {
		return "", nil
	}
	return string(unsafe.Slice(outBlob.Data, outBlob.Size)), nil
}
