package rulesview

import (
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/terms"
)

// TermHit is one term of a concept in force at a point.
type TermHit struct {
	ConceptID  string
	Term       string
	Locale     string
	Status     string
	Definition string
	Domain     string
	// Discouraged reports a term the concept says to avoid, Replacement what
	// to say instead, and Advisory a concept whose check fails nothing.
	Discouraged bool
	Replacement string
	Advisory    bool
	// Validity is the term's own time scoping, when it has any.
	Validity *graph.Validity
}

// TermsInForce projects the concepts bound at a point into hits: the terms
// whose own window covers the instant, in locale (every locale when empty),
// discouraged ones first, then by wording. It caps the list at limit and
// returns the total in force as well, so a capped list says what it is a
// part of.
//
// The discouraged-first order is the useful one when the list is capped. A
// rule a writer must not break outranks a word it may reuse.
func TermsInForce(concepts []terms.Concept, locale model.LocaleID, at time.Time, limit int) ([]TermHit, int) {
	var all []TermHit
	for _, c := range concepts {
		for _, t := range c.Terms {
			if locale != "" && t.Locale != locale {
				continue
			}
			if !at.IsZero() && !t.Validity.Matches(graph.ScopeAt(at)) {
				continue
			}
			hit := TermHit{
				ConceptID:   c.ID,
				Term:        t.Text,
				Locale:      string(t.Locale),
				Status:      string(t.Status),
				Definition:  c.Definition,
				Domain:      c.Domain,
				Discouraged: t.Status.Discouraged(),
				Validity:    t.Validity,
			}
			if hit.Discouraged {
				hit.Replacement = DiscouragedReplacement(c, t)
				hit.Advisory = c.Advisory
			}
			all = append(all, hit)
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if ri, rj := termRank(all[i]), termRank(all[j]); ri != rj {
			return ri < rj
		}
		if a, b := strings.ToLower(all[i].Term), strings.ToLower(all[j].Term); a != b {
			return a < b
		}
		return all[i].Locale < all[j].Locale
	})
	total := len(all)
	if limit > 0 && len(all) > limit {
		all = all[:limit]
	}
	return all, total
}

// termRank orders a term: a rule to act on, then a word to reuse, then the
// rest.
func termRank(h TermHit) int {
	switch {
	case h.Discouraged:
		return 0
	case h.Status == string(model.TermPreferred):
		return 1
	default:
		return 2
	}
}

// DiscouragedReplacement is what to say instead of the discouraged term t of
// c: the concept's preferred wording in t's language, or the wording t's note
// names (terms.ReplacementFromNote).
func DiscouragedReplacement(c terms.Concept, t terms.Term) string {
	for _, p := range c.Terms {
		if p.Locale == t.Locale && p.Status == model.TermPreferred {
			return p.Text
		}
	}
	return terms.ReplacementFromNote(t.Note)
}

// WordRule is one line of the rules that hold at a point: what to say, what
// not to.
type WordRule struct {
	// Say is the wording to use. Empty for a rule that only bans a word.
	Say string
	// Also are other admitted wordings for the same thing.
	Also []string
	// Not are the wordings to avoid.
	Not []string
	// Note says what the rule is about.
	Note string
	// Locale is the language the rule is stated in, empty for a rule held
	// outside the terms store.
	Locale string
	// From names where the rule is held: terms, workspace, or the pack or
	// voice file whose terms apply.
	From []string
	// Advisory marks a rule whose check fails nothing.
	Advisory bool
}

