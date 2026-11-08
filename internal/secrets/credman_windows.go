//go:build windows

package secrets

import (
	"fmt"

	"github.com/danieljoos/wincred"
)

// CredentialTarget is the Credential Manager target codenv stores secrets
// under. The mapping from project declaration to stored value lives in the
// project's codenv.json; the Credential Manager holds the value itself.
func CredentialTarget(name string) string {
	return "codenv:secret:" + name
}

// getCredential reads a secret from the Windows Credential Manager.
func getCredential(name string) (string, bool) {
	cred, err := wincred.GetGenericCredential(CredentialTarget(name))
	if err != nil || cred == nil {
		return "", false
	}
	if len(cred.CredentialBlob) == 0 {
		return "", false
	}
	return string(cred.CredentialBlob), true
}

// StoreCredential saves VALUE under codenv:secret:<NAME>. Any previous value
// for the name is replaced.
func StoreCredential(name, value string) error {
	cred := wincred.NewGenericCredential(CredentialTarget(name))
	cred.CredentialBlob = []byte(value)
	cred.Persist = wincred.PersistLocalMachine
	return cred.Write()
}

// DeleteCredential removes a stored secret. It reports whether a cred existed.
func DeleteCredential(name string) (bool, error) {
	cred, err := wincred.GetGenericCredential(CredentialTarget(name))
	if err != nil {
		// ERROR_NOT_FOUND means there was nothing to delete, which callers
		// should see as "already gone", not as a failure.
		return false, nil
	}
	if cred == nil {
		return false, nil
	}
	if err := cred.Delete(); err != nil {
		return true, fmt.Errorf("deleting %s: %w", CredentialTarget(name), err)
	}
	return true, nil
}
