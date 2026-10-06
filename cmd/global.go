package cmd

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Tijani127/codenv/internal/config"
	"github.com/Tijani127/codenv/internal/envbuild"
	"github.com/Tijani127/codenv/internal/pkgspec"
	"github.com/Tijani127/codenv/internal/shellrun"
	"github.com/Tijani127/codenv/internal/ui"
	"github.com/spf13/cobra"
)

var globalCmd = &cobra.Command{
	Use:   "global",
	Short: "Manage packages available in every codenv project",
	Long: "Manage a set of packages installed once and made available to every codenv shell,\n" +
		"whether or not a project declares them.\n\n" +
		"Global packages live in their own store so they never collide with a project's pins.",
}

var globalAddCmd = &cobra.Command{
	Use:   "add <pkg>...",
	Short: "Add a package to the global configuration",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(true)
		if err != nil {
			return err
		}
		specs, err := pkgspec.Normalize(args)
		if err != nil {
			return fail(2, "%s", err)
		}
		existing := existingSpecs(proj)
		proj.Config.Packages = config.Packages(rawOf(pkgspec.NormalizeSpecs(append(existing, specs...))))
		if err := proj.Save(); err != nil {
			return fail(1, "writing %s: %s", proj.ConfigPath, err)
		}
		ui.Success("Added %s to the global configuration", strings.Join(args, ", "))
		sess, err := openSession(proj, sessionOpts{SyncBuckets: true})
		if err != nil {
			return err
		}
		reportInstallSummary(sess)
		return nil
	},
}

var globalRmCmd = &cobra.Command{
	Use:   "rm <pkg>...",
	Short: "Remove a package from the global configuration",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(true)
		if err != nil {
			return err
		}
		targets, err := pkgspec.Normalize(args)
		if err != nil {
			return fail(2, "%s", err)
		}
		var kept []pkgspec.Spec
		var gone []string
		for _, s := range existingSpecs(proj) {
			if matchesAny(s, targets) {
				gone = append(gone, s.String())
				continue
			}
			kept = append(kept, s)
		}
		if len(gone) == 0 {
			ui.Warn("nothing to remove")
			return nil
		}
		proj.Config.Packages = config.Packages(rawOf(kept))
		if err := proj.Save(); err != nil {
			return fail(1, "writing %s: %s", proj.ConfigPath, err)
		}
		ui.Success("Removed %s", strings.Join(gone, ", "))
		lock := config.NewLock()
		specs, err := pkgspec.Normalize([]string(proj.Config.Packages))
		if err == nil && len(specs) > 0 {
			if sess, err := openSession(proj, sessionOpts{SkipResolve: true}); err == nil {
				if resolved, rerr := sess.Engine.Resolve(specs); rerr == nil {
					lock = resolved
				}
			}
		}
		proj.Lock = lock
		if err := proj.SaveLock(); err != nil {
			return fail(1, "writing lockfile: %s", err)
		}
		return nil
	},
}

func matchesAny(s pkgspec.Spec, targets []pkgspec.Spec) bool {
	for _, t := range targets {
		if t.Qualified() == s.Qualified() || t.Name == s.Name {
			return true
		}
	}
	return false
}

var globalListCmd = &cobra.Command{
	Use:   "list",
	Short: "List globally installed packages",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(true)
		if err != nil {
			return err
		}
		sess, err := openSession(proj, sessionOpts{SkipInstall: true, SkipResolve: true})
		if err != nil {
			return err
		}
		if len(sess.Lock.Top) == 0 {
			ui.Info("no global packages configured")
			ui.Hint("add one with 'codenv global add <pkg>'")
			return nil
		}
		for _, key := range sess.Lock.Top {
			pkg, ok := sess.Lock.Resolved[key]
			if !ok {
				continue
			}
			status := ui.Paint(ui.Green, "ok")
			if !sess.Store.HasVersion(pkg.Name, pkg.Version) {
				status = ui.Paint(ui.Yellow, "missing")
			}
			ui.Printf("  %-40s %-14s %s\n", pkg.Qualified(), pkg.Version, status)
		}
		return nil
	},
}

var globalInstallCmd = &cobra.Command{
	Use:   "install",
	Short: "Install every package in the global configuration",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(true)
		if err != nil {
			return err
		}
		if len(proj.Config.Packages) == 0 {
			ui.Info("no global packages configured")
			return nil
		}
		sess, err := openSession(proj, sessionOpts{SyncBuckets: true})
		if err != nil {
			return err
		}
		reportInstallSummary(sess)
		return nil
	},
}

var globalUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update global packages to their latest versions",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(true)
		if err != nil {
			return err
		}
		specs := existingSpecs(proj)
		if len(specs) == 0 {
			ui.Info("no global packages configured")
			return nil
		}
		sess, err := openSession(proj, sessionOpts{SkipResolve: true, SyncBuckets: true})
		if err != nil {
			return err
		}
		lock, err := sess.Engine.Resolve(specs)
		if err != nil {
			return fail(2, "%s", err)
		}
		if _, err := sess.Engine.Ensure(lock); err != nil {
			return fail(1, "%s", err)
		}
		proj.Lock = lock
		if err := proj.SaveLock(); err != nil {
			return fail(1, "writing lockfile: %s", err)
		}
		for _, key := range lock.Top {
			if pkg, ok := lock.Resolved[key]; ok {
				ui.Success("%s %s", pkg.Qualified(), pkg.Version)
			}
		}
		return nil
	},
}

