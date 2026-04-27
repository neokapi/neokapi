package registry

import (
	"strconv"
	"strings"
)

// CompareSemver returns -1, 0, 1 comparing two semver-ish version strings.
// Pre-release/build metadata is ignored for ordering.
func CompareSemver(a, b string) int {
	ap := parseVersion(a)
	bp := parseVersion(b)
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

// MatchConstraint reports whether v satisfies the constraint.
//
// Supported forms:
//
//	*, "" → any
//	1.4.0  → exact match (after normalisation)
//	^1.4.0 → same major, >= constraint
//	~1.4.0 → same major+minor, >= constraint
//	>=1.4.0, >1.4.0, <=1.4.0, <1.4.0 → comparison
func MatchConstraint(constraint, v string) bool {
	if constraint == "" || constraint == "*" {
		return true
	}
	switch {
	case strings.HasPrefix(constraint, "^"):
		base := strings.TrimPrefix(constraint, "^")
		bp := parseVersion(base)
		vp := parseVersion(v)
		if bp[0] != vp[0] {
			return false
		}
		return CompareSemver(v, base) >= 0
	case strings.HasPrefix(constraint, "~"):
		base := strings.TrimPrefix(constraint, "~")
		bp := parseVersion(base)
		vp := parseVersion(v)
		if bp[0] != vp[0] || bp[1] != vp[1] {
			return false
		}
		return CompareSemver(v, base) >= 0
	case strings.HasPrefix(constraint, ">="):
		return CompareSemver(v, strings.TrimPrefix(constraint, ">=")) >= 0
	case strings.HasPrefix(constraint, ">"):
		return CompareSemver(v, strings.TrimPrefix(constraint, ">")) > 0
	case strings.HasPrefix(constraint, "<="):
		return CompareSemver(v, strings.TrimPrefix(constraint, "<=")) <= 0
	case strings.HasPrefix(constraint, "<"):
		return CompareSemver(v, strings.TrimPrefix(constraint, "<")) < 0
	}
	return normalize(v) == normalize(constraint)
}

func parseVersion(s string) [3]int {
	s = normalize(s)
	parts := strings.SplitN(s, ".", 3)
	var out [3]int
	for i := 0; i < len(parts) && i < 3; i++ {
		out[i], _ = strconv.Atoi(parts[i])
	}
	return out
}

func normalize(v string) string {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '-'); i >= 0 {
		v = v[:i]
	}
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	return v
}
