package memory_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tmxWith(tus string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header creationtool="test" creationtoolversion="1.0" segtype="sentence" adminlang="en-US" srclang="en" datatype="plaintext"/>
  <body>` + tus + `</body>
</tmx>`
}

func importTMX(t *testing.T, tm *memory.SQLiteStore, origin, body string) {
	t.Helper()
	_, _, err := memory.ImportTMXSession(t.Context(), tm, strings.NewReader(body), memory.ImportTMXOptions{OriginKey: origin})
	require.NoError(t, err)
}

func entryTexts(t *testing.T, tm *memory.SQLiteStore) []string {
	t.Helper()
	entries, err := tm.Entries(t.Context())
	require.NoError(t, err)
	var out []string
	for _, e := range entries {
		out = append(out, e.VariantText("en"))
	}
	return out
}

// TestImportTMXKeepsAFileWithoutTUIDsApartFromOneWithThem pins that a unit
// with no tuid and a unit whose tuid is the name the first is given ("tu-1")
// are two entries. Before, both were stored under one id and the second
// replaced the first.
func TestImportTMXKeepsAFileWithoutTUIDsApartFromOneWithThem(t *testing.T) {
	tm, err := memory.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	importTMX(t, tm, "f.tmx", tmxWith(`
    <tu><tuv xml:lang="en"><seg>First</seg></tuv><tuv xml:lang="fr"><seg>Premier</seg></tuv></tu>
    <tu tuid="tu-1"><tuv xml:lang="en"><seg>Second</seg></tuv><tuv xml:lang="fr"><seg>Deuxième</seg></tuv></tu>`))

	assert.ElementsMatch(t, []string{"First", "Second"}, entryTexts(t, tm))
}

// TestImportTMXKeepsOriginsApart pins that one file imported under two names
// whose name and tuid split at a ":" differently keeps every entry. Before,
// ("docs:a", "b") and ("docs", "a:b") were stored under one id.
func TestImportTMXKeepsOriginsApart(t *testing.T) {
	tm, err := memory.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	body := tmxWith(`
    <tu tuid="b"><tuv xml:lang="en"><seg>Bee</seg></tuv><tuv xml:lang="fr"><seg>Abeille</seg></tuv></tu>
    <tu tuid="a:b"><tuv xml:lang="en"><seg>Colon</seg></tuv><tuv xml:lang="fr"><seg>Deux-points</seg></tuv></tu>`)
	importTMX(t, tm, "docs:a", body)
	importTMX(t, tm, "docs", body)

	assert.Len(t, entryTexts(t, tm), 4, "two imports of two units each")
}

// TestImportTMXKeepsTheIDOfAPlainUnit pins that a unit whose origin and tuid
// hold neither ":" nor "%" keeps the id it has always had, so a store written
// before keeps matching it.
func TestImportTMXKeepsTheIDOfAPlainUnit(t *testing.T) {
	tm, err := memory.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	body := tmxWith(`
    <tu tuid="t1"><tuv xml:lang="en"><seg>Kept</seg></tuv><tuv xml:lang="fr"><seg>Gardé</seg></tuv></tu>
    <tu><tuv xml:lang="en"><seg>Unnamed</seg></tuv><tuv xml:lang="fr"><seg>Sans nom</seg></tuv></tu>`)
	session, _, err := memory.ImportTMXSession(t.Context(), tm, strings.NewReader(body), memory.ImportTMXOptions{OriginKey: "f.tmx"})
	require.NoError(t, err)
	_, ok, err := tm.GetEntry(t.Context(), "f.tmx:t1@"+session[:12])
	require.NoError(t, err)
	assert.True(t, ok)
	_, ok, err = tm.GetEntry(t.Context(), "f.tmx:tu-2@"+session[:12])
	require.NoError(t, err)
	assert.True(t, ok)
}

// TestImportTMXMovesAnEntryStoredUnderItsOldID pins that importing a file
// again moves an entry an earlier release stored under the id before
// escaping, rather than adding a second copy beside it.
func TestImportTMXMovesAnEntryStoredUnderItsOldID(t *testing.T) {
	tm, err := memory.NewSQLiteStore(":memory:")
	require.NoError(t, err)
	defer tm.Close()

	body := tmxWith(`
    <tu tuid="b"><tuv xml:lang="en"><seg>Bee</seg></tuv><tuv xml:lang="fr"><seg>Abeille</seg></tuv></tu>`)
	session, _, err := memory.ImportTMXSession(t.Context(), tm, strings.NewReader(body), memory.ImportTMXOptions{OriginKey: "docs:a"})
	require.NoError(t, err)
	entries, err := tm.Entries(t.Context())
	require.NoError(t, err)
	require.Len(t, entries, 1)
	// Store the entry under the id an earlier release gave it.
	old := entries[0]
	require.NoError(t, tm.Delete(t.Context(), old.ID))
	old.ID = "docs:a:b@" + session[:12]
	require.NoError(t, tm.Add(t.Context(), old))

	importTMX(t, tm, "docs:a", body)

	entries, err = tm.Entries(t.Context())
	require.NoError(t, err)
	require.Len(t, entries, 1, "the import moves the entry rather than adding a copy")
	assert.Equal(t, "docs%0a:b@"+session[:12], entries[0].ID)
}
