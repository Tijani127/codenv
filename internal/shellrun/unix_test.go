package shellrun

import "testing"

// TestKindOfRecognisesUnixShells guards a defect where only pwsh, powershell
// and bash were recognised, so zsh, fish, sh and ksh silently fell through to
// KindCmd and were launched with `/c`, which no unix shell accepts.
func TestKindOfRecognisesUnixShells(t *testing.T) {
	cases := map[string]Kind{
		"/bin/zsh":                              KindPosix,
		"/usr/bin/zsh":                          KindPosix,
		"zsh":                                   KindPosix,
		"/usr/bin/fish":                         KindPosix,
		"/bin/sh":                               KindPosix,
		"sh":                                    KindPosix,
		"/bin/dash":                             KindPosix,
		"/usr/bin/ksh":                          KindPosix,
		"sh.exe":                                KindPosix,
		"/bin/bash":                             KindBash,
		"C:\\Program Files\\Git\\bin\\bash.exe": KindBash,
		"C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe": KindPowerShell,
		"C:\\Program Files\\PowerShell\\7\\pwsh.exe":                     KindPowerShell,
		"C:\\Windows\\System32\\cmd.exe":                                 KindCmd,
	}
	for path, want := range cases {
		if got := kindOf(path); got != want {
			t.Errorf("kindOf(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestScriptFlagsForUnixShells(t *testing.T) {
	// zsh, fish, sh and ksh all take -c. Getting this wrong is what made
	// 'codenv shell' fail on those shells.
	for _, k := range []Kind{KindBash, KindPosix} {
		got := (Shell{Kind: k}).ScriptFlags()
		if len(got) != 1 || got[0] != "-c" {
			t.Errorf("ScriptFlags(%q) = %v, want [-c]", k, got)
		}
	}
	if got := (Shell{Kind: KindCmd}).ScriptFlags(); len(got) != 1 || got[0] != "/c" {
		t.Errorf("cmd ScriptFlags = %v, want [/c]", got)
	}
	ps := (Shell{Kind: KindPowerShell}).ScriptFlags()
	if len(ps) != 2 || ps[0] != "-NoProfile" || ps[1] != "-Command" {
		t.Errorf("powershell ScriptFlags = %v", ps)
	}
}

// TestKindOfDoesNotMatchSubstrings guards against a naive Contains: a program
// called "bashful" or "powershell-helper" should not be treated as that shell.
func TestKindOfDoesNotMatchUnrelatedNames(t *testing.T) {
	if got := kindOf("/usr/bin/bashtop"); got == KindBash {
		t.Error("bashtop should not be detected as bash")
	}
	if got := kindOf("C:\\tools\\resharper.exe"); got == KindPowerShell {
		t.Error("resharper should not be detected as powershell")
	}
}
