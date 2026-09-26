package host

import (
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
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
}

// ruleList merges rules into one list, keyed by the wording to use, so a rule
// held in two places is one line.
type ruleList struct {
	rules []ContextRule
	// bySay indexes rules by their folded Say wording.
	bySay map[string]int
	// avoided holds every folded wording some rule already says to avoid.
	avoided map[string]bool
}

func newRuleList() *ruleList {
	return &ruleList{bySay: map[string]int{}, avoided: map[string]bool{}}
}

func fold(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// add merges one rule into the list. A rule whose Say is already listed adds
// its avoided wordings and its note to that line; a bare ban already stated by
// another line is dropped.
func (l *ruleList) add(r ContextRule, from string) {
	r.Say = strings.TrimSpace(r.Say)
	if r.Say == "" {
		var fresh []string
		for _, n := range r.Not {
			if !l.avoided[fold(n)] {
				fresh = append(fresh, n)
			}
		}
		if len(fresh) == 0 {
			return
		}
		r.Not = fresh
	}
	if r.Say != "" {
		if i, held := l.bySay[fold(r.Say)]; held {
			existing := &l.rules[i]
			for _, n := range r.Not {
				if !l.avoided[fold(n)] && fold(n) != fold(existing.Say) {
					existing.Not = append(existing.Not, n)
					l.avoided[fold(n)] = true
				}
			}
			if existing.Note == "" {
				existing.Note = r.Note
			}
			if !slices.Contains(existing.From, from) {
				existing.From = append(existing.From, from)
			}
			return
		}
		l.bySay[fold(r.Say)] = len(l.rules)
	}
	for _, n := range r.Not {
		l.avoided[fold(n)] = true
	}
	r.From = []string{from}
	l.rules = append(l.rules, r)
}

// sayThisNotThat builds the one list for a point from the terms in force there,
// the rules established across the workspace, and the terms the bound voice's
// file carries, in that order, and caps it at limit. It returns the total as well, so a capped
// list says what it is a part of.
func sayThisNotThat(hits []ContextTermHit, binding []coreprofile.TermRule, voice *coreprofile.VoiceProfile, limit int) ([]ContextRule, int) {
	l := newRuleList()

	// One line per concept and language: its preferred wording, the other
	// admitted ones, and the discouraged ones.
	type conceptKey struct{ id, locale string }
	var order []conceptKey
	grouped := map[conceptKey]*ContextRule{}
	for _, h := range hits {
		key := conceptKey{h.ConceptID, h.Locale}
		r, held := grouped[key]
		if !held {
			r = &ContextRule{Locale: h.Locale, Note: h.Definition}
			grouped[key] = r
			order = append(order, key)
		}
		switch {
		case h.Discouraged:
			r.Not = append(r.Not, h.Term)
			if r.Say == "" && h.Replacement != "" {
				r.Say = h.Replacement
			}
		case r.Say == "" || fold(r.Say) == fold(h.Term):
			r.Say = h.Term
		case h.Status == string(model.TermPreferred) && !slices.Contains(r.Also, r.Say):
			r.Also = append(r.Also, r.Say)
			r.Say = h.Term
		default:
			r.Also = append(r.Also, h.Term)
		}
	}
	for _, key := range order {
		r := grouped[key]
		// The replacement a discouraged hit carries may be one of the admitted
		// wordings already listed; it is the Say, never an Also as well.
		r.Also = without(r.Also, r.Say)
		l.add(*r, "terms")
	}

	for _, rule := range binding {
		l.add(ContextRule{Say: rule.Replacement, Not: wordOrNone(rule.Term), Note: rule.Note}, "workspace")
	}

	carried := voice.CarriedTerms()
	for _, t := range carried.Rules {
		if t.Replacement == t.Term {
			// A replacement identical to its term is a convention about how
			// to write the word, not a ban on it.
			l.add(ContextRule{Say: t.Term, Note: t.Note}, carried.From)
			continue
		}
		l.add(ContextRule{Say: t.Replacement, Not: wordOrNone(t.Term), Note: t.Note}, carried.From)
	}

	total := len(l.rules)
	if limit > 0 && total > limit {
		return l.rules[:limit], total
	}
	return l.rules, total
}

func wordOrNone(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	return []string{s}
}

func without(list []string, s string) []string {
	var out []string
	for _, v := range list {
		if fold(v) != fold(s) {
			out = append(out, v)
		}
	}
	return out
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
