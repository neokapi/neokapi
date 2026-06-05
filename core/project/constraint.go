package project

import (
	"strings"

	"golang.org/x/mod/semver"
)

// This file is the single version-constraint grammar for .kapi recipes.
// Both the syntactic validator (validVersionConstraint, used by
// validateRequiresSyntax) and the matcher (MatchVersionConstraint, used by
// CheckPlugins) parse the same operator prefixes here and lean on
// golang.org/x/mod/semver for the actual comparison, so the accepted syntax
// and the ordering stay in lockstep.
//
// Accepted forms:
//
//	"" or "*"   → any version
//	"^1.2.3"    → caret: same major, installed >= constraint
//	"~1.2.3"    → tilde (accepted syntactically; matched like >=)
//	">=1.2.3"   → installed >= constraint
//	"<=1.2.3"   → installed <= constraint
//	">1.2.3"    → installed >  constraint
//	"<1.2.3"    → installed <  constraint
//	"=1.2.3"    → exact
//	"1.2.3"     → exact
//
// Version bodies are lenient: a leading "v" is optional and pre-release /
// build metadata ("-rc1", "+build42") is permitted and ignored for matching.

// constraintOps lists the recognized operator prefixes, longest first so
// ">=" / "<=" win over ">" / "<".
var constraintOps = []string{"^", "~", ">=", "<=", ">", "<", "="}

// splitConstraint splits a constraint string into its operator prefix (one of
// constraintOps, or "" for a bare version) and the remaining version body.
func splitConstraint(c string) (op, body string) {
	for _, p := range []string{">=", "<="} {
		if strings.HasPrefix(c, p) {
			return p, c[len(p):]
		}
	}
	for _, p := range []string{"^", "~", ">", "<", "="} {
		if strings.HasPrefix(c, p) {
			return p, c[len(p):]
		}
	}
	return "", c
}

// validVersionBody reports whether body is a syntactically acceptable version
// (lenient: optional leading "v", then dotted alphanumerics with "-"/"+"). An
// empty body is rejected.
func validVersionBody(body string) bool {
	body = strings.TrimPrefix(body, "v")
	if body == "" {
		return false
	}
	for _, r := range body {
		if r == '.' || r == '-' || r == '+' || (r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
			continue
		}
		return false
	}
	return true
}

// validVersionConstraint returns true when c is a syntactically valid version
// constraint string. Empty strings are rejected — every entry must declare a
// constraint (use "*" for "any version").
func validVersionConstraint(c string) bool {
	if c == "" {
		return false
	}
	if c == "*" {
		return true
	}
	_, body := splitConstraint(c)
	return validVersionBody(body)
}

// canonicalSemver normalizes a lenient version body into the canonical
// "vMAJOR.MINOR.PATCH" form that x/mod/semver compares. A missing minor/patch
// defaults to zero; pre-release and build metadata are dropped so the
// comparison is on the release triple only (matching the historical behavior).
func canonicalSemver(v string) string {
	v = strings.TrimPrefix(v, "v")
	// Strip pre-release / build metadata.
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.SplitN(v, ".", 3)
	for len(parts) < 3 {
		parts = append(parts, "0")
	}
	return "v" + parts[0] + "." + parts[1] + "." + parts[2]
}

// MatchVersionConstraint checks whether an installed version satisfies a
// version constraint string. See the package grammar above for the supported
// forms.
func MatchVersionConstraint(constraint, installed string) bool {
	if constraint == "" || constraint == "*" {
		return true
	}
	op, body := splitConstraint(constraint)
	want := canonicalSemver(body)
	have := canonicalSemver(installed)
	cmp := semver.Compare(have, want)
	switch op {
	case "^", "~":
		// Caret/tilde: same major, installed >= constraint.
		if semver.Major(have) != semver.Major(want) {
			return false
		}
		return cmp >= 0
	case ">=":
		return cmp >= 0
	case "<=":
		return cmp <= 0
	case ">":
		return cmp > 0
	case "<":
		return cmp < 0
	default:
		// "=" or bare version → exact match on the release triple.
		return cmp == 0
	}
}
