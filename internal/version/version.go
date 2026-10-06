package version

var (
	Version  = "0.0.0-dev"
	Commit   = "none"
	Date     = "unknown"
	Codename = ""
)

// v1.0 is Saskatchewan. Codenames are Canadian provinces and territories in
// alphabetical order; see .github/CODENAMES.txt for the map the release
// workflow reads. These defaults only matter for a local `go build`: released
// binaries get Version, Commit, Date and Codename injected via -ldflags.

func String() string {
	return Version
}

func Full() string {
	return Version + " (" + Commit + ", " + Date + ")"
}

func Display() string {
	if Codename == "" {
		return Full()
	}
	return Version + " " + Codename
}

func Long() string {
	if Codename == "" {
		return Full()
	}
	return Version + " " + Codename + " (" + Commit + ", " + Date + ")"
}
