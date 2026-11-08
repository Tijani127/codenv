//go:build !windows

package secrets

// Non-Windows build stubs. codenv itself is Windows-only; these keep the
// package compiling (and tests honest) on other platforms.

func CredentialTarget(name string) string {
	return "codenv:secret:" + name
}

func getCredential(name string) (string, bool) {
	return "", false
}

func StoreCredential(name, value string) error {
	return errCredManUnavailable
}

func DeleteCredential(name string) (bool, error) {
	return false, errCredManUnavailable
}
