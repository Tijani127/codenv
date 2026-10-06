package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Tijani127/codenv/internal/manifest"
	"github.com/Tijani127/codenv/internal/pkgspec"
	"github.com/Tijani127/codenv/internal/store"
	"github.com/Tijani127/codenv/internal/ui"
	"github.com/Tijani127/codenv/internal/version"
	"github.com/spf13/cobra"
)

var (
	flagSearchSync  bool
	flagSearchJSON  bool
	flagSearchLimit int
)

var searchCmd = &cobra.Command{
	Use:   "search <query>",
	Short: "Search the configured Scoop buckets",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		query := ""
		if len(args) == 1 {
			query = args[0]
		}
		proj, err := currentProject(false)
		if err != nil {
			return err
		}
		sess, err := openSession(proj, sessionOpts{
			SkipInstall: true,
			SkipResolve: true,
			SyncBuckets: flagSearchSync,
		})
		if err != nil {
			return err
		}
		results, err := sess.Engine.Catalog.Search(query)
		if err != nil {
			return fail(1, "%s", err)
		}
		if flagSearchLimit > 0 && len(results) > flagSearchLimit {
			results = results[:flagSearchLimit]
		}
		if flagSearchJSON {
			for _, r := range results {
				ui.Println(`{"bucket":%q,"name":%q,"version":%q,"description":%q}`,
					r.Bucket, r.Name, r.Version, r.Description)
			}
			return nil
		}
		if len(results) == 0 {
			ui.Info("no packages matched %q", query)
			return nil
		}
		nameWidth := len("PACKAGE")
		for _, r := range results {
			if len(r.Name) > nameWidth {
				nameWidth = len(r.Name)
			}
		}
		ui.Println("%-*s  %-12s  %-14s  %s", nameWidth, "PACKAGE", "BUCKET", "VERSION", "DESCRIPTION")
		for _, r := range results {
			ui.Println("%-*s  %-12s  %-14s  %s",
				nameWidth, r.Name,
				ui.Paint(ui.Cyan, r.Bucket),
				ui.Paint(ui.Gray, r.Version),
				truncate(r.Description, 60))
		}
		return nil
	},
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 3 {
		return s[:n]
	}
	return s[:n-3] + "..."
}

var flagInfoJSON bool

var infoCmd = &cobra.Command{
	Use:   "info <pkg>...",
	Short: "Show details about packages, including dependencies and binaries",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(false)
		if err != nil {
			return err
		}
		sess, err := openSession(proj, sessionOpts{SkipInstall: true, SkipResolve: true})
		if err != nil {
			return err
		}
		catalog := sess.Engine.Catalog
		for i, raw := range args {
			spec, err := pkgspec.Parse(raw)
			if err != nil {
				return fail(2, "%s", err)
			}
			m, err := catalog.Find(spec)
			if err != nil {
				if i > 0 {
					ui.Errln("")
				}
				ui.Errln("%s", err)
				continue
			}
			if flagInfoJSON {
				ui.Println(`{"bucket":%q,"name":%q,"version":%q,"description":%q,"homepage":%q,"dependencies":%q,"bin":%q}`,
					m.Bucket, spec.Name, m.Version, m.Description, m.Homepage,
					strings.Join(catalog.Dependencies(m), ","),
					strings.Join(m.BinEntries(), ","))
				continue
			}
			if i > 0 {
				ui.Println("")
			}
			printManifest(catalog, m, spec)
		}
		return nil
	},
}

func printManifest(catalog *manifest.Catalog, m *manifest.Manifest, spec pkgspec.Spec) {
	ui.Println("%s %s", ui.Paint(ui.Bold, spec.Qualified()), ui.Paint(ui.Gray, m.Version))
	if m.Description != "" {
		ui.Println("%s %s", ui.Paint(ui.Gray, "  description:"), firstLine(m.Description))
	}
	if m.Homepage != "" {
		ui.Println("%s %s", ui.Paint(ui.Gray, "  homepage:   "), m.Homepage)
	}
	if lic := licenseString(m.License); lic != "" {
		ui.Println("%s %s", ui.Paint(ui.Gray, "  license:    "), lic)
	}
	if deps := catalog.Dependencies(m); len(deps) > 0 {
		ui.Println("%s %s", ui.Paint(ui.Gray, "  depends on: "), strings.Join(deps, ", "))
	}
	if bins := m.BinEntries(); len(bins) > 0 {
		ui.Println("%s %s", ui.Paint(ui.Gray, "  binaries:   "), strings.Join(bins, ", "))
	}
	if notes := m.NoteLines(); len(notes) > 0 {
		ui.Println("%s", ui.Paint(ui.Gray, "  notes:"))
		for _, n := range notes {
			for _, line := range strings.Split(n, "\n") {
				ui.Println("%s", "    "+strings.TrimSpace(line))
			}
		}
	}
}

