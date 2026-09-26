// Package gate implements ship gates — the coverage thresholds that decide when
// content in a language is shippable, selected by rules over (collection, locale).
//
// A Gate is a set of coverage thresholds: state name → minimum percent. A scope
// (a locale, a document, the project) satisfies a gate when, for every threshold
// (state, pct), at least pct% of the scope's units have reached that state or
// higher on the lifecycle ladder. The composite "translated 100, established
// 80" expresses "a person establishes the important 80%, the long tail ships
// translated with its checks green".
//
// Only a person establishes a unit (an agent pre-reviews and never decides),
// so an `established` threshold always counts people, and a threshold is a
// bare percent.
//
// Gates are selected by a RuleSet: an ordered list of rules, each a selector
// (collections and/or locales) plus a gate. The most-specific matching rule wins
// wholesale — the rule matching the most selector axes — with ties broken by
// source order. This keeps cross-axis conflicts visible (you add a more-specific
// rule) rather than hidden behind a precedence convention. See
// strategy/skill-dogfood/convergence-model.md.
package gate

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"sort"

	"github.com/neokapi/neokapi/core/model"
	"gopkg.in/yaml.v3"
)

// Threshold is one gate requirement: the minimum percent of units at (or above)
// a rung.
type Threshold struct {
	Pct int `json:"pct"`
}

// errApproverClass names the fix for a recipe still written in the retired
// {pct, by} form.
func errApproverClass(raw string) error {
	return fmt.Errorf("gate threshold %s: the {pct, by} form is gone, because only a person establishes a unit; write the percent alone, e.g. `established: 100`", raw)
}

// UnmarshalYAML accepts a bare percent (`established: 100`). The retired
// extended form (`{pct: 100, by: human}`) fails with the fix.
func (t *Threshold) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.ScalarNode {
		if node.Kind == yaml.MappingNode {
			return errApproverClass(fmt.Sprintf("at line %d", node.Line))
		}
		return fmt.Errorf("gate threshold: expected a percent, got %v", node.Kind)
	}
	var pct int
	if err := node.Decode(&pct); err != nil {
		return fmt.Errorf("gate threshold: %w", err)
	}
	*t = Threshold{Pct: pct}
	return nil
}

// MarshalYAML encodes the threshold as its percent, so a recipe round-trips
// verbatim.
func (t Threshold) MarshalYAML() (any, error) { return t.Pct, nil }

// UnmarshalJSON accepts a bare number, mirroring YAML.
func (t *Threshold) UnmarshalJSON(data []byte) error {
	var pct int
	if err := json.Unmarshal(data, &pct); err != nil {
		if len(data) > 0 && data[0] == '{' {
			return errApproverClass(string(data))
		}
		return fmt.Errorf("gate threshold: %w", err)
	}
	*t = Threshold{Pct: pct}
	return nil
}

// MarshalJSON emits the percent as a bare number.
func (t Threshold) MarshalJSON() ([]byte, error) { return json.Marshal(t.Pct) }

// Gate is a set of coverage thresholds: state name → threshold (minimum percent
// in [0,100]). A threshold of 0 means "not required".
// An empty Gate is satisfied by anything.
type Gate map[string]Threshold

// Ladder is an ordered list of lifecycle state names, lowest to highest. Rank
// and the "at least" coverage semantics derive from membership and order.
type Ladder []string

// TargetLadder is the ladder for committed translations, derived from the
// canonical model order (draft→translated→established).
func TargetLadder() Ladder {
	statuses := model.TargetStatusLadder()
	l := make(Ladder, len(statuses))
	for i, s := range statuses {
		l[i] = string(s)
	}
	return l
}

// SourceLadder is the ladder for source authoring readiness, derived from the
// canonical model order (written→established). A source gate
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

