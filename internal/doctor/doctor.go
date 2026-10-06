package doctor

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/Tijani127/codenv/internal/config"
	"github.com/Tijani127/codenv/internal/store"
)

type Level string

const (
	OK   Level = "ok"
	Warn Level = "warn"
	Fail Level = "fail"
)

type Check struct {
	Name   string
	Level  Level
	Detail string
	Hint   string
}

type Report struct {
	Checks  []Check
	Fixable bool
}

func (r *Report) Add(c Check) {
	r.Checks = append(r.Checks, c)
}

func (r *Report) Count(level Level) int {
	n := 0
	for _, c := range r.Checks {
		if c.Level == level {
			n++
		}
	}
	return n
}

func (r *Report) Worst() Level {
	if r.Count(Fail) > 0 {
		return Fail
	}
	if r.Count(Warn) > 0 {
		return Warn
	}
	return OK
}

type Options struct {
	Store      *store.Store
	Config     *config.Config
	Lock       *config.Lock
	ProjectDir string
	Fix        bool
}

func Run(opts Options) *Report {
	r := &Report{}
	checkOS(r)
	checkGit(r)
	checkStore(r, opts)
	checkShims(r, opts.Store)
	checkBuckets(r, opts.Store)
	checkIndex(r, opts.Store, opts.Lock)
	checkPackages(r, opts.Store, opts.Lock)
	checkSecrets(r, opts)
	checkPATHShadowing(r, opts.Store, opts.Lock)
	return r
}

type FixResult struct {
	PrunedIndex  int
	ResetBuckets []string
	RemovedShims int
}

func (f *FixResult) Any() bool {
	return f.PrunedIndex > 0 || len(f.ResetBuckets) > 0 || f.RemovedShims > 0
}

// Fix repairs the problems codenv can safely repair on its own: index entries for
// packages that are no longer on disk, bucket manifests left dirty by autoupdate,
// and a stray shims directory. It never installs or removes packages.
func Fix(opts Options) (*FixResult, error) {
	res := &FixResult{}
	if opts.Store == nil {
		return res, nil
	}
	st := opts.Store

	if _, err := os.Stat(st.ShimsDir()); err == nil {
		entries, rerr := os.ReadDir(st.ShimsDir())
		if rerr == nil {
			for _, e := range entries {
				if !e.IsDir() {
					res.RemovedShims++
				}
			}
		}
		if err := os.RemoveAll(st.ShimsDir()); err != nil {
			return res, fmt.Errorf("removing shims: %w", err)
		}
	}

	for _, name := range st.Buckets() {
		dir := filepath.Join(st.BucketsDir(), name)
		out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
		if err != nil || strings.TrimSpace(string(out)) == "" {
			continue
		}
		cmd := exec.Command("git", "-C", dir, "checkout", "--", ".")
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if err := cmd.Run(); err != nil {
			return res, fmt.Errorf("resetting bucket %s: %w", name, err)
		}
		res.ResetBuckets = append(res.ResetBuckets, name)
	}

	ix, err := store.LoadIndex(st)
	if err != nil {
		return res, nil
	}
	changed := false
	for app, byVersion := range ix.Apps {
		for version, dirs := range byVersion {
			if !st.HasVersion(app, version) {
				ix.Remove(app, version)
				res.PrunedIndex++
				changed = true
				continue
			}
			live := 0
			for _, d := range dirs {
				if info, err := os.Stat(d); err == nil && info.IsDir() {
					live++
				}
			}
			if live == 0 {
				ix.Remove(app, version)
				res.PrunedIndex++
				changed = true
			}
		}
	}
	if changed {
		if err := ix.Save(); err != nil {
			return res, fmt.Errorf("saving index: %w", err)
		}
	}
	return res, nil
}

func checkOS(r *Report) {
	if runtime.GOOS != "windows" {
		r.Add(Check{
			Name:   "operating system",
			Level:  Fail,
			Detail: fmt.Sprintf("running on %s", runtime.GOOS),
			Hint:   "codenv depends on Scoop, which only works on Windows",
		})
		return
	}
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x86_64"
	case "arm64":
		arch = "aarch64"
	}
	r.Add(Check{
		Name:   "operating system",
		Level:  OK,
		Detail: fmt.Sprintf("windows/%s", arch),
	})
}

