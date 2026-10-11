package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFakeExe(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("MZ"), 0o755); err != nil {
		t.Fatal(err)
	}
}

// TestProbeLayoutDirsFindsBinOnlyExecutables guards the defect that made
// codenv silently resolve the wrong java. A JDK installed into the java bucket
// declares no bin in its manifest and keeps java.exe in bin\, so without a
// layout probe only the app root was put on PATH and the system JDK won.
func TestProbeLayoutDirsFindsBinOnlyExecutables(t *testing.T) {
	base := t.TempDir()
	writeFakeExe(t, filepath.Join(base, "bin", "java.exe"))
	// The app root must not be reported: a JDK root holds no executable.
	writeFakeExe(t, filepath.Join(base, "release")) // not executable, no ext

	got := probeLayoutDirs(base)
	if len(got) != 1 || filepath.Base(got[0]) != "bin" {
		t.Fatalf("probeLayoutDirs = %v, want just the bin directory", got)
	}
}

func TestProbeLayoutDirsFindsNestedArchives(t *testing.T) {
	base := t.TempDir()
	writeFakeExe(t, filepath.Join(base, "apache-ant-1.10.18", "bin", "ant.bat"))

	got := probeLayoutDirs(base)
	if len(got) != 1 || filepath.Base(got[0]) != "bin" {
		t.Fatalf("probeLayoutDirs = %v, want the nested bin directory", got)
	}
}

func TestProbeLayoutDirsIncludesRootWhenItHasAnExe(t *testing.T) {
	base := t.TempDir()
	writeFakeExe(t, filepath.Join(base, "tool.exe"))

	got := probeLayoutDirs(base)
	if len(got) != 1 || got[0] != base {
		t.Fatalf("probeLayoutDirs = %v, want the app root", got)
	}
}

func TestProbeLayoutDirsIgnoresDirectoriesWithoutExecutables(t *testing.T) {
	base := t.TempDir()
	// A JDK root: lib, conf, include, legal. No executables anywhere at root.
	for _, d := range []string{"lib", "conf", "include", "legal", "jmods"} {
		if err := os.MkdirAll(filepath.Join(base, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if got := probeLayoutDirs(base); len(got) != 0 {
		t.Errorf("probeLayoutDirs = %v, want none", got)
	}
}

func TestProbeLayoutDirsMissingBase(t *testing.T) {
	if got := probeLayoutDirs(filepath.Join(t.TempDir(), "nope")); len(got) != 0 {
		t.Errorf("probeLayoutDirs = %v, want none for a missing base", got)
	}
}
