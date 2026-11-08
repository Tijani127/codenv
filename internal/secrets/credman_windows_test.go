//go:build windows

package secrets

import (
	"testing"

	"github.com/Tijani127/codenv/internal/config"
)

const selfTestName = "CODEENV_SELFTEST"

func TestCredentialRoundtrip(t *testing.T) {
	// Clean up even when the test fails halfway.
	t.Cleanup(func() {
		_, _ = DeleteCredential(selfTestName)
	})

	if err := StoreCredential(selfTestName, "s3cr3t-value"); err != nil {
		t.Skipf("cannot write to the Credential Manager: %v", err)
	}
	got, ok := getCredential(selfTestName)
	if !ok {
		t.Fatal("secret not found immediately after storing it")
	}
	if got != "s3cr3t-value" {
		t.Fatalf("round-trip mismatch: got %q", got)
	}

	existed, err := DeleteCredential(selfTestName)
	if err != nil {
		t.Fatal(err)
	}
	if !existed {
		t.Fatal("delete reported the secret did not exist")
	}
	if _, ok := getCredential(selfTestName); ok {
		t.Fatal("secret still readable after delete")
	}
}

func TestResolveFallsBackToCredentialManager(t *testing.T) {
	t.Cleanup(func() {
		_, _ = DeleteCredential(selfTestName)
	})
	// Make sure the environment does not provide the value.
	t.Setenv(selfTestName, "")

	if err := StoreCredential(selfTestName, "stored-here"); err != nil {
		t.Skipf("cannot write to the Credential Manager: %v", err)
	}

	cfg := &config.Config{
		Secrets: &config.Secrets{
			Names: []string{selfTestName, "CODEENV_SELFTEST_UNSTORED"},
		},
	}
	out, err := ResolveFromEnv(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if got := out.Values[selfTestName]; got != "stored-here" {
		t.Fatalf("expected the credential manager value, got %q", got)
	}
	if out.Sources[selfTestName] != "credential manager" {
		t.Fatalf("expected source %q, got %q", "credential manager", out.Sources[selfTestName])
	}
	if !out.Redacted["CODEENV_SELFTEST_UNSTORED"] {
		t.Fatal("undeclared-and-unstored name should be redacted")
	}
}
