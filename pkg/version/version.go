package version

import "fmt"

var (
	// Version is the semantic version injected at build time.
	Version = "dev"

	// Commit is the source revision injected at build time.
	Commit = "none"

	// Date is the UTC build timestamp injected at build time.
	Date = "unknown"
)

// String returns the human-readable build version.
func String() string {
	return fmt.Sprintf("sis %s (%s, %s)", Version, Commit, Date)
}
