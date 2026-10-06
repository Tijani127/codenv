package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Tijani127/codenv/internal/config"
	"github.com/Tijani127/codenv/internal/engine"
	"github.com/Tijani127/codenv/internal/pkgspec"
	"github.com/Tijani127/codenv/internal/project"
	"github.com/Tijani127/codenv/internal/ui"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init [dir]",
	Short: "Initialize a directory as a codenv project",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		applyGlobals()
		dir := "."
		if len(args) == 1 {
			dir = args[0]
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return fail(1, "%s", err)
		}
		if err := os.MkdirAll(abs, 0o755); err != nil {
			return fail(1, "creating %s: %s", abs, err)
		}
		path := filepath.Join(abs, config.FileName)
		if _, err := os.Stat(path); err == nil {
			ui.Warn("%s already exists", path)
			return nil
		}
		cfg := config.NewConfig()
		if err := config.Save(path, cfg); err != nil {
			return fail(1, "writing %s: %s", path, err)
		}
		ui.Success("Created %s", path)
		ui.Hint("add packages with 'codenv add <pkg>', then start a shell with 'codenv shell'")
		return nil
	},
}

var (
	flagAddOutputs    []string
	flagAddNoInstall  bool
	flagAddSyncBucket bool
)

var addCmd = &cobra.Command{
	Use:   "add <pkg>...",
	Short: "Add packages to this codenv project",
	Long: "Add one or more Scoop packages to codenv.json and install them.\n\n" +
		"Package references accept an optional bucket and version:\n" +
		"  ripgrep                 latest ripgrep from any bucket\n" +
		"  ripgrep@14.1.0          a specific version\n" +
		"  extras/vcredist2022     a package from a specific bucket",
	Args: cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(false)
		if err != nil {
			return err
		}
		specs, err := pkgspec.Normalize(args)
		if err != nil {
			return fail(2, "%s", err)
		}
		existing := existingSpecs(proj)
		merged := pkgspec.NormalizeSpecs(append(existing, specs...))
		proj.Config.Packages = config.Packages(rawOf(merged))

		if err := proj.Save(); err != nil {
			return fail(1, "writing %s: %s", proj.ConfigPath, err)
		}
		ui.Success("Added %s to %s", strings.Join(args, ", "), filepath.Base(proj.ConfigPath))

		if flagAddNoInstall {
			return nil
		}
		sess, err := openSession(proj, sessionOpts{SyncBuckets: flagAddSyncBucket})
		if err != nil {
			return err
		}
		reportInstallSummary(sess)
		return reportDrift(proj, sess)
	},
}

func rawOf(specs []pkgspec.Spec) []string {
	out := make([]string, 0, len(specs))
	for _, s := range specs {
		out = append(out, s.String())
	}
	return out
}

func existingSpecs(proj *project.Project) []pkgspec.Spec {
	specs, err := pkgspec.Normalize(proj.Config.Packages)
	if err != nil {
		return nil
	}
	return specs
}

func reportInstallSummary(sess *session) {
	if ui.Quiet() {
		return
	}
	ui.Success("Environment ready")
}

func reportDrift(proj *project.Project, sess *session) error {
	if len(sess.Lock.Top) == 0 {
		return nil
	}
	for _, key := range sess.Lock.Top {
		pkg, ok := sess.Lock.Resolved[key]
		if !ok {
			continue
		}
		if sess.Store.HasVersion(pkg.Name, pkg.Version) {
			ui.Detail("%s %s", pkg.Qualified(), pkg.Version)
		}
	}
	return nil
}

var (
	flagInstallNoSync bool
	flagInstallGlobal bool
)

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the packages defined in codenv.json",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(flagInstallGlobal)
		if err != nil {
			return err
		}
		if len(proj.Config.Packages) == 0 {
			ui.Info("no packages in %s", filepath.Base(proj.ConfigPath))
			return nil
		}
		sess, err := openSession(proj, sessionOpts{SyncBuckets: !flagInstallNoSync})
		if err != nil {
			return err
		}
		reportInstallSummary(sess)
		return reportDrift(proj, sess)
	},
}

var (
	flagRmPurge bool
	flagRmForce bool
)

