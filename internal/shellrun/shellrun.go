package shellrun

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type Kind string

const (
	KindPowerShell Kind = "powershell"
	KindCmd        Kind = "cmd"
	KindBash       Kind = "bash"
)

type Shell struct {
	Path string
	Kind Kind
}

func Detect() Shell {
	if v := os.Getenv("CODEENV_SHELL_BIN"); v != "" {
		return Shell{Path: v, Kind: kindOf(v)}
	}
	for _, candidate := range []string{"pwsh", "powershell"} {
		if p, err := exec.LookPath(candidate); err == nil {
			return Shell{Path: p, Kind: KindPowerShell}
		}
	}
	if p, err := exec.LookPath("cmd"); err == nil {
		return Shell{Path: p, Kind: KindCmd}
	}
	return Shell{Path: "cmd", Kind: KindCmd}
}

func kindOf(path string) Kind {
	base := strings.ToLower(filepath.Base(path))
	switch {
	case strings.Contains(base, "pwsh") || strings.Contains(base, "powershell"):
		return KindPowerShell
	case strings.Contains(base, "bash"):
		return KindBash
	default:
		return KindCmd
	}
}

func (s Shell) ScriptFlags() []string {
	switch s.Kind {
	case KindPowerShell:
		return []string{"-NoProfile", "-Command"}
	case KindBash:
		return []string{"-c"}
	default:
		return []string{"/c"}
	}
}

func EnvMap(env []string) map[string]string {
	out := map[string]string{}
	for _, kv := range env {
		if i := strings.IndexByte(kv, '='); i > 0 {
			out[kv[:i]] = kv[i+1:]
		}
	}
	return out
}

func LookPath(env []string, name string) (string, error) {
	return LookPathIn(env, "", name)
}

func LookPathIn(env []string, dir, name string) (string, error) {
	vars := EnvMap(env)
	if strings.ContainsAny(name, `/\`) {
		abs := name
		if !filepath.IsAbs(abs) {
			base := dir
			if base == "" {
				base = "."
			}
			joined, err := filepath.Abs(filepath.Join(base, abs))
			if err != nil {
				return "", fmt.Errorf("%s: not found", name)
			}
			abs = joined
		}
		exts := extensionsFor(name, vars["PATHEXT"])
		for _, ext := range exts {
			if isExecutable(abs+ext, vars["PATHEXT"]) {
				return abs + ext, nil
			}
		}
		return "", fmt.Errorf("%s: not found", name)
	}
	sep := string(os.PathListSeparator)
	exts := extensionsFor(name, vars["PATHEXT"])
	for _, dir := range strings.Split(vars["PATH"], sep) {
		if dir == "" {
			continue
		}
		for _, ext := range exts {
			candidate := filepath.Join(dir, name+ext)
			if isExecutable(candidate, vars["PATHEXT"]) {
				return candidate, nil
			}
		}
	}
	return "", fmt.Errorf("%s: command not found in the codenv environment", name)
}

func extensionsFor(name, pathext string) []string {
	var known []string
	for _, e := range strings.Split(pathext, ";") {
		e = strings.TrimSpace(e)
		if e != "" {
			known = append(known, strings.ToLower(e))
		}
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext == "" {
		return append([]string{""}, known...)
	}
	for _, e := range known {
		if ext == e {
			return []string{""}
		}
	}
	return []string{""}
}

func isExecutable(path, pathext string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == ".exe" || ext == ".com" || ext == ".bat" || ext == ".cmd" {
		return true
	}
	for _, e := range strings.Split(pathext, ";") {
		if e != "" && ext == strings.ToLower(strings.TrimSpace(e)) {
			return true
		}
	}
	return false
}

func Exec(env []string, dir, name string, args []string, stdin, stdout, stderr *os.File) (int, error) {
	bin, err := LookPathIn(env, dir, name)
	if err != nil {
		return 127, err
	}
	shell := Detect()
	if strings.EqualFold(filepath.Ext(bin), ".bat") || strings.EqualFold(filepath.Ext(bin), ".cmd") {
		argv := append([]string{}, shell.ScriptFlags()...)
		argv = append(argv, bin)
		argv = append(argv, args...)
		cmd := exec.Command(shell.Path, argv...)
		cmd.Dir = dir
		cmd.Env = env
		cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
		return run(cmd)
	}
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	return run(cmd)
}

func ScriptCommand(script string) *exec.Cmd {
	shell := Detect()
	var cmd *exec.Cmd
	switch shell.Kind {
	case KindPowerShell:
		cmd = exec.Command(shell.Path, "-NoProfile", "-Command", script)
	case KindBash:
		cmd = exec.Command(shell.Path, "-c", script)
	default:
		cmd = exec.Command(shell.Path, "/c", script)
	}
	return cmd
}

func Script(env []string, dir, script string, extra []string, stdin, stdout, stderr *os.File) (int, error) {
	line := script
	if len(extra) > 0 {
		line = script + " " + joinArgs(extra)
	}
	cmd := ScriptCommand(line)
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = stdin, stdout, stderr
	return run(cmd)
}

func Interactive(env []string, dir string, initSnippet string) error {
	shell := Detect()
	var cmd *exec.Cmd
	switch shell.Kind {
	case KindPowerShell:
		args := []string{"-NoLogo", "-NoExit"}
		if initSnippet != "" {
			args = append(args, "-Command", initSnippet)
		}
		cmd = exec.Command(shell.Path, args...)
	case KindBash:
		args := []string{"-i"}
		if initSnippet != "" {
			args = append(args, "-c", initSnippet)
		}
		cmd = exec.Command(shell.Path, args...)
	default:
		cmd = exec.Command(shell.Path)
		if initSnippet != "" {
			cmd = exec.Command(shell.Path, "/k", initSnippet)
		}
	}
	cmd.Dir = dir
	cmd.Env = env
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if asExit(err, &ee) {
			return &ExitError{Code: ee.ExitCode()}
		}
		return err
	}
	return nil
}

type ExitError struct {
	Code int
}

func (e *ExitError) Error() string {
	return fmt.Sprintf("process exited with status %d", e.Code)
}

func run(cmd *exec.Cmd) (int, error) {
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if asExit(err, &ee) {
			return ee.ExitCode(), nil
		}
		return 1, err
	}
	return 0, nil
}

func asExit(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}

func joinArgs(args []string) string {
	parts := make([]string, 0, len(args))
	for _, a := range args {
		parts = append(parts, quoteArg(a))
	}
	return strings.Join(parts, " ")
}

func quoteArg(a string) string {
	if a == "" {
		return `""`
	}
	if !strings.ContainsAny(a, " \t\n\"'|&<>^()%!,;") {
		return a
	}
	return `"` + strings.ReplaceAll(a, `"`, `\"`) + `"`
}
