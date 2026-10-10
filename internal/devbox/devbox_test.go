package devbox

import (
	"os"
	"path/filepath"
	"testing"
)

func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPlanListForm(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "devbox.json", `{
      "packages": ["ripgrep", "go", "python3"],
      "env": { "NODE_ENV": "dev" },
      "shell": { "init_hook": "echo hi", "scripts": { "t": "go test ./..." } }
    }`)
	res, err := Plan(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Converted) != 3 {
		t.Fatalf("converted %d, want 3: %+v", len(res.Converted), res.Converted)
	}
	if res.Env["NODE_ENV"] != "dev" {
		t.Errorf("env = %v", res.Env)
	}
	if res.InitHook != "echo hi" {
		t.Errorf("init_hook = %q", res.InitHook)
	}
	if res.Scripts["t"] != "go test ./..." {
		t.Errorf("scripts = %v", res.Scripts)
	}
}

func TestPlanMapFormAndVersionObjects(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "devbox.json", `{
      "packages": [
        "ripgrep",
        { "go": "1.22" },
        { "pkg": "python3", "version": "3.12" },
        { "name": "fd" }
      ]
    }`)
	res, err := Plan(p)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, c := range res.Converted {
		names[c.Nix] = true
	}
	for _, want := range []string{"ripgrep", "go", "python3", "fd"} {
		if !names[want] {
			t.Errorf("missing %q in %v", want, names)
		}
	}
}

func TestPlanIgnoresNixOutputs(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "devbox.json", `{
      "packages": ["ripgrep", "hello"]
    }`)
	res, err := Plan(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Converted) != 1 || res.Converted[0].Nix != "ripgrep" {
		t.Fatalf("converted = %+v", res.Converted)
	}
	if len(res.Unmapped) != 1 || res.Unmapped[0].Nix != "hello" {
		t.Fatalf("unmapped = %+v", res.Unmapped)
	}
}

func TestUnmappedCarriesAReasonWhereKnown(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "devbox.json", `{"packages": ["tmux", "some-unknown-thing"]}`)
	res, err := Plan(p)
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]string{}
	for _, u := range res.Unmapped {
		byName[u.Nix] = u.Reason
	}
	if r := byName["tmux"]; r == "" {
		t.Error("tmux should carry a suggested alternative")
	}
	if _, ok := byName["some-unknown-thing"]; !ok {
		t.Error("unknown package should still be reported")
	}
}

func TestNixBaseName(t *testing.T) {
	cases := map[string]string{
		"python310Packages.python310": "python310",
		"flake:nixpkgs#go":            "go",
		"legacyPackages.ripgrep":      "ripgrep",
		"ripgrep":                     "ripgrep",
		"nixpkgs#fd":                  "fd",
		"apps.python":                 "python",
	}
	for in, want := range cases {
		if got := nixBaseName(in); got != want {
			t.Errorf("nixBaseName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestToConfigDeduplicatesAndOmitsUnmapped(t *testing.T) {
	r := &Result{
		Converted: []Conversion{
			{Nix: "rustc", Scoop: []string{"rust"}},
			{Nix: "cargo", Scoop: []string{"rust"}},
		},
		Unmapped: []Unmapped{{Nix: "zsh"}},
	}
	c := r.ToConfig()
	count := 0
	for _, p := range c.Packages {
		if p == "rust" {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected both entries to survive, got %v", c.Packages)
	}
	for _, p := range c.Packages {
		if p == "zsh" {
			t.Error("unmapped package leaked into the config")
		}
	}
}

func TestPlanRejectsInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "devbox.json", `{ not json`)
	if _, err := Plan(p); err == nil {
		t.Error("expected an error for invalid JSON")
	}
}

func TestPlanDeduplicatesRepeatedNames(t *testing.T) {
	dir := t.TempDir()
	p := writeFile(t, dir, "devbox.json", `{"packages": ["go", "go", "go"]}`)
	res, err := Plan(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Converted) != 1 {
		t.Errorf("converted = %+v, want a single entry", res.Converted)
	}
}
