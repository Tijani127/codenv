package cmd

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Tijani127/codenv/internal/config"
	"github.com/Tijani127/codenv/internal/engine"
	"github.com/Tijani127/codenv/internal/envbuild"
	"github.com/Tijani127/codenv/internal/pkgspec"
	"github.com/Tijani127/codenv/internal/project"
	"github.com/Tijani127/codenv/internal/secrets"
	"github.com/Tijani127/codenv/internal/store"
	"github.com/Tijani127/codenv/internal/ui"
	"github.com/Tijani127/codenv/internal/version"
	"github.com/spf13/cobra"
)

var (
	flagConfig string
	flagQuiet  bool
)

type sessionOpts struct {
	SkipInstall bool
	SkipResolve bool
	NoBootstrap bool
	SyncBuckets bool
	Specs       []string
}

type session struct {
	Project *project.Project
	Store   *store.Store
	Engine  *engine.Engine
	Lock    *config.Lock
	Secrets *secrets.Resolved
}

var rootCmd = &cobra.Command{
	Use:   "codenv",
	Short: "Instant, isolated, reproducible development environments on Windows",
	Long: "codenv creates isolated development environments backed by Scoop with shims disabled.\n" +
		"Define your toolchain in codenv.json and every machine gets the exact same versions,\n" +
		"on PATH, without shims and without touching your system PATH.",
	SilenceUsage:  true,
	SilenceErrors: true,
	Version:       versionString(),
}

func versionString() string {
	return version.Full()
}

func Execute() int {
	if err := rootCmd.Execute(); err != nil {
		reportError(err)
		return exitCodeFor(err)
	}
	return 0
}

type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string { return e.err.Error() }
func (e *exitError) Unwrap() error { return e.err }

func reportError(err error) {
	var ee *exitError
	if ok := asExitError(err, &ee); ok {
		if ee.err != nil && !flagQuiet {
			ui.Error("%s", ee.err)
		}
		return
	}
	ui.Error("%s", err)
}

func asExitError(err error, target **exitError) bool {
	if e, ok := err.(*exitError); ok {
		*target = e
		return true
	}
	return false
}

func exitCodeFor(err error) int {
	var ee *exitError
	if asExitError(err, &ee) {
		return ee.code
	}
	return 1
}

func fail(code int, format string, args ...any) error {
	return &exitError{code: code, err: fmt.Errorf(format, args...)}
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&flagConfig, "config", "c", "", "path to a directory containing codenv.json")
	rootCmd.PersistentFlags().BoolVarP(&flagQuiet, "quiet", "q", false, "suppress logs")
	rootCmd.SetVersionTemplate("codenv {{.Version}}\n")
	rootCmd.CompletionOptions.DisableDefaultCmd = false
}

func applyGlobals() {
	ui.SetQuiet(flagQuiet)
}

func currentProject(useGlobal bool) (*project.Project, error) {
	applyGlobals()
	if useGlobal {
		return project.LoadGlobal()
	}
	dir, err := project.ResolveConfigDir(flagConfig)
	if err != nil {
		return nil, fail(2, "%s", err)
	}
	return project.Load(dir)
}

func openSession(proj *project.Project, opts sessionOpts) (*session, error) {
	st, err := proj.OpenStore()
	if err != nil {
		return nil, fail(1, "opening store: %s", err)
	}
	eng, err := engine.New(st)
	if err != nil {
		return nil, fail(1, "opening engine: %s", err)
	}
	sess := &session{Project: proj, Store: st, Engine: eng, Lock: proj.Lock}

	if !opts.NoBootstrap {
		if opts.SyncBuckets {
			ui.Step("Syncing buckets")
			if err := eng.SyncBuckets(); err != nil {
				return nil, fail(1, "syncing buckets: %s", err)
			}
		}
		if err := eng.Bootstrap(proj.Config.Buckets); err != nil {
			return nil, fail(1, "bootstrapping store: %s", err)
		}
	}

	if !opts.SkipResolve {
		specs, err := specsFor(proj, opts.Specs)
		if err != nil {
			return nil, fail(2, "%s", err)
		}
		if !opts.SkipInstall && len(specs) > 0 {
			lock, err := eng.Resolve(specs)
			if err != nil {
				return nil, fail(2, "%s", err)
			}
			if _, err := eng.Ensure(lock); err != nil {
				return nil, fail(1, "%s", err)
			}
			proj.Lock = lock
			if err := proj.SaveLock(); err != nil {
				return nil, fail(1, "writing lockfile: %s", err)
			}
		}
		sess.Lock = proj.Lock
	}
	return sess, nil
}

func specsFor(proj *project.Project, override []string) ([]pkgspec.Spec, error) {
	raw := []string(proj.Config.Packages)
	if len(override) > 0 {
		raw = override
	}
	return pkgspec.Normalize(raw)
}

func (s *session) buildEnv(opts envbuild.Options) *envbuild.Env {
	env := envbuild.Build(envbuild.Sources{
		PathDirs: s.Engine.PathsFor(s.Lock),
		Store:    s.Store,
		Project:  s.Project.Dir,
		Global:   s.Project.Global,
	}, s.Project.Config, opts)
	if s.Secrets != nil {
		s.Secrets.Apply(env.Vars)
	}
	return env
}

func (s *session) resolveSecrets(strict bool) error {
	resolved, err := secrets.ResolveFromEnv(s.Project.Config)
	if err != nil {
		return fail(2, "%s", err)
	}
	s.Secrets = resolved
	if strict {
		if err := resolved.Report(); err != nil {
			return fail(2, "%s", err)
		}
	}
	for _, name := range resolved.Missing {
		ui.Warn("secret source %s is not set in your environment", name)
	}
	if len(resolved.Redacted) > 0 {
		names := make([]string, 0, len(resolved.Redacted))
		for n := range resolved.Redacted {
			names = append(names, n)
		}
		sort.Strings(names)
		ui.Hint("declared secrets not present in the environment: %s", strings.Join(names, ", "))
	}
	return nil
}

func (s *session) banner() string {
	var parts []string
	if !s.Project.Global {
		parts = append(parts, s.Project.Name())
	} else {
		parts = append(parts, "global")
	}
	if len(s.Lock.Top) > 0 {
		parts = append(parts, fmt.Sprintf("%d package(s)", len(s.Lock.Top)))
	}
	return strings.Join(parts, " · ")
}

func splitEnvAssignments(items []string) map[string]string {
	out := map[string]string{}
	for _, item := range items {
		if idx := strings.Index(item, "="); idx > 0 {
			out[item[:idx]] = item[idx+1:]
		}
	}
	return out
}

func readEnvFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fail(2, "reading env file: %s", err)
	}
	out := map[string]string{}
	for _, line := range strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		if idx := strings.Index(line, "="); idx > 0 {
			key := strings.TrimSpace(line[:idx])
			val := strings.TrimSpace(line[idx+1:])
			val = strings.Trim(val, `"'`)
			out[key] = val
		}
	}
	return out, nil
}
