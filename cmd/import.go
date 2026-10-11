package cmd

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Tijani127/codenv/internal/config"
	"github.com/Tijani127/codenv/internal/devbox"
	"github.com/Tijani127/codenv/internal/ui"
	"github.com/spf13/cobra"
)

var importCmd = &cobra.Command{
	Use:   "import",
	Short: "Import an environment defined by another tool",
}

var importDevboxCmd = &cobra.Command{
	Use:   "devbox [path]",
	Short: "Convert a devbox.json into codenv.json",
	Long: "Convert a devbox.json into codenv.json.\n\n" +
		"Each Nix package is mapped to the Scoop package that provides it. Packages with no\n" +
		"Scoop equivalent are reported and left out rather than replaced by a guess.\n\n" +
		"Nix outputs and version pinning are not converted: Scoop packages are prebuilt and\n" +
		"codenv pins versions by writing codenv.lock.json after install, so pin exact versions\n" +
		"with the versions bucket instead.",
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		path := "devbox.json"
		if len(args) == 1 {
			path = args[0]
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return fail(1, "%s", err)
		}

		res, err := devbox.Plan(abs)
		if err != nil {
			return fail(1, "%s", err)
		}

		// The whole report goes to stdout in a fixed order. Sending warnings to
		// stderr would interleave them unpredictably with the rest, and this is
		// informational output rather than a diagnostic.
		names := make([]string, 0, len(res.Converted))
		byName := map[string][]string{}
		for _, c := range res.Converted {
			names = append(names, c.Nix)
			byName[c.Nix] = c.Scoop
		}
		sort.Strings(names)

		if len(res.Converted) > 0 {
			ui.Step("Converted %d package(s):", len(res.Converted))
			for _, n := range names {
				ui.Detail("%-18s -> %s", n, strings.Join(byName[n], ", "))
			}
		}

		if len(res.Unmapped) > 0 {
			sort.Slice(res.Unmapped, func(i, j int) bool {
				return res.Unmapped[i].Nix < res.Unmapped[j].Nix
			})
			ui.Step("%d package(s) have no Scoop equivalent and were left out:", len(res.Unmapped))
			for _, u := range res.Unmapped {
				if u.Reason != "" {
					ui.Detail("%-18s %s", u.Nix, u.Reason)
				} else {
					ui.Detail("%-18s no equivalent in any bucket codenv knows about", u.Nix)
				}
			}
		}

		for _, note := range res.NonFatal {
			ui.Detail("note: %s", note)
		}

		if len(res.Converted) == 0 && len(res.Unmapped) == 0 {
			ui.Warn("%s listed no importable packages", path)
		}

		if flagImportDryRun {
			ui.Info("Dry run, so %s was not written.", config.FileName)
			return nil
		}

		cfg := res.ToConfig()
		if existing := fileExists(filepath.Join(".", config.FileName)); existing && !flagImportForce {
			return fail(1, "%s already exists (pass --force to overwrite)", config.FileName)
		}
		if err := config.Save(config.FileName, cfg); err != nil {
			return fail(1, "writing %s: %s", config.FileName, err)
		}
		ui.Success("Wrote %s", config.FileName)

		if len(res.Converted) > 0 {
			fmt.Println()
			ui.Info("Run 'codenv install' to install the packages and write %s.", config.LockName)
		}
		return nil
	},
}

var (
	flagImportForce  bool
	flagImportDryRun bool
)

func init() {
	importDevboxCmd.Flags().BoolVar(&flagImportForce, "force", false, "overwrite an existing "+config.FileName)
	importDevboxCmd.Flags().BoolVar(&flagImportDryRun, "dry-run", false, "report the conversion without writing "+config.FileName)
	importCmd.AddCommand(importDevboxCmd)
	rootCmd.AddCommand(importCmd)
}
