package gc

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Tijani127/codenv/internal/config"
	"github.com/Tijani127/codenv/internal/store"
)

func mustDelete(t *testing.T, path string) {
	t.Helper()
	if err := os.RemoveAll(path); err != nil {
		t.Fatal(err)
	}
}

func fakeStore(t *testing.T) (*store.Store, *store.Index) {
	t.Helper()
	base := t.TempDir()
	st, err := store.Open(store.ModeProject, base)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	ix, err := store.LoadIndex(st)
	if err != nil {
		t.Fatal(err)
	}
	return st, ix
}

func installVersion(t *testing.T, st *store.Store, app, version string, files map[string]string) {
	t.Helper()
	dir := st.VersionDir(app, version)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func lockWith(t *testing.T, pins ...string) *config.Lock {
	t.Helper()
	lock := config.NewLock()
	for _, pin := range pins {
		// pin form: app@version
		name, version, ok := splitPin(pin)
		if !ok {
			t.Fatalf("bad pin %q", pin)
		}
		lock.Resolved[name+"@"+version] = config.LockedPackage{Name: name, Version: version, Bucket: "main"}
	}
	return lock
}

func splitPin(pin string) (string, string, bool) {
	for i := len(pin) - 1; i >= 0; i-- {
		if pin[i] == '@' {
			return pin[:i], pin[i+1:], true
		}
	}
	return "", "", false
}

func TestSweepKeepsPinnedAndCurrent(t *testing.T) {
	st, ix := fakeStore(t)

	installVersion(t, st, "7zip", "24.09", map[string]string{"7z.exe": "x"})
	installVersion(t, st, "7zip", "26.03", map[string]string{"7z.exe": "xx"})
	installVersion(t, st, "ripgrep", "14.1.0", map[string]string{"rg.exe": "y"})
	// The store's own Scoop installation must never be swept.
	installVersion(t, st, "scoop", "latest", map[string]string{"scoop.ps1": "s"})

	ix.Set("7zip", "24.09", []string{st.VersionDir("7zip", "24.09")})
	ix.Set("7zip", "26.03", []string{st.VersionDir("7zip", "26.03")})
	// A stale index entry: 13.0.2 was deleted manually some time ago.
	ix.Set("ripgrep", "13.0.2", []string{st.VersionDir("ripgrep", "13.0.2")})
	if err := ix.Save(); err != nil {
		t.Fatal(err)
	}
	ix, err := store.LoadIndex(st)
	if err != nil {
		t.Fatal(err)
	}

	lock := lockWith(t, "7zip@24.09", "ripgrep@14.1.0")
	sweep := SweepStore(st, ix, RefsOf(lock))

	if len(sweep.Entries) != 1 {
		t.Fatalf("expected exactly 1 deletion candidate (7zip 26.03), got %+v", sweep.Entries)
	}
	if got := sweep.Entries[0]; got.App != "7zip" || got.Version != "26.03" {
		t.Fatalf("wrong candidate: %+v", got)
	}
	stales := map[string]bool{}
	for _, e := range sweep.Stale {
		stales[e.App+"@"+e.Version] = true
	}
	if len(stales) != 1 || !stales["ripgrep@13.0.2"] {
		t.Fatalf("expected only ripgrep@13.0.2 stale, got %v", stales)
	}

	if err := sweep.Apply(st, ix); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if _, err := os.Stat(st.VersionDir("7zip", "24.09")); err != nil {
		t.Fatal("pinned version was deleted")
	}
	if _, err := os.Stat(st.VersionDir("ripgrep", "14.1.0")); err != nil {
		t.Fatal("referenced version was deleted")
	}
	if _, err := os.Stat(st.VersionDir("scoop", "latest")); err != nil {
		t.Fatal("guarded scoop app was deleted")
	}
	if _, err := os.Stat(st.VersionDir("7zip", "26.03")); !os.IsNotExist(err) {
		t.Fatal("unpinned version survived")
	}
	if ix.Has("7zip", "26.03") {
		t.Fatal("index entry for deleted version survived")
	}
	if ix.Has("ripgrep", "13.0.2") {
		t.Fatal("stale index entry survived")
	}
}

func TestSweepSparesCurrentJunctionTarget(t *testing.T) {
	st, ix := fakeStore(t)

	installVersion(t, st, "7zip", "24.09", map[string]string{"7z.exe": "x"})
	// Junctions can be created without admin rights; symlinks cannot. If the
	// junction cannot be created in this environment, the current-protection
	// rule simply cannot be exercised here.
	current := st.CurrentPath("7zip")
	if out, err := exec.Command("cmd", "/c", "mklink", "/J", current, st.VersionDir("7zip", "24.09")).CombinedOutput(); err != nil {
		t.Skipf("mklink /J unavailable: %s", out)
	}
	t.Cleanup(func() { mustDelete(t, current) })

	sweep := SweepStore(st, ix, RefsOf(config.NewLock()))
	for _, e := range sweep.Entries {
		if e.App == "7zip" && e.Version == "24.09" {
			t.Fatal("the version the current junction points at must be spared")
		}
	}
	if err := sweep.Apply(st, ix); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(st.VersionDir("7zip", "24.09")); err != nil {
		t.Fatal("current junction target was deleted")
	}
}

func TestApplyRemovesEmptyAppDir(t *testing.T) {
	st, ix := fakeStore(t)

	installVersion(t, st, "onlyapp", "1.0", map[string]string{"tool.exe": "x"})
	sweep := SweepStore(st, ix, nil)
	if len(sweep.Entries) != 1 {
		t.Fatalf("expected the only version to be swept, got %+v", sweep.Entries)
	}
	if err := sweep.Apply(st, ix); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(st.AppDir("onlyapp")); !os.IsNotExist(err) {
		t.Fatal("empty app directory survived")
	}
}
