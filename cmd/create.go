package cmd

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Tijani127/codenv/internal/templates"
	"github.com/Tijani127/codenv/internal/ui"
	"github.com/spf13/cobra"
)

var (
	flagCreateForce    bool
	flagCreateNoReadme bool
)

var createCmd = &cobra.Command{
	Use:   "create <template> [dir]",
	Short: "Create a new codenv project from a template",
	Long: "Create a new directory containing codenv.json, a README and a .gitignore.\n\n" +
		"Templates:\n" +
		templateList(),
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		applyGlobals()
		name := strings.ToLower(strings.TrimSpace(args[0]))
		if !templates.Exists(name) {
			return fail(2, "unknown template %q (available: %s)", name, strings.Join(templates.Names(), ", "))
		}
		dir := name
		if len(args) == 2 {
			dir = args[1]
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return fail(1, "%s", err)
		}
		project := filepath.Base(abs)
		files, err := templates.Files(name)
		if err != nil {
			return fail(1, "loading template %q: %s", name, err)
		}

		names := make([]string, 0, len(files))
		for f := range files {
			names = append(names, f)
		}
		sort.Strings(names)

		var written []string
		for _, f := range names {
			if flagCreateNoReadme && strings.EqualFold(f, "readme.md") {
				continue
			}
			target := filepath.Join(abs, outputName(f))
			if _, err := os.Stat(target); err == nil && !flagCreateForce {
				return fail(1, "%s already exists (pass --force to overwrite)", target)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fail(1, "%s", err)
			}
			content := templates.Substitute(string(files[f]), project, titleFor(name))
			if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
				return fail(1, "writing %s: %s", target, err)
			}
			written = append(written, outputName(f))
		}

		ui.Success("Created a %s project in %s", name, abs)
		for _, w := range written {
			ui.Detail("%s", w)
		}
		ui.Hint("cd %s && codenv install && codenv shell", relFromCwd(abs))
		return nil
	},
}

func relFromCwd(abs string) string {
	cwd := cwdOrDot()
	if rel, err := filepath.Rel(cwd, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return abs
}

func outputName(f string) string {
	switch f {
	case "gitignore":
		return ".gitignore"
	case "gomod.tmpl":
		return "go.mod"
	case "main.go.tmpl":
		return "main.go"
	case "main_test.go.tmpl":
		return "main_test.go"
	}
	return f
}

func titleFor(name string) string {
	for _, t := range templates.List() {
		if t.Name == name {
			return t.Title
		}
	}
	return name
}

func cwdOrDot() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

func templateList() string {
	var b strings.Builder
	for _, t := range templates.List() {
		b.WriteString("  " + pad(t.Name, 10) + t.Description + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func pad(s string, n int) string {
	for len(s) < n {
		s += " "
	}
	return s
}

var listTemplatesCmd = &cobra.Command{
	Use:   "templates",
	Short: "List the available project templates",
	Args:  cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		applyGlobals()
		width := 4
		for _, t := range templates.List() {
			if len(t.Name) > width {
				width = len(t.Name)
			}
		}
		ui.Println("%-*s  %s", width, "NAME", "DESCRIPTION")
		for _, t := range templates.List() {
			ui.Println("%-*s  %s", width, t.Name, ui.Paint(ui.Gray, t.Description))
		}
		return nil
	},
}

func init() {
	createCmd.Flags().BoolVar(&flagCreateForce, "force", false, "overwrite existing files")
	createCmd.Flags().BoolVar(&flagCreateNoReadme, "no-readme", false, "skip the README")
	createCmd.AddCommand(listTemplatesCmd)
	rootCmd.AddCommand(createCmd)
}
