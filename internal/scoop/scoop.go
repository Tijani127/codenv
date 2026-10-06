package scoop

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Tijani127/codenv/internal/store"
	"github.com/Tijani127/codenv/internal/ui"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[a-zA-Z]`)

const (
	currentLinkName = "current"
	persistDirName  = "persist"
)

var noisePatterns = []*regexp.Regexp{
	regexp.MustCompile(`^WARN\s+Scoop is out of date\.?$`),
	regexp.MustCompile(`^Cannot find path '.*shims' because it does not exist\.?$`),
	regexp.MustCompile(`^Updating (Scoop|Buckets)\.{3}$`),
	regexp.MustCompile(`^Adding .*\\shims to your path\.$`),
	regexp.MustCompile(`^Adding .*\\shims to the (global )?path\.$`),
}

type Runner struct {
	Store *store.Store
	Shell string
}

func New(s *store.Store) *Runner {
	return &Runner{Store: s, Shell: detectShell()}
}

func detectShell() string {
	if v := os.Getenv("CODEENV_POWERSHELL"); v != "" {
		return v
	}
	if p, err := exec.LookPath("pwsh"); err == nil {
		return p
	}
	if p, err := exec.LookPath("powershell"); err == nil {
		return p
	}
	return "powershell"
}

func (r *Runner) invoke(args []string, stream bool) (string, error) {
	payload, err := json.Marshal(args)
	if err != nil {
		return "", err
	}

	dir, err := os.MkdirTemp("", "codenv-scoop-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)

	scriptPath := filepath.Join(dir, "run.ps1")
	if err := os.WriteFile(scriptPath, []byte(wrapScript), 0o644); err != nil {
		return "", err
	}

	cmd := exec.Command(r.Shell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", scriptPath)
	cmd.Env = r.childEnv(base64.StdEncoding.EncodeToString(payload))

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", err
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Start(); err != nil {
		return "", err
	}

	var captured strings.Builder
	done := make(chan struct{})
	go func() {
		defer close(done)
		var pending strings.Builder
		chunk := make([]byte, 4096)
		for {
			n, err := stdout.Read(chunk)
			if n > 0 {
				pending.Write(chunk[:n])
				for {
					s := pending.String()
					idx := strings.IndexAny(s, "\r\n")
					if idx < 0 {
						break
					}
					line := s[:idx]
					pending.Reset()
					pending.WriteString(s[idx+1:])
					emitLine(&captured, line, stream)
				}
			}
			if err != nil {
				break
			}
		}
		if rest := pending.String(); rest != "" {
			emitLine(&captured, rest, stream)
		}
	}()

	waitErr := cmd.Wait()
	<-done

	exit := 0
	if waitErr != nil {
		var ee *exec.ExitError
		if asExitError(waitErr, &ee) {
			exit = ee.ExitCode()
		} else {
			exit = 1
		}
	}

	combined := captured.String() + scrub(stderr.String())
	if exit != 0 {
		return combined, &Error{Args: args, ExitCode: exit, Output: combined, Streamed: stream}
	}
	return combined, nil
}

func emitLine(captured *strings.Builder, raw string, stream bool) {
	line := strings.TrimRight(ansiRe.ReplaceAllString(raw, ""), " \t")
	if isNoise(line) {
		return
	}
	captured.WriteString(line)
	captured.WriteString("\n")
	if stream {
		ui.Println("%s", line)
	}
}

func scrub(s string) string {
	var out strings.Builder
	for _, line := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		clean := strings.TrimRight(ansiRe.ReplaceAllString(line, ""), " \t")
		if clean == "" || isNoise(clean) {
			continue
		}
		out.WriteString(clean)
		out.WriteString("\n")
	}
	return out.String()
}

func isNoise(line string) bool {
	if line == "" {
		return true
	}
	for _, p := range noisePatterns {
		if p.MatchString(line) {
			return true
		}
	}
	return false
}

func asExitError(err error, target **exec.ExitError) bool {
	if ee, ok := err.(*exec.ExitError); ok {
		*target = ee
		return true
	}
	return false
}

func (r *Runner) childEnv(argsB64 string) []string {
	env := filterEnv(os.Environ(), "SCOOP", "SCOOP_GLOBAL", "SCOOP_CACHE", "SCOOP_PATH", "CODEENV_SCOOP_ARGS", "CODEENV_SCOOP_SCRIPT")
	env = append(env,
		"SCOOP="+r.Store.Root(),
		"SCOOP_GLOBAL="+r.Store.Root(),
		"SCOOP_CACHE="+r.Store.CacheDir(),
		"CODEENV_SCOOP_ARGS="+argsB64,
		"CODEENV_SCOOP_SCRIPT="+r.Store.ScoopScript(),
		"GIT_TERMINAL_PROMPT=0",
	)
	return env
}

func filterEnv(env []string, drop ...string) []string {
	skip := map[string]bool{}
	for _, d := range drop {
		skip[strings.ToUpper(d)] = true
	}
	out := make([]string, 0, len(env))
	for _, kv := range env {
		name := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			name = kv[:i]
		}
		if skip[strings.ToUpper(name)] {
			continue
		}
		out = append(out, kv)
	}
	return out
}

func (r *Runner) Exec(args ...string) error {
	_, err := r.invoke(args, true)
	if err != nil {
		return err
	}
	return nil
}

func (r *Runner) Output(args ...string) (string, error) {
	out, err := r.invoke(args, false)
	if err != nil {
		return out, err
	}
	return Clean(out), nil
}

func Clean(s string) string {
	var lines []string
	for _, raw := range strings.Split(strings.ReplaceAll(s, "\r\n", "\n"), "\n") {
		line := strings.TrimRight(ansiRe.ReplaceAllString(raw, ""), " \t")
		if line == "" {
			continue
		}
		skip := false
		for _, p := range noisePatterns {
			if p.MatchString(line) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

type Error struct {
	Args     []string
	ExitCode int
	Output   string
	Streamed bool
}

func (e *Error) Error() string {
	head := fmt.Sprintf("scoop %s failed (exit %d)", strings.Join(e.Args, " "), e.ExitCode)
	if e.Streamed {
		return head
	}
	clean := Clean(e.Output)
	if clean == "" {
		return head
	}
	return head + "\n" + indent(clean, "  ")
}

func (r *Runner) Install(specs ...string) error {
	if len(specs) == 0 {
		return nil
	}
	before := snapshotShortcuts()
	args := append([]string{"install"}, specs...)
	args = append(args, "-u")
	err := r.Exec(args...)
	r.Store.ResetBuckets()
	if err != nil {
		return err
	}
	r.Harvest()
	removeNewShortcuts(before)
	return nil
}

func (r *Runner) Update(apps []string) error {
	args := []string{"update", "-u"}
	args = append(args, apps...)
	err := r.Exec(args...)
	r.Store.ResetBuckets()
	if err != nil {
		return err
	}
	r.Harvest()
	return nil
}

func (r *Runner) Uninstall(apps []string) error {
	for _, app := range apps {
		before := snapshotShortcuts()
		if err := r.Exec("uninstall", app); err != nil {
			return err
		}
		removeNewShortcuts(before)
	}
	return nil
}

func (r *Runner) Harvest() error {
	harvested, err := r.Harvested()
	if err != nil {
		return err
	}
	if len(harvested) == 0 {
		return nil
	}
	ix, err := store.LoadIndex(r.Store)
	if err != nil {
		return err
	}
	for app, byVersion := range harvested {
		for version, dirs := range byVersion {
			ix.Set(app, version, dirs)
		}
	}
	if err := ix.Save(); err != nil {
		return err
	}
	return r.purgeShims()
}

func (r *Runner) Harvested() (map[string]map[string][]string, error) {
	out := map[string]map[string][]string{}
	shims := r.Store.ShimsDir()
	entries, err := os.ReadDir(shims)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || strings.EqualFold(filepath.Ext(e.Name()), ".exe") {
			continue
		}
		full := filepath.Join(shims, e.Name())
		data, err := os.ReadFile(full)
		if err != nil || len(data) == 0 {
			continue
		}
		first := strings.SplitN(string(data), "\n", 2)[0]
		target, ok := parseShimTarget(first)
		if !ok {
			continue
		}
		app, version, dir := r.classify(target)
		if app == "" || dir == "" {
			continue
		}
		if out[app] == nil {
			out[app] = map[string][]string{}
		}
		out[app][version] = append(out[app][version], dir)
	}
	return out, nil
}

func parseShimTarget(line string) (string, bool) {
	line = strings.TrimSpace(strings.TrimPrefix(line, "@rem"))
	switch {
	case strings.HasPrefix(line, "path ="):
		v := strings.TrimSpace(strings.TrimPrefix(line, "path ="))
		v = strings.Trim(v, `"`)
		if v != "" {
			return v, true
		}
	case strings.HasPrefix(line, "#") && len(line) > 2 && isDrivePath(line[2:]):
		return strings.TrimSpace(line[1:]), true
	}
	return "", false
}

