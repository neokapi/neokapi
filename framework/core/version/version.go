// Package version holds build-time version information.
// Variables are set via -ldflags at build time.
package version

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
)
