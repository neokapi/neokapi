package registry

import (
	"strings"

	"golang.org/x/mod/semver"
)

// canon normalises a lenient version string into the canonical
// "vMAJOR.MINOR.PATCH" form golang.org/x/mod/semver compares: it tolerates a
// missing leading "v" and a missing minor/patch (padded with .0), and drops
// pre-release/build metadata so ordering is on the release triple only — the
// historical behaviour of this package.
func canon(v string) string {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.SplitN(v, ".", 3)
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	return "v" + parts[0] + "." + parts[1] + "." + parts[2]
}

// CompareSemver returns -1, 0, 1 comparing two semver-ish version strings.
// Pre-release/build metadata is ignored for ordering.
func CompareSemver(a, b string) int {
	return semver.Compare(canon(a), canon(b))
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
		if semver.Major(canon(base)) != semver.Major(canon(v)) {
			return false
		}
		return CompareSemver(v, base) >= 0
	case strings.HasPrefix(constraint, "~"):
		base := strings.TrimPrefix(constraint, "~")
		if semver.MajorMinor(canon(base)) != semver.MajorMinor(canon(v)) {
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
	return CompareSemver(v, constraint) == 0
}