func isDrivePath(s string) bool {
	s = strings.TrimSpace(s)
	return len(s) >= 3 && (s[1] == ':') && (s[2] == '\\' || s[2] == '/')
}

func (r *Runner) classify(target string) (app, version, dir string) {
	clean := filepath.Clean(target)
	rel, err := filepath.Rel(r.Store.Root(), clean)
	if err != nil {
		return "", "", ""
	}
	parts := strings.Split(rel, string(filepath.Separator))
	if len(parts) >= 3 && parts[0] == "apps" {
		app = parts[1]
		seg := parts[2]
		if seg == currentLinkName {
			if v, err := r.Store.ResolveCurrentVersion(app); err == nil && v != "" {
				seg = v
			}
		}
		full := filepath.Join(r.Store.AppsDir(), app, seg, filepath.Join(parts[3:]...))
		return app, seg, filepath.Dir(full)
	}
	if len(parts) >= 2 && parts[0] == persistDirName {
		return parts[1], persistDirName, filepath.Dir(clean)
	}
	return "", "", ""
}

func (r *Runner) purgeShims() error {
	dir := r.Store.ShimsDir()
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return os.RemoveAll(dir)
}

func shortcutDirs() []string {
	var dirs []string
	if v := os.Getenv("APPDATA"); v != "" {
		dirs = append(dirs, filepath.Join(v, "Microsoft", "Windows", "Start Menu", "Programs", "Scoop Apps"))
	}
	if v := os.Getenv("ProgramData"); v != "" {
		dirs = append(dirs, filepath.Join(v, "Microsoft", "Windows", "Start Menu", "Programs", "Scoop Apps"))
	}
	return dirs
}

