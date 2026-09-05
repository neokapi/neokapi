package terms

import (
	"slices"

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
		if model.NormalizeLocale(c.Terms[i].Locale) == c.Terms[i].Locale {
			continue
		}
		terms := slices.Clone(c.Terms)
		for j := range terms {
			terms[j].Locale = model.NormalizeLocale(terms[j].Locale)
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
