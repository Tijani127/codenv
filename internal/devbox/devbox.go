package devbox

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/Tijani127/codenv/internal/config"
)

// devboxFile is the subset of devbox.json that matters for conversion. Nix
// outputs are ignored: they describe how to build the packages, which has no
// Scoop equivalent.
type devboxFile struct {
	Packages []json.RawMessage `json:"packages"`
	Env      map[string]string `json:"env"`
	Shell    *devboxShell      `json:"shell"`
}

type devboxShell struct {
	InitHook string            `json:"init_hook"`
	Scripts  map[string]string `json:"scripts"`
}

// Result is the outcome of an import.
type Result struct {
	Converted []Conversion
	Unmapped  []Unmapped
	Env       map[string]string
	InitHook  string
	Scripts   map[string]string
	// NonFatal records problems that did not stop the import, such as an
	// unrecognised package entry.
	NonFatal []string
}

type Conversion struct {
	Nix   string
	Scoop []string
}

type Unmapped struct {
	Nix    string
	Reason string
}

// Plan reads a devbox.json and works out what a codenv.json would contain,
// without writing anything.
func Plan(path string) (*Result, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var df devboxFile
	if err := json.Unmarshal(data, &df); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}

	res := &Result{
		Env:     map[string]string{},
		Scripts: map[string]string{},
	}

	seen := map[string]bool{}
	for i, raw := range df.Packages {
		entries, err := decodePackageEntry(raw)
		if err != nil {
			res.NonFatal = append(res.NonFatal,
				fmt.Sprintf("packages[%d]: %v", i, err))
			continue
		}
		for _, entry := range entries {
			name := entry.name
			if entry.skip {
				res.NonFatal = append(res.NonFatal,
					fmt.Sprintf("skipped nix output %q; codenv does not build from source", name))
				continue
			}
			if name == "" || seen[name] {
				continue
			}
			seen[name] = true

			targets, ok := Lookup(name)
			if !ok {
				reason, _ := Reason(name)
				res.Unmapped = append(res.Unmapped, Unmapped{Nix: name, Reason: reason})
				continue
			}
			res.Converted = append(res.Converted, Conversion{Nix: name, Scoop: targets})
		}
	}

	for k, v := range df.Env {
		res.Env[k] = v
	}
	if df.Shell != nil {
		res.InitHook = df.Shell.InitHook
		for k, v := range df.Shell.Scripts {
			res.Scripts[k] = v
		}
	}
	return res, nil
}

type pkgEntry struct {
	name string
	skip bool
}

// decodePackageEntry accepts the several shapes devbox allows: a bare string,
// a version map such as {"go": "1.22"}, or an object with "version" and
// nix-specific keys we ignore.
func decodePackageEntry(raw json.RawMessage) ([]pkgEntry, error) {
	trimmed := strings.TrimSpace(string(raw))
	switch {
	case trimmed == "" || trimmed == "null":
		return nil, nil

	case strings.HasPrefix(trimmed, `"`):
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
		return []pkgEntry{{name: nixBaseName(s)}}, nil

	case strings.HasPrefix(trimmed, "{"):
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			return nil, err
		}
		// An explicit name key wins, even when it is the only key, so that
		// {"name": "fd"} resolves to fd rather than to the literal "name".
		for _, key := range []string{"pkg", "package", "name"} {
			if v, ok := obj[key].(string); ok && v != "" && v != key {
				return []pkgEntry{{name: nixBaseName(v)}}, nil
			}
		}
		// {"go": "1.22"} is the common shorthand: a single key that is not a
		// version-bearing field, whose value is the version.
		if len(obj) == 1 {
			for k := range obj {
				return []pkgEntry{{name: nixBaseName(k)}}, nil
			}
		}
		for _, key := range []string{"pkg", "package", "name"} {
			if v, ok := obj[key].(string); ok && v != "" {
				return []pkgEntry{{name: nixBaseName(v)}}, nil
			}
		}
		return nil, fmt.Errorf("cannot find a package name in %s", trimmed)

	case strings.HasPrefix(trimmed, "["):
		var list []json.RawMessage
		if err := json.Unmarshal(raw, &list); err != nil {
			return nil, err
		}
		var out []pkgEntry
		for _, item := range list {
			sub, err := decodePackageEntry(item)
			if err != nil {
				continue
			}
			out = append(out, sub...)
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported package entry %s", trimmed)
}

// nixBaseName strips nix attribute paths and flake references so that
// "python310Packages.python310", "flake:nixpkgs#go" and "legacyPackages.go"
// all reduce to something we can look up.
func nixBaseName(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "flake:")
	if i := strings.Index(s, "#"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.LastIndex(s, "."); i >= 0 && i < len(s)-1 {
		s = s[i+1:]
	}
	if i := strings.Index(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// ToConfig renders the plan as a codenv.json config. Unmapped packages are
// left out; the caller is expected to report them.
func (r *Result) ToConfig() *config.Config {
	c := config.NewConfig()
	for _, conv := range r.Converted {
		for _, s := range conv.Scoop {
			c.Packages = append(c.Packages, s)
		}
	}
	if len(r.Env) > 0 {
		c.Env = r.Env
	}
	if r.InitHook != "" || len(r.Scripts) > 0 {
		shell := c.EnsureShell()
		if r.InitHook != "" {
			shell.InitHook = r.InitHook
		}
		if len(r.Scripts) > 0 {
			shell.Scripts = r.Scripts
		}
	}
	return c
}