var globalShellenvFlags struct {
	format string
}

var globalShellenvCmd = &cobra.Command{
	Use:   "shellenv",
	Short: "Print shell commands that add global packages to PATH",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		format, err := envbuild.ParseFormat(globalShellenvFlags.format)
		if err != nil {
			return fail(2, "%s", err)
		}
		if globalShellenvFlags.format == "" || globalShellenvFlags.format == "auto" {
			format = envbuild.DetectFormat()
		}
		proj, err := currentProject(true)
		if err != nil {
			return err
		}
		sess, err := openSession(proj, sessionOpts{SkipInstall: true, SkipResolve: true})
		if err != nil {
			return err
		}
		env := sess.buildEnv(envbuild.Options{Format: format})
		ui.Print(env.Script(format))
		return nil
	},
}

var globalRunFlags envFlags

var globalRunCmd = &cobra.Command{
	Use:   "run <script|command> [args...]",
	Short: "Run a command in the global environment",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(true)
		if err != nil {
			return err
		}
		opts, err := globalRunFlags.options()
		if err != nil {
			return err
		}
		sess, err := openSession(proj, sessionOpts{})
		if err != nil {
			return err
		}
		env := sess.buildEnv(opts)
		childEnv := env.Apply(os.Environ())
		if err := runInitHook(childEnv, proj.Dir, proj.Config.InitHook()); err != nil {
			return err
		}
		name, rest := splitRunArgs(args)
		if script, ok := proj.Config.Script(name); ok {
			code, err := shellrun.Script(childEnv, proj.Dir, script, rest, os.Stdin, os.Stdout, os.Stderr)
			if err != nil {
				return fail(1, "%s", err)
			}
			if code != 0 {
				return &exitError{code: code, err: nil}
			}
			return nil
		}
		code, err := shellrun.Exec(childEnv, proj.Dir, name, rest, os.Stdin, os.Stdout, os.Stderr)
		if err != nil {
			return fail(127, "%s", err)
		}
		if code != 0 {
			return &exitError{code: code, err: nil}
		}
		return nil
	},
}

var globalPullCmd = &cobra.Command{
	Use:   "pull <file-or-url>",
	Short: "Load a global configuration from a file or URL",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(true)
		if err != nil {
			return err
		}
		data, err := fetchBytes(args[0])
		if err != nil {
			return fail(1, "%s", err)
		}
		incoming, err := config.Load(writeTempConfig(data))
		if err != nil {
			return fail(2, "parsing %s: %s", args[0], err)
		}
		proj.Config = incoming
		if err := proj.Save(); err != nil {
			return fail(1, "writing %s: %s", proj.ConfigPath, err)
		}
		ui.Success("Loaded global configuration from %s", args[0])
		return nil
	},
}

var globalPushCmd = &cobra.Command{
	Use:   "push",
	Short: "Print the global configuration",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(true)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(proj.ConfigPath)
		if err != nil {
			if os.IsNotExist(err) {
				ui.Info("no global configuration at %s", proj.ConfigPath)
				return nil
			}
			return fail(1, "%s", err)
		}
		ui.Print(string(data))
		return nil
	},
}

var globalPathCmd = &cobra.Command{
	Use:   "path",
	Short: "Print the location of the global configuration and store",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(true)
		if err != nil {
			return err
		}
		st, err := proj.OpenStore()
		if err != nil {
			return fail(1, "%s", err)
		}
		ui.Println("%s %s", ui.Paint(ui.Gray, "config:"), proj.ConfigPath)
		ui.Println("%s %s", ui.Paint(ui.Gray, "lock:   "), proj.LockPath)
		ui.Println("%s %s", ui.Paint(ui.Gray, "store:  "), st.Root())
		return nil
	},
}

func writeTempConfig(data []byte) string {
	dir, err := os.MkdirTemp("", "codenv-cfg-")
	if err != nil {
		return ""
	}
	path := filepath.Join(dir, config.FileName)
	_ = os.WriteFile(path, data, 0o644)
	return path
}

func init() {
	globalShellenvCmd.Flags().StringVar(&globalShellenvFlags.format, "format", "auto", "shell format to print (powershell, bash, cmd)")
	globalRunCmd.Flags().SetInterspersed(false)
	globalRunFlags.register(globalRunCmd)
	globalCmd.AddCommand(
		globalAddCmd,
		globalRmCmd,
		globalListCmd,
		globalInstallCmd,
		globalUpdateCmd,
		globalShellenvCmd,
		globalRunCmd,
		globalPullCmd,
		globalPushCmd,
		globalPathCmd,
	)
	rootCmd.AddCommand(globalCmd)
}