var rmCmd = &cobra.Command{
	Use:   "rm <pkg>...",
	Short: "Remove packages from this codenv project",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(false)
		if err != nil {
			return err
		}
		targets, err := pkgspec.Normalize(args)
		if err != nil {
			return fail(2, "%s", err)
		}
		existing := existingSpecs(proj)
		removed := map[string]bool{}
		for _, t := range targets {
			removed[t.Qualified()] = true
			removed[t.Name] = true
		}
		var kept []pkgspec.Spec
		var gone []string
		for _, s := range existing {
			if removed[s.Qualified()] || removed[s.Name] {
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

		if flagRmPurge {
			st, err := proj.OpenStore()
			if err != nil {
				return fail(1, "%s", err)
			}
			eng, err := engine.New(st)
			if err != nil {
				return fail(1, "%s", err)
			}
			var names []string
			for _, g := range gone {
				if sp, err := pkgspec.Parse(g); err == nil {
					names = append(names, sp.Name)
				}
			}
			if err := eng.Purge(dedupeStrings(names)); err != nil {
				ui.Warn("purge failed: %s", err)
			} else {
				ui.Success("Purged from the store")
			}
		}

		sess, err := openSession(proj, sessionOpts{SkipResolve: false})
		if err != nil {
			return err
		}
		reportInstallSummary(sess)
		return nil
	},
}

func dedupeStrings(in []string) []string {
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

var (
	flagUpdateNoSync bool
	flagUpdateAll    bool
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update packages to the latest available version",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(false)
		if err != nil {
			return err
		}
		if len(proj.Config.Packages) == 0 {
			ui.Info("no packages in %s", filepath.Base(proj.ConfigPath))
			return nil
		}
		specs := existingSpecs(proj)
		if !flagUpdateAll {
			var unpinned []pkgspec.Spec
			for _, s := range specs {
				if s.Version == "" {
					unpinned = append(unpinned, s)
				}
			}
			if len(unpinned) == 0 {
				ui.Info("every package is pinned in %s; nothing to update", filepath.Base(proj.ConfigPath))
				ui.Hint("run 'codenv update --all' to re-resolve pinned versions too")
				return nil
			}
			specs = unpinned
		}

		sess, err := openSession(proj, sessionOpts{
			SkipResolve: true,
			SyncBuckets: !flagUpdateNoSync,
		})
		if err != nil {
			return err
		}

		before := map[string]string{}
		for _, key := range proj.Lock.Top {
			if pkg, ok := proj.Lock.Resolved[key]; ok {
				before[pkg.Name] = pkg.Version
			}
		}

		lock, err := sess.Engine.Resolve(specs)
		if err != nil {
			return fail(2, "%s", err)
		}
		merged := mergeLocks(proj.Lock, lock)
		if _, err := sess.Engine.Ensure(merged); err != nil {
			return fail(1, "%s", err)
		}
		proj.Lock = merged
		if err := proj.SaveLock(); err != nil {
			return fail(1, "writing lockfile: %s", err)
		}

		changed := 0
		for _, key := range merged.Top {
			pkg, ok := merged.Resolved[key]
			if !ok {
				continue
			}
			prev, had := before[pkg.Name]
			if had && prev != pkg.Version {
				ui.Success("%s %s -> %s", pkg.Qualified(), prev, pkg.Version)
				changed++
			} else if !had {
				ui.Detail("%s %s", pkg.Qualified(), pkg.Version)
			}
		}
		if changed == 0 {
			ui.Success("Everything is already up to date")
		}
		return nil
	},
}

func mergeLocks(base, update *config.Lock) *config.Lock {
	out := config.NewLock()
	for k, v := range base.Resolved {
		out.Resolved[k] = v
	}
	for k, v := range update.Resolved {
		out.Resolved[k] = v
	}
	seen := map[string]bool{}
	for _, k := range update.Top {
		if seen[k] {
			continue
		}
		seen[k] = true
		out.Top = append(out.Top, k)
	}
	for _, k := range base.Top {
		if seen[k] {
			continue
		}
		seen[k] = true
		out.Top = append(out.Top, k)
	}
	if len(update.Top) > 0 && flagUpdateAll {
		out.Top = append([]string{}, update.Top...)
	}
	return out
}

var flagListGlobal bool

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List the packages available in this environment",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(flagListGlobal)
		if err != nil {
			return err
		}
		sess, err := openSession(proj, sessionOpts{SkipInstall: true, SkipResolve: true})
		if err != nil {
			return err
		}
		if len(sess.Lock.Top) == 0 {
			ui.Info("no packages configured")
			return nil
		}
		rows := make([][3]string, 0, len(sess.Lock.Top))
		for _, key := range sess.Lock.Top {
			pkg, ok := sess.Lock.Resolved[key]
			if !ok {
				continue
			}
			status := "ok"
			if !sess.Store.HasVersion(pkg.Name, pkg.Version) {
				status = "missing"
			}
			rows = append(rows, [3]string{pkg.Qualified(), pkg.Version, status})
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i][0] < rows[j][0] })
		width := len("PACKAGE")
		for _, r := range rows {
			if len(r[0]) > width {
				width = len(r[0])
			}
		}
		ui.Println("%-*s  %-16s  %s", width, "PACKAGE", "VERSION", "STATUS")
		for _, r := range rows {
			style := ui.Green
			if r[2] != "ok" {
				style = ui.Yellow
			}
			ui.Println("%-*s  %-16s  %s", width, r[0], r[1], ui.Paint(style, r[2]))
		}
		return nil
	},
}

func init() {
	addCmd.Flags().BoolVar(&flagAddNoInstall, "no-install", false, "only edit codenv.json, do not install")
	addCmd.Flags().BoolVar(&flagAddSyncBucket, "sync", true, "refresh bucket manifests before resolving")
	installCmd.Flags().BoolVar(&flagInstallNoSync, "no-sync", false, "do not git-pull buckets")
	installCmd.Flags().BoolVar(&flagInstallGlobal, "global", false, "install the global configuration instead")
	rmCmd.Flags().BoolVar(&flagRmPurge, "purge", false, "also remove packages from the store")
	rmCmd.Flags().BoolVar(&flagRmForce, "force", false, "ignore a missing package")
	updateCmd.Flags().BoolVar(&flagUpdateNoSync, "no-sync", false, "do not git-pull buckets")
	updateCmd.Flags().BoolVar(&flagUpdateAll, "all", false, "re-resolve pinned versions as well")
	listCmd.Flags().BoolVar(&flagListGlobal, "global", false, "list the global configuration")

	rootCmd.AddCommand(initCmd, addCmd, installCmd, rmCmd, updateCmd, listCmd)
}
