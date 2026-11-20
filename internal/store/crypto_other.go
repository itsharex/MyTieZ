//go:build !windows

package store

// EncryptPrefix marks DPAPI-protected fields persisted in SQLite.
const EncryptPrefix = "dpapi:"

func encryptString(plain string) string {
	return plain
}

func decryptString(value string) string {
	return value
}
