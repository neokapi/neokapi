// Package version holds build-time version information.
// Variables are set via -ldflags at build time.
package version

var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"

	// InstallSource records how this binary was distributed, so the CLI can
	// tailor its update behavior (self-replace vs. nudge the package manager)
	// and print the exact upgrade command. It is stamped per packaging channel
	// at release time via -ldflags, e.g.
	//
	//	-X github.com/neokapi/neokapi/core/version.InstallSource=homebrew
	//
	// Recognized values: "homebrew", "winget", "tarball", "deb", "rpm", "scoop".
	// The empty default means "built from source / unknown".
	InstallSource = ""
)
