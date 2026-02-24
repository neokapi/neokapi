package registry

import (
	"strconv"
	"strings"
)

// PluginRef identifies a plugin pack, optionally pinned to a specific version.
type PluginRef struct {
	Name    string // e.g. "okapi"
	Version string // e.g. "1.46.0", empty = latest
}

// ParsePluginRef parses a string like "okapi@1.46.0" or "okapi" into a PluginRef.
func ParsePluginRef(s string) PluginRef {
	if i := strings.LastIndex(s, "@"); i > 0 {
		return PluginRef{Name: s[:i], Version: s[i+1:]}
	}
	return PluginRef{Name: s}
}

// String returns "name@version" if versioned, or just "name".
func (r PluginRef) String() string {
	if r.Version != "" {
		return r.Name + "@" + r.Version
	}
	return r.Name
}

// IsVersioned reports whether this ref specifies an explicit version.
func (r PluginRef) IsVersioned() bool {
	return r.Version != ""
}

// FormatRef identifies a format, optionally pinned to a specific version or preset.
// The name part is a format name (e.g. "okapi-html"), not a pack name.
type FormatRef struct {
	Name    string // e.g. "okapi-html"
	Version string // e.g. "1.46.0" (semver suffix)
	Preset  string // e.g. "wellFormed" (non-semver suffix)
}

// ParseFormatRef parses a string like "okapi-html@1.46.0" or "okapi-html@wellFormed"
// into a FormatRef. If the suffix after @ consists solely of digits and dots
// (e.g. "1.46.0"), it is treated as a version; otherwise it is treated as a preset.
func ParseFormatRef(s string) FormatRef {
	if i := strings.LastIndex(s, "@"); i > 0 {
		suffix := s[i+1:]
		if isSemver(suffix) {
			return FormatRef{Name: s[:i], Version: suffix}
		}
		return FormatRef{Name: s[:i], Preset: suffix}
	}
	return FormatRef{Name: s}
}

// String returns "name@version" if versioned, "name@preset" if a preset, or just "name".
func (r FormatRef) String() string {
	if r.Version != "" {
		return r.Name + "@" + r.Version
	}
	if r.Preset != "" {
		return r.Name + "@" + r.Preset
	}
	return r.Name
}

// IsVersioned reports whether this ref specifies an explicit version.
func (r FormatRef) IsVersioned() bool {
	return r.Version != ""
}

// IsPreset reports whether this ref specifies a preset.
func (r FormatRef) IsPreset() bool {
	return r.Preset != ""
}

// isSemver reports whether s looks like a semver string: consists solely of
// digits and dots, starts with a digit, and ends with a digit.
func isSemver(s string) bool {
	if len(s) == 0 {
		return false
	}
	for _, c := range s {
		if c != '.' && (c < '0' || c > '9') {
			return false
		}
	}
	return s[0] >= '0' && s[0] <= '9' && s[len(s)-1] >= '0' && s[len(s)-1] <= '9'
}

// CompareSemver compares two semantic version strings (major.minor.patch).
// Returns -1 if a < b, 0 if a == b, +1 if a > b.
// Malformed versions sort before well-formed ones.
func CompareSemver(a, b string) int {
	ap := parseSemverParts(a)
	bp := parseSemverParts(b)
	for i := range 3 {
		if ap[i] < bp[i] {
			return -1
		}
		if ap[i] > bp[i] {
			return 1
		}
	}
	return 0
}

// LatestVersion returns the highest semantic version from the given slice.
// Returns "" if the slice is empty.
func LatestVersion(versions []string) string {
	if len(versions) == 0 {
		return ""
	}
	best := versions[0]
	for _, v := range versions[1:] {
		if CompareSemver(v, best) > 0 {
			best = v
		}
	}
	return best
}

// parseSemverParts splits "1.2.3" into [1, 2, 3]. Malformed parts become -1.
func parseSemverParts(s string) [3]int {
	var parts [3]int
	fields := strings.SplitN(s, ".", 3)
	for i := range 3 {
		if i < len(fields) {
			n, err := strconv.Atoi(fields[i])
			if err != nil {
				parts[i] = -1
			} else {
				parts[i] = n
			}
		} else {
			parts[i] = -1
		}
	}
	return parts
}
