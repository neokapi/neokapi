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
//
// A working-tree build is judged by the tag it was built from, so a build off
// v1.2.0-rc9 stays on the beta track.
func IsPrerelease() bool {
	tag, _ := releaseTag(Version)
	if !isSemver(tag) {
		return false
	}
	v := strings.TrimPrefix(tag, "v")
	if i := strings.IndexByte(v, '+'); i >= 0 { // drop build metadata
		v = v[:i]
	}
	return strings.IndexByte(v, '-') >= 0
}

// IsDevBuild reports whether this binary was built from a working tree rather
// than from a released tag: an unstamped build ("dev", "unknown", ""), a
// `git describe` string that is ahead of its tag or dirty
// ("v1.2.0-rc9-238-gb1a3033dc-dirty"), or anything that isn't a semver version
// at all. Such a build corresponds to no published release, so the CLI must
// never compare it against one or offer to update it.
func IsDevBuild() bool {
	tag, dev := releaseTag(Version)
	return dev || !isSemver(tag)
}

// releaseTag strips the decorations `git describe --tags --always --dirty`
// appends to the tag it found — a "-dirty" marker and/or a
// "-<commits-ahead>-g<abbrev-sha>" suffix — and reports whether any were
// present (i.e. this build is ahead of, or dirty relative to, that tag).
func releaseTag(v string) (tag string, dev bool) {
	tag = strings.TrimSpace(v)
	switch tag {
	case "", "dev", "unknown":
		return tag, true
	}
	if s, ok := strings.CutSuffix(tag, "-dirty"); ok {
		tag, dev = s, true
	}
	// "-<commits-ahead>-g<abbrev-sha>": the two trailing fields git describe
	// adds once HEAD has moved past the tag.
	if i := strings.LastIndex(tag, "-g"); i > 0 {
		if j := strings.LastIndex(tag[:i], "-"); j >= 0 {
			if isDigits(tag[j+1:i]) && isHex(tag[i+2:]) {
				tag, dev = tag[:j], true
			}
		}
	}
	return tag, dev
}

// isSemver reports whether v has a "MAJOR.MINOR.PATCH" core (with an optional
// "v" prefix, prerelease, and build metadata). It deliberately rejects the
// package tags that share this repo's tag namespace — "contract-types-v0.1.0",
// "sat-v1.0.2", "vision-models-v1" — which git describe will otherwise happily
// report as the nearest tag to HEAD.
func isSemver(v string) bool {
	core := strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(core, "-+"); i >= 0 {
		core = core[:i]
	}
	parts := strings.Split(core, ".")
	if len(parts) != 3 {
		return false
	}
	for _, p := range parts {
		if !isDigits(p) {
			return false
		}
	}
	return true
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func isHex(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f', r >= 'A' && r <= 'F':
		default:
			return false
		}
	}
	return true
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
