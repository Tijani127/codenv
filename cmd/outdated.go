package cmd

import (
	"path/filepath"
	"sort"

	"github.com/Tijani127/codenv/internal/pkgspec"
	"github.com/Tijani127/codenv/internal/ui"
	"github.com/spf13/cobra"
)

var (
	flagOutdatedNoSync  bool
	flagOutdatedGlobal  bool
	flagOutdatedPinned  bool
	flagOutdatedMissing bool
)

// outdated compares every top-level locked package against the latest
// manifest in its bucket. It is deliberately read-only: nothing is
// installed, resolved into the lockfile or written anywhere.
var outdatedCmd = &cobra.Command{
	Use:   "outdated",
	Short: "Show configured packages that have a newer version in their bucket",
	Long: "List the packages this project (or codenv global with --global) pins or tracks\n" +
		"that have a newer version available in their bucket.\n\n" +
		"Unpinned packages move with 'codenv install'; pinned ones only move with\n" +
		"'codenv update --all', which re-resolves the pin.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(flagOutdatedGlobal)
		if err != nil {
			return err
		}
		if len(proj.Config.Packages) == 0 {
			ui.Info("no packages in %s", filepath.Base(proj.ConfigPath))
			return nil
		}

		sess, err := openSession(proj, sessionOpts{
			SkipInstall: true,
			SkipResolve: true,
			SyncBuckets: !flagOutdatedNoSync,
		})
		if err != nil {
			return err
		}

		// Remember which specs carry an explicit version pin so the table
		// can say how the package moves.
		pinned := map[string]bool{}
		if specs, err := pkgspec.Normalize(proj.Config.Packages); err == nil {
			for _, s := range specs {
				if s.Version != "" {
					pinned[s.Name] = true
				}
			}
		}

		type row struct{ pkg, current, latest, note string }
		var rows []row
		for _, key := range sess.Lock.Top {
			pkg, ok := sess.Lock.Resolved[key]
			if !ok {
				continue
			}
			latest, err := sess.Engine.Catalog.Latest(pkg.Bucket, pkg.Name)
			switch {
			case err != nil:
				if flagOutdatedMissing {
					rows = append(rows, row{pkg.Qualified(), pkg.Version, "-", "no manifest"})
				}
				continue
			case pkg.Unresolved:
				if flagOutdatedMissing {
					rows = append(rows, row{pkg.Qualified(), pkg.Version, latest.Version, "unresolved pin"})
				}
				continue
			case latest.Version == pkg.Version:
				continue
			}
			note := ""
			if pinned[pkg.Name] {
				note = "pinned; moves with 'codenv update --all'"
			} else {
				note = "moves with 'codenv install'"
			}
			rows = append(rows, row{pkg.Qualified(), pkg.Version, latest.Version, note})
		}

		if len(rows) == 0 {
			ui.Success("Everything is up to date")
			return nil
		}

		sort.Slice(rows, func(i, j int) bool { return rows[i].pkg < rows[j].pkg })
		width := len("PACKAGE")
		for _, r := range rows {
			if len(r.pkg) > width {
				width = len(r.pkg)
			}
		}
		ui.Println("%-*s  %-16s  %-16s  %s", width, "PACKAGE", "CURRENT", "LATEST", "MOVES WITH")
		for _, r := range rows {
			ui.Println("%-*s  %-16s  %-16s  %s", width, r.pkg, r.current,
				ui.Paint(ui.Yellow, r.latest), ui.Paint(ui.Gray, r.note))
		}
		if !flagOutdatedGlobal {
			ui.Hint("run 'codenv update' to move unpinned packages, 'codenv update --all' for pinned ones")
		} else {
			ui.Hint("run 'codenv global update' to move unpinned packages, 'codenv global update --all' for pinned ones")
		}
		return nil
	},
}

func init() {
	outdatedCmd.Flags().BoolVar(&flagOutdatedNoSync, "no-sync", false, "do not git-pull buckets")
	outdatedCmd.Flags().BoolVar(&flagOutdatedGlobal, "global", false, "check the global configuration")
	outdatedCmd.Flags().BoolVar(&flagOutdatedMissing, "missing", false, "also list packages whose manifest vanished from the buckets")
	rootCmd.AddCommand(outdatedCmd)
}
