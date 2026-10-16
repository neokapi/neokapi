// Package axis types the axes of the context space.
//
// Content sits at a point: a value on each axis a project names, such as the
// brand it belongs to, the market it ships in or the audience it addresses
// (C-02). An axis is an open category unless the recipe types it: values are
// then compared by equality, which is how every axis compared before this
// package existed. A typed axis says more about its values:
//
//   - a category axis with declared values refuses any other value;
//   - a geo axis holds ISO 3166-1 country codes, UN M49 region slugs, World,
//     and groups the recipe names, and a region contains the countries in it;
//   - a range axis holds numeric intervals such as an audience age of 13-17,
//     and an interval contains the intervals inside it;
//   - an ordered axis holds declared values in order, such as a lifecycle,
//     compared by equality.
//
// Containment is what lets a rule scoped to the Nordic market answer for
// content written for Norway. The package depends on nothing else in neokapi,
// so the recipe, the operation log, the terms store and the host share one
// reading of a value.
package axis

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// Kind is how an axis compares its values.
type Kind string

const (
	// KindCategory compares values by equality. It is the default.
	KindCategory Kind = "category"
	// KindGeo holds countries, regions and declared groups, where a region or
	// a group contains what is inside it.
	KindGeo Kind = "geo"
	// KindRange holds numeric intervals: "13-17", an open upper bound "65-",
	// or a single number.
	KindRange Kind = "range"
	// KindOrdered holds declared values in order, compared by equality.
	KindOrdered Kind = "ordered"
)

// Kinds is every axis kind, in the order a recipe reference lists them.
var Kinds = []Kind{KindCategory, KindGeo, KindRange, KindOrdered}

// Spec declares one axis in a recipe's `axes:` block.
type Spec struct {
	// Kind is how the axis compares values; empty is a category.
	Kind Kind `yaml:"kind,omitempty" json:"kind,omitempty"`
	// Label is the axis's display name.
	Label string `yaml:"label,omitempty" json:"label,omitempty"`
	// Values closes the axis to these values, in display order. A category
	// axis without them stays open; an ordered axis needs them.
	Values []string `yaml:"values,omitempty" json:"values,omitempty"`
	// Labels are display labels per value.
	Labels map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	// Groups names sets of countries and regions on a geo axis, such as
	// nordics: [no, se, dk, fi, is]. A group contains its members and
	// everything inside them.
	Groups map[string][]string `yaml:"groups,omitempty" json:"groups,omitempty"`
	// Unit names what a range axis counts, such as "years".
	Unit string `yaml:"unit,omitempty" json:"unit,omitempty"`
}

// EffectiveKind is the spec's kind, with empty read as a category.
func (s Spec) EffectiveKind() Kind {
	if s.Kind == "" {
		return KindCategory
	}
	return s.Kind
}

// Closed reports whether the axis refuses values it does not declare.
func (s Spec) Closed() bool {
	k := s.EffectiveKind()
	return (k == KindCategory || k == KindOrdered) && len(s.Values) > 0
}

// Space is the typed axes of a project, by axis name. An axis the map does not
// hold is an open category.
type Space map[string]Spec

// Spec returns the axis's spec, the zero spec (an open category) when the
// space does not declare it.
func (s Space) Spec(axis string) Spec { return s[axis] }

// slugPattern is the shape of an axis name, a declared value and a group name.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

const slugRule = "lowercase letters, digits and hyphens, starting with a letter or digit"

// Check validates the space itself: every axis name is a slug, every kind is
// known, every declared value is a slug declared once, an ordered axis
// declares its values, labels name declared values, and a geo group names
// only countries, regions and other groups. Errors name the recipe key at
// fault under prefix ("axes").
func (s Space) Check(prefix string) error {
	for _, name := range sortedKeys(s) {
		if err := s.checkAxis(prefix, name); err != nil {
			return err
		}
	}
	return nil
}

