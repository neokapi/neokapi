// Package version holds build-time version information.
// Variables are set via -ldflags at build time.
package version

var (
	Version   = "0.8.0"
	Commit    = "unknown"
	BuildDate = "unknown"
)
