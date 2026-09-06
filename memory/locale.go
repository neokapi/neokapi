package memory

import (
	"maps"

	"github.com/neokapi/neokapi/core/model"
)

// The store normalizes the locale beside the text. Every variant, entity value
// and lookup locale is keyed by its canonical BCP-47 tag (model.NormalizeLocale),
// so an entry written as "nb_NO" and a lookup asking for "nb-NO" meet at one
// row. The text has always been normalized on the same statement (NormalizeText);
// a locale spelled two ways beside it was two rows and a silent miss.

// NormalizeEntryLocales rewrites an entry's locale keys to their canonical
// form, in place: the variants, the source-language hint and every entity's
// per-locale values. Two spellings of one locale in a single entry collapse to
// the one written last, which is what the store would have done row by row.
//
// Every backend applies it on the write path, so a caller that builds an entry
// by hand (an importer, `kapi apply`) and a caller that reads one back from
// another store both land on the same keys.
func NormalizeEntryLocales(e *Entry) {
	if e == nil {
		return
	}
	e.HintSrcLang = model.NormalizeLocale(e.HintSrcLang)
	e.Variants = normalizeLocaleKeys(e.Variants)
	for i := range e.Entities {
		e.Entities[i].Values = normalizeLocaleKeys(e.Entities[i].Values)
	}
}

// normalizeLocaleKeys returns m with every key normalized. A map whose keys are
// already canonical is returned as it is, so the common path allocates nothing.
func normalizeLocaleKeys[V any](m map[model.LocaleID]V) map[model.LocaleID]V {
	if len(m) == 0 {
		return m
	}
	clean := true
	for loc := range m {
		if model.NormalizeLocale(loc) != loc {
			clean = false
			break
		}
	}
	if clean {
		return m
	}
	out := make(map[model.LocaleID]V, len(m))
	for loc, v := range maps.All(m) {
		out[model.NormalizeLocale(loc)] = v
	}
	return out
}

// NormalizeSearchLocales returns the search parameters with their locale
// filters in canonical form, the form the variant rows are keyed by.
func NormalizeSearchLocales(p SearchParams) SearchParams {
	p.AnyLocale = string(model.NormalizeLocale(model.LocaleID(p.AnyLocale)))
	p.RequireLocale = string(model.NormalizeLocale(model.LocaleID(p.RequireLocale)))
	return p
}
