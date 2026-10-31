package host

import (
	"strings"

	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/rulesview"
)

// The word rules a writer keeps to at one point, as one list.
//
// A rule about a word is a term, held in one of three places: the terms store
// (a concept with a preferred term and discouraged ones), the rules established
// across the workspace, and a starter pack bound as the voice, whose terms
// apply beside the project's own. A writer asks one question of all of them,
// "what do I say, and what not", so the answer merges them into one list at
// render time and states each word once.

// ContextRule is one line of that list: the wording to use, the wording to
// avoid, and what the rule is about.
type ContextRule struct {
	// Say is the wording to use. Empty for a rule that only bans a word.
	Say string `json:"say,omitempty"`
	// Also are other admitted wordings for the same thing.
	Also []string `json:"also,omitempty"`
	// Not are the wordings to avoid.
	Not []string `json:"not,omitempty"`
	// Note says what the rule is about: the concept's definition or the
	// rule's note.
	Note string `json:"note,omitempty"`
	// Locale is the language the rule is stated in, empty for a rule held
	// outside the terms store.
	Locale string `json:"locale,omitempty"`
	// From names where the rule is held: `terms`, `workspace`, or the pack
	// (`pack technical-docs`) or voice file whose terms apply.
	From []string `json:"from"`
	// Advisory marks a rule a check reports a use of the avoided wording
	// under and fails nothing on.
	Advisory bool `json:"advisory,omitempty"`
}

func fold(s string) string { return rulesview.FoldWording(s) }

// FoldWording folds a wording the way a by-location answer compares two: in
// lower case, with the space around it taken off. A surface that matches its
// own rows against the answer's rules, as the text answer does when it leaves
// out a suggestion a rule already states, folds with it.
func FoldWording(s string) string { return fold(s) }

// sayThisNotThat builds the one list for a point from the terms in force
// there, the rules established across the workspace, and the terms the bound
// voice's file carries, in that order, and caps it at limit
// (rulesview.SayThisNotThat). It returns the total as well, so a capped list
// says what it is a part of.
func sayThisNotThat(hits []ContextTermHit, binding []coreprofile.TermRule, voice *coreprofile.VoiceProfile, limit int) ([]ContextRule, int) {
	in := make([]rulesview.TermHit, len(hits))
	for i, h := range hits {
		in[i] = rulesview.TermHit{
			ConceptID: h.ConceptID, Term: h.Term, Locale: h.Locale, Status: h.Status, Definition: h.Definition,
			Domain: h.Domain, Discouraged: h.Discouraged, Replacement: h.Replacement, Advisory: h.Advisory,
		}
	}
	rules, total := rulesview.SayThisNotThat(in, binding, voice, limit)
	out := make([]ContextRule, len(rules))
	for i, r := range rules {
		out[i] = contextRuleFrom(r)
	}
	return out, total
}

// contextRuleFrom shapes one word rule for the answer.
func contextRuleFrom(r rulesview.WordRule) ContextRule {
	return ContextRule{Say: r.Say, Also: r.Also, Not: r.Not, Note: r.Note, Locale: r.Locale, From: r.From, Advisory: r.Advisory}
}

// ruleLine renders one rule as a line of the list: the wording to use, the
// wording to avoid, then what it is about. showLocale adds the language when
// the list spans more than one.
func ruleLine(r ContextRule, showLocale bool) string {
	var b strings.Builder
	b.WriteString("- ")
	switch {
	case r.Say != "" && len(r.Not) > 0:
		b.WriteString(r.Say + ", not " + quotedAlternatives(r.Not))
	case r.Say != "":
		b.WriteString(r.Say)
	default:
		b.WriteString("Avoid " + quotedAlternatives(r.Not))
	}
	if len(r.Also) > 0 {
		b.WriteString(" (also " + quotedAlternatives(r.Also) + ")")
	}
	if showLocale && r.Locale != "" {
		b.WriteString(" [" + r.Locale + "]")
	}
	if note := strings.TrimSpace(r.Note); note != "" {
		b.WriteString(": " + note)
	}
	return b.String()
}

// quotedAlternatives renders wordings as `"a"`, `"a" or "b"`, or
// `"a", "b" or "c"`.
func quotedAlternatives(words []string) string {
	q := make([]string, len(words))
	for i, w := range words {
		q[i] = `"` + w + `"`
	}
	if len(q) == 1 {
		return q[0]
	}
	return strings.Join(q[:len(q)-1], ", ") + " or " + q[len(q)-1]
}

// rulesSpanLocales reports whether the list holds rules in more than one
// language, which is when a line names its own.
func rulesSpanLocales(rules []ContextRule) bool {
	seen := ""
	for _, r := range rules {
		if r.Locale == "" {
			continue
		}
		if seen != "" && r.Locale != seen {
			return true
		}
		seen = r.Locale
	}
	return false
}