func checkGit(r *Report) {
	path, err := exec.LookPath("git")
	if err != nil {
		r.Add(Check{
			Name:   "git",
			Level:  Fail,
			Detail: "not found on PATH",
			Hint:   "Scoop is a git checkout and codenv clones its buckets; install git first",
		})
		return
	}
	out, err := exec.Command(path, "--version").Output()
	if err != nil {
		r.Add(Check{Name: "git", Level: Warn, Detail: path, Hint: "found, but 'git --version' failed"})
		return
	}
	version := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	r.Add(Check{Name: "git", Level: OK, Detail: version})
}

func checkStore(r *Report, opts Options) {
	if opts.Store == nil {
		r.Add(Check{Name: "store", Level: Warn, Detail: "no store selected"})
		return
	}
	root := opts.Store.Root()
	info, err := os.Stat(root)
	if err != nil {
		r.Add(Check{
			Name:   "store",
			Level:  OK,
			Detail: fmt.Sprintf("%s (not created yet)", root),
		})
		return
	}
	if !info.IsDir() {
		r.Add(Check{Name: "store", Level: Fail, Detail: root + " exists but is not a directory"})
		return
	}
	probe := filepath.Join(root, ".codenv-write-test")
	if err := os.WriteFile(probe, []byte("ok"), 0o644); err != nil {
		r.Add(Check{
			Name:   "store",
			Level:  Fail,
			Detail: fmt.Sprintf("%s is not writable: %s", root, err),
			Hint:   "check permissions, or set CODEENV_HOME to a writable location",
		})
		return
	}
	_ = os.Remove(probe)

	if _, err := os.Stat(opts.Store.ScoopScript()); err != nil {
		detail := fmt.Sprintf("%s (scoop not bootstrapped yet)", root)
		r.Add(Check{Name: "store", Level: OK, Detail: detail})
		return
	}
	r.Add(Check{
		Name:   "store",
		Level:  OK,
		Detail: fmt.Sprintf("%s (%s)", root, opts.Store.Mode()),
	})
}

func checkShims(r *Report, st *store.Store) {
	if st == nil {
		return
	}
	dir := st.ShimsDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		r.Add(Check{
			Name:   "shims",
			Level:  OK,
			Detail: "absent, as expected: packages are on PATH directly",
		})
		return
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() {
			count++
		}
	}
	if count == 0 {
		r.Add(Check{Name: "shims", Level: OK, Detail: "present but empty"})
		return
	}
	r.Add(Check{
		Name:   "shims",
		Level:  Warn,
		Detail: fmt.Sprintf("%d file(s) in %s", count, dir),
		Hint:   "codenv deletes the shims directory after each install; run 'codenv install' to clean it",
	})
}

func checkBuckets(r *Report, st *store.Store) {
	if st == nil {
		return
	}
	names := st.Buckets()
	if len(names) == 0 {
		r.Add(Check{
			Name:   "buckets",
			Level:  Warn,
			Detail: "none cloned yet",
			Hint:   "run 'codenv install' to clone the default buckets",
		})
		return
	}
	var dirty []string
	for _, name := range names {
		dir := filepath.Join(st.BucketsDir(), name)
		out, err := exec.Command("git", "-C", dir, "status", "--porcelain").Output()
		if err != nil {
			continue
		}
		if strings.TrimSpace(string(out)) != "" {
			dirty = append(dirty, name)
		}
	}
	sort.Strings(names)
	detail := strings.Join(names, ", ")
	if len(dirty) > 0 {
		r.Add(Check{
			Name:   "buckets",
			Level:  Warn,
			Detail: fmt.Sprintf("%s (modified: %s)", detail, strings.Join(dirty, ", ")),
			Hint:   "Scoop's autoupdate rewrites manifests when resolving historical versions; 'codenv install' resets them",
		})
		return
	}
	r.Add(Check{Name: "buckets", Level: OK, Detail: detail})
}

