package terms

import (
	"encoding/json"
	"slices"
	"time"

	"github.com/neokapi/neokapi/core/model"
)

// The store normalizes the locale beside the text. A term's text is stored
// case-folded on the same row as its locale, and every lookup keys on both, so
// the locale is held in its canonical BCP-47 form (model.NormalizeLocale) the
// way the text is held in its normalized one: a term written under "en_US" and
// a lookup asking in "en-US" meet at one row.

// NormalizedConcept returns the concept with every term's locale in canonical
// form. The terms slice is copied when any locale changes and shared when none
// does, so a caller's concept is never rewritten under it. Every backend applies
// it on the write path.
func NormalizedConcept(c Concept) Concept {
	for i := range c.Terms {
		if model.NormalizeLocale(c.Terms[i].Locale) == c.Terms[i].Locale &&
			slices.Equal(NormalizeForms(c.Terms[i].Text, c.Terms[i].Forms), c.Terms[i].Forms) {
			continue
		}
		terms := slices.Clone(c.Terms)
		for j := range terms {
			terms[j].Locale = model.NormalizeLocale(terms[j].Locale)
			terms[j].Forms = NormalizeForms(terms[j].Text, terms[j].Forms)
		}
		c.Terms = terms
		break
	}
	return c
}

// sameLocale reports whether two locale spellings name one locale.
func sameLocale(a, b model.LocaleID) bool {
	return model.NormalizeLocale(a) == model.NormalizeLocale(b)
}

// SameConcept reports whether two concepts say the same thing, whenever each
// was written. The write path asks it before an upsert that carries no
// timestamp of its own: writing a concept the store already holds word for
// word would stamp it with a new updated_at, so reading the same terms file
// twice would leave the store saying something new. The comparison is over
// the normalized concept's JSON, so a nil list and an empty one are the same,
// and a field added to Concept later is compared without anyone listing it.
func SameConcept(a, b Concept) bool {
	canonical := func(c Concept) []byte {
		c = NormalizedConcept(c)
		c.CreatedAt, c.UpdatedAt = time.Time{}, time.Time{}
		if c.Source == "" {
			c.Source = TermSourceTerminology // what the write path stores for an empty source
		}
		data, err := json.Marshal(c)
		if err != nil {
			return nil
		}
		return data
	}
	ca, cb := canonical(a), canonical(b)
	return ca != nil && string(ca) == string(cb)
}
