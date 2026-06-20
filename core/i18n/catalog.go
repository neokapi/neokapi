package i18n

import (
	"github.com/leonelquinteros/gotext"

	"github.com/neokapi/neokapi/core/model"
)

// Scope is the gettext msgctxt for a translatable metadata string. It's
// the dot-separated full key path of the value in the canonical metadata
// JSON (e.g. "tools.translate.displayName",
// "formats.okf_html.displayName"). Using the full path means homonym
// source text ("Description") across tools/formats/plugins resolves
// unambiguously. The dot separator matches what the JSON filter emits
// by default (useKeyAsName=true, useFullKeyPath=false).
type Scope string

// Translator resolves (scope, source) pairs to locale-specific strings.
// Misses return the English source unchanged — no fallback magic, no
// artificial "key not found" markers. Safe for concurrent use.
type Translator interface {
	// T looks up a translation for (scope, source) in the active catalog
	// and returns the translated string, or source when not found.
	T(scope Scope, source string) string

	// Locale returns the active target locale.
	Locale() model.LocaleID
}

// NoopTranslator is a Translator that always returns the source string.
// Useful as a zero value and for tests.
type NoopTranslator struct{}

// T returns source unchanged.
func (NoopTranslator) T(_ Scope, source string) string { return source }

// Locale returns "en".
func (NoopTranslator) Locale() model.LocaleID { return "en" }

// gotextTranslator is backed by one or more gettext MO catalogs. Lookup
// tries each catalog in order and returns the first hit.
type gotextTranslator struct {
	locale   model.LocaleID
	catalogs []*gotext.Mo
}

// T implements Translator by delegating to gotext.Mo.GetC (pgettext).
func (g *gotextTranslator) T(scope Scope, source string) string {
	if source == "" {
		return ""
	}
	for _, cat := range g.catalogs {
		if cat == nil {
			continue
		}
		if cat.IsTranslatedC(source, string(scope)) {
			return cat.GetC(source, string(scope))
		}
	}
	return source
}

// Locale implements Translator.
func (g *gotextTranslator) Locale() model.LocaleID { return g.locale }

// NewTranslator builds a Translator over the given MO catalogs for the
// given locale. Empty or "en" locale returns a NoopTranslator (lookup is
// unnecessary when the source IS the active locale). Non-English locales
// with no catalogs still get a real Translator — Locale() returns the
// resolved locale and every T call falls through to source. That way a
// user running in a locale nobody has translated to yet still gets a
// sensible Locale() report (matching --lang / KAPI_LANG) instead of the
// misleading "en" that NoopTranslator would hand back.
//
// Catalogs merge in precedence order: first hit wins. Nils are filtered.
func NewTranslator(locale model.LocaleID, catalogs ...*gotext.Mo) Translator {
	if locale.IsEmpty() || locale == "en" {
		return NoopTranslator{}
	}
	filtered := catalogs[:0]
	for _, c := range catalogs {
		if c != nil {
			filtered = append(filtered, c)
		}
	}
	return &gotextTranslator{locale: locale, catalogs: filtered}
}
