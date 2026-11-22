package rulesview

import (
	"fmt"
	"sort"
	"strings"

	coreprofile "github.com/neokapi/neokapi/core/profile"
)

// RuleTally counts the rules in force across the places of a project, each
// rule once however many places hold it. It is the rule count an Overview
// reports: `kapi look`, the desktop and the platform add each place's rules
// to one tally, so the three report the same number for the same project.
//
// A place contributes its word rules (the list [SayThisNotThat] builds there),
// the notes a person kept that hold there, and the pattern rules of the voice
// bound there: the patterns [coreprofile.PatternRuleCount] counts.
type RuleTally struct {
	keys map[string]bool
}

// AddWordRule adds one word rule, keyed by its folded wording to use, the
// folded wordings to avoid and its language.
func (t *RuleTally) AddWordRule(say string, not []string, locale string) {
	folded := make([]string, len(not))
	for i, n := range not {
		folded[i] = FoldWording(n)
	}
	sort.Strings(folded)
	t.add("rule|" + FoldWording(say) + "|" + strings.Join(folded, ",") + "|" + locale)
}

// AddWordRules adds each rule of a place's word-rule list.
func (t *RuleTally) AddWordRules(rules []WordRule) {
	for _, r := range rules {
		t.AddWordRule(r.Say, r.Not, r.Locale)
	}
}

// AddNote adds a note a person kept, by the id of the operation that records
// it.
func (t *RuleTally) AddNote(operation string) {
	t.add("note|" + operation)
}

// AddVoice adds the pattern rules of the voice bound at a place: its
// prohibited and required patterns and its applicable prohibited-pattern
// constraints. A pattern with no expression is no rule.
func (t *RuleTally) AddVoice(v *coreprofile.VoiceProfile) {
	if v == nil {
		return
	}
	for _, list := range [][]coreprofile.Pattern{v.Style.ProhibitedPatterns, v.Style.RequiredPatterns} {
		for _, p := range list {
			if strings.TrimSpace(p.Regex) != "" {
				t.add("voice|" + v.Name + "|" + p.Regex)
			}
		}
	}
	for _, r := range coreprofile.ConstraintResolutions(v) {
		if r.Status == "applicable" && r.Constraint.Kind == coreprofile.ConstraintProhibitedPattern {
			t.add(fmt.Sprintf("constraint|%s|%s|%d", v.Name, r.Constraint.ID, r.Constraint.Version))
		}
	}
}

// Len is the number of rules the tally holds.
func (t *RuleTally) Len() int {
	return len(t.keys)
}

func (t *RuleTally) add(key string) {
	if t.keys == nil {
		t.keys = map[string]bool{}
	}
	t.keys[key] = true
}
