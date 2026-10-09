package envbuild

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Tijani127/codenv/internal/config"
	"github.com/Tijani127/codenv/internal/store"
)

type Format string

const (
	FormatPowerShell Format = "powershell"
	FormatBash       Format = "bash"
	FormatCmd        Format = "cmd"
)

type Options struct {
	Pure      bool
	Format    Format
	Overrides map[string]string
	EnvFile   map[string]string
	Unset     []string
}

type Env struct {
	PathDirs []string
	Vars     map[string]string
	Unset    []string
	Pure     bool
	BasePath string
}

var preservedInPure = []string{
	"ALLUSERSPROFILE",
	"APPDATA",
	"CommonProgramFiles",
	"CommonProgramFiles(x86)",
	"CommonProgramW6432",
	"COMPUTERNAME",
	"COMSPEC",
	"HOMEDRIVE",
	"HOMEPATH",
	"LOCALAPPDATA",
	"NUMBER_OF_PROCESSORS",
	"OS",
	"PATHEXT",
	"PROCESSOR_ARCHITEW6432",
	"PROCESSOR_ARCHITECTURE",
	"ProgramData",
	"ProgramFiles",
	"ProgramFiles(x86)",
	"ProgramW6432",
	"PSModulePath",
	"PUBLIC",
	"SystemDrive",
	"SystemRoot",
	"TEMP",
	"TMP",
	"USERDOMAIN",
	"USERNAME",
	"USERPROFILE",
	"windir",
}

type Sources struct {
	PathDirs []string
	Config   *config.Config
	Store    *store.Store
	Project  string
	Global   bool
}

func Build(src Sources, cfg *config.Config, opts Options) *Env {
	env := &Env{
		Vars:     map[string]string{},
		Pure:     opts.Pure,
		BasePath: currentPath(opts.Pure),
	}
	env.PathDirs = uniqueDirs(src.PathDirs)

	if cfg != nil {
		for k, v := range cfg.Env {
			env.Vars[k] = v
		}
	}
	for k, v := range opts.EnvFile {
		env.Vars[k] = v
	}
	for k, v := range opts.Overrides {
		env.Vars[k] = v
	}

	env.Vars["CODEENV_SHELL"] = "1"
	if src.Global {
		env.Vars["CODEENV_GLOBAL"] = "1"
	} else {
		env.Vars["CODEENV_PROJECT_DIR"] = src.Project
	}
	if src.Store != nil {
		env.Vars["CODEENV_STORE"] = src.Store.Root()
	}
	if len(env.PathDirs) > 0 {
		env.Vars["CODEENV_PATH_PREFIX"] = strings.Join(env.PathDirs, string(os.PathListSeparator))
	}
	env.Unset = opts.Unset

	if opts.Pure {
		for _, name := range preservedInPure {
			if v := os.Getenv(name); v != "" {
				env.Vars[name] = v
			}
		}
		if v := os.Getenv("HOME"); v != "" {
			env.Vars["HOME"] = v
		}
	}

	return env
}

func currentPath(pure bool) string {
	p := os.Getenv("PATH")
	if pure {
		var keep []string
		for _, entry := range strings.Split(p, string(os.PathListSeparator)) {
			if entry == "" {
				continue
			}
			keep = append(keep, entry)
		}
		return strings.Join(keep, string(os.PathListSeparator))
	}
	return p
}

func uniqueDirs(dirs []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, d := range dirs {
		if d == "" {
			continue
		}
		clean := filepath.Clean(d)
		key := strings.ToLower(clean)
		if seen[key] {
			continue
		}
		info, err := os.Stat(clean)
		if err != nil || !info.IsDir() {
			continue
		}
		seen[key] = true
		out = append(out, clean)
	}
	return out
}

func (e *Env) Path() string {
	sep := string(os.PathListSeparator)
	var parts []string
	parts = append(parts, e.PathDirs...)
	if e.BasePath != "" {
		for _, entry := range strings.Split(e.BasePath, sep) {
			if entry == "" {
				continue
			}
			parts = append(parts, entry)
		}
	}
	seen := map[string]bool{}
	var out []string
	for _, p := range parts {
		k := strings.ToLower(filepath.Clean(p))
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, p)
	}
	return strings.Join(out, sep)
}

func (e *Env) Apply(base []string) []string {
	env := map[string]bool{}
	var order []string
	for _, kv := range base {
		if i := strings.IndexByte(kv, '='); i > 0 {
			key := kv[:i]
			if !env[key] {
				env[key] = true
				order = append(order, key)
			}
		}
	}
	put := func(k, v string) {
		if !env[k] {
			env[k] = true
			order = append(order, k)
		}
		base = append(base, k+"="+v)
	}
	set := func(k, v string) {
		prefix := k + "="
		replaced := false
		for i, kv := range base {
			if strings.HasPrefix(kv, prefix) {
				base[i] = prefix + v
				replaced = true
				break
			}
		}
		if !replaced {
			put(k, v)
		}
	}
	remove := func(k string) {
		prefix := k + "="
		out := base[:0]
		for _, kv := range base {
			if strings.HasPrefix(kv, prefix) {
				continue
			}
			out = append(out, kv)
		}
		base = out
	}

	if e.Pure {
		base = filterEnv(base, preservedSet())
	}

	set("PATH", e.Path())
	for _, k := range sortedKeys(e.Vars) {
		set(k, e.Vars[k])
	}
	for _, k := range e.Unset {
		remove(k)
	}
	return base
}