func snapshotShortcuts() map[string]map[string]bool {
	snap := map[string]map[string]bool{}
	for _, d := range shortcutDirs() {
		set := map[string]bool{}
		_ = filepath.WalkDir(d, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil
			}
			if strings.EqualFold(filepath.Ext(entry.Name()), ".lnk") {
				rel, rerr := filepath.Rel(d, path)
				if rerr == nil {
					set[filepath.ToSlash(rel)] = true
				}
			}
			return nil
		})
		if len(set) > 0 {
			snap[d] = set
		}
	}
	return snap
}

func removeNewShortcuts(snap map[string]map[string]bool) {
	for dir, before := range snap {
		var fresh []string
		_ = filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return nil
			}
			if !strings.EqualFold(filepath.Ext(entry.Name()), ".lnk") {
				return nil
			}
			rel, rerr := filepath.Rel(dir, path)
			if rerr != nil {
				return nil
			}
			if !before[filepath.ToSlash(rel)] {
				fresh = append(fresh, path)
			}
			return nil
		})
		for _, path := range fresh {
			_ = os.Remove(path)
		}
		if len(fresh) > 0 {
			pruneEmptyDirs(dir)
		}
	}
}

func pruneEmptyDirs(root string) {
	var dirs []string
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err == nil && entry.IsDir() && path != root {
			dirs = append(dirs, path)
		}
		return nil
	})
	for i := len(dirs) - 1; i >= 0; i-- {
		_ = os.Remove(dirs[i])
	}
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}
