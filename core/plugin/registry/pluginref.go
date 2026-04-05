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

// FormatRef identifies a format, optionally pinned to a specific version and/or preset.
// The name part is a format name (e.g. "okapi-html"), not a pack name.
//
// Syntax: name[@version][:preset]
//
//	okf_openxml           — latest version, default config
//	okf_openxml@0.38      — pinned version, default config
//	okf_openxml:wellFormed — latest version, preset config
//	okf_openxml@0.38:wellFormed — pinned version + preset config
type FormatRef struct {
	Name    string // e.g. "okapi-html"
	Version string // e.g. "1.46.0"
	Preset  string // e.g. "wellFormed"
}

// ParseFormatRef parses a format reference string using the syntax name[@version][:preset].
// The "@" separator denotes a version and ":" denotes a preset. Both are optional and
// can be combined (e.g. "okf_openxml@0.38:wellFormed").
func ParseFormatRef(s string) FormatRef {
	var ref FormatRef

	// Split on ":" first to extract preset.
	if i := strings.Index(s, ":"); i > 0 {
		ref.Preset = s[i+1:]
		s = s[:i]
	}

	// Split on "@" to extract version.
	if i := strings.LastIndex(s, "@"); i > 0 {
		ref.Version = s[i+1:]
		s = s[:i]
	}

	ref.Name = s
	return ref
}

// String returns the canonical string representation: name[@version][:preset].
func (r FormatRef) String() string {
	var b strings.Builder
	b.WriteString(r.Name)
	if r.Version != "" {
		b.WriteByte('@')
		b.WriteString(r.Version)
	}
	if r.Preset != "" {
		b.WriteByte(':')
		b.WriteString(r.Preset)
	}
	return b.String()
}

// RegistryName returns the name used for format registry lookups:
// "name@version" if versioned, or just "name" for latest.
func (r FormatRef) RegistryName() string {
	if r.Version != "" {
		return r.Name + "@" + r.Version
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

// SemverRange represents a parsed semver version constraint.
type SemverRange struct {
	op  string // "=", "^", "~", ">=", ">", "<=", "<", "*"
	ver [3]int // parsed version parts
}

// ParseSemverRange parses a semver range string.
// Supported forms: "1.2.3" (exact), "^1.2.3" (compatible), "~1.2.3" (patch),
// ">=1.2.3", ">1.2.3", "<=1.2.3", "<1.2.3", "*" (any).
func ParseSemverRange(s string) SemverRange {
	s = strings.TrimSpace(s)
	if s == "*" || s == "" || s == "latest" {
		return SemverRange{op: "*"}
	}
	for _, prefix := range []string{">=", "<=", ">", "<", "^", "~"} {
		if strings.HasPrefix(s, prefix) {
			return SemverRange{op: prefix, ver: parseSemverParts(strings.TrimSpace(s[len(prefix):]))}
		}
	}
	return SemverRange{op: "=", ver: parseSemverParts(s)}
}

// Match reports whether the given version satisfies this range.
func (r SemverRange) Match(version string) bool {
	v := parseSemverParts(version)
	switch r.op {
	case "*":
		return true
	case "=":
		return v == r.ver
	case ">=":
		return CompareSemver(version, semverString(r.ver)) >= 0
	case ">":
		return CompareSemver(version, semverString(r.ver)) > 0
	case "<=":
		return CompareSemver(version, semverString(r.ver)) <= 0
	case "<":
		return CompareSemver(version, semverString(r.ver)) < 0
	case "^":
		// Compatible: >=ver, <next major.
		if CompareSemver(version, semverString(r.ver)) < 0 {
			return false
		}
		// For ^0.x.y, the ceiling is the next minor (npm convention).
		if r.ver[0] == 0 {
			ceiling := [3]int{0, r.ver[1] + 1, 0}
			return CompareSemver(version, semverString(ceiling)) < 0
		}
		ceiling := [3]int{r.ver[0] + 1, 0, 0}
		return CompareSemver(version, semverString(ceiling)) < 0
	case "~":
		// Patch-level: >=ver, <next minor.
		if CompareSemver(version, semverString(r.ver)) < 0 {
			return false
		}
		ceiling := [3]int{r.ver[0], r.ver[1] + 1, 0}
		return CompareSemver(version, semverString(ceiling)) < 0
	}
	return false
}

// String returns the range as a string.
func (r SemverRange) String() string {
	if r.op == "*" {
		return "*"
	}
	if r.op == "=" {
		return semverString(r.ver)
	}
	return r.op + semverString(r.ver)
}

func semverString(v [3]int) string {
	return strconv.Itoa(v[0]) + "." + strconv.Itoa(v[1]) + "." + strconv.Itoa(v[2])
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