func licenseString(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []any:
		var parts []string
		for _, item := range t {
			if s, ok := item.(string); ok {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	default:
		return fmt.Sprint(v)
	}
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[:i])
	}
	return strings.TrimSpace(s)
}

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Inspect and clean the codenv download cache",
}

var cacheInfoCmd = &cobra.Command{
	Use:   "info",
	Short: "Show cache and store statistics",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(false)
		if err != nil {
			return err
		}
		sess, err := openSession(proj, sessionOpts{SkipInstall: true, SkipResolve: true, NoBootstrap: true})
		if err != nil {
			return err
		}
		home, err := homeDir()
		if err != nil {
			return fail(1, "%s", err)
		}
		ui.Println("%s %s", ui.Paint(ui.Gray, "codenv home:"), home)
		ui.Println("%s %s", ui.Paint(ui.Gray, "store mode:"), sess.Store.Mode())
		ui.Println("%s %s", ui.Paint(ui.Gray, "store root:"), sess.Store.Root())
		ui.Println("%s %s", ui.Paint(ui.Gray, "shims dir:"), shimState(sess.Store.ShimsDir()))
		ui.Println("%s %d", ui.Paint(ui.Gray, "indexed apps:"), len(sess.Engine.Index.Apps))
		if size, count, err := dirStats(sess.Store.CacheDir()); err == nil {
			ui.Println("%s %s (%d files)", ui.Paint(ui.Gray, "cache:"), humanBytes(size), count)
		}
		if size, count, err := dirStats(sess.Store.Root()); err == nil {
			ui.Println("%s %s (%d files)", ui.Paint(ui.Gray, "store size:"), humanBytes(size), count)
		}
		return nil
	},
}

func shimState(dir string) string {
	if _, err := os.Stat(dir); err == nil {
		return ui.Paint(ui.Yellow, dir+" (present, run 'codenv install' to prune)")
	}
	return ui.Paint(ui.Green, "absent")
}

var cacheCleanCmd = &cobra.Command{
	Use:   "clean",
	Short: "Delete cached downloads",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		proj, err := currentProject(false)
		if err != nil {
			return err
		}
		sess, err := openSession(proj, sessionOpts{SkipInstall: true, SkipResolve: true, NoBootstrap: true})
		if err != nil {
			return err
		}
		size, count, err := dirStats(sess.Store.CacheDir())
		if err != nil {
			return fail(1, "%s", err)
		}
		entries, err := os.ReadDir(sess.Store.CacheDir())
		if err != nil {
			return fail(1, "%s", err)
		}
		for _, e := range entries {
			_ = os.RemoveAll(filepath.Join(sess.Store.CacheDir(), e.Name()))
		}
		ui.Success("Removed %s from the cache (%d files)", humanBytes(size), count)
		return nil
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print version information",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		applyGlobals()
		ui.Println("codenv %s", version.Long())
		if home, err := homeDir(); err == nil {
			ui.Println("%s %s", ui.Paint(ui.Gray, "home:"), home)
		}
		if st, err := store.SharedRoot(); err == nil {
			ui.Println("%s %s", ui.Paint(ui.Gray, "store:"), st)
		}
		return nil
	},
}

var versionUpdateCmd = &cobra.Command{
	Use:   "update",
	Short: "Check how to update codenv itself",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		applyGlobals()
		ui.Println("codenv %s", version.Long())
		ui.Info("codenv is distributed as a single static binary")
		ui.Hint("rebuild or replace the binary, then run 'codenv version'")
		return nil
	},
}

func homeDir() (string, error) {
	if v := os.Getenv("CODEENV_HOME"); v != "" {
		return v, nil
	}
	if v := os.Getenv("LOCALAPPDATA"); v != "" {
		return filepath.Join(v, "codenv"), nil
	}
	return os.UserHomeDir()
}

func dirStats(dir string) (int64, int, error) {
	var total int64
	var count int
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() {
			return nil
		}
		total += info.Size()
		count++
		return nil
	})
	return total, count, err
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGT"[exp])
}

func init() {
	searchCmd.Flags().BoolVar(&flagSearchSync, "sync", false, "git-pull buckets before searching")
	searchCmd.Flags().BoolVar(&flagSearchJSON, "json", false, "emit JSON lines")
	searchCmd.Flags().IntVar(&flagSearchLimit, "limit", 50, "maximum results (0 for all)")
	infoCmd.Flags().BoolVar(&flagInfoJSON, "json", false, "emit JSON lines")

	cacheCmd.AddCommand(cacheInfoCmd, cacheCleanCmd)
	versionCmd.AddCommand(versionUpdateCmd)
	rootCmd.AddCommand(searchCmd, infoCmd, cacheCmd, versionCmd)
}
