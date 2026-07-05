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
// # Approver class
//
// A threshold optionally carries an approver class (`by`), written in the
// recipe's extended form:
//
//	gates:
//	  ship:
//	    reviewed: {pct: 100, by: human}
//
// The class decides which review decisions count toward a decision rung
// (reviewed / signed-off): "human" (the default) counts only decisions made by
// a person or an agent acting for one, while "any" also counts autonomous AI
// approvals (decisions whose recorded identity is prefixed "ai/", e.g. an AI
// pre-review auto-approval). The legacy short form (`reviewed: 100`) keeps its
// spelling and, like the extended default, requires human review: an
// AI-approved unit reads as reviewed in status displays (with an "(ai)"
// qualifier) but does NOT satisfy a reviewed/signed-off threshold unless the
// gate says `by: any`. This is the deliberate, honest default — an autonomous
// AI approving its own work never silently ships a gate that was written when
// "reviewed" could only mean a person.
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
	"maps"
	"slices"
	"sort"

	"github.com/neokapi/neokapi/core/model"
	"gopkg.in/yaml.v3"
)

// Approver classes for a threshold's By axis.
const (
	// ByHuman counts only human (or human-directed agent) decisions toward a
	// decision rung — the default for reviewed/signed-off thresholds.
	ByHuman = "human"
	// ByAny counts every decision, including autonomous AI approvals
	// (identities prefixed "ai/").
	ByAny = "any"
)

// Threshold is one gate requirement: the minimum percent of units at (or above)
// a rung, plus the approver class the rung must be reached by. The zero By
// means the default class: human for decision rungs (see the package doc).
type Threshold struct {
	Pct int    `json:"pct"`
	By  string `json:"by,omitempty"`
}

// thresholdYAML is the extended-form mapping shape ({pct: 100, by: human}).
type thresholdYAML struct {
	Pct int    `yaml:"pct" json:"pct"`
	By  string `yaml:"by,omitempty" json:"by,omitempty"`
}

// UnmarshalYAML accepts the short form (a bare percent: `reviewed: 100`) or the
// extended form (`reviewed: {pct: 100, by: human}`).
func (t *Threshold) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.ScalarNode:
		var pct int
		if err := node.Decode(&pct); err != nil {
			return fmt.Errorf("gate threshold: %w", err)
		}
		*t = Threshold{Pct: pct}
		return nil
	case yaml.MappingNode:
		var m thresholdYAML
		if err := node.Decode(&m); err != nil {
			return fmt.Errorf("gate threshold: %w", err)
		}
		*t = Threshold(m)
		return nil
	default:
		return fmt.Errorf("gate threshold: expected a percent or a {pct, by} map, got %v", node.Kind)
	}
}

// MarshalYAML re-encodes the short form when only a percent is set, so a recipe
// round-trips verbatim; the extended form is kept when By is set.
func (t Threshold) MarshalYAML() (any, error) {
	if t.By == "" {
		return t.Pct, nil
	}
	return thresholdYAML(t), nil
}

// UnmarshalJSON accepts a bare number or a {pct, by} object, mirroring YAML.
func (t *Threshold) UnmarshalJSON(data []byte) error {
	var pct int
	if err := json.Unmarshal(data, &pct); err == nil {
		*t = Threshold{Pct: pct}
		return nil
	}
	var m thresholdYAML
	if err := json.Unmarshal(data, &m); err != nil {
		return fmt.Errorf("gate threshold: %w", err)
	}
	*t = Threshold(m)
	return nil
}

// MarshalJSON emits a bare number for the short form, an object otherwise.
func (t Threshold) MarshalJSON() ([]byte, error) {
	if t.By == "" {
		return json.Marshal(t.Pct)
	}
	return json.Marshal(thresholdYAML(t))
}

// Gate is a set of coverage thresholds: state name → threshold (minimum percent
// in [0,100], optional approver class). A threshold of 0 means "not required".
// An empty Gate is satisfied by anything.
type Gate map[string]Threshold

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

