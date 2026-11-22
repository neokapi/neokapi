package memory_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/memory"
)

const enUSFrenchTMX = `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1" segtype="sentence" o-tmf="x" adminlang="en-US" srclang="en_US" datatype="plaintext"/>
  <body>
    <tu><tuv xml:lang="en-US"><seg>Color</seg></tuv><tuv xml:lang="fr"><seg>Couleur</seg></tuv></tu>
  </body>
</tmx>`

// The header's srclang is read in canonical form; "*all*" names no single
// language.
func TestTMXSourceLanguage(t *testing.T) {
	got, err := memory.TMXSourceLanguage(strings.NewReader(enUSFrenchTMX))
	require.NoError(t, err)
	assert.Equal(t, model.LocaleID("en-US"), got)

	got, err = memory.TMXSourceLanguage(strings.NewReader(strings.Replace(enUSFrenchTMX, `srclang="en_US"`, `srclang="*all*"`, 1)))
	require.NoError(t, err)
	assert.Empty(t, got)
}

// A bilingual import whose source language no translation unit holds stores
// nothing, records no session, and says so, instead of storing entries that
// hold only the target.
func TestImportTMXWithoutTheSourceLanguageStoresNothing(t *testing.T) {
	tm := memory.NewInMemoryStore()
	n, err := memory.ImportTMXWithOptions(context.Background(), tm, strings.NewReader(enUSFrenchTMX), "en-GB", "fr", memory.ImportTMXOptions{OriginKey: "f.tmx"})
	require.ErrorIs(t, err, memory.ErrNoUsablePair)
	assert.Zero(t, n)
	assert.Empty(t, mustEntries(t, tm))
	sessions, err := tm.ListImportSessions(context.Background())
	require.NoError(t, err)
	assert.Empty(t, sessions, "an import that stored nothing records no session")

	n, err = memory.ImportTMXWithOptions(context.Background(), tm, strings.NewReader(enUSFrenchTMX), "en-US", "fr", memory.ImportTMXOptions{OriginKey: "f.tmx"})
	require.NoError(t, err)
	assert.Equal(t, 1, n)
	entries := mustEntries(t, tm)
	require.Len(t, entries, 1)
	assert.Equal(t, "Color", entries[0].VariantText("en-US"))
}
