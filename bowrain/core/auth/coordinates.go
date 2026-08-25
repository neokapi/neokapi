package auth

import (
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strings"
)

// CoordinateFilter is a grant's region of the context space: a *partial* point.
// Content sits at a full point (brand, product, channel, …); a filter names only
// the axes it cares about and stays silent about the rest, so silence is the
// wildcard and there is no separate wildcard value to get wrong.
//
// The zero value — nil, or an empty map — reaches every point. That is what
// every membership written before regions existed carries, so the unconstrained
// case has to stay the permissive one or the feature would revoke access on the
// way in.
type CoordinateFilter map[string]string

// Matches reports whether point lies inside the filter's region: every axis the
// filter names is satisfied, and axes it does not name are free.
//
// A point that omits an axis the filter names does not match. A filter for
// brand=acme is a claim about acme content, and content that has not said which
// brand it belongs to is not acme content — it sits at the default point, which
// is a real place with its own custodian rather than a wildcard.
func (f CoordinateFilter) Matches(point map[string]string) bool {
	for axis, want := range f {
		if point[axis] != want {
			return false
		}
	}
	return true
}

// Unconstrained reports whether the filter reaches the whole space.
func (f CoordinateFilter) Unconstrained() bool { return len(f) == 0 }

// Clone returns an independent copy, so a stored filter cannot be mutated
// through a value handed to a caller.
func (f CoordinateFilter) Clone() CoordinateFilter {
	if f == nil {
		return nil
	}
	out := make(CoordinateFilter, len(f))
	maps.Copy(out, f)
	return out
}

// String renders the filter in the form the scope grammar writes, axes sorted so
// the rendering is stable for logs, audit entries and tests.
func (f CoordinateFilter) String() string {
	if len(f) == 0 {
		return ""
	}
	axes := make([]string, 0, len(f))
	for axis := range f {
		axes = append(axes, axis)
	}
	sort.Strings(axes)
	parts := make([]string, 0, len(axes))
	for _, axis := range axes {
		parts = append(parts, axis+"="+f[axis])
	}
	return strings.Join(parts, ",")
}

// ParseCoordinateFilter reads the `axis=value,axis=value` form used by the scope
// grammar and the API. An empty string is the unconstrained filter.
func ParseCoordinateFilter(s string) (CoordinateFilter, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, nil
	}
	out := CoordinateFilter{}
	for pair := range strings.SplitSeq(s, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		axis, value, ok := strings.Cut(pair, "=")
		axis, value = strings.TrimSpace(axis), strings.TrimSpace(value)
		if !ok || axis == "" || value == "" {
			return nil, fmt.Errorf("invalid coordinate %q: expected axis=value", pair)
		}
		if prior, dup := out[axis]; dup && prior != value {
			return nil, fmt.Errorf("axis %q given twice with different values (%q, %q)", axis, prior, value)
		}
		out[axis] = value
	}
	if len(out) == 0 {
		return nil, nil
	}
	return out, nil
}

// CoordinateReach is the union of the filters a caller holds — one per project
// membership, group binding or token scope that granted them anything. A point
// is in reach when *any* filter matches it, which is what makes two narrow
// memberships add up rather than cancel out.
//
// An empty reach is unconstrained, matching the language rule in
// ResolveProjectPermissions: a grant that names no constraint grants all of
// them.
type CoordinateReach []CoordinateFilter

// Unconstrained reports whether the reach covers the whole space — either
// because nothing constrains it, or because one of its filters does not.
func (r CoordinateReach) Unconstrained() bool {
	if len(r) == 0 {
		return true
	}
	for _, f := range r {
		if f.Unconstrained() {
			return true
		}
	}
	return false
}

// Reaches reports whether the caller's grants extend to point.
func (r CoordinateReach) Reaches(point map[string]string) bool {
	if r.Unconstrained() {
		return true
	}
	for _, f := range r {
		if f.Matches(point) {
			return true
		}
	}
	return false
}

// Add appends a filter, collapsing to the unconstrained reach as soon as an
// unconstrained filter arrives — carrying narrower filters beside it would be
// dead weight that every later check has to walk.
func (r CoordinateReach) Add(f CoordinateFilter) CoordinateReach {
	if r.Unconstrained() && len(r) > 0 {
		return r
	}
	if f.Unconstrained() {
		return CoordinateReach{nil}
	}
	for _, have := range r {
		if have.String() == f.String() {
			return r
		}
	}
	return append(r, f)
}

// Intersect narrows one reach by another, which is what a token scope does to a
// membership: the caller may act only where both agree.
//
// Two conjunctive filters intersect to their union of axes when they agree on
// every shared axis, and to nothing when they disagree — brand=acme narrowed by
// brand=other reaches no point at all. A reach that survives as empty therefore
// means "nowhere", which is why the caller gets an explicit second return rather
// than an empty slice it would read as "everywhere".
func (r CoordinateReach) Intersect(other CoordinateReach) (result CoordinateReach, anywhere bool) {
	if other.Unconstrained() {
		return r, true
	}
	if r.Unconstrained() {
		return other, true
	}
	var out CoordinateReach
	for _, a := range r {
		for _, b := range other {
			merged, ok := mergeFilters(a, b)
			if !ok {
				continue
			}
			out = out.Add(merged)
		}
	}
	return out, len(out) > 0
}

// mergeFilters conjoins two filters, reporting false when they disagree on a
// shared axis and therefore describe disjoint regions.
func mergeFilters(a, b CoordinateFilter) (CoordinateFilter, bool) {
	out := make(CoordinateFilter, len(a)+len(b))
	maps.Copy(out, a)
	for axis, value := range b {
		if prior, ok := out[axis]; ok && prior != value {
			return nil, false
		}
		out[axis] = value
	}
	return out, true
}

// String renders the reach for logs and audit entries. The unconstrained reach
// renders as "*" so an audit line never shows an empty field where a scope was
// meant to be.
func (r CoordinateReach) String() string {
	if r.Unconstrained() {
		return "*"
	}
	parts := make([]string, 0, len(r))
	for _, f := range r {
		parts = append(parts, "{"+f.String()+"}")
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

// MarshalCoordinates renders a filter for the coordinates column. The
// unconstrained filter is stored as "{}" rather than NULL so every row reads
// back the same way.
func MarshalCoordinates(f CoordinateFilter) string {
	if len(f) == 0 {
		return "{}"
	}
	b, err := json.Marshal(map[string]string(f))
	if err != nil {
		return "{}"
	}
	return string(b)
}

// UnmarshalCoordinates reads the coordinates column. Unreadable or empty JSON
// yields the unconstrained filter, which is the pre-regions behaviour: a row
// whose scope cannot be read must not silently become a narrower grant that
// quietly stops routing work to someone.
func UnmarshalCoordinates(s string) CoordinateFilter {
	s = strings.TrimSpace(s)
	if s == "" || s == "{}" || s == "null" {
		return nil
	}
	var m map[string]string
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil
	}
	if len(m) == 0 {
		return nil
	}
	out := make(CoordinateFilter, len(m))
	for axis, value := range m {
		axis, value = strings.TrimSpace(axis), strings.TrimSpace(value)
		if axis == "" || value == "" {
			continue
		}
		out[axis] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
