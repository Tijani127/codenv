package envbuild

import (
	"os"
	"strings"
	"testing"
)

func TestCompactPerFormat(t *testing.T) {
	base := os.Getenv("PATH")
	if base == "" {
		t.Skip("PATH is empty")
	}
	prefix := `C:\codenv\test\apps\ripgrep\15.2.0`
	e := &Env{
		PathDirs: []string{prefix},
		BasePath: base,
	}

	cases := []struct {
		format Format
		want   string
	}{
		{FormatPowerShell, "$env:PATH = '"},
		{FormatBash, "export PATH='"},
		{FormatCmd, `set "PATH=`},
	}
	for _, tc := range cases {
		got := e.Compact(tc.format)
		if !strings.HasPrefix(got, tc.want) {
			t.Errorf("Compact(%s) = %.40q, want prefix %q", tc.format, got, tc.want)
		}
		if strings.Contains(got, "\n") {
			t.Errorf("Compact(%s) must be a single line", tc.format)
		}
		if !strings.Contains(got, prefix) {
			t.Errorf("Compact(%s) dropped the prefix: %.80q", tc.format, got)
		}
		if !strings.Contains(got, "ripgrep") {
			t.Errorf("Compact(%s) lost the package dir", tc.format)
		}
	}
}

func TestCompactDoesNotEscapePathSeparator(t *testing.T) {
	// On Windows the separator is ';', which must survive inside the quotes.
	if os.PathListSeparator != ';' {
		t.Skip("not a Windows separator test")
	}
	e := &Env{PathDirs: []string{`C:\a\b`}, BasePath: `C:\Windows\System32`}
	got := e.Compact(FormatPowerShell)
	if !strings.Contains(got, `C:\a\b;C:\Windows\System32`) {
		t.Errorf("separator lost or reordered: %q", got)
	}
}

func TestCompactEmptyPathIsQuoted(t *testing.T) {
	e := &Env{PathDirs: nil, BasePath: ""}
	if got := e.Compact(FormatBash); got != "export PATH=''" {
		t.Errorf("Compact with empty PATH = %q", got)
	}
	if got := e.Compact(FormatPowerShell); got != "$env:PATH = ''" {
		t.Errorf("Compact with empty PATH = %q", got)
	}
}

func TestShellQuoteOneHandlesQuotes(t *testing.T) {
	if got := shellQuoteOne(`it's`); got != `'it'\''s'` {
		t.Errorf("shellQuoteOne = %q", got)
	}
	if got := shellQuoteOne(""); got != "''" {
		t.Errorf("shellQuoteOne empty = %q", got)
	}
}

func TestUniqueDirsRejectsFiles(t *testing.T) {
	dir := t.TempDir()
	file := dir + "/afile"
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := uniqueDirs([]string{dir, file, dir})
	if len(got) != 1 || got[0] != dir {
		t.Errorf("uniqueDirs = %v, want just the directory", got)
	}
}
