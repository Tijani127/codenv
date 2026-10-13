package envbuild

import (
	"strings"
	"testing"
)

func TestParseFormatAcceptsUnixShells(t *testing.T) {
	// sh, zsh, fish, ksh and dash all run the same POSIX script unchanged.
	for _, name := range []string{"bash", "sh", "zsh", "fish", "ksh", "dash", "SH", "Zsh"} {
		got, err := ParseFormat(name)
		if err != nil {
			t.Errorf("ParseFormat(%q) errored: %v", name, err)
			continue
		}
		if got != FormatBash {
			t.Errorf("ParseFormat(%q) = %q, want bash", name, got)
		}
	}
	for _, name := range []string{"powershell", "pwsh", "cmd", "bat"} {
		if _, err := ParseFormat(name); err != nil {
			t.Errorf("ParseFormat(%q) errored: %v", name, err)
		}
	}
}

func TestParseFormatErrorNamesTheRealOptions(t *testing.T) {
	_, err := ParseFormat("tcsh")
	if err == nil {
		t.Fatal("expected an error")
	}
	msg := err.Error()
	for _, want := range []string{"powershell", "bash", "zsh", "fish", "cmd"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q should mention %q so the user knows it is valid", msg, want)
		}
	}
}

func TestDetectFormatFromShellEnv(t *testing.T) {
	cases := map[string]Format{
		"/bin/bash":     FormatBash,
		"/usr/bin/zsh":  FormatBash,
		"/usr/bin/fish": FormatBash,
		"/bin/sh":       FormatBash,
		"/bin/dash":     FormatBash,
		"/bin/ksh":      FormatBash,
		"":              FormatPowerShell,
	}
	for shell, want := range cases {
		t.Setenv("CODEENV_SHELL_FORMAT", "")
		t.Setenv("MSYSTEM", "")
		t.Setenv("SHELL", shell)
		if got := DetectFormat(); got != want {
			t.Errorf("SHELL=%q -> %q, want %q", shell, got, want)
		}
	}
}

func TestDetectFormatPrefersExplicitEnvVar(t *testing.T) {
	t.Setenv("CODEENV_SHELL_FORMAT", "cmd")
	t.Setenv("SHELL", "/bin/bash")
	if got := DetectFormat(); got != FormatCmd {
		t.Errorf("CODEENV_SHELL_FORMAT=cmd -> %q, want cmd", got)
	}
	// An unrecognised value must fall back to detection rather than erroring
	// or silently choosing something arbitrary.
	t.Setenv("CODEENV_SHELL_FORMAT", "nonsense")
	t.Setenv("SHELL", "/bin/zsh")
	if got := DetectFormat(); got != FormatBash {
		t.Errorf("bad value should fall through to SHELL detection, got %q", got)
	}
}

func TestBashScriptUsesThePlatformSeparator(t *testing.T) {
	e := &Env{
		PathDirs: []string{"/opt/tools/bin", "/opt/tools/lib"},
		Vars:     map[string]string{},
	}
	script := e.bashScript()
	// The separator must come from the platform, not a hardcoded colon. On
	// Windows PATH is joined with ';', and a script that assumed ':' silently
	// failed to split it.
	want := pathSep()
	if !strings.Contains(script, "IFS='"+want+"' read") {
		t.Errorf("bash script does not use the platform separator %q:\n%s", want, script)
	}
	if !strings.Contains(script, "IFS='"+want+"'; echo") {
		t.Errorf("bash export does not use the platform separator %q:\n%s", want, script)
	}
}

func TestScriptsAvoidBashOnlySyntax(t *testing.T) {
	// The output is shared with sh, zsh, ksh, dash and fish, so it must not
	// rely on bash extensions. Arrays and $() are POSIX or near enough;
	// [[ ]], local, source and arrays of arrays are not.
	e := &Env{
		PathDirs: []string{"/opt/tools/bin"},
		Vars:     map[string]string{"FOO": "bar"},
	}
	script := e.bashScript()
	for _, bad := range []string{"[[", "]]", "source ", "local ", "declare ", "mapfile"} {
		if strings.Contains(script, bad) {
			t.Errorf("script uses non-POSIX construct %q:\n%s", bad, script)
		}
	}
}

func TestPowerShellAndCmdUseThePlatformSeparator(t *testing.T) {
	e := &Env{
		PathDirs: []string{`C:\tools\bin`, `C:\tools\lib`},
		Vars:     map[string]string{},
	}
	ps := e.powershellScript()
	want := pathSep()
	if !strings.Contains(ps, "$codenvSep = '"+want+"'") {
		t.Errorf("powershell script separator mismatch:\n%s", ps)
	}
	cmd := e.cmdScript()
	if !strings.Contains(cmd, strings.Join(e.PathDirs, want)) {
		t.Errorf("cmd script separator mismatch:\n%s", cmd)
	}
}
