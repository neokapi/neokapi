package its

import (
	"fmt"
	"strings"
)

// Selector is a parsed XPath-subset expression used by ITS rules. It
// represents one or more `Alternate` paths (joined by `|`); a node
// matches the Selector when any Alternate matches it.
//
// The supported subset covers the patterns observed in real-world ITS
// rule documents and okapi's filter test fixtures:
//
//   - Absolute path: `/myDoc/head` (root-anchored sequence)
//   - Descendant axis: `//element` (any ancestor)
//   - Element step: `name`, `prefix:name`, `*`
//   - Attribute step (last only): `/@attr`, `/@*`, `/@prefix:name`
//   - Predicate: `[@attr='value']`, `[@attr="value"]`,
//     `[ancestor::name]`
//   - Union: `a|b|c` (whitespace allowed around `|`)
//
// Unsupported XPath features (functions, position predicates beyond
// trivial cases, axes other than descendant/ancestor, namespace
// resolution beyond the prefix the rule set declares) parse-error so
// the caller can surface authoring mistakes instead of silently
// matching nothing.
type Selector struct {
	Alternates []Alternate
}

// Alternate is one |-separated branch of a Selector.
type Alternate struct {
	// Steps walks element axes from root (when Absolute) or matches
	// against the element itself (when not Absolute and Descendant
	// on the first step).
	Steps []Step

	// Attribute, when non-nil, indicates the selector targets an
	// attribute on the element matched by Steps. Names a specific
	// attribute (or `*` for any).
	Attribute *NameMatch

	// Predicates apply to the final step's match: every predicate
	// must hold for the alternate to match. Predicates on attribute
	// selectors (`//@*[ancestor::del]`) test against the element
	// owning the attribute.
	Predicates []Predicate
}

// Step is one location step in the selector (an axis + name match).
type Step struct {
	// Descendant marks `//` (descendant-or-self::) — match against
	// any ancestor in the path stack. Not Descendant means `/`
	// (child::) — match against the immediately-following position.
	Descendant bool

	// Name is what the step matches. NameMatch{Local: "*"} matches
	// any element name.
	Name NameMatch
}

// NameMatch is a (prefix, local) pair with `*` wildcards. Empty
// Prefix means the step doesn't pin a namespace. The matcher resolves
// prefixes against the rule set's declared namespace map at parse
// time so callers don't need to repeat the lookup.
type NameMatch struct {
	// NamespaceURI is the resolved URI for the prefix (empty when
	// no prefix was declared).
	NamespaceURI string
	// Local is the local-name portion. "*" matches any local name.
	Local string
}

// Predicate is one `[…]` filter expression on a step.
type Predicate struct {
	// Kind discriminates the predicate variant.
	Kind PredicateKind

	// AttrName is the attribute name for PredAttrEquals / PredAttrExists.
	AttrName NameMatch

	// AttrValue is the literal compared against the attribute for
	// PredAttrEquals.
	AttrValue string

	// AncestorName is the element name for PredAncestor.
	AncestorName NameMatch
}

// PredicateKind discriminates Predicate variants.
type PredicateKind int

const (
	// PredAttrEquals: `[@name='value']` — element/attribute carries
	// `name` with that exact value.
	PredAttrEquals PredicateKind = iota + 1
	// PredAttrExists: `[@name]` — element carries the attribute.
	PredAttrExists
	// PredAncestor: `[ancestor::name]` — element has an ancestor
	// with that name.
	PredAncestor
)

// ElementContext is the streaming-friendly view of an element a
// caller is asking the Selector to evaluate against. Readers
// maintain this context while walking the document.
type ElementContext struct {
	// Path is the element ancestry from root to this element. The
	// last entry is the element itself.
	Path []NameMatch

	// Attributes carries this element's attributes for predicate
	// evaluation.
	Attributes []Attribute
}

// Attribute is a name+value pair used for predicate evaluation.
type Attribute struct {
	Name  NameMatch
	Value string
}

// MatchElement reports whether the selector matches the element
// described by ctx. Selectors with an attribute step never match
// element-only callers: use MatchAttribute for those.
func (s *Selector) MatchElement(ctx *ElementContext) bool {
	if s == nil {
		return false
	}
	for i := range s.Alternates {
		alt := &s.Alternates[i]
		if alt.Attribute != nil {
			continue
		}
		if matchAlternateElement(alt, ctx) {
			return true
		}
	}
	return false
}

// MatchAttribute reports whether the selector matches the named
// attribute on the element described by ctx. Element-only selectors
// (no `@…` step) never match attribute callers.
func (s *Selector) MatchAttribute(ctx *ElementContext, attr NameMatch) bool {
	if s == nil {
		return false
	}
	for i := range s.Alternates {
		alt := &s.Alternates[i]
		if alt.Attribute == nil {
			continue
		}
		if !nameMatch(*alt.Attribute, attr) {
			continue
		}
		if matchAlternateElement(alt, ctx) {
			return true
		}
	}
	return false
}

// matchAlternateElement evaluates the element-side of one alternate
// (its Steps + Predicates) against ctx, ignoring whether the
// alternate has an Attribute step (the caller already filtered).
func matchAlternateElement(alt *Alternate, ctx *ElementContext) bool {
	if !matchSteps(alt.Steps, ctx.Path) {
		return false
	}
	for i := range alt.Predicates {
		if !matchPredicate(&alt.Predicates[i], ctx) {
			return false
		}
	}
	return true
}

