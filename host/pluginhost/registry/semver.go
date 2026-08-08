package registry

import (
	"strings"

	"golang.org/x/mod/semver"
)

// canon normalises a lenient version string into the canonical
// "vMAJOR.MINOR.PATCH" form golang.org/x/mod/semver compares: it tolerates a
// missing leading "v" and a missing minor/patch (padded with .0), and drops
// pre-release/build metadata — so it carries the release triple only, and
// CompareSemver layers the pre-release ordering on top.
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

// prerelease returns the pre-release part of a version ("rc17" in
// "1.2.0-rc17"), with build metadata stripped. Empty for a release.
func prerelease(v string) string {
	v = strings.TrimPrefix(v, "v")
	v, _, _ = strings.Cut(v, "+")
	if _, pre, ok := strings.Cut(v, "-"); ok {
		return pre
	}
	return ""
}

// CompareSemver returns -1, 0, 1 comparing two semver-ish version strings,
// pre-release included: a release outranks its own pre-releases, and
// pre-release identifiers with a numeric tail order numerically — rc7 < rc17,
// which strict-semver lexical ordering gets backwards for the undotted rcN
// spelling every release tag here uses. Collapsing all of 1.2.0's rcs into
// one "equal" version once let an exact rc pin install whichever rc map
// order surfaced first.
func CompareSemver(a, b string) int {
	if c := semver.Compare(canon(a), canon(b)); c != 0 {
		return c
	}
	pa, pb := prerelease(a), prerelease(b)
	switch {
	case pa == "" && pb == "":
		return 0
	case pa == "":
		return 1 // release > pre-release
	case pb == "":
		return -1
	}
	return comparePrerelease(pa, pb)
}

// comparePrerelease orders dot-separated pre-release identifier lists.
// All-numeric identifiers compare numerically (semver rule); alphanumeric
// identifiers split into their alpha prefix and numeric tail, so rc7 < rc17.
func comparePrerelease(a, b string) int {
	as, bs := strings.Split(a, "."), strings.Split(b, ".")
	for i := 0; i < len(as) && i < len(bs); i++ {
		if c := comparePrereleaseIdent(as[i], bs[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(as) < len(bs):
		return -1
	case len(as) > len(bs):
		return 1
	}
	return 0
}

func comparePrereleaseIdent(a, b string) int {
	an, aNum := splitNumericTail(a)
	bn, bNum := splitNumericTail(b)
	if an != bn {
		if an < bn {
			return -1
		}
		return 1
	}
	switch {
	case aNum < bNum:
		return -1
	case aNum > bNum:
		return 1
	}
	return 0
}

// splitNumericTail splits "rc17" into ("rc", 17); a fully-numeric or
// tail-less identifier yields (-1) or the whole string respectively.
func splitNumericTail(s string) (prefix string, n int) {
	i := len(s)
	for i > 0 && s[i-1] >= '0' && s[i-1] <= '9' {
		i--
	}
	if i == len(s) {
		return s, -1
	}
	n = 0
	for _, c := range s[i:] {
		n = n*10 + int(c-'0')
	}
	return s[:i], n
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
