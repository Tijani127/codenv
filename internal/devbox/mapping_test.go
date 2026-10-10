package devbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEveryMappingResolvesToARealManifest guards against inventing Scoop
// package names. It only runs where a live store exists.
func TestEveryMappingResolvesToARealManifest(t *testing.T) {
	root := os.Getenv("CODEENV_BUCKET_ROOT")
	if root == "" {
		home := os.Getenv("LOCALAPPDATA")
		if home == "" {
			t.Skip("no LOCALAPPDATA")
		}
		candidate := filepath.Join(home, "codenv", "store", "buckets")
		if _, err := os.Stat(candidate); err != nil {
			t.Skipf("no bucket store at %s", candidate)
		}
		root = candidate
	}

	buckets := []string{"main", "extras", "versions"}
	for nixName, targets := range Known() {
		for _, target := range targets {
			found := false
			for _, b := range buckets {
				if _, err := os.Stat(filepath.Join(root, b, "bucket", target+".json")); err == nil {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("mapping %q -> %q does not exist in any bucket", nixName, target)
			}
		}
	}

	// The nix-style version suffix idiom must also resolve to something real,
	// otherwise an import silently drops a pin the user asked for.
	for _, nixName := range []string{"postgresql_16", "python311", "python310"} {
		targets, ok := Lookup(nixName)
		if !ok {
			t.Errorf("Lookup(%q) found nothing", nixName)
			continue
		}
		for _, target := range targets {
			bucket, name, split := strings.Cut(target, "/")
			if !split {
				t.Errorf("version-suffixed lookup %q -> %q should be bucket-qualified", nixName, target)
				continue
			}
			if _, err := os.Stat(filepath.Join(root, bucket, "bucket", name+".json")); err != nil {
				t.Errorf("Lookup(%q) -> %q does not exist", nixName, target)
			}
		}
	}
}

func TestLookupResolvesNixVersionSuffixes(t *testing.T) {
	cases := map[string]string{
		"postgresql_16": "versions/postgresql16",
		"python311":     "versions/python311",
		"postgresql":    "postgresql",
		"python3":       "python",
	}
	for in, want := range cases {
		got, ok := Lookup(in)
		if !ok {
			t.Errorf("Lookup(%q) found nothing", in)
			continue
		}
		if got[0] != want {
			t.Errorf("Lookup(%q)[0] = %q, want %q", in, got[0], want)
		}
	}
}

func TestSplitVersionSuffix(t *testing.T) {
	cases := []struct {
		in     string
		base   string
		digits string
		ok     bool
	}{
		{"postgresql_16", "postgresql", "16", true},
		{"python311", "python", "311", true},
		{"redis", "", "", false},
		{"vim", "", "", false},
		{"nodejs_20", "nodejs", "20", true},
	}
	for _, c := range cases {
		base, digits, ok := splitVersionSuffix(c.in)
		if ok != c.ok || base != c.base || digits != c.digits {
			t.Errorf("splitVersionSuffix(%q) = %q, %q, %v; want %q, %q, %v",
				c.in, base, digits, ok, c.base, c.digits, c.ok)
		}
	}
}

func TestLookupKnownAndUnknown(t *testing.T) {
	got, ok := Lookup("ripgrep")
	if !ok || len(got) != 1 || got[0] != "ripgrep" {
		t.Errorf("Lookup(ripgrep) = %v, %v", got, ok)
	}
	got, ok = Lookup("python3")
	if !ok || got[0] != "python" {
		t.Errorf("Lookup(python3) = %v, %v", got, got)
	}
	if _, ok := Lookup("definitely-not-a-nix-package"); ok {
		t.Error("unknown name should not resolve")
	}
}

func TestReasonIsUseful(t *testing.T) {
	if r, ok := Reason("tmux"); !ok || !strings.Contains(r, "cygwin") && !strings.Contains(r, "msys2") {
		t.Errorf("Reason(tmux) = %q, %v; should point at a real alternative", r, ok)
	}
	if _, ok := Reason("ripgrep"); ok {
		t.Error("mapped packages should have no unmapped reason")
	}
}

func TestMappingsDoNotMapShellsThatOnlyShipWithGit(t *testing.T) {
	// zsh and fish are bundled in git-for-windows but are not standalone Scoop
	// packages. Mapping them to git would install something the user did not
	// ask for, so they must stay unmapped.
	for _, name := range []string{"zsh", "fish", "tmux"} {
		if _, ok := Lookup(name); ok {
			t.Errorf("%q should be reported as unmapped", name)
		}
		if _, ok := Reason(name); !ok {
			t.Errorf("%q should carry a reason explaining why", name)
		}
	}
}

func TestKnownIsACopy(t *testing.T) {
	a := Known()
	if len(a) == 0 {
		t.Fatal("Known is empty")
	}
	for k := range a {
		a[k][0] = "mutated"
		break
	}
	again := Known()
	for k, v := range again {
		if v[0] == "mutated" {
			t.Errorf("Known returned a shallow copy; %q was mutated by the caller", k)
		}
		break
	}
}
