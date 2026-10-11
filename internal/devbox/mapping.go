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
	"golang":     {"go"},
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

	// Java lives in the java bucket, which codenv knows about but does not
	// sync by default. Version pins use the same numbered suffixes.
	"java":              {"java/temurin-jdk"},
	"jdk":               {"java/temurin-jdk"},
	"openjdk":           {"java/openjdk"},
	"temurin":           {"java/temurin-jdk"},
	"zulu":              {"java/zulu-jdk"},
	"corretto":          {"java/corretto-jdk"},
	"graalvm":           {"java/graalvm-jdk"},
	"liberica":          {"java/liberica-jdk"},
	"sapmachine":        {"java/sapmachine-jdk"},
	"microsoft-openjdk": {"java/microsoft-jdk"},
	"oraclejdk":         {"java/oraclejdk"},
	"java-jdk":          {"java/temurin-jdk"},

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

// versionedForms gives the per-version manifest name for packages whose
// versioned builds are not a simple "base + digits" concatenation. The
// java bucket uses vendor-prefixed names such as temurin21-jdk, so
// java21 cannot be derived arithmetically from the base name.
var versionedForms = map[string]func(num string) string{
	"java":                func(num string) string { return "java/temurin" + num + "-jdk" },
	"jdk":                 func(num string) string { return "java/temurin" + num + "-jdk" },
	"temurin":             func(num string) string { return "java/temurin" + num + "-jdk" },
	"openjdk":             func(num string) string { return "java/openjdk" + num },
	"zulu":                func(num string) string { return "java/zulu" + num + "-jdk" },
	"corretto":            func(num string) string { return "java/corretto" + num + "-jdk" },
	"microsoft-openjdk":   func(num string) string { return "java/microsoft" + num + "-jdk" },
	"liberica":            func(num string) string { return "java/liberica" + num + "-jdk" },
	"sapmachine":          func(num string) string { return "java/sapmachine" + num + "-jdk" },
	"graalvm":             func(num string) string { return "java/graalvm" + num + "-jdk" },
	"postgresql":          func(num string) string { return "versions/postgresql" + num },
	"nodejs":              func(num string) string { return "versions/nodejs" + num },
	"nodejs-lts":          func(num string) string { return "versions/nodejs" + num },
	"python":              func(num string) string { return "versions/python" + num },
	"ruby":                func(num string) string { return "versions/ruby" + num },
	"gradle":              func(num string) string { return "versions/gradle" + num },
	"maven":               func(num string) string { return "versions/maven" + num },
	"dotnet-sdk":          func(num string) string { return "versions/dotnet-sdk" + num },
	"nuget":               func(num string) string { return "versions/nuget" + num },
	"openssl":             func(num string) string { return "versions/openssl" + num },
	"llvm":                func(num string) string { return "versions/llvm" + num },
	"gcc":                 func(num string) string { return "versions/gcc" + num },
	"mysql":               func(num string) string { return "versions/mysql" + num },
	"mongodb":             func(num string) string { return "versions/mongodb" + num },
	"redis":               func(num string) string { return "versions/redis" + num },
	"sqlite":              func(num string) string { return "versions/sqlite" + num },
	"deno":                func(num string) string { return "versions/deno" + num },
	"bun":                 func(num string) string { return "versions/bun" + num },
	"julia":               func(num string) string { return "versions/julia" + num },
	"zig":                 func(num string) string { return "versions/zig" + num },
	"terraform":           func(num string) string { return "versions/terraform" + num },
	"kubernetes-cli":      func(num string) string { return "versions/kubernetes-cli" + num },
	"kubectl":             func(num string) string { return "versions/kubectl" + num },
	"dart":                func(num string) string { return "versions/dart" + num },
	"flutter":             func(num string) string { return "versions/flutter" + num },
	"gh":                  func(num string) string { return "versions/gh" + num },
	"neovim":              func(num string) string { return "versions/neovim" + num },
	"vim":                 func(num string) string { return "versions/vim" + num },
	"lua":                 func(num string) string { return "versions/lua" + num },
	"php":                 func(num string) string { return "versions/php" + num },
	"elixir":              func(num string) string { return "versions/elixir" + num },
	"erlang":              func(num string) string { return "versions/erlang" + num },
	"crystal":             func(num string) string { return "versions/crystal" + num },
	"nim":                 func(num string) string { return "versions/nim" + num },
	"haskell":             func(num string) string { return "versions/ghc" + num },
	"gcc-toolchain":       func(num string) string { return "versions/gcc" + num },
	"postgresql_embedded": func(num string) string { return "versions/postgresql" + num },
	"python310":           func(num string) string { return "versions/python" + num },
	"python311":           func(num string) string { return "versions/python" + num },
	"python312":           func(num string) string { return "versions/python" + num },
	"python313":           func(num string) string { return "versions/python" + num },
}