// Validate checks that every threshold names a ladder state and is a percent
// in [0,100].
func (g Gate) Validate(l Ladder) error {
	for state, th := range g {
		if !l.Has(state) {
			return fmt.Errorf("gate: unknown state %q (ladder: %v)", state, l)
		}
		if th.Pct < 0 || th.Pct > 100 {
			return fmt.Errorf("gate: threshold for %q is %d%%, must be 0..100", state, th.Pct)
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
		c.Add(s)
	}
	return c
}

// Add tallies one unit at state.
func (c *Coverage) Add(state string) {
	if c.Counts == nil {
		c.Counts = map[string]int{}
	}
	c.Total++
	c.Counts[state]++
}

// AtLeastPct returns the percentage of units at `state` or higher on the ladder,
// in [0,100]. With no units the value is 100 (a vacuous scope is fully covered).
func (c Coverage) AtLeastPct(l Ladder, state string) float64 {
	return atLeastPct(c.Counts, c.Total, l, state)
}

// AtLeastCount returns the number of units at `state` or higher on the ladder
// — the raw count behind AtLeastPct, for surfaces that render absolute
// numbers (e.g. "12 of 30 translated") rather than percentages.
func (c Coverage) AtLeastCount(l Ladder, state string) int {
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
	return n
}

func atLeastPct(counts map[string]int, total int, l Ladder, state string) float64 {
	if total == 0 {
		return 100
	}
	target := l.rank(state)
	if target < 0 {
		return 0
	}
	n := 0
	for s, cnt := range counts {
		if r := l.rank(s); r >= target {
			n += cnt
		}
	}
	return 100 * float64(n) / float64(total)
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
	// Progress is how far the scope has come toward the gate, in [0,100]: the
	// mean fractional attainment of the gate's required thresholds, each capped
	// at its requirement. See [Progress].
	Progress int `json:"progress"`
	// Blocking names the lowest unmet rung — the one gate to clear next. Empty
	// when the gate passes.
	Blocking string `json:"blocking,omitempty"`
}

// Progress reports how far a coverage has come toward a gate, in [0,100].
//
// It is the mean, over the gate's required thresholds, of each threshold's
// fractional attainment capped at 1 (exceeding a requirement does not
// compensate for missing another). So a gate of {translated: 100, established:
// 100} against fully translated content nobody has established reads 50%: half the bar
// cleared. A gate with no requirements is vacuously complete at 100.
//
// This is a *distance to the gate*, deliberately not a lifecycle percentage:
// the per-rung percentages are already reported separately, and a single number
// that mixes "how translated" with "how established" would mean nothing. Here the
// number answers exactly one question — how much of the ship bar is left.
func Progress(g Gate, c Coverage, l Ladder) int {
	required := 0
	var sum float64
	for state, th := range g {
		if th.Pct <= 0 {
			continue
		}
		required++
		actual := c.AtLeastPct(l, state)
		sum += min(actual/float64(th.Pct), 1)
	}
	if required == 0 {
		return 100
	}
	return int(math.Round(100 * sum / float64(required)))
}

// Evaluate reports whether the coverage satisfies the gate. A threshold of 0 is
// always met. Shortfalls are returned sorted by ladder rank for stable output.
func Evaluate(g Gate, c Coverage, l Ladder) Result {
	res := Result{Pass: true}
	for state, th := range g {
		if th.Pct <= 0 {
			continue
		}
		actual := c.AtLeastPct(l, state)
		// Compare with a tiny epsilon so exact-percentage coverage (e.g. 2/2 =
		// 100) is not tripped by float rounding.
		if actual+1e-9 < float64(th.Pct) {
			res.Pass = false
			res.Shortfalls = append(res.Shortfalls, Shortfall{State: state, Required: th.Pct, Actual: actual})
		}
	}
	sort.Slice(res.Shortfalls, func(i, j int) bool {
		return l.rank(res.Shortfalls[i].State) < l.rank(res.Shortfalls[j].State)
	})
	res.Progress = Progress(g, c, l)
	// Shortfalls are ladder-ordered, so the first is the lowest unmet rung: the
	// gate to clear next, and the one a verdict should name. Blocking on
	// "established" while "translated" is also short would send someone to the
	// wrong work.
	if len(res.Shortfalls) > 0 {
		res.Blocking = res.Shortfalls[0].State
	}
	return res
}
