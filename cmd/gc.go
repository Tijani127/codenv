package cmd

import (
	"os"
	"path/filepath"

	"github.com/Tijani127/codenv/internal/config"
	"github.com/Tijani127/codenv/internal/gc"
	"github.com/Tijani127/codenv/internal/project"
	"github.com/Tijani127/codenv/internal/store"
	"github.com/Tijani127/codenv/internal/ui"
	"github.com/spf13/cobra"
)

var (
	flagGcDryRun bool
	flagGcForce  bool
)

var gcCmd = &cobra.Command{
	Use:   "gc [--dry-run] [--force]",
	Short: "Reclaim store space by deleting package versions no project pins",
	Long: "Delete store versions that no registered project's codenv.lock.json pins,\n" +
		"and prune index entries for versions that no longer exist on disk.\n\n" +
		"The store's own Scoop installation, every version a project still pins,\n" +
		"and every version the current junction points at are always spared. codenv\n" +
		"registers projects automatically; a project pinned before this feature\n" +
		"arrived is invisible to gc until it runs any codenv command again — or\n" +
		"pass --force to sweep without its references.",
	Args: cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		applyGlobals()
		reg, err := project.LoadRegistry()
		if err != nil {
			return fail(1, "reading the project registry: %s", err)
		}

		// A project directory that no longer exists cannot pin anything.
		var live []project.RegistryEntry
		for _, e := range reg.Projects {
			if info, statErr := os.Stat(e.Dir); statErr == nil && info.IsDir() {
				live = append(live, e)
			} else {
				ui.Info("forgetting %s (directory is gone)", e.Dir)
			}
		}

		// Shared-store references: the union of every registered shared-mode
		// project's lockfile.
		var refs []gc.Ref
		var unreadable []string
		for _, e := range live {
			if e.Mode == "project" {
				// A project-mode store belongs to exactly one directory; it
				// is swept below with that project's own lock.
				continue
			}
			lockPath := filepath.Join(e.Dir, config.LockName)
			switch {
			case lockIsMissing(lockPath):
				// The project exists but has never resolved; there is nothing
				// it could be pinning behind our back. Mention it so a user
				// who did mean to pin something is not surprised.
				ui.Detail("no lockfile in %s (run codenv there once if it pins packages)", e.Dir)
			default:
				lock, err := config.LoadLock(lockPath)
				if err != nil {
					unreadable = append(unreadable, lockPath)
					continue
				}
				refs = append(refs, gc.RefsOf(lock)...)
			}
		}
		if len(unreadable) > 0 && !flagGcForce {
			return fail(2, "cannot read the lockfiles of registered projects:\n  %s\npass --force to ignore them", joinLines(unreadable))
		}

		// Shared store.
		if st, err := store.Open(store.ModeShared, ""); err == nil {
			if err := sweepStore(st, refs, flagGcDryRun); err != nil {
				return err
			}
		}
		// Global store: only its own lock pins versions there.
		if st, err := store.Open(store.ModeGlobal, ""); err == nil {
			globalLock, _ := config.LoadLock(filepath.Join(mustGlobalDir(), config.LockName))
			if err := sweepStore(st, gc.RefsOf(globalLock), flagGcDryRun); err != nil {
				return err
			}
		}
		// Project-mode stores: each is private to its directory.
		for _, e := range live {
			if e.Mode != "project" {
				continue
			}
			st, err := store.Open(store.ModeProject, e.Dir)
			if err != nil {
				continue
			}
			lockPath := filepath.Join(e.Dir, config.LockName)
			lock, err := config.LoadLock(lockPath)
			switch {
			case err == nil:
				if err := sweepStore(st, gc.RefsOf(lock), flagGcDryRun); err != nil {
					return err
				}
			case lockIsMissing(lockPath):
				if err := sweepStore(st, nil, flagGcDryRun); err != nil {
					return err
				}
			default:
				if !flagGcForce {
					return fail(2, "cannot read %s (pass --force to ignore)", lockPath)
				}
			}
		}
		return nil
	},
}

func mustGlobalDir() string {
	dir, err := project.GlobalDir()
	if err != nil {
		return ""
	}
	return dir
}

func lockIsMissing(path string) bool {
	_, err := os.Stat(path)
	return os.IsNotExist(err)
}

// sweepStore plans and applies (or prints) one store's sweep.
func sweepStore(st *store.Store, refs []gc.Ref, dryRun bool) error {
	label := displayStore(st)
	ix, err := store.LoadIndex(st)
	if err != nil {
		return fail(1, "reading the %s index: %s", label, err)
	}
	sweep := gc.SweepStore(st, ix, refs)
	if len(sweep.Entries) == 0 && len(sweep.Stale) == 0 {
		ui.Detail("%s: nothing to reclaim", label)
		return nil
	}
	if dryRun {
		ui.Step("%s: would delete %d version(s)", label, len(sweep.Entries))
		for _, e := range sweep.Entries {
			ui.Detail("%s %s (%s)", e.App, e.Version, humanBytes(e.Bytes))
		}
		ui.Success("would reclaim %s in %s", humanBytes(sweep.Bytes), label)
		return nil
	}

	ui.Step("%s: deleting %d version(s)", label, len(sweep.Entries))
	for _, e := range sweep.Entries {
		ui.Detail("%s %s (%s)", e.App, e.Version, humanBytes(e.Bytes))
	}
	if err := sweep.Apply(st, ix); err != nil {
		return fail(1, "%s", err)
	}
	if err := ix.Save(); err != nil {
		return fail(1, "writing the %s index: %s", label, err)
	}
	if len(sweep.Stale) > 0 {
		ui.Detail("pruned %d stale index entr(y/ies)", len(sweep.Stale))
	}
	ui.Success("reclaimed %s in %s", humanBytes(sweep.Bytes), label)
	return nil
}

// displayStore names the store for output.
func displayStore(st *store.Store) string {
	switch st.Mode() {
	case store.ModeGlobal:
		return "global store"
	case store.ModeProject:
		return "project store"
	default:
		return "shared store"
	}
}

func joinLines(lines []string) string {
	out := ""
	for i, l := range lines {
		if i > 0 {
			out += "\n  "
		}
		out += l
	}
	return out
}

func init() {
	gcCmd.Flags().BoolVar(&flagGcDryRun, "dry-run", false, "print what would be deleted without deleting")
	gcCmd.Flags().BoolVar(&flagGcForce, "force", false, "proceed even when a registered project's lockfile cannot be read")
	rootCmd.AddCommand(gcCmd)
}
