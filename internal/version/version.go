package version

var (
	Version  = "0.0.0-dev"
	Commit   = "none"
	Date     = "unknown"
	Codename = ""
)

// Releases are named after Canadian provinces and territories: the major
// version is the province, and each minor version names a place inside it
// (v1.0 Saskatchewan, v1.1 Regina, v2.0 Manitoba, v2.2 Winnipeg). See
// .github/CODENAMES.txt for the map the release workflow reads. These defaults
// only matter for a local `go build`: released binaries get Version, Commit,
// Date and Codename injected via -ldflags.

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
