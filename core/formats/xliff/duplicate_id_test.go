package xliff_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/xliff"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #609. XLIFF 1.2 requires `trans-unit/@id` to be unique within a `<file>` and
// documents in the wild repeat it — okapi's PAS conformance corpus among them.
// Everything downstream treats (source document, block id) as a block's
// identity, so a repeated id made several units one: one block row survived
// extraction, and the translate tools' overlay cache served the first unit's
// target to every later one.
//
// The id is therefore made an identity at the reader. It cannot be repaired at
// the key instead: a `.kpz` handed on without its source reconstructs its blocks
// from a skeleton carrying their ids and nothing else (core/flow.partsFromSkeleton),
// so the id is all a merge has to go on.

const dupIDMenu = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="menu" source-language="en" target-language="nb" datatype="plaintext">
    <body>
      <trans-unit id="1"><source>File</source></trans-unit>
      <trans-unit id="1"><source>Edit</source></trans-unit>
      <trans-unit id="1"><source>Help</source></trans-unit>
      <trans-unit id="2"><source>Cancel</source></trans-unit>
    </body>
  </file>
</xliff>`

// snippetRoundtripNoSkeleton reads an XLIFF snippet and writes it back with no
// skeleton store wired, so the writer builds each `<trans-unit>` tag from the
// block rather than replaying the document's own bytes.
func snippetRoundtripNoSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := xliff.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NoError(t, reader.Close())

	writer := xliff.NewWriter()
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(parts)))
	require.NoError(t, writer.Close())
	return buf.String()
}

func countSubstring(s, sub string) int {
	return strings.Count(s, sub)
}

// readBlocks reads every block of an XLIFF snippet, in document order.
func readBlocks(t *testing.T, input string) []*model.Block {
	t.Helper()
	ctx := t.Context()
	reader := xliff.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	var blocks []*model.Block
	for _, p := range testutil.CollectParts(t, reader.Read(ctx)) {
		if b, ok := p.Resource.(*model.Block); ok {
			blocks = append(blocks, b)
		}
	}
	require.NoError(t, reader.Close())
	return blocks
}

func TestReader_RepeatedTransUnitIDsGetDistinctBlockIDs(t *testing.T) {
	blocks := readBlocks(t, dupIDMenu)
	require.Len(t, blocks, 4)

	ids := make(map[string]bool, len(blocks))
	for _, b := range blocks {
		assert.False(t, ids[b.ID], "block id %q is not an identity", b.ID)
		ids[b.ID] = true
	}

	// The first unit of a repeated id keeps it, so a conforming document — and
	// the conforming part of this one — is untouched.
	assert.Equal(t, "1", blocks[0].ID)
	assert.Equal(t, "2", blocks[3].ID)
	assert.Empty(t, blocks[0].Properties[xliff.TransUnitIDProperty],
		"an id that did not have to change records nothing")
	assert.Empty(t, blocks[3].Properties[xliff.TransUnitIDProperty])

	// The repeats carry the id the document spells, so the writer can emit it.
	for _, i := range []int{1, 2} {
		assert.Equal(t, "1", blocks[i].Properties[xliff.TransUnitIDProperty],
			"a separated block must remember the id its document spells")
	}

	// Each unit keeps its own wording — the symptom the collision produced.
	texts := []string{}
	for _, b := range blocks {
		texts = append(texts, b.SourceText())
	}
	assert.Equal(t, []string{"File", "Edit", "Help", "Cancel"}, texts)
}

// A separated id is a store identity, never something the document is made to
// carry: the round trip writes back the ids it was given.
func TestWriter_RepeatedTransUnitIDsRoundTripUnchanged(t *testing.T) {
	assert.Equal(t, dupIDMenu, snippetRoundtripWithSkeleton(t, dupIDMenu))
}

// The same holds for a fresh emission, which builds the `<trans-unit>` tag from
// the block rather than replaying the document's own bytes.
func TestWriter_FreshEmissionUsesTheDocumentsID(t *testing.T) {
	out := snippetRoundtripNoSkeleton(t, dupIDMenu)
	assert.Equal(t, 3, countSubstring(out, `<trans-unit id="1"`),
		"the three units the document calls 1 must be written as 1")
	assert.NotContains(t, out, "#2", "a store-side separator must not reach the file")
	assert.NotContains(t, out, "#3")
	for _, want := range []string{"File", "Edit", "Help", "Cancel"} {
		assert.Contains(t, out, "<source>"+want+"</source>")
	}
}

// A document whose ids collide with the separated spelling still gets distinct
// block ids: the separator is retried until it is free.
func TestReader_SeparatorCollisionStillYieldsDistinctIDs(t *testing.T) {
	blocks := readBlocks(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="odd" source-language="en" target-language="nb" datatype="plaintext">
    <body>
      <trans-unit id="1"><source>One</source></trans-unit>
      <trans-unit id="1#2"><source>Already taken</source></trans-unit>
      <trans-unit id="1"><source>Two</source></trans-unit>
    </body>
  </file>
</xliff>`)
	require.Len(t, blocks, 3)
	ids := map[string]bool{}
	for _, b := range blocks {
		require.False(t, ids[b.ID], "block id %q is not an identity", b.ID)
		ids[b.ID] = true
	}
	assert.Equal(t, "1", blocks[2].Properties[xliff.TransUnitIDProperty])
}

// Two <file> elements may each spell `id="1"` and both be conforming — the
// spec's guarantee is per <file>, while a block store keys on the document.
func TestReader_IDsAreSeparatedAcrossFilesOfOneDocument(t *testing.T) {
	blocks := readBlocks(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="a" source-language="en" target-language="nb" datatype="plaintext">
    <body><trans-unit id="1"><source>From A</source></trans-unit></body>
  </file>
  <file original="b" source-language="en" target-language="nb" datatype="plaintext">
    <body><trans-unit id="1"><source>From B</source></trans-unit></body>
  </file>
</xliff>`)
	require.Len(t, blocks, 2)
	assert.NotEqual(t, blocks[0].ID, blocks[1].ID,
		"one document, one id space — the store keys on the document")
	assert.Equal(t, "1", blocks[1].Properties[xliff.TransUnitIDProperty])
}
