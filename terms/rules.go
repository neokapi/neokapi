package terms

import (
	"strings"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/profile"
)

// RulesFromConcepts derives the term rules a set of concepts imposes on content
// written in source and translated into target, in concept order. It is the one
// derivation every surface uses, so the CLI gate, the platform's jobs and the
// evaluation hold content to the same renderings.
func RulesFromConcepts(concepts []Concept, source, target model.LocaleID) []profile.TermRule {
	var rules []profile.TermRule
	for _, c := range concepts {
		if rule, ok := RuleForConcept(c, source, target); ok {
			rules = append(rules, rule)
		}
	}
	return rules
}

// RuleForConcept derives one concept's term rule, and reports false when the
// concept imposes nothing on this language pair.
//
// The rule is keyed on the concept's head term in the source language and
// carries that term's forms. A do-not-translate concept yields a rule marked so,
// with no replacement: its claim is that the source term is the same string in
// every locale. Any other concept needs a term in the target language. Its
// preferred term becomes Replacement, the wording a translation is asked to use.
// Every other admitted, approved or unmarked term there becomes an accepted
// rendering, because a reviewer who chose an admitted term chose a term the
// concept allows. Proposed, deprecated and forbidden terms are never renderings.
func RuleForConcept(c Concept, source, target model.LocaleID) (profile.TermRule, bool) {
	src := c.HeadTerm(source)
	if src == nil || strings.TrimSpace(src.Text) == "" {
		return profile.TermRule{}, false
	}
	rule := profile.TermRule{
		Term:      src.Text,
		Forms:     NormalizeForms(src.Text, src.Forms),
		ConceptID: c.ID,
	}
	if c.DoNotTranslate {
		rule.DoNotTranslate = true
		return rule, true
	}

	tgt := c.PreferredTerm(target)
	if tgt == nil || strings.TrimSpace(tgt.Text) == "" {
		return profile.TermRule{}, false
	}
	rule.Replacement = tgt.Text
	rule.ReplacementForms = NormalizeForms(tgt.Text, tgt.Forms)
	for _, t := range c.Terms {
		if !sameLocale(t.Locale, target) || strings.EqualFold(t.Text, tgt.Text) || !acceptedRendering(t.Status) {
			continue
		}
		rule.Accepted = append(rule.Accepted, profile.Rendering{Text: t.Text, Forms: NormalizeForms(t.Text, t.Forms)})
	}
	return rule, true
}

// acceptedRendering reports whether a term with this status satisfies a rule.
// An unmarked status is accepted because PreferredTerm already treats an
// unmarked term as usable.
func acceptedRendering(s model.TermStatus) bool {
	switch s {
	case model.TermPreferred, model.TermAdmitted, model.TermApproved, "":
		return true
	}
	return false
}
