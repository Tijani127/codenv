package shellrun

import (
	"os"
	"strings"
	"testing"
)

func TestExtensionsFor(t *testing.T) {
	const pathext = ".COM;.EXE;.BAT;.CMD;.VBS;.JS;.WSF;.MSC"

	cases := []struct {
		name string
		want []string
	}{
		{"rg", []string{"", ".com", ".exe", ".bat", ".cmd", ".vbs", ".js", ".wsf", ".msc"}},
		{"where.exe", []string{""}},
		{"cmd.EXE", []string{""}},
		{"npm.cmd", []string{""}},
		{"foo.CMD", []string{""}},
		{"7z", []string{"", ".com", ".exe", ".bat", ".cmd", ".vbs", ".js", ".wsf", ".msc"}},
		{"gofmt.exe", []string{""}},
	}

	for _, tc := range cases {
		got := extensionsFor(tc.name, pathext)
		if strings.Join(got, ",") != strings.Join(tc.want, ",") {
			t.Errorf("extensionsFor(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestExtensionsForEmptyPathExt(t *testing.T) {
	got := extensionsFor("where.exe", "")
	if len(got) != 1 || got[0] != "" {
		t.Errorf("extensionsFor with empty PATHEXT = %v, want [\"\"]", got)
	}
}

func TestLookPathFindsExplicitExtension(t *testing.T) {
	sysDir := os.Getenv("SystemRoot")
	if sysDir == "" {
		t.Skip("SystemRoot not set")
	}
	dir := sysDir + `\System32`
	env := []string{
		"PATH=" + dir,
		"PATHEXT=.COM;.EXE;.BAT;.CMD",
	}
	for _, name := range []string{"where.exe", "where", "cmd.exe"} {
		got, err := LookPath(env, name)
		if err != nil {
			t.Fatalf("LookPath(%q) failed: %v", name, err)
		}
		if !strings.EqualFold(filepathBase(got), strings.ToLower(name)) && filepathBase(got) == "" {
			t.Fatalf("LookPath(%q) returned %q", name, got)
		}
	}
}

func TestLookPathPrefersStoreDir(t *testing.T) {
	dir := t.TempDir()
	exe := dir + `\fake-tool.exe`
	if err := writeEmptyExe(exe); err != nil {
		t.Fatal(err)
	}
	env := []string{
		"PATH=" + dir,
		"PATHEXT=.COM;.EXE",
	}
	got, err := LookPath(env, "fake-tool")
	if err != nil {
		t.Fatalf("LookPath failed: %v", err)
	}
	if !strings.EqualFold(got, exe) {
		t.Errorf("LookPath = %q, want %q", got, exe)
	}
	if got, err := LookPath(env, "fake-tool.exe"); err != nil || !strings.EqualFold(got, exe) {
		t.Errorf("LookPath with explicit .exe = %q, %v", got, err)
	}
}

func TestLookPathMissing(t *testing.T) {
	env := []string{"PATH=" + t.TempDir(), "PATHEXT=.COM;.EXE"}
	if _, err := LookPath(env, "definitely-not-a-real-tool-xyz"); err == nil {
		t.Error("expected an error for a missing command")
	}
}

func filepathBase(p string) string {
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		return p[i+1:]
	}
	return p
}

func writeEmptyExe(path string) error {
	return os.WriteFile(path, []byte("MZ"), 0o755)
}

func TestLookPathInResolvesRelativeToDir(t *testing.T) {
	dir := t.TempDir()
	if err := writeEmptyExe(dir + `\tool.exe`); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + dir, "PATHEXT=.COM;.EXE"}

	for _, name := range []string{`tool.exe`, `.\tool.exe`, `./tool.exe`, `tool`} {
		got, err := LookPathIn(env, dir, name)
		if err != nil {
			t.Errorf("LookPathIn(%q) failed: %v", name, err)
			continue
		}
		if !strings.EqualFold(filepathBase(got), "tool.exe") {
			t.Errorf("LookPathIn(%q) = %q", name, got)
		}
	}

	// A relative path must be resolved against the given dir. PATH happens to
	// contain the same dir here, so use a PATH that does not, to prove the
	// relative form is anchored to dir rather than to the process cwd.
	narrowEnv := []string{"PATH=" + t.TempDir(), "PATHEXT=.COM;.EXE"}
	if _, err := LookPathIn(narrowEnv, dir, `.\tool.exe`); err != nil {
		t.Errorf("LookPathIn should find .\\tool.exe via dir even when PATH lacks it: %v", err)
	}
	if _, err := LookPathIn(narrowEnv, t.TempDir(), `.\tool.exe`); err == nil {
		t.Error("LookPathIn should fail when neither dir nor PATH contains the file")
	}
}

func TestLookPathInAddsExtensionForRelativePaths(t *testing.T) {
	dir := t.TempDir()
	if err := writeEmptyExe(dir + `\prog.exe`); err != nil {
		t.Fatal(err)
	}
	env := []string{"PATH=" + dir, "PATHEXT=.EXE"}
	got, err := LookPathIn(env, dir, `.\prog`)
	if err != nil {
		t.Fatalf("expected extension probing, got %v", err)
	}
	if !strings.HasSuffix(strings.ToLower(got), "prog.exe") {
		t.Errorf("got %q, want prog.exe", got)
	}
}
