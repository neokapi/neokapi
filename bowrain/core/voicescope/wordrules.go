package voicescope

import (
	"context"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
)

// WordRules returns the word rules a workspace's terms store imposes on
// content written in loc (terms.SourceWordRules). An empty loc reads every
// language the store holds terms in, each rule once. A nil store holds none.
func WordRules(ctx context.Context, tb terms.Terminology, loc model.LocaleID) ([]coreprofile.TermRule, error) {
	if tb == nil {
		return nil, nil
	}
	concepts, err := tb.Concepts(ctx)
	if err != nil {
		return nil, err
	}
	if loc != "" {
		return terms.SourceWordRules(concepts, loc), nil
	}
	var out []coreprofile.TermRule
	seen := map[string]bool{}
	for _, l := range termLocales(concepts) {
		for _, r := range terms.SourceWordRules(concepts, l) {
			key := r.ConceptID + "\x00" + strings.ToLower(r.Term)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, r)
		}
	}
	return out, nil
}

// WordRuleSets returns rules as the rule set the core/profile matchers take,
// or none for no rules.
func WordRuleSets(rules []coreprofile.TermRule) []coreprofile.TermRuleSet {
	if len(rules) == 0 {
		return nil
	}
	return []coreprofile.TermRuleSet{{Rules: rules, Kind: coreprofile.VocabForbidden}}
}

// WordRuleCounts counts rules the way the voice surfaces report them: the
// distinct preferred terms the rules point to, the competitor rules, and the
// rest as forbidden.
func WordRuleCounts(rules []coreprofile.TermRule) (preferred, forbidden, competitor int) {
	named := map[string]bool{}
	for _, r := range rules {
		if r.Replacement != "" && !named[strings.ToLower(r.Replacement)] {
			named[strings.ToLower(r.Replacement)] = true
			preferred++
		}
		switch {
		case r.Competitor:
			competitor++
		case r.Term != "":
			forbidden++
		}
	}
	return preferred, forbidden, competitor
}

// termLocales lists the distinct languages the concepts hold terms in, in
// first-seen order.
func termLocales(concepts []terms.Concept) []model.LocaleID {
	var out []model.LocaleID
	seen := map[model.LocaleID]bool{}
	for _, c := range concepts {
		for _, t := range c.Terms {
			l := model.NormalizeLocale(t.Locale)
			if l == "" || seen[l] {
				continue
			}
			seen[l] = true
			out = append(out, l)
		}
	}
	return out
}
