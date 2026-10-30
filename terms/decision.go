package terms

import (
	"maps"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/model"
)

// Decision is one decision about a term: the term, its language and standing,
// what to use instead, and which concept it joins.
type Decision struct {
	Text        string
	Locale      model.LocaleID
	Status      model.TermStatus
	Replacement string
	// Replaces names the concept the term joins: a concept id, or the text of a
	// term that concept already declares.
	Replaces string
	// DoNotTranslate sets (true) or clears (false) the do-not-translate flag on
	// the concept the term joins; nil leaves it.
	DoNotTranslate *bool
	// Advisory, for a discouraged term, marks the concept's rules advisory: a
	// use of the term reports without failing a check.
	Advisory bool
	// TextScope, for a discouraged term, names where in a text the concept's
	// rules apply (Concept.TextScope): prose, code or headings. Empty is
	// everywhere.
	TextScope string
	// Competitor records the term as a competitor's name.
	Competitor bool
	// Forms are the other surface shapes the term takes.
	Forms []string
}

// UpsertDecision lands a term decision in the concept set.
//
// The term joins the concept that already declares it, else the one
// `replaces` names, else the one that declares the replacement, so "what
// should this say instead" is the preferred term of the term's concept. A
// replacement no concept declares yet becomes that concept's preferred term.
//
// A concept carries one advisory marking and one text scope for its rules
// (sameRuleFlags). A term marked otherwise than the other rules of its concept
// moves to a concept of its own, and left is the index of the concept it
// left, or -1. A term marked otherwise than its replacement's concept opens a
// concept of its own too.
//
// It is idempotent: a decision already recorded returns changed=false. The
// caller writes back target, and left when it is not -1.
func UpsertDecision(concepts []Concept, d Decision) (out []Concept, target, left int, changed bool) {
	// Canonical before it reaches the store or the concept id, so a decision
	// written as "en_US" joins the concept "en-US" already holds.
	d.Locale = model.NormalizeLocale(d.Locale)
	now := time.Now().UTC()
	noteWant := ReplacementNote(d.Replacement)

	left = -1
	var moving *Term
	target = IndexOfTerm(concepts, d.Text, d.Locale)
	if target >= 0 && !sameRuleFlags(concepts[target], d) {
		// The term moves to a concept of its own, which keeps the point, the
		// domain and the do-not-translate flag of the one it leaves.
		c := &concepts[target]
		ti := TermIndex(c, d.Text, d.Locale)
		t := c.Terms[ti]
		moving = &t
		c.Terms = slices.Delete(c.Terms, ti, ti+1)
		c.UpdatedAt = now
		left, target = target, -1
	}
	if target < 0 && moving == nil && d.Replaces != "" {
		target = IndexOfConcept(concepts, d.Replaces, d.Locale)
	}
	if target < 0 && d.Replacement != "" {
		target = IndexOfTerm(concepts, d.Replacement, d.Locale)
		if target >= 0 && !sameRuleFlags(concepts[target], d) {
			target = -1
		}
	}

	if target < 0 {
		fresh := Concept{
			ID:        freeConceptID(concepts, d),
			Source:    TermSourceTerminology,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if left >= 0 {
			from := concepts[left]
			fresh.ProjectID, fresh.Domain, fresh.Definition = from.ProjectID, from.Domain, from.Definition
			fresh.Source, fresh.DoNotTranslate = from.Source, from.DoNotTranslate
			fresh.Properties = maps.Clone(from.Properties)
		}
		concepts = append(concepts, fresh)
		target = len(concepts) - 1
		changed = true
	}
	if moving != nil {
		concepts[target].Terms = append(concepts[target].Terms, *moving)
	}

	c := &concepts[target]
	if ti := TermIndex(c, d.Text, d.Locale); ti >= 0 {
		t := &c.Terms[ti]
		forms := NormalizeForms(d.Text, d.Forms)
		formsChanged := len(forms) > 0 && !slices.Equal(t.Forms, forms)
		if t.Status != d.Status || t.Text != d.Text || (noteWant != "" && t.Note != noteWant) || t.CompetitorTerm != d.Competitor || formsChanged {
			t.Status = d.Status
			t.Text = d.Text
			t.CompetitorTerm = d.Competitor
			if formsChanged {
				t.Forms = forms
			}
			if noteWant != "" {
				t.Note = noteWant
			}
			changed = true
		}
	} else {
		c.Terms = append(c.Terms, Term{
			Text:           d.Text,
			Locale:         d.Locale,
			Status:         d.Status,
			Note:           noteWant,
			CompetitorTerm: d.Competitor,
			Forms:          NormalizeForms(d.Text, d.Forms),
		})
		changed = true
	}
	if d.Status.Discouraged() && c.Advisory != d.Advisory {
		c.Advisory = d.Advisory
		changed = true
	}
	if d.Status.Discouraged() && c.TextScope() != d.TextScope {
		c.SetTextScope(d.TextScope)
		changed = true
	}

	// The replacement becomes the concept's preferred term when the graph does
	// not have it yet — including under another concept, which would otherwise
	// end up declaring the same word twice. A term the entry itself declares
	// preferred is not retired in favour of anything, so its replacement, if it
	// named one, stays in the note rather than contradicting it.
	if d.Replacement != "" && d.Status.Discouraged() && IndexOfTerm(concepts, d.Replacement, d.Locale) < 0 {
		c.Terms = append(c.Terms, Term{
			Text:   d.Replacement,
			Locale: d.Locale,
			Status: model.TermPreferred,
		})
		changed = true
	}

	if d.DoNotTranslate != nil && c.DoNotTranslate != *d.DoNotTranslate {
		c.DoNotTranslate = *d.DoNotTranslate
		changed = true
	}

	if changed {
		c.UpdatedAt = now
	}
	return concepts, target, left, changed
}

// freeConceptID is the id of the concept a decision opens: the stable id of
// its term (DecisionConceptID), or, when a concept already carries that id,
// that id with the term's marking appended.
func freeConceptID(concepts []Concept, d Decision) string {
	id := DecisionConceptID(d.Text, d.Locale)
	if !slices.ContainsFunc(concepts, func(c Concept) bool { return c.ID == id }) {
		return id
	}
	mark := "fails"
	if d.Advisory {
		mark = "advisory"
	}
	if d.TextScope != "" {
		mark += "-" + d.TextScope
	}
	return id + ":" + mark
}

// sameRuleFlags reports whether a discouraged term decided by d can join c by
// sharing its replacement. A concept carries one advisory marking and one text
// scope for all of its rules. A term marked otherwise than the rules c holds in
// d's language would change theirs: an advisory "leverage → use" joined to
// "utilize → use" would stop utilize from failing. Such a term opens a concept
// of its own and names the replacement in its note (ReplacementNote), where
// PreferredTerm reads it. A concept holding no rule in that language takes the
// term's marking, and a term that is not discouraged carries none.
func sameRuleFlags(c Concept, d Decision) bool {
	if !d.Status.Discouraged() {
		return true
	}
	holdsRule := slices.ContainsFunc(c.Terms, func(t Term) bool {
		return t.Locale == d.Locale && (t.Status.Discouraged() || t.CompetitorTerm) && !strings.EqualFold(t.Text, d.Text)
	})
	return !holdsRule || (c.Advisory == d.Advisory && c.TextScope() == d.TextScope)
}

// IndexOfTerm returns the index of the concept declaring text in locale, or -1.
func IndexOfTerm(concepts []Concept, text string, locale model.LocaleID) int {
	for ci := range concepts {
		if TermIndex(&concepts[ci], text, locale) >= 0 {
			return ci
		}
	}
	return -1
}

// IndexOfConcept resolves a join key — a concept id, or the text of a term the
// concept declares in locale — to a concept index, or -1.
func IndexOfConcept(concepts []Concept, key string, locale model.LocaleID) int {
	for ci := range concepts {
		if concepts[ci].ID == key {
			return ci
		}
	}
	return IndexOfTerm(concepts, key, locale)
}

// TermIndex returns the index of the concept's term with this text in this
// locale, or -1.
func TermIndex(c *Concept, text string, locale model.LocaleID) int {
	for ti := range c.Terms {
		if c.Terms[ti].Locale == locale && strings.EqualFold(c.Terms[ti].Text, text) {
			return ti
		}
	}
	return -1
}

// DecisionConceptID derives a stable, filesystem-safe concept id from the term text and
// locale so re-applying the same term re-seeds the same concept. The locale is
// embedded in canonical form: the id is persisted, and two spellings of one
// locale must mint one concept.
func DecisionConceptID(text string, locale model.LocaleID) string {
	return "term:" + string(model.NormalizeLocale(locale)) + ":" + slug(text)
}

var slugPattern = regexp.MustCompile(`[^a-z0-9]+`)

// slug converts text to a stable, filename-safe id part.
func slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugPattern.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