// matchSteps walks the alternate's steps and reports whether the
// path ends at a position satisfying every step. The last step's
// match must land on the element itself (the final entry of path).
//
// Algorithm:
//   - Walk path positions from candidate start positions.
//   - For absolute paths (first step not Descendant), require the
//     first step to match path[0].
//   - For descendant-axis first step, find any starting index where
//     the chain matches.
//   - The last step must align with len(path)-1 so the selector
//     identifies *this* element, not an ancestor.
func matchSteps(steps []Step, path []NameMatch) bool {
	if len(steps) == 0 {
		// Selector with no steps (e.g. `/` alone) is an authoring
		// error in our subset — never match.
		return false
	}
	if len(path) == 0 {
		return false
	}
	// last step must align with the current element (path[len-1])
	lastIdx := len(path) - 1
	return matchStepsFrom(steps, path, 0, 0, lastIdx)
}

// matchStepsFrom is the recursive matcher. It tries to align step
// `si` with path index `pi`, advancing on success and recursing on
// descendant steps that may need to skip intermediate path entries.
func matchStepsFrom(steps []Step, path []NameMatch, si, pi, lastIdx int) bool {
	if si == len(steps) {
		return false // shouldn't reach — the caller validates
	}
	step := steps[si]
	if step.Descendant {
		// Try every position from pi to lastIdx as the candidate
		// for this step's match. Required because `//` allows
		// arbitrary ancestor depth.
		for k := pi; k <= lastIdx; k++ {
			if !nameMatch(step.Name, path[k]) {
				continue
			}
			if si == len(steps)-1 {
				if k == lastIdx {
					return true
				}
				continue
			}
			if matchStepsFrom(steps, path, si+1, k+1, lastIdx) {
				return true
			}
		}
		return false
	}
	// Absolute / child-axis: must match exactly at pi.
	if pi > lastIdx {
		return false
	}
	if !nameMatch(step.Name, path[pi]) {
		return false
	}
	if si == len(steps)-1 {
		return pi == lastIdx
	}
	return matchStepsFrom(steps, path, si+1, pi+1, lastIdx)
}

// matchPredicate evaluates one predicate against ctx.
func matchPredicate(p *Predicate, ctx *ElementContext) bool {
	switch p.Kind {
	case PredAttrEquals:
		for _, a := range ctx.Attributes {
			if nameMatch(p.AttrName, a.Name) && a.Value == p.AttrValue {
				return true
			}
		}
		return false
	case PredAttrExists:
		for _, a := range ctx.Attributes {
			if nameMatch(p.AttrName, a.Name) {
				return true
			}
		}
		return false
	case PredAncestor:
		// The element itself is the last entry of path; ancestors
		// are everything before. Match any ancestor by name.
		if len(ctx.Path) <= 1 {
			return false
		}
		for i := 0; i < len(ctx.Path)-1; i++ {
			if nameMatch(p.AncestorName, ctx.Path[i]) {
				return true
			}
		}
		return false
	}
	return false
}

// nameMatch reports whether `pattern` (which may use "*" for the
// local part) matches `actual`. Namespace URIs must agree exactly:
// patterns without a namespace match only no-namespace actuals,
// because that's the ITS rule selector convention.
func nameMatch(pattern, actual NameMatch) bool {
	if pattern.NamespaceURI != actual.NamespaceURI {
		return false
	}
	if pattern.Local == "*" {
		return true
	}
	return pattern.Local == actual.Local
}

// String renders the parsed selector back into XPath syntax (modulo
// the original prefix bindings). Used for diagnostic output and tests.
func (s *Selector) String() string {
	if s == nil {
		return ""
	}
	parts := make([]string, len(s.Alternates))
	for i, a := range s.Alternates {
		parts[i] = a.String()
	}
	return strings.Join(parts, "|")
}

// String renders one alternate.
func (a Alternate) String() string {
	var b strings.Builder
	for i, step := range a.Steps {
		if i == 0 && !step.Descendant {
			b.WriteByte('/')
		}
		if step.Descendant {
			b.WriteString("//")
		} else if i > 0 {
			b.WriteByte('/')
		}
		b.WriteString(step.Name.String())
	}
	if a.Attribute != nil {
		b.WriteString("/@")
		b.WriteString(a.Attribute.String())
	}
	for _, p := range a.Predicates {
		b.WriteString(p.String())
	}
	return b.String()
}

// String renders one name match.
func (n NameMatch) String() string {
	if n.NamespaceURI != "" {
		return fmt.Sprintf("{%s}%s", n.NamespaceURI, n.Local)
	}
	return n.Local
}

// String renders one predicate.
func (p Predicate) String() string {
	switch p.Kind {
	case PredAttrEquals:
		return fmt.Sprintf("[@%s='%s']", p.AttrName.String(), p.AttrValue)
	case PredAttrExists:
		return fmt.Sprintf("[@%s]", p.AttrName.String())
	case PredAncestor:
		return fmt.Sprintf("[ancestor::%s]", p.AncestorName.String())
	}
	return "[?]"
}
