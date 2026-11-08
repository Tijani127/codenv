package templates

import (
	"embed"
	"io/fs"
	"os"
	"path/filepath"
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

// LocalDir returns the directory users keep their own project templates in:
// CODEENV_TEMPLATES when set, otherwise ~/.codenv/templates. It reports false
// when the directory does not exist, so callers fall through to repo sources.
func LocalDir() (string, bool) {
	dir := os.Getenv("CODEENV_TEMPLATES")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		dir = filepath.Join(home, ".codenv", "templates")
	}
	if info, err := os.Stat(dir); err != nil || !info.IsDir() {
		return "", false
	}
	return dir, true
}

// LocalExists reports whether NAME names a directory under LocalDir(). This
// does not shadow the builtin templates; callers check builtin first.
func LocalExists(name string) (string, bool) {
	dir, ok := LocalDir()
	if !ok {
		return "", false
	}
	p := filepath.Join(dir, name)
	info, err := os.Stat(p)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return p, true
}

// RepoSource resolves NAME as a template git repository and returns the clone
// URL. Accepted forms:
//
//	github:owner/repo    -> https://github.com/owner/repo
//	owner/repo           -> https://github.com/owner/repo
//	git@host:owner/repo  -> as-is (ssh)
//	https://... scheme   -> as-is
//
// It reports false when NAME is not a repository reference.
func RepoSource(name string) (string, bool) {
	switch {
	case strings.HasPrefix(name, "github:"):
		repo := strings.TrimPrefix(name, "github:")
		if repo == "" || strings.Contains(repo, ":") {
			return "", false
		}
		return "https://github.com/" + repo, true
	case strings.Contains(name, "://"):
		return name, true
	case strings.HasPrefix(name, "git@"):
		return name, true
	}
	// owner/repo: a single slash is the only separator allowed.
	if strings.Count(name, "/") == 1 {
		owner, repo, _ := strings.Cut(name, "/")
		if owner != "" && repo != "" && !strings.Contains(owner, ":") {
			return "https://github.com/" + name, true
		}
	}
	return "", false
}

// LoadDir walks root recursively and returns every file as a slash-separated
// relative path -> content. Hidden directories such as .git are skipped.
func LoadDir(root string) (map[string][]byte, error) {
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	out := map[string][]byte{}
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path == root {
				return nil
			}
			name := d.Name()
			if name == ".git" || name == ".codenv" {
				return fs.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if strings.HasPrefix(rel, ".git/") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		out[rel] = data
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
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
