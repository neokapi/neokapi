package mdspike

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
)

// SourceVocabulary derives profile vocabulary rules from the terms store
// instead of restating them in the profile.
//
// The mapping is the terms store's own lifecycle vocabulary:
//
//   - a term whose status is in src.Prefer becomes a preferred term, carrying
//     the concept definition as its note;
//   - a term whose status is in src.Forbid becomes a forbidden term, whose
//     replacement is the concept's preferred term in the same locale — so the
//     profile says "deprecated X, use preferred Y" without anyone restating
//     either half;
//   - a term flagged CompetitorTerm in the store becomes a competitor term,
//     whatever its status.
//
// Every rule carries the concept ID, so a finding raised by MatchVocabulary can
// pivot back to the concept — the ConceptID field that exists today only when
// the platform promotes a rule from a correction.
//
// Output is deterministic: rules are sorted by term text within each list.
func SourceVocabulary(ctx context.Context, store terms.Terminology, src TermsSource, scope *graph.Scope) (profile.VocabularyRules, error) {
	var out profile.VocabularyRules
	if store == nil {
		return out, errors.New("terms store is nil")
	}
	locale := src.Locale
	if locale == "" {
		locale = model.LocaleID("en")
	}

	concepts, err := store.Concepts(ctx)
	if err != nil {
		return out, fmt.Errorf("list concepts: %w", err)
	}

	for _, c := range concepts {
		if len(src.Domains) > 0 && !slices.Contains(src.Domains, c.Domain) {
			continue
		}
		preferred := ""
		if pt := c.PreferredTerm(locale); pt != nil {
			preferred = pt.Text
		}
		for _, t := range c.Terms {
			if t.Locale != locale || !terms.MatchesScope(t.Validity, scope) {
				continue
			}
			rule := profile.TermRule{
				Term:      t.Text,
				Note:      firstNonEmpty(t.Note, c.Definition),
				ConceptID: c.ID,
			}
			switch {
			case t.CompetitorTerm:
				rule.Severity = src.CompetitorSeverity
				rule.Replacement = preferred
				out.CompetitorTerms = append(out.CompetitorTerms, rule)
			case slices.Contains(src.Forbid, t.Status):
				rule.Severity = src.ForbidSeverity
				if preferred != t.Text {
					rule.Replacement = preferred
				}
				out.ForbiddenTerms = append(out.ForbiddenTerms, rule)
			case slices.Contains(src.Prefer, t.Status):
				out.PreferredTerms = append(out.PreferredTerms, rule)
			}
		}
	}

	sortRules(out.PreferredTerms)
	sortRules(out.ForbiddenTerms)
	sortRules(out.CompetitorTerms)
	return out, nil
}

func sortRules(rules []profile.TermRule) {
	sort.SliceStable(rules, func(i, j int) bool { return rules[i].Term < rules[j].Term })
}
