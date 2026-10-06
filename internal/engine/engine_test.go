package engine

import (
	"strings"
	"testing"

	"github.com/Tijani127/codenv/internal/config"
)

func TestMissingErrorMessage(t *testing.T) {
	lock := config.NewLock()
	lock.Resolved["main/7zip@24.09"] = config.LockedPackage{
		Bucket: "main", Name: "7zip", Version: "24.09", Architecture: "64bit",
	}
	lock.Resolved["main/ripgrep@14.1.1"] = config.LockedPackage{
		Bucket: "main", Name: "ripgrep", Version: "14.1.1", Architecture: "64bit",
	}

	err := &MissingError{
		Packages: []string{"main/7zip@24.09", "main/ripgrep@14.1.1"},
		Lock:     lock,
	}
	msg := err.Error()

	for _, want := range []string{
		"main/7zip@24.09",
		"main/ripgrep@14.1.1",
		"will not substitute",
		"reproducib",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("MissingError missing %q\n got: %s", want, msg)
		}
	}
}

func TestInstallSpecIncludesVersion(t *testing.T) {
	got := installSpec(config.LockedPackage{Bucket: "main", Name: "7zip", Version: "24.09"})
	if got != "main/7zip@24.09" {
		t.Errorf("installSpec = %q, want main/7zip@24.09", got)
	}
	got = installSpec(config.LockedPackage{Name: "fd", Version: "10.5.0"})
	if got != "fd@10.5.0" {
		t.Errorf("installSpec = %q, want fd@10.5.0", got)
	}
}

func TestReadyRequiresExactVersion(t *testing.T) {
	e := &Engine{}
	if e.ready("anything", "") {
		t.Error("ready with empty version should be false")
	}
	if e.ready("", "1.0") {
		t.Error("ready with empty name should be false")
	}
}
