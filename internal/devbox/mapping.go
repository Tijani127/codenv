package devbox

import "strings"

// Mapping from Nix package names as they appear in devbox.json to the Scoop
// packages that provide them. Names were checked against the real main, extras
// and versions buckets rather than guessed, so an entry here is known to
// resolve to an existing manifest.
//
// A nix name may map to several Scoop packages, for example rustc and cargo
// both come from the rust package. An empty or absent entry means there is no
// reasonable Scoop equivalent and the caller should warn.
var nixToScoop = map[string][]string{
	// Straightforward one-to-one renames.
	"go":         {"go"},
	"nodejs":     {"nodejs-lts"},
	"nodejs-lts": {"nodejs-lts"},
	"ripgrep":    {"ripgrep"},
	"fd":         {"fd"},
	"jq":         {"jq"},
	"yq":         {"yq"},
	"git":        {"git"},
	"curl":       {"curl"},
	"wget":       {"wget"},
	"openssh":    {"openssh"},
	"gh":         {"gh"},
	"terraform":  {"terraform"},
	"kubectl":    {"kubectl"},
	"neovim":     {"neovim"},
	"vim":        {"vim"},
	"emacs":      {"emacs"},
	"bazel":      {"bazel"},
	"yarn":       {"yarn"},
	"deno":       {"deno"},
	"bun":        {"bun"},
	"lua":        {"lua"},
	"ruby":       {"ruby"},
	"php":        {"php"},
	"julia":      {"julia"},
	"haskell":    {"haskell"},
	"gcc":        {"gcc"},
	"make":       {"make"},
	"cmake":      {"cmake"},
	"pkg-config": {"pkg-config"},
	"openssl":    {"openssl"},
	"unzip":      {"unzip"},
	"zip":        {"zip"},
	"less":       {"less"},
	"btop":       {"btop"},
	"docker":     {"docker"},
	"podman":     {"podman"},
	"gradle":     {"gradle"},
	"maven":      {"maven"},
	"elixir":     {"elixir"},
	"erlang":     {"erlang"},
	"crystal":    {"crystal"},
	"nim":        {"nim"},
	"zig":        {"zig"},
	"dart":       {"dart"},
	"flutter":    {"flutter"},

	// Nix splits these by interpreter or tool; Scoop ships them together.
	"python3":         {"python"},
	"python":          {"python"},
	"pypy3":           {"pypy3"},
	"rustc":           {"rust"},
	"cargo":           {"rust"},
	"rustup":          {"rustup"},
	"clang":           {"llvm"},
	"ca-certificates": {"openssl"},

	// Database servers are the same in both ecosystems. Nix commonly spells
	// these with an underscore version suffix such as postgresql_16, which
	// normaliseVersionSuffix turns into a versions-bucket pin.
	"postgresql": {"postgresql"},
	"postgres":   {"postgresql"},
	"mysql":      {"mysql"},
	"mariadb":    {"mariadb"},
	"sqlite":     {"sqlite"},
	"redis":      {"redis"},
	"mongodb":    {"mongodb"},
	"duckdb":     {"duckdb"},

	// Media tooling.
	"ffmpeg":         {"ffmpeg"},
	"imagemagick":    {"imagemagick"},
	"graphicsmagick": {"graphicsmagick"},
	"graphviz":       {"graphviz"},

	// Editors and shells that only ship inside git-for-windows are deliberately
	// not mapped: `codenv add git` would silently install a shell you did not
	// ask for. These are reported as unmapped so the user decides.
}

// unmappedReason explains, where known, why a nix package has no Scoop
// counterpart. Used to make the import report actionable instead of just
// listing names the user has to research.
var unmappedReason = map[string]string{
	"zsh":               "zsh is not distributed as a standalone Scoop package",
	"fish":              "fish is not distributed as a standalone Scoop package",
	"tmux":              "tmux needs a POSIX layer; use cygwin or msys2 from Scoop instead",
	"htop":              "use gdu, which is the closest Scoop equivalent",
	"dotnet":            "use dotnet-sdk, or versions/dotnet-sdk-lts for a pinned LTS",
	"python310":         "use the versions bucket, for example versions/python310",
	"nixpkgs":           "codenv resolves packages from Scoop buckets, so nixpkgs itself does not apply",
	"rust-overlay":      "rust-overlay is a nix-specific package set",
	"python311Packages": "list the interpreter directly, for example versions/python311",
}

// Lookup returns the Scoop packages that provide a nix package name.
//
// Nix attributes frequently carry a version suffix, either as postgresql_16 or
// as python311. Those are turned into a versions-bucket pin where one exists,
// so a pin written the nix way survives the import rather than being dropped.
func Lookup(nixName string) ([]string, bool) {
	if pkgs, ok := nixToScoop[nixName]; ok && len(pkgs) > 0 {
		return pkgs, true
	}
	if base, num, ok := splitVersionSuffix(nixName); ok {
		if pkgs, ok := nixToScoop[base]; ok && len(pkgs) > 0 {
			pinned := make([]string, 0, len(pkgs))
			for range pkgs {
				pinned = append(pinned, "versions/"+base+num)
			}
			return pinned, true
		}
	}
	return nil, false
}

// splitVersionSuffix breaks "postgresql_16" into "postgresql" and "16", or
// "python311" into "python" and "311". It returns false for names that carry
// no trailing digits, so "vim" and "redis" are left alone.
func splitVersionSuffix(name string) (base, digits string, ok bool) {
	if i := strings.LastIndex(name, "_"); i > 0 && i < len(name)-1 {
		candidate := name[i+1:]
		if allDigits(candidate) {
			return name[:i], candidate, true
		}
	}
	// Concatenated digits, as in python311.
	for i := 0; i < len(name); i++ {
		if name[i] >= '0' && name[i] <= '9' {
			digits = name[i:]
			if len(digits) >= 2 && allDigits(digits) {
				return name[:i], digits, true
			}
			return "", "", false
		}
	}
	return "", "", false
}

func allDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// Reason returns an explanation for an unmapped name, if one is known.
func Reason(nixName string) (string, bool) {
	reason, ok := unmappedReason[nixName]
	return reason, ok
}

// Known returns every nix name this importer recognises, for testing and for
// the report summary.
func Known() map[string][]string {
	out := make(map[string][]string, len(nixToScoop))
	for k, v := range nixToScoop {
		cp := append([]string(nil), v...)
		out[k] = cp
	}
	return out
}
