package templates

import (
	"embed"
	"io/fs"
	"sort"
	"strings"
)

//go:embed all:builtin
var builtin embed.FS

type Template struct {
	Name        string
	Title       string
	Description string
}

func List() []Template {
	return []Template{
		{Name: "go", Title: "Go", Description: "Go module with test, vet and build scripts"},
		{Name: "python", Title: "Python", Description: "Python project with a virtualenv-friendly layout"},
		{Name: "node", Title: "Node.js", Description: "Node.js project pinned to an LTS runtime"},
		{Name: "rust", Title: "Rust", Description: "Cargo project with fmt, clippy and test scripts"},
		{Name: "minimal", Title: "Minimal", Description: "Just codenv.json, with example scripts"},
	}
}

func Exists(name string) bool {
	for _, t := range List() {
		if t.Name == name {
			return true
		}
	}
	return false
}

func Names() []string {
	var out []string
	for _, t := range List() {
		out = append(out, t.Name)
	}
	sort.Strings(out)
	return out
}

// Files returns the embedded template files as relative path -> content.
func Files(name string) (map[string][]byte, error) {
	root := "builtin/" + name
	entries, err := fs.ReadDir(builtin, root)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := builtin.ReadFile(root + "/" + e.Name())
		if err != nil {
			return nil, err
		}
		out[e.Name()] = data
	}
	return out, nil
}

// Substitute replaces {{PROJECT}} and {{TITLE}} in template content.
func Substitute(content, project, title string) string {
	r := strings.NewReplacer(
		"{{PROJECT}}", project,
		"{{TITLE}}", title,
		"{{MODULE}}", modulePath(project),
	)
	return r.Replace(content)
}

func modulePath(project string) string {
	return "github.com/Tijani127/" + project
}