func checkIndex(r *Report, st *store.Store, lock *config.Lock) {
	if st == nil {
		return
	}
	ix, err := store.LoadIndex(st)
	if err != nil {
		r.Add(Check{Name: "package index", Level: Warn, Detail: "unreadable: " + err.Error()})
		return
	}
	var stale []string
	for app, byVersion := range ix.Apps {
		for version, dirs := range byVersion {
			if !st.HasVersion(app, version) {
				stale = append(stale, fmt.Sprintf("%s@%s", app, version))
				continue
			}
			live := 0
			for _, d := range dirs {
				if info, err := os.Stat(d); err == nil && info.IsDir() {
					live++
				}
			}
			if live == 0 {
				stale = append(stale, fmt.Sprintf("%s@%s (no bin dir)", app, version))
			}
		}
	}
	sort.Strings(stale)
	if len(stale) > 0 {
		r.Add(Check{
			Name:   "package index",
			Level:  Warn,
			Detail: fmt.Sprintf("%d indexed, %d stale: %s", len(ix.Apps), len(stale), strings.Join(truncate(stale, 5), ", ")),
			Hint:   "'codenv install' rebuilds the index for packages you still use",
		})
		return
	}
	r.Add(Check{
		Name:   "package index",
		Level:  OK,
		Detail: fmt.Sprintf("%d package(s) indexed", len(ix.Apps)),
	})
}

func checkPackages(r *Report, st *store.Store, lock *config.Lock) {
	if lock == nil || len(lock.Top) == 0 {
		return
	}
	var missing []string
	for _, key := range lock.Top {
		pkg, ok := lock.Resolved[key]
		if !ok {
			missing = append(missing, key)
			continue
		}
		if st != nil && !st.HasVersion(pkg.Name, pkg.Version) {
			missing = append(missing, key)
		}
	}
	if len(missing) > 0 {
		r.Add(Check{
			Name:   "locked packages",
			Level:  Warn,
			Detail: fmt.Sprintf("not installed: %s", strings.Join(missing, ", ")),
			Hint:   "run 'codenv install'",
		})
		return
	}
	r.Add(Check{
		Name:   "locked packages",
		Level:  OK,
		Detail: fmt.Sprintf("all %d installed", len(lock.Top)),
	})
}

func checkSecrets(r *Report, opts Options) {
	if opts.Config == nil || len(opts.Config.SecretNames()) == 0 {
		return
	}
	var unset []string
	for _, name := range opts.Config.SecretNames() {
		source := opts.Config.SourceFor(name)
		if source == "" {
			source = name
		}
		first := strings.Split(source, ",")[0]
		first = strings.TrimSpace(first)
		if v, ok := os.LookupEnv(first); !ok || v == "" {
			unset = append(unset, first)
		}
	}
	if len(unset) > 0 {
		r.Add(Check{
			Name:   "secrets",
			Level:  Warn,
			Detail: fmt.Sprintf("source variables not set: %s", strings.Join(unset, ", ")),
			Hint:   "set them before 'codenv shell'; 'codenv secrets list' shows status",
		})
		return
	}
	r.Add(Check{
		Name:   "secrets",
		Level:  OK,
		Detail: fmt.Sprintf("%d declared, all sources present", len(opts.Config.SecretNames())),
	})
}

func checkPATHShadowing(r *Report, st *store.Store, lock *config.Lock) {
	if st == nil || lock == nil || len(lock.Top) == 0 {
		return
	}
	sep := string(os.PathListSeparator)
	var systemEntries []string
	for _, e := range strings.Split(os.Getenv("PATH"), sep) {
		if e != "" {
			systemEntries = append(systemEntries, strings.ToLower(filepath.Clean(e)))
		}
	}
	var shadowed []string
	for _, key := range lock.Top {
		pkg, ok := lock.Resolved[key]
		if !ok {
			continue
		}
		dir := st.VersionDir(pkg.Name, pkg.Version)
		for _, e := range systemEntries {
			if e == strings.ToLower(filepath.Clean(dir)) {
				shadowed = append(shadowed, pkg.Name)
			}
		}
	}
	if len(shadowed) > 0 {
		sort.Strings(shadowed)
		r.Add(Check{
			Name:   "path shadowing",
			Level:  Warn,
			Detail: fmt.Sprintf("already on your system PATH: %s", strings.Join(dedupe(shadowed), ", ")),
			Hint:   "codenv prepends its own directories, so its version still wins inside 'codenv shell'",
		})
		return
	}
	r.Add(Check{
		Name:   "path shadowing",
		Level:  OK,
		Detail: "no duplicate package directories on your system PATH",
	})
}

func truncate(in []string, n int) []string {
	if len(in) <= n {
		return in
	}
	out := append([]string{}, in[:n]...)
	return append(out, fmt.Sprintf("+%d more", len(in)-n))
}

func dedupe(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, v := range in {
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}
