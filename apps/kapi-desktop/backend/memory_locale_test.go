package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A locale reaches these methods as whatever the caller typed: a text field, a
// URL, a POSIX environment. The stores key on the canonical BCP-47 form, so a
// locale that is merely spelled differently has to be recognised as the same
// language before it is used to look anything up.
func TestMemory_CanonicalizesLocaleArguments(t *testing.T) {
	app := newTestApp(t)
	handle := openTestMemory(t, app)

	require.NoError(t, app.AddMemoryEntry(handle, AddMemoryEntryRequest{
		HintSrcLang: "en",
		Variants: map[string]VariantInputDTO{
			"en":    {Text: "Shopping cart"},
			"nb_NO": {Text: "Handlekurv"},
		},
	}))

	entries := app.SearchMemoryEntries(handle, "Shopping", "", "", 0, 10)
	require.Len(t, entries.Entries, 1)

	// Stored under the canonical spelling, not the one that was typed.
	got := entries.Entries[0].Variants
	assert.Contains(t, got, "nb-NO", "nb_NO must be stored as nb-NO")
	assert.NotContains(t, got, "nb_NO")

	// A search scoped to that locale finds it however the caller spells it.
	// AnyLocale asks which language the query has to match in, so the query is
	// the Norwegian wording.
	for _, spelling := range []string{"nb-NO", "nb_NO", "NB-no"} {
		res := app.SearchMemoryEntries(handle, "Handlekurv", spelling, "", 0, 10)
		assert.Lenf(t, res.Entries, 1, "searching any-locale %q", spelling)
	}
}

// The app renders a pseudo-locale, and a person may add a custom one CLDR has
// never heard of. Both are locales; only a primary subtag that names no
// language is not. A boundary that validated strictly would turn the app's own
// pseudo-locale away.
func TestMemory_AcceptsAPseudoLocale(t *testing.T) {
	app := newTestApp(t)
	handle := openTestMemory(t, app)

	require.NoError(t, app.AddMemoryEntry(handle, AddMemoryEntryRequest{
		HintSrcLang: "en",
		Variants: map[string]VariantInputDTO{
			"en":          {Text: "Shopping cart"},
			"qps-Ploc":    {Text: "[Šĥöppíñg çäŕţ]"},
			"en_US.UTF-8": {Text: "Shopping cart"},
		},
	}))

	res := app.SearchMemoryEntries(handle, "Shopping", "", "", 0, 10)
	require.Len(t, res.Entries, 1)

	got := res.Entries[0].Variants
	assert.Contains(t, got, "qps-Ploc", "the pseudo-locale must survive whole")
	assert.Contains(t, got, "en-US", "a POSIX locale with a codeset must be cleaned, not refused")
}

func TestMemory_RefusesALocaleThatNamesNothing(t *testing.T) {
	app := newTestApp(t)
	handle := openTestMemory(t, app)

	err := app.AddMemoryEntry(handle, AddMemoryEntryRequest{
		HintSrcLang: "en",
		Variants:    map[string]VariantInputDTO{"en": {Text: "Shopping cart"}, "not a locale": {Text: "x"}},
	})
	require.Error(t, err, "a variant keyed by a non-locale must not be stored")
	assert.Contains(t, err.Error(), "invalid locale")

	// A search scoped to a locale nothing could name finds nothing, the same
	// answer an unknown handle gets, rather than searching for the literal text.
	assert.Empty(t, app.SearchMemoryEntries(handle, "Shopping", "not a locale", "", 0, 10).Entries)
}
