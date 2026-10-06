package doctor

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/Tijani127/codenv/internal/config"
	"github.com/Tijani127/codenv/internal/store"
)

func TestRunWithoutStoreDoesNotPanic(t *testing.T) {
	r := Run(Options{})
	if len(r.Checks) == 0 {
		t.Error("expected at least the OS check")
	}
	if r.Worst() == Fail {
		t.Errorf("no store should not be fatal, got %v", r.Worst())
	}
}

func TestOSCheckPassesOnWindows(t *testing.T) {
	r := &Report{}
	checkOS(r)
	if len(r.Checks) != 1 {
		t.Fatalf("expected 1 check, got %d", len(r.Checks))
	}
	if r.Checks[0].Level != OK {
		t.Errorf("OS check should pass on windows, got %s", r.Checks[0].Level)
	}
}

func TestShimsCheckPassesWhenAbsent(t *testing.T) {
	base := t.TempDir()
	st2, err := store.Open(store.ModeProject, base)
	if err != nil {
		t.Fatal(err)
	}
	r := &Report{}
	checkShims(r, st2)
	if r.Checks[0].Level != OK {
		t.Errorf("absent shims should be ok, got %s (%s)", r.Checks[0].Level, r.Checks[0].Detail)
	}
}

func TestShimsCheckWarnsWhenPresent(t *testing.T) {
	base := t.TempDir()
	st, err := store.Open(store.ModeProject, base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(st.ShimsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"tool.exe", "tool.shim"} {
		if err := os.WriteFile(filepath.Join(st.ShimsDir(), name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	r := &Report{}
	checkShims(r, st)
	if r.Checks[0].Level != Warn {
		t.Errorf("leftover shims should warn, got %s", r.Checks[0].Level)
	}
	if r.Checks[0].Hint == "" {
		t.Error("a warning should carry a hint")
	}
}

func TestFixRemovesShims(t *testing.T) {
	base := t.TempDir()
	st, err := store.Open(store.ModeProject, base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(st.ShimsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.ShimsDir(), "x.exe"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := Fix(Options{Store: st})
	if err != nil {
		t.Fatal(err)
	}
	if res.RemovedShims != 1 {
		t.Errorf("RemovedShims = %d, want 1", res.RemovedShims)
	}
	if _, err := os.Stat(st.ShimsDir()); err == nil {
		t.Error("shims dir should be gone")
	}
}

func TestFixPrunesStaleIndex(t *testing.T) {
	base := t.TempDir()
	st, err := store.Open(store.ModeProject, base)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(st.AppsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	ix, err := store.LoadIndex(st)
	if err != nil {
		t.Fatal(err)
	}
	ix.Set("ghost", "9.9.9", []string{filepath.Join(base, "nope")})
	if err := ix.Save(); err != nil {
		t.Fatal(err)
	}

	res, err := Fix(Options{Store: st})
	if err != nil {
		t.Fatal(err)
	}
	if res.PrunedIndex != 1 {
		t.Errorf("PrunedIndex = %d, want 1", res.PrunedIndex)
	}
	reloaded, err := store.LoadIndex(st)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Apps) != 0 {
		t.Errorf("index should be empty, got %v", reloaded.Apps)
	}
}

func TestFixKeepsLiveIndexEntries(t *testing.T) {
	base := t.TempDir()
	st, err := store.Open(store.ModeProject, base)
	if err != nil {
		t.Fatal(err)
	}
	versionDir := st.VersionDir("keeper", "1.0")
	if err := os.MkdirAll(versionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ix, err := store.LoadIndex(st)
	if err != nil {
		t.Fatal(err)
	}
	ix.Set("keeper", "1.0", []string{versionDir})
	if err := ix.Save(); err != nil {
		t.Fatal(err)
	}

	if _, err := Fix(Options{Store: st}); err != nil {
		t.Fatal(err)
	}
	reloaded, _ := store.LoadIndex(st)
	if !reloaded.Has("keeper", "1.0") {
		t.Error("a live index entry must not be pruned")
	}
}

func TestBucketsCheckWarnsWhenDirty(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	base := t.TempDir()
	st, err := store.Open(store.ModeProject, base)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(st.BucketsDir(), "main")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) {
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
			"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
		_ = cmd.Run()
	}
	run("init", "-q")
	if err := os.WriteFile(filepath.Join(dir, "m.json"), []byte(`{"version":"1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "-A")
	run("commit", "-qm", "init")
	if err := os.WriteFile(filepath.Join(dir, "m.json"), []byte(`{"version":"2"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	r := &Report{}
	checkBuckets(r, st)
	if r.Checks[0].Level != Warn {
		t.Errorf("dirty bucket should warn, got %s (%s)", r.Checks[0].Level, r.Checks[0].Detail)
	}

	res, err := Fix(Options{Store: st})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.ResetBuckets) != 1 || res.ResetBuckets[0] != "main" {
		t.Errorf("ResetBuckets = %v, want [main]", res.ResetBuckets)
	}
	r2 := &Report{}
	checkBuckets(r2, st)
	if r2.Checks[0].Level != OK {
		t.Errorf("bucket should be clean after fix, got %s", r2.Checks[0].Level)
	}
}

func TestSecretsCheckWarnsWhenSourceUnset(t *testing.T) {
	t.Setenv("CODEENV_TEST_PRESENT", "value")
	t.Setenv("CODEENV_TEST_ABSENT", "")
	cfg := &config.Config{Secrets: &config.Secrets{
		Names: []string{"A", "B"},
		From:  map[string]string{"A": "CODEENV_TEST_PRESENT", "B": "CODEENV_TEST_ABSENT"},
	}}
	r := &Report{}
	checkSecrets(r, Options{Config: cfg})
	if r.Checks[0].Level != Warn {
		t.Errorf("unset secret source should warn, got %s", r.Checks[0].Level)
	}
}

func TestLockedPackagesCheck(t *testing.T) {
	base := t.TempDir()
	st, err := store.Open(store.ModeProject, base)
	if err != nil {
		t.Fatal(err)
	}
	lock := config.NewLock()
	lock.Top = []string{"main/ghost@1.0.0"}
	lock.Resolved["main/ghost@1.0.0"] = config.LockedPackage{
		Bucket: "main", Name: "ghost", Version: "1.0.0",
	}
	r := &Report{}
	checkPackages(r, st, lock)
	if r.Checks[0].Level != Warn {
		t.Errorf("uninstalled lock entry should warn, got %s", r.Checks[0].Level)
	}
}

func TestReportCounting(t *testing.T) {
	r := &Report{}
	r.Add(Check{Name: "a", Level: OK})
	r.Add(Check{Name: "b", Level: Warn})
	r.Add(Check{Name: "c", Level: Warn})
	if r.Count(Warn) != 2 {
		t.Errorf("Count(Warn) = %d, want 2", r.Count(Warn))
	}
	if r.Worst() != Warn {
		t.Errorf("Worst = %v, want warn", r.Worst())
	}
	r.Add(Check{Name: "d", Level: Fail})
	if r.Worst() != Fail {
		t.Errorf("Worst = %v, want fail", r.Worst())
	}
}
