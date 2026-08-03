package tmx_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A block's Name is the context half of its identity (core/model/structural.go,
// core/reconcile). A TMX translation unit has no position worth recording — the
// file is a set of units, and reordering it changes nothing — so these tests pin
// that the name is the unit's own key and never a count of units gone past.

func tmxNames(t *testing.T, input string) []string {
	t.Helper()
	blocks := readTMXBlocks(t, input)
	names := make([]string, len(blocks))
	for i, b := range blocks {
		names[i] = b.Name
	}
	return names
}

// tmxDoc wraps TU markup in the minimal TMX envelope.
func tmxDoc(units string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
<header creationtool="test" creationtoolversion="1" segtype="sentence" o-tmf="test" adminlang="en" srclang="en" datatype="plaintext"/>
<body>` + units + `
</body>
</tmx>`
}

// tu builds one <tu> with the given optional tuid and source/target segments.
func tu(tuid, source, target string) string {
	attr := ""
	if tuid != "" {
		attr = ` tuid="` + tuid + `"`
	}
	return `
<tu` + attr + `>
<tuv xml:lang="en"><seg>` + source + `</seg></tuv>
<tuv xml:lang="fr"><seg>` + target + `</seg></tuv>
</tu>`
}

// A tuid is the producer's own key for the unit, so where there is one it is
// the name.
func TestTUIDIsTheName(t *testing.T) {
	t.Parallel()
	names := tmxNames(t, tmxDoc(tu("greeting", "Hello", "Bonjour")+tu("farewell", "Bye", "Au revoir")))
	assert.Equal(t, []string{"greeting", "farewell"}, names)
}

// Without a tuid the unit is named by its source segment — what a translation
// memory matches on anyway — rather than by "tu3".
func TestUnitWithoutTUIDIsNamedBySource(t *testing.T) {
	t.Parallel()
	names := tmxNames(t, tmxDoc(tu("", "Hello", "Bonjour")+tu("", "Goodbye", "Au revoir")))
	assert.Equal(t, []string{"Hello", "Goodbye"}, names)
}

func TestTMXDeletingAnEarlierUnitKeepsLaterNames(t *testing.T) {
	t.Parallel()
	before := tmxNames(t, tmxDoc(tu("", "Alpha", "A")+tu("", "Bravo", "B")+tu("", "Charlie", "C")))
	after := tmxNames(t, tmxDoc(tu("", "Alpha", "A")+tu("", "Charlie", "C")))

	require.Len(t, before, 3)
	require.Len(t, after, 2)
	assert.Equal(t, before[0], after[0])
	assert.Equal(t, before[2], after[1], "the surviving later unit keeps its name")
}

// Two units with the same source and no tuid are indistinguishable by key; the
// ordinal is scoped to the repeated key, not to the document.
func TestTMXIdenticalSourcesGetDistinctNames(t *testing.T) {
	t.Parallel()
	names := tmxNames(t, tmxDoc(tu("", "Alpha", "A")+tu("", "Hello", "X")+tu("", "Alpha", "B")))
	require.Len(t, names, 3)
	assert.Equal(t, "Alpha", names[0])
	assert.Equal(t, "Hello", names[1])
	assert.NotEqual(t, names[0], names[2])

	// Inserting a unit between the two "Alpha"s does not change either name: the
	// ordinal counts occurrences of that key, not units in the file.
	dense := tmxNames(t, tmxDoc(tu("", "Alpha", "A")+tu("", "Alpha", "B")))
	assert.Equal(t, []string{names[0], names[2]}, dense)
}

func TestTMXNamesAreStableAcrossReads(t *testing.T) {
	t.Parallel()
	input := tmxDoc(tu("greeting", "Hello", "Bonjour") + tu("", "Goodbye", "Au revoir"))
	assert.Equal(t, tmxNames(t, input), tmxNames(t, input))
}

// A unit whose source carries inline codes is named by its text content, with
// the codes contributing nothing — otherwise re-tagging would rename it.
func TestTMXInlineCodesDoNotAppearInTheName(t *testing.T) {
	t.Parallel()
	names := tmxNames(t, tmxDoc(`
<tu>
<tuv xml:lang="en"><seg>Hello <ph type="fmt">%s</ph> world</seg></tuv>
<tuv xml:lang="fr"><seg>Bonjour</seg></tuv>
</tu>`))
	require.Len(t, names, 1)
	assert.Equal(t, "Hello world", names[0])
}
