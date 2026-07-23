package version

import "fmt"

// Set via -ldflags at build time.
var (
	Version   = "0.1.0"
	Commit    = "dev"
	BuildDate = "unknown"
)

func String() string {
	return fmt.Sprintf("eco %s (commit %s, built %s)", Version, Commit, BuildDate)
}

func Detail() map[string]string {
	return map[string]string{
		"version":    Version,
		"commit":     Commit,
		"build_date": BuildDate,
		"go_version": goVersion(),
	}
}