// alreadyScoopNames are names that are already valid bucket-qualified Scoop
// specs rather than nix attributes, so they pass through untouched.
var alreadyScoopNames = map[string]bool{
	"versions/python311": true,
}

// goVersions maps a Go release line to the versions-bucket manifest that ships
// it. The bucket strips the dot, so 1.22 lives at versions/go122.
var goVersions = map[string]string{
	"113": "go113", "114": "go114", "115": "go115", "116": "go116",
	"117": "go117", "118": "go118", "119": "go119", "120": "go120",
	"121": "go121", "122": "go122", "123": "go123", "124": "go124",
}

// Lookup returns the Scoop packages that provide a nix package name.
//
// Nix attributes frequently carry a version suffix, either as postgresql_16 or
// as python311. Those are turned into a versions-bucket pin where one exists,
// so a pin written the nix way survives the import rather than being dropped.
func Lookup(nixName string) ([]string, bool) {
	if alreadyScoopNames[nixName] {
		return []string{nixName}, true
	}
	// A vendor JDK pin such as temurin21-jdk already names a manifest in the
	// java bucket, so it only needs the bucket attached.
	if bucket, ok := javaBucketPin(nixName); ok {
		return []string{bucket + "/" + nixName}, true
	}
	if pkgs, ok := nixToScoop[nixName]; ok && len(pkgs) > 0 {
		return pkgs, true
	}
	if base, num, ok := splitVersionSuffix(nixName); ok {
		// Go spells its release lines 1.22, and the bucket drops the dot.
		if base == "golang" || base == "go" {
			if manifest, found := goVersions[num]; found {
				return []string{"versions/" + manifest}, true
			}
			return nil, false
		}
		if pkgs, ok := nixToScoop[base]; ok && len(pkgs) > 0 {
			if form, special := versionedForms[base]; special {
				return []string{form(num)}, true
			}
			pinned := make([]string, 0, len(pkgs))
			for range pkgs {
				pinned = append(pinned, "versions/"+base+num)
			}
			return pinned, true
		}
	}
	return nil, false
}

// jdkVendors are the vendor prefixes used by the java bucket.
var jdkVendors = []string{
	"temurin", "corretto", "zulu", "liberica", "sapmachine",
	"microsoft", "graalvm", "dragonwell", "semeru",
}

// javaBucketPin recognises a manifest that already carries its own version,
// such as temurin21-jdk or openjdk21, and reports which bucket holds it.
func javaBucketPin(name string) (string, bool) {
	for _, vendor := range jdkVendors {
		if strings.HasPrefix(name, vendor) && strings.HasSuffix(name, "-jdk") {
			return "java", true
		}
	}
	if strings.HasPrefix(name, "openjdk") && allDigits(strings.TrimPrefix(name, "openjdk")) {
		return "java", true
	}
	return "", false
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
	// Dotted versions, as in golang1.22 or openssl3.3. The leading digit of
	// the release line stays attached to the base, so golang1.22 yields the
	// base "golang" and the digits "122".
	if i := strings.LastIndex(name, "."); i > 0 && i < len(name)-1 {
		candidate := name[i+1:]
		if allDigits(candidate) {
			prefix := name[:i]
			if end := len(prefix) - 1; end > 0 && prefix[end] >= '0' && prefix[end] <= '9' {
				trimmed := prefix[:end]
				if j := len(trimmed) - 1; j >= 0 && trimmed[j] >= '0' && trimmed[j] <= '9' {
					trimmed = trimmed[:j]
				}
				if trimmed != "" {
					return trimmed, prefix[len(trimmed):] + candidate, true
				}
			}
			return prefix, candidate, true
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
