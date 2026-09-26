package profile

import (
	"fmt"
	"slices"
	"strings"
)

// ApplySuggestedRule promotes a correction-derived rule into a list of word
// rules: the term a team kept correcting away becomes a rule whose replacement
// is what they corrected it to. A correction made once becomes a
// deterministic check on every future piece of content.
//
// It is idempotent: promoting the same term again updates the existing rule's
// replacement, concept and provenance note instead of adding a duplicate. It
// returns the list, fresh when it changed, and whether it changed.
func ApplySuggestedRule(rules []TermRule, r SuggestedRule) ([]TermRule, bool) {
	if strings.TrimSpace(r.Term) == "" {
		return rules, false
	}
	note := provenanceNote(r.CorrectionCount)
	for i := range rules {
		if !strings.EqualFold(rules[i].Term, r.Term) {
			continue
		}
		rule := rules[i]
		changed := false
		if r.Replacement != "" && rule.Replacement != r.Replacement {
			rule.Replacement = r.Replacement
			changed = true
		}
		// Carry the concept forward when the suggestion is concept-backed: a
		// re-promotion can attach (or re-point) the concept on an existing rule
		// that was first promoted standalone.
		if r.ConceptID != "" && rule.ConceptID != r.ConceptID {
			rule.ConceptID = r.ConceptID
			changed = true
		}
		if rule.Note != note {
			rule.Note = note
			changed = true
		}
		if !changed {
			return rules, false
		}
		out := slices.Clone(rules)
		out[i] = rule
		return out, true
	}
	out := append(slices.Clone(rules), TermRule{
		Term:        r.Term,
		Replacement: r.Replacement,
		Note:        note,
		ConceptID:   r.ConceptID,
	})
	return out, true
}

// RemoveRule removes the rule for term from a list of word rules, and reports
// whether the list changed. The inverse of ApplySuggestedRule.
func RemoveRule(rules []TermRule, term string) ([]TermRule, bool) {
	if strings.TrimSpace(term) == "" {
		return rules, false
	}
	kept := make([]TermRule, 0, len(rules))
	for _, t := range rules {
		if strings.EqualFold(t.Term, term) {
			continue
		}
		kept = append(kept, t)
	}
	if len(kept) == len(rules) {
		return rules, false
	}
	return kept, true
}

// PromotionNote is the note a promoted rule carries: how many corrections it
// was promoted from.
func PromotionNote(count int) string { return provenanceNote(count) }

func provenanceNote(count int) string {
	if count == 1 {
		return "promoted from 1 correction"
	}
	return fmt.Sprintf("promoted from %d corrections", count)
}
