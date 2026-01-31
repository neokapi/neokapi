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

// FormatRef identifies a format, optionally pinned to a specific version.
// The name part is a format name (e.g. "okapi-html"), not a pack name.
type FormatRef struct {
	Name    string // e.g. "okapi-html"
	Version string // e.g. "1.46.0", empty = latest
}

// ParseFormatRef parses a string like "okapi-html@1.46.0" or "okapi-html" into a FormatRef.
func ParseFormatRef(s string) FormatRef {
	if i := strings.LastIndex(s, "@"); i > 0 {
		return FormatRef{Name: s[:i], Version: s[i+1:]}
	}
	return FormatRef{Name: s}
}

// String returns "name@version" if versioned, or just "name".
func (r FormatRef) String() string {
	if r.Version != "" {
		return r.Name + "@" + r.Version
	}
	return r.Name
}

// IsVersioned reports whether this ref specifies an explicit version.
func (r FormatRef) IsVersioned() bool {
	return r.Version != ""
}

// CompareSemver compares two semantic version strings (major.minor.patch).
// Returns -1 if a < b, 0 if a == b, +1 if a > b.
// Malformed versions sort before well-formed ones.
func CompareSemver(a, b string) int {
	ap := parseSemverParts(a)
	bp := parseSemverParts(b)
	for i := 0; i < 3; i++ {
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
	for i := 0; i < 3; i++ {
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
