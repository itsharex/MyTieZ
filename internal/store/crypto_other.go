//go:build !windows

package store

// EncryptPrefix marks DPAPI-protected fields persisted in SQLite.
const EncryptPrefix = "dpapi:"

func encryptString(plain string) (string, error) {
	return plain, nil
}

func decryptString(value string) (string, error) {
	return value, nil
}
