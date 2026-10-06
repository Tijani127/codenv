package version

var (
	Version = "0.1.0"
	Commit  = "none"
	Date    = "unknown"
)

func String() string {
	return Version
}

func Full() string {
	return Version + " (" + Commit + ", " + Date + ")"
}
