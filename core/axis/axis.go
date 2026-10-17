// Package axis types the axes of the context space.
//
// Content sits at a point: a value on each axis a project names, such as the
// brand it belongs to, the market it ships in or the audience it addresses
// (C-02). An axis is an open category unless the recipe types it: values are
// then compared by equality. A typed axis says more about its values:
//
//   - a category axis with declared values refuses any other value;
//   - a geo axis holds place ids from the project's gazetteer (core/place):
//     world, UN M49 regions, ISO 3166-1 countries, ISO 3166-2 subdivisions
//     and the places the recipe declares under `places:`, and a place
//     contains the places that sit within it;
//   - a range axis holds numeric intervals such as an audience age of 13-17,
//     and an interval contains the intervals inside it;
//   - an ordered axis holds declared values in order, such as a lifecycle,
//     compared by equality.
//
// Containment is what lets a rule scoped to the Nordic market answer for
// content written for Oslo. The package depends on nothing in neokapi but
// core/place, so the recipe, the operation log, the terms store and the host
// share one reading of a value.
package axis

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/neokapi/neokapi/core/place"
	"gopkg.in/yaml.v3"
)

// Kind is how an axis compares its values.
type Kind string

const (
	// KindCategory compares values by equality. It is the default.
	KindCategory Kind = "category"
	// KindGeo holds place ids, where a place contains the places within it.
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
	// Labels are display labels per value of a category or ordered axis. A
	// place's name is declared on the place.
	Labels map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	// Unit names what a range axis counts, such as "years".
	Unit string `yaml:"unit,omitempty" json:"unit,omitempty"`
}

// specKeys are the keys an axis takes in a recipe.
var specKeys = []string{"kind", "label", "values", "labels", "unit"}

// UnmarshalYAML reads an axis spec and refuses a key the spec does not take,
// so a misspelt key fails the recipe rather than leaving the axis untyped.
func (s *Spec) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			key := n.Content[i].Value
			if slices.Contains(specKeys, key) {
				continue
			}
			return fmt.Errorf("line %d: an axis takes no key %q (it takes %s)", n.Content[i].Line, key, strings.Join(specKeys, ", "))
		}
	}
	type plain Spec
	return n.Decode((*plain)(s))
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

// Space is the typed axes of a project and the gazetteer its geo axes read.
// An axis the space does not declare is an open category. The zero Space
// holds no typed axis and the built-in places.
type Space struct {
	axes   map[string]Spec
	places *place.Gazetteer
}

// NewSpace is the space of the typed axes, by axis name, over a gazetteer. A
// nil gazetteer is place.Builtin().
func NewSpace(axes map[string]Spec, places *place.Gazetteer) Space {
	return Space{axes: axes, places: places}
}

// Spec returns the axis's spec, the zero spec (an open category) when the
// space does not declare it.
func (s Space) Spec(axis string) Spec { return s.axes[axis] }

// Typed reports whether the space declares the axis.
func (s Space) Typed(axis string) bool {
	_, ok := s.axes[axis]
	return ok
}

// Names lists the typed axes, sorted.
func (s Space) Names() []string { return sortedKeys(s.axes) }

// Places is the gazetteer the space's geo axes read, never nil.
func (s Space) Places() *place.Gazetteer {
	if s.places == nil {
		return place.Builtin()
	}
	return s.places
}

// slugPattern is the shape of an axis name and a declared value.
var slugPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

const slugRule = "lowercase letters, digits and hyphens, starting with a letter or digit"

// Check validates the space itself: every axis name is a slug, every kind is
// known, every declared value is a slug declared once, an ordered axis
// declares its values, labels name declared values, and a geo axis takes
// neither values nor labels. Errors name the recipe key at fault under
// prefix ("axes").
func (s Space) Check(prefix string) error {
	for _, name := range sortedKeys(s.axes) {
		if err := s.checkAxis(prefix, name); err != nil {
			return err
		}
	}
	return nil
}

func (s Space) checkAxis(prefix, name string) error {
	spec := s.axes[name]
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
	if spec.Unit != "" && kind != KindRange {
		return fmt.Errorf("%s.unit: only a range axis takes a unit (this axis is %s)", where, kind)
	}
	if len(spec.Labels) > 0 && kind == KindGeo {
		return fmt.Errorf("%s.labels: a geo axis takes no labels; name a place under `places:` (its `name:` and `names:`)", where)
	}
	for _, v := range sortedKeys(spec.Labels) {
		if err := s.Validate(name, v); err != nil {
			return fmt.Errorf("%s.labels: %w", where, err)
		}
	}
	return nil
}

// Validate reports whether value is legal on an axis: declared on a closed
// axis, a place in the gazetteer on a geo axis, an interval on a range axis.
// Any value is legal on an open category, which is what every undeclared axis
// is.
func (s Space) Validate(axis, value string) error {
	if value == "" {
		return fmt.Errorf("axis %s: empty value", axis)
	}
	spec, typed := s.axes[axis]
	if !typed {
		return nil
	}
	switch spec.EffectiveKind() {
	case KindGeo:
		if s.Places().Has(value) {
			return nil
		}
		return fmt.Errorf("axis %s: %q is not a place. %s", axis, value, place.Hint(s.Places(), value))
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
// on a category or ordered axis, a place within another on a geo axis
// (place.Gazetteer.Contains), an interval inside another on a range axis. A
// value always contains itself, so an undeclared axis compares by equality.
func (s Space) Contains(axis, outer, inner string) bool {
	if outer == inner {
		return outer != ""
	}
	spec, typed := s.axes[axis]
	if !typed {
		return false
	}
	switch spec.EffectiveKind() {
	case KindGeo:
		return s.Places().Contains(outer, inner)
	case KindRange:
		o, oerr := ParseRange(outer)
		i, ierr := ParseRange(inner)
		return oerr == nil && ierr == nil && o.Contains(i)
	}
	return false
}

// Overlaps reports whether two values can meet on an axis: one contains the
// other, two places share ground (place.Gazetteer.Overlaps), or two intervals
// intersect.
func (s Space) Overlaps(axis, a, b string) bool {
	if s.Contains(axis, a, b) || s.Contains(axis, b, a) {
		return true
	}
	spec, typed := s.axes[axis]
	if !typed {
		return false
	}
	switch spec.EffectiveKind() {
	case KindGeo:
		return s.Places().Overlaps(a, b)
	case KindRange:
		ra, aerr := ParseRange(a)
		rb, berr := ParseRange(b)
		return aerr == nil && berr == nil && ra.Overlaps(rb)
	}
	return false
}

// Label is a value's display label: the recipe's label for it on a category
// or ordered axis, the place's name on a geo axis, or empty for the caller to
// render the value.
func (s Space) Label(axis, value string) string {
	spec := s.axes[axis]
	if spec.EffectiveKind() == KindGeo {
		return s.Places().Name(value)
	}
	return spec.Labels[value]
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
		return "the places of the gazetteer, built in or declared under `places:`"
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
