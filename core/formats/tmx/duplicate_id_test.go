package tmx_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2067. TMX declares `tuid` as `CDATA #IMPLIED` rather than an XML `ID` and
// leaves the value to the producer, so a memory whose units all call themselves
// the same thing is a conforming file — the weakest uniqueness claim of any
// format kapi reads a block id from, and the one most likely to be broken,
// because a memory is assembled by merging other people's files.
//
// Everything downstream treats (source document, block id) as a block's
// identity, so a repeated tuid made several units one: the translate tools'
// overlay cache served the first unit's target to every later one. The id is
// therefore made an identity at the reader, and the writer emits the tuid the
// document spelled.

const dupTuidMemory = `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
<header creationtool="probe" creationtoolversion="1" segtype="sentence" o-tmf="tmx" adminlang="en" srclang="en" datatype="plaintext"/>
<body>
<tu tuid="1"><tuv xml:lang="en"><seg>File</seg></tuv></tu>
<tu tuid="1"><tuv xml:lang="en"><seg>Edit</seg></tuv></tu>
<tu tuid="2"><tuv xml:lang="en"><seg>Cancel</seg></tuv></tu>
</body>
</tmx>`

func TestReader_RepeatedTuidsGetDistinctBlockIDs(t *testing.T) {
	blocks := readTMXBlocks(t, dupTuidMemory)
	require.Len(t, blocks, 3)

	ids := map[string]bool{}
	for _, b := range blocks {
		assert.False(t, ids[b.ID], "block id %q is not an identity", b.ID)
		ids[b.ID] = true
	}

	// The first unit of a repeated tuid keeps it, so a memory whose units
	// already identify themselves is untouched.
	assert.Equal(t, "1", blocks[0].ID)
	assert.Equal(t, "2", blocks[2].ID)
	assert.Empty(t, blocks[0].Properties[model.PropDocumentID],
		"an id that did not have to change records nothing")

	assert.Equal(t, "1", blocks[1].Properties[model.PropDocumentID])
	assert.Equal(t, "1", model.DocumentID(blocks[1]))

	var texts []string
	for _, b := range blocks {
		texts = append(texts, b.SourceText())
	}
	assert.Equal(t, []string{"File", "Edit", "Cancel"}, texts)
}

// A separated id is a store identity, never something the file is made to
// carry: the writer emits the tuid the document spelled.
func TestWriter_RepeatedTuidsAreWrittenAsTheDocumentSpellsThem(t *testing.T) {
	out, blocks := roundTrip(t, dupTuidMemory)
	assert.Equal(t, 2, strings.Count(out, `tuid="1"`),
		"the two units the memory calls 1 must be written as 1")
	assert.NotContains(t, out, "1#2", "a store-side separator must not reach the file")

	require.Len(t, blocks, 3)
	var texts []string
	for _, b := range blocks {
		texts = append(texts, b.SourceText())
	}
	assert.Equal(t, []string{"File", "Edit", "Cancel"}, texts,
		"a second read must find the same three units")
}

// A unit with no tuid is numbered positionally, and that number can meet a tuid
// the memory spells literally — so the synthesized ids share the id space.
func TestReader_PositionalFallbackDoesNotCollideWithALiteralTuid(t *testing.T) {
	blocks := readTMXBlocks(t, `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
<header creationtool="probe" creationtoolversion="1" segtype="sentence" o-tmf="tmx" adminlang="en" srclang="en" datatype="plaintext"/>
<body>
<tu tuid="tu2"><tuv xml:lang="en"><seg>Named after a counter</seg></tuv></tu>
<tu><tuv xml:lang="en"><seg>Numbered by position</seg></tuv></tu>
</body>
</tmx>`)
	require.Len(t, blocks, 2)
	assert.NotEqual(t, blocks[0].ID, blocks[1].ID,
		"a synthesized id must not answer for one the memory spelled")
}
