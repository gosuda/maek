package version

import "fmt"

var (
	// Version is injected by GoReleaser or build flags.
	Version = "dev"
	// Commit is the git commit SHA injected at build time.
	Commit = "none"
	// Date is the ISO8601 build date injected at build time.
	Date = "unknown"
)

// Info contains build and version details.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

// Get returns the current version info.
func Get() Info {
	return Info{
		Version: Version,
		Commit:  Commit,
		Date:    Date,
	}
}

// String returns a human-readable representation of the version.
func String() string {
	return fmt.Sprintf("maek version %s (commit: %s, date: %s)", Version, Commit, Date)
}
