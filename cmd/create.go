package cmd

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Tijani127/codenv/internal/store"
	"github.com/Tijani127/codenv/internal/templates"
	"github.com/Tijani127/codenv/internal/ui"
	"github.com/spf13/cobra"
)

var (
	flagCreateForce    bool
	flagCreateNoReadme bool
	flagCreateRef      string
)

var createCmd = &cobra.Command{
	Use:   "create <template|repo> [dir]",
	Short: "Create a new codenv project from a template",
	Long: "Create a new directory containing codenv.json, a README and a .gitignore.\n\n" +
		"TEMPLATE can be a builtin template, the name of a template in " + templateDirDocs() +
		", or a git repository:\n" +
		"  github:owner/repo    clone https://github.com/owner/repo\n" +
		"  owner/repo           clone https://github.com/owner/repo\n" +
		"  https://... / git@...  clone the URL as-is\n\n" +
		"Builtin templates:\n" +
		templateList() + "\n\n" +
		"Every file is copied to the output directory, with the substitutions\n" +
		"{{PROJECT}}, {{TITLE}} and {{MODULE}} expanded.",
	Args: cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		applyGlobals()
		name := strings.ToLower(strings.TrimSpace(args[0]))

		var files map[string][]byte
		var title string
		var from string
		switch {
		case templates.Exists(name):
			// Builtin templates win when a local template shares a name.
			var err error
			files, err = templates.Files(name)
			if err != nil {
				return fail(1, "loading template %q: %s", name, err)
			}
			title = titleFor(name)
			from = "builtin"
		default:
			if localDir, ok := templates.LocalExists(name); ok {
				var err error
				files, err = templates.LoadDir(localDir)
				if err != nil {
					return fail(1, "loading the template in %s: %s", localDir, err)
				}
				if len(files) == 0 {
					return fail(2, "the template %s in %s contains no files", name, templateLocalDir())
				}
				title = name
				from = localDir
			} else {
				url, ok := templates.RepoSource(name)
				if !ok {
					return fail(2, "unknown template %q (available: %s)\n"+
						"or use a repository reference: owner/repo, github:owner/repo or a full git URL",
						name, strings.Join(templates.Names(), ", "))
				}
				var err error
				files, err = cloneTemplate(url)
				if err != nil {
					return err
				}
				if len(files) == 0 {
					return fail(2, "the repository at %s contains no template files", url)
				}
				title = name
				if idx := strings.LastIndexAny(name, "/:"); idx >= 0 {
					title = name[idx+1:]
				}
				from = url
			}
		}

		dir := name
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			dir = name[idx+1:]
		}
		if len(args) == 2 {
			dir = args[1]
		}
		abs, err := filepath.Abs(dir)
		if err != nil {
			return fail(1, "%s", err)
		}
		project := filepath.Base(abs)

		rels := make([]string, 0, len(files))
		for f := range files {
			rels = append(rels, f)
		}
		sort.Strings(rels)

		var written []string
		for _, f := range rels {
			if flagCreateNoReadme && strings.EqualFold(filepath.Base(f), "readme.md") {
				continue
			}
			outName := f
			if from == "builtin" {
				outName = outputName(f)
			}
			target := filepath.Join(abs, outName)
			if _, err := os.Stat(target); err == nil && !flagCreateForce {
				return fail(1, "%s already exists (pass --force to overwrite)", target)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return fail(1, "%s", err)
			}
			content := templates.Substitute(string(files[f]), project, title)
			if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
				return fail(1, "writing %s: %s", target, err)
			}
			written = append(written, outName)
		}

		ui.Success("Created a %s project in %s", title, abs)
		for _, w := range written {
			ui.Detail("%s", w)
		}
		ui.Hint("cd %s && codenv install && codenv shell", relFromCwd(abs))
		return nil
	},
}

// cloneTemplate fetches a template repository into a throwaway directory and
// loads its files. With --ref, the branch or tag is checked out explicitly.
func cloneTemplate(url string) (map[string][]byte, error) {
	dest, err := os.MkdirTemp("", "codenv-template-*")
	if err != nil {
		return nil, fail(1, "%s", err)
	}
	defer os.RemoveAll(dest)
	if flagCreateRef != "" {
		if err := store.CloneRepo(url, dest, flagCreateRef); err != nil {
			return nil, fail(1, "%s", err)
		}
	} else {
		if err := store.CloneRepoDefault(url, dest); err != nil {
			return nil, fail(1, "%s", err)
		}
	}
	files, err := templates.LoadDir(dest)
	if err != nil {
		return nil, fail(1, "loading the cloned template: %s", err)
	}
	return files, nil
}

func templateLocalDir() string {
	dir, ok := templates.LocalDir()
	if ok {
		return dir
	}
	return "~/.codenv/templates"
}

func templateDirDocs() string {
	return "\n" + templateLocalDir() + ", or set CODEENV_TEMPLATES"
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

// templateScripts parses a template's codenv.json and returns its script
// names, so `templates` can show what each scaffold ships with. Templates
// without a codenv.json or without scripts return nil.
func templateScripts(files map[string][]byte) []string {
	raw, ok := files["codenv.json"]
	if !ok {
		return nil
	}
	var parsed struct {
		Shell struct {
			Scripts map[string]string `json:"scripts"`
		} `json:"shell"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil
	}
	names := make([]string, 0, len(parsed.Shell.Scripts))
	for n := range parsed.Shell.Scripts {
		names = append(names, n)
	}
	if len(names) == 0 {
		return nil
	}
	sort.Strings(names)
	return names
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
		for _, t := range templates.List() {
			files, err := templates.Files(t.Name)
			if err != nil {
				continue
			}
			if scripts := templateScripts(files); len(scripts) > 0 {
				ui.Detail("%s: %s", t.Name, strings.Join(scripts, ", "))
			}
		}
		if dir, ok := templates.LocalDir(); ok {
			ui.Println("")
			ui.Println("Local templates in %s:", dir)
			entries, err := os.ReadDir(dir)
			if err != nil {
				return nil
			}
			for _, e := range entries {
				if e.IsDir() && !templates.Exists(strings.ToLower(e.Name())) {
					ui.Println("%-*s  %s", width, e.Name(), ui.Paint(ui.Gray, "local template"))
				}
			}
		} else {
			ui.Hint("add your own templates in %s (or set CODEENV_TEMPLATES); git repositories work too: codenv create owner/repo", templateLocalDir())
		}
		return nil
	},
}

func init() {
	createCmd.Flags().BoolVar(&flagCreateForce, "force", false, "overwrite existing files")
	createCmd.Flags().BoolVar(&flagCreateNoReadme, "no-readme", false, "skip the README")
	createCmd.Flags().StringVar(&flagCreateRef, "ref", "", "branch or tag to clone when TEMPLATE is a git repository")
	createCmd.AddCommand(listTemplatesCmd)
	rootCmd.AddCommand(createCmd, listTemplatesCmd)
}
