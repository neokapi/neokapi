package host

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The two persisted ids `kapi apply` mints embed the locale in canonical form,
// so a decision written in any spelling of a locale names the entry or concept
// every other spelling names.

func TestMemoryEntryID_CanonicalAcrossSpellings(t *testing.T) {
	want := memoryEntryID("Berth", "en-US", "nb-NO")
	assert.Contains(t, want, "apply:en-US:nb-NO:")
	for _, pair := range [][2]model.LocaleID{{"en_US", "nb_NO"}, {"EN-us", "NB-no"}, {"en_US.UTF-8", "nb_NO"}} {
		assert.Equal(t, want, memoryEntryID("Berth", pair[0], pair[1]), "%s -> %s", pair[0], pair[1])
	}
	assert.NotEqual(t, want, memoryEntryID("Berth", "en-GB", "nb-NO"), "a different locale is a different id")
}

func TestConceptID_CanonicalAcrossSpellings(t *testing.T) {
	want := conceptID("berth", "en-GB")
	assert.Equal(t, "term:en-GB:berth", want)
	assert.Equal(t, want, conceptID("berth", "en_GB"))
	assert.Equal(t, want, conceptID("berth", "EN-gb"))
}

func TestUpsertMemoryPair_WritesCanonicalLocales(t *testing.T) {
	entries, changed := upsertMemoryPair(nil, "Berth", "Kai", "en_US", "nb_NO", "")
	require.True(t, changed)
	require.Len(t, entries, 1)
	e := entries[0]
	assert.Equal(t, memoryEntryID("Berth", "en-US", "nb-NO"), e.ID)
	assert.Equal(t, model.LocaleID("en-US"), e.HintSrcLang)
	assert.Equal(t, []model.LocaleID{"en-US", "nb-NO"}, e.Locales())

	// The same pair written in the canonical spelling is the same entry, and
	// an unchanged target is a no-op.
	entries, changed = upsertMemoryPair(entries, "Berth", "Kai", "en-US", "nb-NO", "")
	assert.False(t, changed)
	assert.Len(t, entries, 1)

	// A corrected target lands on the existing entry under canonical keys.
	entries, changed = upsertMemoryPair(entries, "Berth", "Kaiplass", "EN-us", "NB-no", "")
	assert.True(t, changed)
	require.Len(t, entries, 1)
	assert.Equal(t, "Kaiplass", entries[0].VariantText("nb-NO"))
	assert.Equal(t, []model.LocaleID{"en-US", "nb-NO"}, entries[0].Locales())
}

func TestUpsertTerm_CanonicalLocaleJoinsTheConcept(t *testing.T) {
	existing := []terms.Concept{{
		ID:    "c-berth",
		Terms: []terms.Term{{Text: "berth", Locale: "en-GB", Status: model.TermPreferred}},
	}}
	// A decision spelled en_GB about the same term is the same term.
	got, changed := upsertTerm(existing, termDecision{Text: "berth", Locale: "en_GB", Status: model.TermPreferred})
	assert.False(t, changed, "already recorded under the canonical spelling")
	assert.Len(t, got, 1)

	// A new concept minted from a POSIX spelling carries the canonical id and
	// locale.
	got, changed = upsertTerm(nil, termDecision{Text: "dock", Locale: "en_GB", Status: model.TermForbidden, Replacement: "berth"})
	require.True(t, changed)
	require.Len(t, got, 1)
	assert.Equal(t, "term:en-GB:dock", got[0].ID)
	for _, term := range got[0].Terms {
		assert.Equal(t, model.LocaleID("en-GB"), term.Locale)
	}
}