// Validate checks that every threshold names a ladder state, is a percent in
// [0,100], and carries a known approver class (empty, human, or any).
func (g Gate) Validate(l Ladder) error {
	for state, th := range g {
		if !l.Has(state) {
			return fmt.Errorf("gate: unknown state %q (ladder: %v)", state, l)
		}
		if th.Pct < 0 || th.Pct > 100 {
			return fmt.Errorf("gate: threshold for %q is %d%%, must be 0..100", state, th.Pct)
		}
		if th.By != "" && th.By != ByHuman && th.By != ByAny {
			return fmt.Errorf("gate: threshold for %q has approver class %q, must be %q or %q", state, th.By, ByHuman, ByAny)
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
//
// Counts is the effective distribution — AI-promoted units (an "ai/…" review
// decision) tally at their decided rung, which is what status displays show.
// HumanCounts tallies the same units at the rung they hold counting only human
// decisions: an AI-promoted unit reads at its pre-decision baseline there
// (typically `translated`). A `by: human` threshold — the default — evaluates
// against HumanCounts; `by: any` evaluates against Counts.
type Coverage struct {
	Total  int            `json:"total"`
	Counts map[string]int `json:"counts"`
	// HumanCounts is the human-decisions-only distribution. Nil when the scope
	// has no AI-decided units (the two distributions are identical then).
	HumanCounts map[string]int `json:"humanCounts,omitempty"`
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

// Add tallies one unit whose state was reached without an AI decision (or with
// no decision at all).
func (c *Coverage) Add(state string) {
	if c.Counts == nil {
		c.Counts = map[string]int{}
	}
	c.Total++
	c.Counts[state]++
	if c.HumanCounts != nil {
		c.HumanCounts[state]++
	}
}

// AddAIDecided tallies one unit whose state was reached by an AI decision (an
// "ai/…" identity): it counts at `state` in the effective distribution and at
// `baseline` (the rung it held before the AI decision, typically `translated`)
// in the human-only distribution.
func (c *Coverage) AddAIDecided(state, baseline string) {
	if c.Counts == nil {
		c.Counts = map[string]int{}
	}
	if c.HumanCounts == nil {
		// Materialize the human-only view lazily, seeded from everything
		// tallied so far (all human until now).
		c.HumanCounts = make(map[string]int, len(c.Counts)+1)
		maps.Copy(c.HumanCounts, c.Counts)
	}
	c.Total++
	c.Counts[state]++
	c.HumanCounts[baseline]++
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

// AtLeastPctBy returns the "at least" percentage counting only the decisions
// the approver class admits: ByAny sees the effective distribution, while
// ByHuman (and the empty default) sees the human-only one, where AI-promoted
// units read at their pre-decision baseline.
func (c Coverage) AtLeastPctBy(l Ladder, state, by string) float64 {
	if by == ByAny || c.HumanCounts == nil {
		return c.AtLeastPct(l, state)
	}
	return atLeastPct(c.HumanCounts, c.Total, l, state)
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
	// By is the threshold's approver class when explicitly set ("human" or
	// "any"), so a report can say which class fell short. Empty for the
	// default (human) class.
	By string `json:"by,omitempty"`
}

// Result is the outcome of evaluating a gate against a coverage.
type Result struct {
	Pass       bool        `json:"pass"`
	Shortfalls []Shortfall `json:"shortfalls,omitempty"`
}

// Evaluate reports whether the coverage satisfies the gate. A threshold of 0 is
// always met. A threshold's approver class decides which decisions count: the
// default (human) excludes AI-promoted units from decision rungs; `by: any`
// admits them (see the package doc). Shortfalls are returned sorted by ladder
// rank for stable output.
func Evaluate(g Gate, c Coverage, l Ladder) Result {
	res := Result{Pass: true}
	for state, th := range g {
		if th.Pct <= 0 {
			continue
		}
		actual := c.AtLeastPctBy(l, state, th.By)
		// Compare with a tiny epsilon so exact-percentage coverage (e.g. 2/2 =
		// 100) is not tripped by float rounding.
		if actual+1e-9 < float64(th.Pct) {
			res.Pass = false
			res.Shortfalls = append(res.Shortfalls, Shortfall{State: state, Required: th.Pct, Actual: actual, By: th.By})
		}
	}
	sort.Slice(res.Shortfalls, func(i, j int) bool {
		return l.rank(res.Shortfalls[i].State) < l.rank(res.Shortfalls[j].State)
	})
	return res
}