// FoldWording folds a wording the way two are compared: in lower case, with
// the space around it taken off.
func FoldWording(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// ruleList merges rules into one list, keyed by the wording to use and by
// whether the rule is advisory, so a rule held in two places is one line and
// two rules with one wording to use and different outcomes stay two.
type ruleList struct {
	rules   []WordRule
	bySay   map[string]int
	avoided map[string]bool
}

// add merges one rule into the list. A rule whose Say is already listed adds
// its avoided wordings and its note to that line; a bare ban already stated
// by another line is dropped.
func (l *ruleList) add(r WordRule, from string) {
	r.Say = strings.TrimSpace(r.Say)
	if r.Say == "" {
		var fresh []string
		for _, n := range r.Not {
			if !l.avoided[FoldWording(n)] {
				fresh = append(fresh, n)
			}
		}
		if len(fresh) == 0 {
			return
		}
		r.Not = fresh
	}
	if r.Say != "" {
		if i, held := l.lineFor(r); held {
			existing := &l.rules[i]
			for _, n := range r.Not {
				if !l.avoided[FoldWording(n)] && FoldWording(n) != FoldWording(existing.Say) {
					existing.Not = append(existing.Not, n)
					l.avoided[FoldWording(n)] = true
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
		l.bySay[sayKey(r)] = len(l.rules)
	}
	for _, n := range r.Not {
		l.avoided[FoldWording(n)] = true
	}
	r.From = []string{from}
	l.rules = append(l.rules, r)
}

// lineFor finds the line r merges into: the line with r's Say and advisory
// marking. A rule that avoids nothing has no outcome of its own, so it merges
// into a line with its Say whatever that line's marking; and a line that
// avoids nothing yet takes the first rule with its Say that does, with that
// rule's marking.
func (l *ruleList) lineFor(r WordRule) (int, bool) {
	if i, held := l.bySay[sayKey(r)]; held {
		return i, true
	}
	other := r
	other.Advisory = !r.Advisory
	i, held := l.bySay[sayKey(other)]
	switch {
	case !held:
		return 0, false
	case len(r.Not) == 0:
		return i, true
	case len(l.rules[i].Not) == 0:
		delete(l.bySay, sayKey(other))
		l.rules[i].Advisory = r.Advisory
		l.bySay[sayKey(r)] = i
		return i, true
	}
	return 0, false
}

// sayKey is the key a rule merges under: its folded Say wording, and whether
// it is advisory.
func sayKey(r WordRule) string {
	if r.Advisory {
		return FoldWording(r.Say) + "\x00advisory"
	}
	return FoldWording(r.Say)
}

// SayThisNotThat builds the one list for a point from the terms in force
// there, the rules established across the workspace, and the terms the bound
// voice's file carries, in that order, and caps it at limit. It returns the
// total as well, so a capped list says what it is a part of.
func SayThisNotThat(hits []TermHit, binding []coreprofile.TermRule, voice *coreprofile.VoiceProfile, limit int) ([]WordRule, int) {
	l := &ruleList{bySay: map[string]int{}, avoided: map[string]bool{}}

	// One line per concept and language: its preferred wording, the other
	// admitted ones, and the discouraged ones.
	type conceptKey struct{ id, locale string }
	var order []conceptKey
	grouped := map[conceptKey]*WordRule{}
	for _, h := range hits {
		key := conceptKey{h.ConceptID, h.Locale}
		r, held := grouped[key]
		if !held {
			r = &WordRule{Locale: h.Locale, Note: h.Definition}
			grouped[key] = r
			order = append(order, key)
		}
		switch {
		case h.Discouraged:
			r.Not = append(r.Not, h.Term)
			r.Advisory = h.Advisory
			if r.Say == "" && h.Replacement != "" {
				r.Say = h.Replacement
			}
		case r.Say == "" || FoldWording(r.Say) == FoldWording(h.Term):
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
		// The replacement a discouraged hit carries may be one of the
		// admitted wordings already listed; it is the Say, never an Also as
		// well.
		r.Also = without(r.Also, r.Say)
		l.add(*r, "terms")
	}

	for _, rule := range binding {
		l.add(WordRule{Say: rule.Replacement, Not: wordOrNone(rule.Term), Note: rule.Note, Advisory: rule.Advisory && rule.Term != ""}, "workspace")
	}

	carried := voice.CarriedTerms()
	for _, t := range carried.Rules {
		if t.Replacement == t.Term {
			// A replacement identical to its term is a convention about how
			// to write the word, not a ban on it.
			l.add(WordRule{Say: t.Term, Note: t.Note}, carried.From)
			continue
		}
		l.add(WordRule{Say: t.Replacement, Not: wordOrNone(t.Term), Note: t.Note, Advisory: t.Advisory && t.Term != ""}, carried.From)
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
		if FoldWording(v) != FoldWording(s) {
			out = append(out, v)
		}
	}
	return out
}