func (s Space) checkAxis(prefix, name string) error {
	spec := s[name]
	where := prefix + "." + name
	if !slugPattern.MatchString(name) {
		return fmt.Errorf("%s: axis name %q is not a slug (%s)", prefix, name, slugRule)
	}
	kind := spec.EffectiveKind()
	if !slices.Contains(Kinds, kind) {
		return fmt.Errorf("%s.kind: %q is not an axis kind. Use one of %s", where, spec.Kind, kindList())
	}
	seen := map[string]bool{}
	for _, v := range spec.Values {
		switch {
		case kind == KindRange || kind == KindGeo:
			return fmt.Errorf("%s.values: a %s axis takes no declared values (its values are %s)", where, kind, kindValues(kind))
		case !slugPattern.MatchString(v):
			return fmt.Errorf("%s.values: %q is not a slug (%s)", where, v, slugRule)
		case seen[v]:
			return fmt.Errorf("%s.values: %q is declared twice", where, v)
		}
		seen[v] = true
	}
	if kind == KindOrdered && len(spec.Values) == 0 {
		return fmt.Errorf("%s: an ordered axis declares its values in order under `values:`", where)
	}
	if len(spec.Groups) > 0 && kind != KindGeo {
		return fmt.Errorf("%s.groups: only a geo axis takes groups (this axis is %s)", where, kind)
	}
	if spec.Unit != "" && kind != KindRange {
		return fmt.Errorf("%s.unit: only a range axis takes a unit (this axis is %s)", where, kind)
	}
	for _, v := range sortedKeys(spec.Labels) {
		if err := s.Validate(name, v); err != nil {
			return fmt.Errorf("%s.labels: %w", where, err)
		}
	}
	for _, group := range sortedKeys(spec.Groups) {
		if !slugPattern.MatchString(group) {
			return fmt.Errorf("%s.groups: group name %q is not a slug (%s)", where, group, slugRule)
		}
		if knownGeo(group) {
			return fmt.Errorf("%s.groups: %q is already a country or region; name the group something else", where, group)
		}
		if len(spec.Groups[group]) == 0 {
			return fmt.Errorf("%s.groups.%s: a group names at least one country or region", where, group)
		}
		for _, member := range spec.Groups[group] {
			if !knownGeo(member) && !hasGroup(spec, member) {
				return fmt.Errorf("%s.groups.%s: %q is not a country code, a region or a group. %s", where, group, member, geoHint(member))
			}
		}
	}
	if cycle := groupCycle(spec); cycle != "" {
		return fmt.Errorf("%s.groups: group %q contains itself", where, cycle)
	}
	return nil
}

// Validate reports whether value is legal on an axis: declared on a closed
// axis, a country, a region, World or a declared group on a geo axis, an
// interval on a range axis. Any value is legal on an open category, which is
// what every undeclared axis is.
func (s Space) Validate(axis, value string) error {
	if value == "" {
		return fmt.Errorf("axis %s: empty value", axis)
	}
	spec, typed := s[axis]
	if !typed {
		return nil
	}
	switch spec.EffectiveKind() {
	case KindGeo:
		if knownGeo(value) || hasGroup(spec, value) {
			return nil
		}
		return fmt.Errorf("axis %s: %q is not a country code, a region or a group of this axis. %s", axis, value, geoHint(value))
	case KindRange:
		if _, err := ParseRange(value); err != nil {
			return fmt.Errorf("axis %s: %w", axis, err)
		}
		return nil
	}
	if spec.Closed() && !slices.Contains(spec.Values, value) {
		return fmt.Errorf("axis %s: %q is not one of its values (%s)", axis, value, strings.Join(spec.Values, ", "))
	}
	return nil
}

// Contains reports whether inner sits within outer on an axis: equal values
// on a category or ordered axis, a country inside a region or group on a geo
// axis, an interval inside another on a range axis. A value always contains
// itself, so an undeclared axis compares by equality.
func (s Space) Contains(axis, outer, inner string) bool {
	if outer == inner {
		return outer != ""
	}
	spec, typed := s[axis]
	if !typed {
		return false
	}
	switch spec.EffectiveKind() {
	case KindGeo:
		return geoContains(spec, outer, inner)
	case KindRange:
		o, oerr := ParseRange(outer)
		i, ierr := ParseRange(inner)
		return oerr == nil && ierr == nil && o.Contains(i)
	}
	return false
}

