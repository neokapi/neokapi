// Package version holds build-time version information.
// Variables are set via -ldflags at build time.
package version

import "strings"

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

// IsPrerelease reports whether this build is a semver prerelease — i.e. its
// version carries a "-...." component before any build metadata (e.g.
// "1.2.0-rc.1", "1.2.0-beta.2"). A clean release ("1.2.0") and a source build
// ("dev") are not prereleases. Used to brand beta builds and to bootstrap the
// default update channel.
func IsPrerelease() bool {
	v := strings.TrimPrefix(Version, "v")
	if i := strings.IndexByte(v, '+'); i >= 0 { // drop build metadata
		v = v[:i]
	}
	return strings.IndexByte(v, '-') >= 0
}

// WindowTitle returns a desktop window title suffixed with " (Beta)" for
// prerelease builds, so it is visually obvious a user is on a pre-release.
func WindowTitle(base string) string {
	if IsPrerelease() {
		return base + " (Beta)"
	}
	return base
}

// Channel returns the release channel this build belongs to, inferred from the
// version: a prerelease ⇒ "beta" (the fast track), otherwise "stable". This
// mirrors how the release workflow routes tags to channels, so a fresh binary's
// default update channel matches the track it shipped on. It is only a
// *bootstrap* default — a persisted update.channel preference (which survives a
// later update to a final, non-prerelease version) takes precedence.
func Channel() string {
	if IsPrerelease() {
		return "beta"
	}
	return "stable"
}
