// Package gate implements ship gates — the coverage thresholds that decide when
// localized content is shippable, selected by rules over (collection, locale).
//
// A Gate is a set of coverage thresholds: state name → minimum percent. A scope
// (a locale, a document, the project) satisfies a gate when, for every threshold
// (state, pct), at least pct% of the scope's units have reached that state or
// higher on the lifecycle ladder. The plain "100% reviewed" is the degenerate
// case; the composite "translated 100, reviewed 80" expresses "review the
// important 80%, ship the long tail machine-translated".
//
// Gates are selected by a RuleSet: an ordered list of rules, each a selector
// (collections and/or locales) plus a gate. The most-specific matching rule wins
// wholesale — the rule matching the most selector axes — with ties broken by
// source order. This keeps cross-axis conflicts visible (you add a more-specific
// rule) rather than hidden behind a precedence convention. See
// strategy/skill-dogfood/convergence-model.md.
package gate

import (
	"fmt"
	"slices"
	"sort"

	"github.com/neokapi/neokapi/core/model"
)

// Gate is a set of coverage thresholds: state name → minimum percent in [0,100].
// A threshold of 0 means "not required". An empty Gate is satisfied by anything.
type Gate map[string]int

// Ladder is an ordered list of lifecycle state names, lowest to highest. Rank
// and the "at least" coverage semantics derive from membership and order.
type Ladder []string

// TargetLadder is the ladder for committed translations, derived from the
// canonical model order (draft → translated → reviewed → signed-off).
func TargetLadder() Ladder {
	statuses := model.TargetStatusLadder()
	l := make(Ladder, len(statuses))
	for i, s := range statuses {
		l[i] = string(s)
	}
	return l
}

// SourceLadder is the ladder for source authoring readiness, derived from the
// canonical model order (authored → checked → approved). A source gate
// (project source_gate) evaluates coverage against it, mirroring TargetLadder.
func SourceLadder() Ladder {
	statuses := model.SourceStatusLadder()
	l := make(Ladder, len(statuses))
	for i, s := range statuses {
		l[i] = string(s)
	}
	return l
}

// rank returns the 0-based position of a state on the ladder, or -1 if unknown.
func (l Ladder) rank(state string) int {
	for i, s := range l {
		if s == state {
			return i
		}
	}
	return -1
}

// Has reports whether state is a rung on the ladder.
func (l Ladder) Has(state string) bool { return l.rank(state) >= 0 }

// Validate checks that every threshold names a ladder state and is a percent in
// [0,100].
func (g Gate) Validate(l Ladder) error {
	for state, pct := range g {
		if !l.Has(state) {
			return fmt.Errorf("gate: unknown state %q (ladder: %v)", state, l)
		}
		if pct < 0 || pct > 100 {
			return fmt.Errorf("gate: threshold for %q is %d%%, must be 0..100", state, pct)
		}
	}
	return nil
}

// Selector matches units by collection and/or locale. An empty axis matches all
// (so an all-empty Selector is the catch-all default rule). Specificity is the
// number of constrained axes (0, 1, or 2).
type Selector struct {
	Collections []string `yaml:"collections,omitempty" json:"collections,omitempty"`
	Locales     []string `yaml:"locales,omitempty" json:"locales,omitempty"`
}

// Matches reports whether the selector applies to a (collection, locale) unit.
func (s Selector) Matches(collection, locale string) bool {
	if len(s.Collections) > 0 && !slices.Contains(s.Collections, collection) {
		return false
	}
	if len(s.Locales) > 0 && !slices.Contains(s.Locales, locale) {
		return false
	}
	return true
}

// specificity counts the constrained axes.
func (s Selector) specificity() int {
	n := 0
	if len(s.Collections) > 0 {
		n++
	}
	if len(s.Locales) > 0 {
		n++
	}
	return n
}

// Rule is a selector plus the gate that applies where it matches. The gate is
// already resolved (any registry-name reference expanded at load time).
type Rule struct {
	When Selector
	Gate Gate
}

// RuleSet is an ordered list of rules. Resolution picks the most-specific
// matching rule; ties break by source order (earliest wins).
type RuleSet struct {
	Rules []Rule
}

// Resolve returns the gate for a (collection, locale) unit and whether a rule
// matched. When several rules match, the one constraining the most axes wins;
// among equal specificity, the earliest-listed wins.
func (rs RuleSet) Resolve(collection, locale string) (Gate, bool) {
	bestIdx := -1
	bestSpec := -1
	for i, r := range rs.Rules {
		if !r.When.Matches(collection, locale) {
			continue
		}
		if spec := r.When.specificity(); spec > bestSpec {
			bestSpec = spec
			bestIdx = i
		}
	}
	if bestIdx < 0 {
		return nil, false
	}
	return rs.Rules[bestIdx].Gate, true
}

// Validate checks every rule's gate against the ladder.
func (rs RuleSet) Validate(l Ladder) error {
	for i, r := range rs.Rules {
		if err := r.Gate.Validate(l); err != nil {
			return fmt.Errorf("rule %d: %w", i, err)
		}
	}
	return nil
}

// Coverage is the state distribution of a scope: a count per state plus the
// total number of units. Counts are keyed by exact state; the "at least"
// rollup is computed against a ladder.
type Coverage struct {
	Total  int            `json:"total"`
	Counts map[string]int `json:"counts"`
}

// NewCoverage tallies the states of a scope's units. An empty state ("" / New)
// is counted but never reaches a ladder rung.
func NewCoverage(states []string) Coverage {
	c := Coverage{Counts: map[string]int{}}
	for _, s := range states {
		c.Total++
		c.Counts[s]++
	}
	return c
}

// AtLeastPct returns the percentage of units at `state` or higher on the ladder,
// in [0,100]. With no units the value is 100 (a vacuous scope is fully covered).
func (c Coverage) AtLeastPct(l Ladder, state string) float64 {
	if c.Total == 0 {
		return 100
	}
	target := l.rank(state)
	if target < 0 {
		return 0
	}
	n := 0
	for s, cnt := range c.Counts {
		if r := l.rank(s); r >= target {
			n += cnt
		}
	}
	return 100 * float64(n) / float64(c.Total)
}

// Shortfall is one unmet gate threshold.
type Shortfall struct {
	State    string  `json:"state"`
	Required int     `json:"required"` // percent
	Actual   float64 `json:"actual"`   // percent
}

// Result is the outcome of evaluating a gate against a coverage.
type Result struct {
	Pass       bool        `json:"pass"`
	Shortfalls []Shortfall `json:"shortfalls,omitempty"`
}

// Evaluate reports whether the coverage satisfies the gate. A threshold of 0 is
// always met. Shortfalls are returned sorted by ladder rank for stable output.
func Evaluate(g Gate, c Coverage, l Ladder) Result {
	res := Result{Pass: true}
	for state, pct := range g {
		if pct <= 0 {
			continue
		}
		actual := c.AtLeastPct(l, state)
		// Compare with a tiny epsilon so exact-percentage coverage (e.g. 2/2 =
		// 100) is not tripped by float rounding.
		if actual+1e-9 < float64(pct) {
			res.Pass = false
			res.Shortfalls = append(res.Shortfalls, Shortfall{State: state, Required: pct, Actual: actual})
		}
	}
	sort.Slice(res.Shortfalls, func(i, j int) bool {
		return l.rank(res.Shortfalls[i].State) < l.rank(res.Shortfalls[j].State)
	})
	return res
}
