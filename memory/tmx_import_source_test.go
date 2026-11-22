package memory_test

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
)

// A unit's source language: an explicit one when the unit holds it, else the
// store's when the unit holds it, else the declared one when the unit holds
// that, else none.
func TestChooseSource(t *testing.T) {
	holds := func(ls ...model.LocaleID) func(model.LocaleID) bool {
		return func(l model.LocaleID) bool { return slices.Contains(ls, l) }
	}
	for _, tc := range []struct {
		name     string
		rule     memory.SourceRule
		declared model.LocaleID
		holds    func(model.LocaleID) bool
		want     model.LocaleID
	}{
		{"explicit wins", memory.SourceRule{Explicit: "de", Store: "en-GB"}, "fr", holds("de", "en-GB", "fr"), "de"},
		{"explicit the unit lacks", memory.SourceRule{Explicit: "en-GB", Store: "en-GB"}, "en-US", holds("en-US", "fr"), ""},
		{"store over declared", memory.SourceRule{Store: "en-GB"}, "fr", holds("fr", "en-GB"), "en-GB"},
		{"declared when the store is missing", memory.SourceRule{Store: "en-GB"}, "en-US", holds("en-US", "fr"), "en-US"},
		{"neither", memory.SourceRule{Store: "en-GB"}, "", holds("de", "fr"), ""},
	} {
		assert.Equal(t, tc.want, memory.ChooseSource(tc.rule, tc.declared, tc.holds), tc.name)
	}
}

const mixedTMX = `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1" segtype="sentence" o-tmf="x" adminlang="en" srclang="fr" datatype="plaintext"/>
  <body>
    <tu><tuv xml:lang="fr"><seg>Bouée</seg></tuv><tuv xml:lang="en-GB"><seg>Buoy</seg></tuv></tu>
    <tu><tuv xml:lang="de"><seg>Kai</seg></tuv><tuv xml:lang="es"><seg>Muelle</seg></tuv></tu>
    <tu><tuv xml:lang="fr"><seg>Quai</seg></tuv><tuv xml:lang="de"><seg>Kai</seg></tuv></tu>
    <tu><tuv xml:lang="en-GB"><seg>Wharf</seg></tuv></tu>
  </body>
</tmx>`

// Each unit is read in the store's language when it holds it, whatever the
// header declares, and every unit the rule cannot use is counted by reason.
func TestImportTMXWithRuleChoosesPerUnitAndCounts(t *testing.T) {
	tm := memory.NewInMemoryStore()
	counts, err := memory.ImportTMXWithRule(context.Background(), tm, strings.NewReader(mixedTMX),
		memory.SourceRule{Store: "en-GB", Target: "fr"}, memory.ImportTMXOptions{OriginKey: "f.tmx"})
	require.NoError(t, err)
	assert.Equal(t, 1, counts.Imported)
	assert.Equal(t, 1, counts.NoSource, "de/es holds neither en-GB nor fr")
	assert.Equal(t, 1, counts.SameLanguage, "fr/de declares fr, the target")
	assert.Equal(t, 1, counts.NoTarget, "en-GB alone holds no fr")
	entries := mustEntries(t, tm)
	require.Len(t, entries, 1)
	assert.Equal(t, model.LocaleID("en-GB"), entries[0].HintSrcLang)
	assert.Equal(t, "Buoy", entries[0].VariantText("en-GB"))
	assert.Equal(t, "Bouée", entries[0].VariantText("fr"))
}

// A file of which the rule keeps no unit stores nothing and records no
// session.
func TestImportTMXWithRuleThatKeepsNothingStoresNothing(t *testing.T) {
	tm := memory.NewInMemoryStore()
	counts, err := memory.ImportTMXWithRule(context.Background(), tm, strings.NewReader(mixedTMX),
		memory.SourceRule{Explicit: "it", Store: "en-GB", Target: "fr"}, memory.ImportTMXOptions{OriginKey: "f.tmx"})
	require.ErrorIs(t, err, memory.ErrNoUsablePair)
	assert.Equal(t, 4, counts.NoSource)
	assert.Empty(t, mustEntries(t, tm))
	sessions, err := tm.ListImportSessions(context.Background())
	require.NoError(t, err)
	assert.Empty(t, sessions)
}

// Every unit of a file lands in exactly one count: imported plus every skip
// reason is the number of units, empty units included.
func TestImportTMXWithRuleCountsEveryUnit(t *testing.T) {
	doc := `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1" segtype="sentence" o-tmf="x" adminlang="en" srclang="en-GB" datatype="plaintext"/>
  <body>
    <tu><tuv xml:lang="en-GB"><seg>Buoy</seg></tuv><tuv xml:lang="fr"><seg>Bouée</seg></tuv></tu>
    <tu><tuv xml:lang="en-GB"><seg></seg></tuv></tu>
    <tu></tu>
    <tu><tuv xml:lang="en-GB"><seg>Quay</seg></tuv></tu>
    <tu><tuv xml:lang="de"><seg>Kai</seg></tuv><tuv xml:lang="fr"><seg>Quai</seg></tuv></tu>
  </body>
</tmx>`
	counts, err := memory.ImportTMXWithRule(context.Background(), memory.NewInMemoryStore(), strings.NewReader(doc),
		memory.SourceRule{Store: "en-GB", Target: "fr"}, memory.ImportTMXOptions{OriginKey: "f.tmx"})
	require.NoError(t, err)
	assert.Equal(t, 1, counts.Imported)
	assert.Equal(t, 2, counts.Empty, "the empty segment and the empty unit")
	assert.Equal(t, 5, counts.Imported+counts.Skipped(), "every unit is counted once")
}