func filterEnv(base []string, keep map[string]bool) []string {
	out := make([]string, 0, len(base))
	for _, kv := range base {
		i := strings.IndexByte(kv, '=')
		if i <= 0 {
			out = append(out, kv)
			continue
		}
		upper := strings.ToUpper(kv[:i])
		if keep[upper] {
			out = append(out, kv)
		}
	}
	return out
}

func preservedSet() map[string]bool {
	set := map[string]bool{"PATH": true}
	for _, k := range preservedInPure {
		set[strings.ToUpper(k)] = true
	}
	set["HOME"] = true
	return set
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func DetectFormat() Format {
	if v := os.Getenv("CODENV_SHELL_FORMAT"); v != "" {
		return Format(strings.ToLower(v))
	}
	shell := strings.ToLower(os.Getenv("SHELL"))
	if os.Getenv("MSYSTEM") != "" || strings.Contains(shell, "bash") || strings.Contains(shell, "zsh") {
		return FormatBash
	}
	if strings.Contains(shell, "fish") {
		return FormatBash
	}
	return FormatPowerShell
}

func ParseFormat(v string) (Format, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "", "auto", "ps", "powershell", "pwsh":
		return FormatPowerShell, nil
	case "bash", "sh", "shellscript", "fish":
		return FormatBash, nil
	case "cmd", "bat", "batch":
		return FormatCmd, nil
	default:
		return "", fmt.Errorf("unknown shell format %q (want powershell, bash, or cmd)", v)
	}
}

func (e *Env) Script(format Format) string {
	switch format {
	case FormatBash:
		return e.bashScript()
	case FormatCmd:
		return e.cmdScript()
	default:
		return e.powershellScript()
	}
}

// Compact returns a single self-contained PATH export, with no preamble.
// Useful in a profile or CI step where only the variable matters.
func (e *Env) Compact(format Format) string {
	full := e.Path()
	switch format {
	case FormatBash:
		return `export PATH=` + shellQuoteOne(full)
	case FormatCmd:
		return "set \"PATH=" + full + "\""
	default:
		return "$env:PATH = " + quotePS(full)
	}
}

func shellQuoteOne(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (e *Env) powershellScript() string {
	var b strings.Builder
	b.WriteString("$codenvDirs = @(" + quoteList(e.PathDirs, `"`) + ")\n")
	b.WriteString("$codenvSep = ';'\n")
	b.WriteString("$codenvKeep = @($codenvDirs)\n")
	b.WriteString("$codenvRest = @($env:PATH -split $codenvSep | Where-Object { $_ -and ($codenvKeep -notcontains $_) })\n")
	b.WriteString("$env:PATH = (($codenvDirs + $codenvRest) -join $codenvSep)\n")
	for _, k := range sortedKeys(e.Vars) {
		if strings.EqualFold(k, "PATH") {
			continue
		}
		b.WriteString("$env:" + k + " = " + quotePS(e.Vars[k]) + "\n")
	}
	return b.String()
}

func (e *Env) bashScript() string {
	var b strings.Builder
	b.WriteString("codenv_dirs=(" + strings.Join(shellQuote(e.PathDirs), " ") + ")\n")
	b.WriteString("IFS=':' read -ra codenv_rest <<< \"$PATH\"\n")
	b.WriteString("codenv_new=()\n")
	b.WriteString("for d in \"${codenv_dirs[@]}\"; do codenv_new+=(\"$d\"); done\n")
	b.WriteString("for d in \"${codenv_rest[@]}\"; do\n")
	b.WriteString("  [ -z \"$d\" ] && continue\n")
	b.WriteString("  skip=0\n")
	b.WriteString("  for c in \"${codenv_dirs[@]}\"; do [ \"$d\" = \"$c\" ] && skip=1 && break; done\n")
	b.WriteString("  [ \"$skip\" = \"0\" ] && codenv_new+=(\"$d\")\n")
	b.WriteString("done\n")
	b.WriteString("export PATH=\"$(IFS=':'; echo \"${codenv_new[*]}\")\"\n")
	for _, k := range sortedKeys(e.Vars) {
		if strings.EqualFold(k, "PATH") {
			continue
		}
		b.WriteString("export " + k + "=" + shellQuote([]string{e.Vars[k]})[0] + "\n")
	}
	return b.String()
}

func (e *Env) cmdScript() string {
	var b strings.Builder
	b.WriteString("@echo off\r\n")
	b.WriteString("set \"PATH=" + strings.Join(e.PathDirs, ";") + ";%PATH%\"\r\n")
	for _, k := range sortedKeys(e.Vars) {
		if strings.EqualFold(k, "PATH") {
			continue
		}
		b.WriteString("set \"" + k + "=" + e.Vars[k] + "\"\r\n")
	}
	return b.String()
}

func quoteList(items []string, q string) string {
	if len(items) == 0 {
		return ""
	}
	parts := make([]string, 0, len(items))
	for _, i := range items {
		parts = append(parts, q+strings.ReplaceAll(i, q, `\`+q)+q)
	}
	return strings.Join(parts, ", ")
}

func quotePS(v string) string {
	return "'" + strings.ReplaceAll(v, "'", "''") + "'"
}

func shellQuote(items []string) []string {
	out := make([]string, 0, len(items))
	for _, i := range items {
		if i == "" {
			out = append(out, "''")
			continue
		}
		out = append(out, "'"+strings.ReplaceAll(i, "'", `'\''`)+"'")
	}
	return out
}