// Overlaps reports whether two values can meet on an axis: one contains the
// other, two geo values share a country, or two intervals intersect.
func (s Space) Overlaps(axis, a, b string) bool {
	if s.Contains(axis, a, b) || s.Contains(axis, b, a) {
		return true
	}
	spec, typed := s[axis]
	if !typed {
		return false
	}
	switch spec.EffectiveKind() {
	case KindGeo:
		for _, p := range places {
			if geoContains(spec, a, p.Code) && geoContains(spec, b, p.Code) {
				return true
			}
		}
	case KindRange:
		ra, aerr := ParseRange(a)
		rb, berr := ParseRange(b)
		return aerr == nil && berr == nil && ra.Overlaps(rb)
	}
	return false
}

// Label is a value's display label: the recipe's label for it, the English
// name of a built-in geo value, or empty for the caller to render the value.
func (s Space) Label(axis, value string) string {
	spec := s[axis]
	if l := spec.Labels[value]; l != "" {
		return l
	}
	if spec.EffectiveKind() == KindGeo {
		return GeoLabel(value)
	}
	return ""
}

// geoContains reports whether inner sits within outer on a geo axis: World
// holds everything, a region holds what its subregions and countries hold, and
// a group holds its members and what is inside them.
func geoContains(spec Spec, outer, inner string) bool {
	return geoWithin(spec, outer, inner, 0)
}

// maxGroupDepth bounds how deep groups nest, so a space nobody checked, whose
// groups name each other in a loop, ends rather than recursing forever.
const maxGroupDepth = 16

func geoWithin(spec Spec, outer, inner string, depth int) bool {
	if outer == inner {
		return true
	}
	if depth > maxGroupDepth {
		return false
	}
	if hasGroup(spec, inner) {
		// A group sits within a value when every member does.
		members := spec.Groups[inner]
		for _, m := range members {
			if !geoWithin(spec, outer, m, depth+1) {
				return false
			}
		}
		return len(members) > 0
	}
	if outer == World {
		return knownGeo(inner)
	}
	if hasGroup(spec, outer) {
		for _, m := range spec.Groups[outer] {
			if geoWithin(spec, m, inner, depth+1) {
				return true
			}
		}
		return false
	}
	for v := GeoParent(inner); v != ""; v = GeoParent(v) {
		if v == outer {
			return true
		}
	}
	return false
}

func hasGroup(spec Spec, name string) bool {
	_, ok := spec.Groups[name]
	return ok
}

// groupCycle names a group that contains itself through other groups, empty
// when none does.
func groupCycle(spec Spec) string {
	for _, name := range sortedKeys(spec.Groups) {
		if reaches(spec, name, name, map[string]bool{}) {
			return name
		}
	}
	return ""
}

func reaches(spec Spec, from, target string, seen map[string]bool) bool {
	if seen[from] {
		return false
	}
	seen[from] = true
	for _, m := range spec.Groups[from] {
		if m == target {
			return true
		}
		if hasGroup(spec, m) && reaches(spec, m, target, seen) {
			return true
		}
	}
	return false
}

// geoHint teaches the spelling a geo value takes.
func geoHint(value string) string {
	lower := strings.ToLower(value)
	if lower != value && knownGeo(lower) {
		return "Write it in lower case: " + lower
	}
	return "Use an ISO 3166-1 alpha-2 code in lower case (no, de), a UN M49 region (northern-europe, europe), world, or a group the axis declares under `groups:`"
}

func kindList() string {
	out := make([]string, len(Kinds))
	for i, k := range Kinds {
		out[i] = string(k)
	}
	return strings.Join(out, ", ")
}

func kindValues(k Kind) string {
	if k == KindGeo {
		return "country codes, regions and the groups declared under `groups:`"
	}
	return "intervals such as 13-17 or 65-"
}

func sortedKeys[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
