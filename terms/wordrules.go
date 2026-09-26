package terms

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
)

// SourceWordRules are the word rules a set of concepts imposes on content
// written in loc: one rule for each forbidden, retired or competitor term the
// concepts hold in that language (or the language beneath it), with the
// concept's preferred term as the replacement. A retired term's rule is
// advisory, and so is every rule of an advisory concept.
//
// It is the terms store's reading as a list of rules, for a surface that shows
// or counts them, or rewrites text by them. A check locates store terms with
// Locate instead, which also matches fuzzily.
func SourceWordRules(concepts []Concept, loc model.LocaleID) []profile.TermRule {
	langs := LookupLocales(loc)
	var out []profile.TermRule
	for _, c := range concepts {
		for _, t := range c.Terms {
			if !inLocales(t.Locale, langs) {
				continue
			}
			retired := t.Status == model.TermDeprecated
			if !t.CompetitorTerm && t.Status != model.TermForbidden && !retired {
				continue
			}
			rule := profile.TermRule{
				Term:       t.Text,
				Forms:      NormalizeForms(t.Text, t.Forms),
				Note:       t.Note,
				Competitor: t.CompetitorTerm,
				Advisory:   c.Advisory || retired,
				ConceptID:  c.ID,
			}
			if pref := c.PreferredTerm(t.Locale); pref != nil && !strings.EqualFold(pref.Text, t.Text) {
				rule.Replacement = pref.Text
			} else {
				rule.Replacement = ReplacementFromNote(t.Note)
			}
			if ReplacementFromNote(rule.Note) != "" {
				rule.Note = ""
			}
			out = append(out, rule)
		}
	}
	return out
}

func inLocales(loc model.LocaleID, langs []model.LocaleID) bool {
	for _, l := range langs {
		if sameLocale(loc, l) {
			return true
		}
	}
	return false
}

// PromoteRule lands a correction-derived rule in a terms store: the term a
// team kept correcting away becomes a forbidden term in loc, joined to the
// concept whose preferred term is what they corrected it to. It reports
// whether the store changed; promoting the same rule again changes nothing.
func PromoteRule(ctx context.Context, store Terminology, loc model.LocaleID, r profile.SuggestedRule) (bool, error) {
	if strings.TrimSpace(r.Term) == "" {
		return false, nil
	}
	concepts, err := store.Concepts(ctx)
	if err != nil {
		return false, fmt.Errorf("read the terms: %w", err)
	}
	concepts, target, changed := UpsertDecision(concepts, Decision{
		Text:        r.Term,
		Locale:      loc,
		Status:      model.TermForbidden,
		Replacement: r.Replacement,
	})
	c := &concepts[target]
	if r.CorrectionCount > 0 {
		count := strconv.Itoa(r.CorrectionCount)
		if c.Properties[PropPromotedFrom] != count {
			if c.Properties == nil {
				c.Properties = map[string]string{}
			}
			c.Properties[PropPromotedFrom] = count
			changed = true
		}
	}
	if !changed {
		return false, nil
	}
	if err := store.AddConcept(ctx, *c); err != nil {
		return false, fmt.Errorf("write concept %s: %w", c.ID, err)
	}
	return true, nil
}

// onlyPreferredIn reports whether terms hold at most one term, preferred and
// in loc: what a promotion leaves once its term is gone.
func onlyPreferredIn(terms []Term, loc model.LocaleID) bool {
	switch len(terms) {
	case 0:
		return true
	case 1:
		return terms[0].Status == model.TermPreferred && !terms[0].CompetitorTerm && sameLocale(terms[0].Locale, loc)
	}
	return false
}

// PropPromotedFrom is the concept property recording how many corrections a
// promoted rule came from.
const PropPromotedFrom = "promoted_from_corrections"

// DemoteRule removes a term from a terms store: the inverse of PromoteRule. The
// term goes and its concept stays, unless a promotion opened the concept for
// the term and nothing but the preferred form it was promoted to remains. A
// concept the promotion joined was there before it, and stays. It reports
// whether the store changed.
func DemoteRule(ctx context.Context, store Terminology, loc model.LocaleID, term string) (bool, error) {
	concepts, err := store.Concepts(ctx)
	if err != nil {
		return false, fmt.Errorf("read the terms: %w", err)
	}
	loc = model.NormalizeLocale(loc)
	ci := IndexOfTerm(concepts, term, loc)
	if ci < 0 {
		return false, nil
	}
	c := concepts[ci]
	ti := TermIndex(&c, term, loc)
	c.Terms = append(c.Terms[:ti], c.Terms[ti+1:]...)
	// A concept the promotion opened carries the id a decision on the term
	// opens; one it joined carries its own.
	opened := c.ID == DecisionConceptID(term, loc) && c.Properties[PropPromotedFrom] != ""
	if opened && onlyPreferredIn(c.Terms, loc) {
		// The concept existed for the promotion alone.
		if err := store.DeleteConcept(ctx, c.ID); err != nil {
			return false, fmt.Errorf("remove concept %s: %w", c.ID, err)
		}
		return true, nil
	}
	if err := store.AddConcept(ctx, c); err != nil {
		return false, fmt.Errorf("write concept %s: %w", c.ID, err)
	}
	return true, nil
}
