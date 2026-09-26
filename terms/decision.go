package terms

import (
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
	// Competitor records the term as a competitor's name.
	Competitor bool
	// Forms are the other surface shapes the term takes.
	Forms []string
}

// UpsertDecision lands a term decision in the concept set.
//
// The term joins a CONCEPT rather than getting one of its own wherever the
// entry says which: the concept that already declares it, else the one
// `replaces` names, else the one that declares the replacement. Only a decision
// that names nothing already in the graph opens a new concept. That is what
// makes the decision answerable later — "what should this say instead" is the
// preferred term of the concept the retired word sits in, so a term filed on
// its own island can never be answered, however clearly the decision was
// written. A replacement no concept declares yet is added to the joined concept
// as its preferred term, for the same reason.
//
// It is idempotent: an entry already recorded this way returns changed=false so
// apply reports a skipped no-op. Terms are matched case-insensitively on text
// within a locale; a new concept is keyed by a stable id so reading the same
// decision again is reproducible.
//
// target is the index of the one concept the decision touched, which is the
// concept the caller writes back to the store.
func UpsertDecision(concepts []Concept, d Decision) (out []Concept, target int, changed bool) {
	// Canonical before it reaches the store or the concept id, so a decision
	// written as "en_US" joins the concept "en-US" already holds.
	d.Locale = model.NormalizeLocale(d.Locale)
	now := time.Now().UTC()
	noteWant := ReplacementNote(d.Replacement)

	target = IndexOfTerm(concepts, d.Text, d.Locale)
	if target < 0 && d.Replaces != "" {
		target = IndexOfConcept(concepts, d.Replaces, d.Locale)
	}
	if target < 0 && d.Replacement != "" {
		target = IndexOfTerm(concepts, d.Replacement, d.Locale)
	}

	if target < 0 {
		concepts = append(concepts, Concept{
			ID:        DecisionConceptID(d.Text, d.Locale),
			Source:    TermSourceTerminology,
			CreatedAt: now,
			UpdatedAt: now,
		})
		target = len(concepts) - 1
		changed = true
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
	return concepts, target, changed
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
