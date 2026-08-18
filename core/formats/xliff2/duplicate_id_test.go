package xliff2_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2067. XLIFF 2 requires `id` on `<unit>` and requires it to be unique within
// the enclosing `<file>` (§2.2). Documents in the wild break that — the writer
// has accepted them since #1599 — and everything downstream treats (source
// document, block id) as a block's identity, so a repeated id made several units
// one: one row survived extraction, and the translate tools' overlay cache
// served the first unit's target to every later one.
//
// The id is therefore made an identity at the reader, exactly as XLIFF 1.2's is.
// It cannot be repaired at the key instead: a `.kpz` handed on without its
// source reconstructs its blocks from a skeleton carrying their ids and nothing
// else (core/flow.partsFromSkeleton), so the id is all a merge has to go on.

const dupIDMenu = `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="nb">
  <file id="menu">
    <unit id="1"><segment><source>File</source></segment></unit>
    <unit id="1"><segment><source>Edit</source></segment></unit>
    <unit id="1"><segment><source>Help</source></segment></unit>
    <unit id="2"><segment><source>Cancel</source></segment></unit>
  </file>
</xliff>
`

// readBlocks reads every block of an XLIFF 2 snippet, in document order.
func readBlocks(t *testing.T, input string) []*model.Block {
	t.Helper()
	ctx := t.Context()
	reader := xliff2.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	blocks := testutil.FilterBlocks(testutil.CollectParts(t, reader.Read(ctx)))
	require.NoError(t, reader.Close())
	return blocks
}

// readPartsStreaming reads a snippet through the skeleton path — a separate
// parse, and the one a `kapi run` takes, because the runner wires a skeleton
// store into every reader that emits one.
func readPartsStreaming(t *testing.T, input string) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := xliff2.NewReader()
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	t.Cleanup(func() { store.Close() })
	reader.SetSkeletonStore(store)
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	require.NoError(t, reader.Close())
	return parts
}

// generateFromBlocks writes a snippet's parts with no source DOM and no skeleton
// to replay, so the writer builds each `<unit>` element from the block itself.
// The streaming read is what supplies parts free of the DOM annotation the
// round-trip path patches.
func generateFromBlocks(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()
	var buf bytes.Buffer
	writer := xliff2.NewWriter()
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(ctx, testutil.PartsToChannel(readPartsStreaming(t, input))))
	require.NoError(t, writer.Close())
	return buf.String()
}

func TestReader_RepeatedUnitIDsGetDistinctBlockIDs(t *testing.T) {
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
	assert.Empty(t, blocks[0].Properties[model.PropDocumentID],
		"an id that did not have to change records nothing")
	assert.Empty(t, blocks[3].Properties[model.PropDocumentID])

	// The repeats carry the id the document spells, so the writer can emit it.
	for _, i := range []int{1, 2} {
		assert.Equal(t, "1", blocks[i].Properties[model.PropDocumentID],
			"a separated block must remember the id its document spells")
		assert.Equal(t, "1", model.DocumentID(blocks[i]))
	}

	// Each unit keeps its own wording — the symptom the collision produced.
	var texts []string
	for _, b := range blocks {
		texts = append(texts, b.SourceText())
	}
	assert.Equal(t, []string{"File", "Edit", "Help", "Cancel"}, texts)
}

// A separated id is a store identity, never something the document is made to
// carry: the round trip writes back the ids it was given, byte for byte.
func TestWriter_RepeatedUnitIDsRoundTripUnchanged(t *testing.T) {
	out, err := readWrite(t, []byte(dupIDMenu))
	require.NoError(t, err)
	assert.Equal(t, dupIDMenu, string(out))
}

// The same holds for a fresh emission, which builds the `<unit>` element from
// the block rather than patching the document's own DOM.
func TestWriter_FreshEmissionUsesTheDocumentsUnitID(t *testing.T) {
	out := generateFromBlocks(t, dupIDMenu)
	assert.Equal(t, 3, strings.Count(out, `<unit id="1"`),
		"the three units the document calls 1 must be written as 1")
	assert.NotContains(t, out, "1#2", "a store-side separator must not reach the file")
	assert.NotContains(t, out, "1#3")
	for _, want := range []string{"File", "Edit", "Help", "Cancel"} {
		assert.Contains(t, out, "<source>"+want+"</source>")
	}
}

// The spec's uniqueness guarantee stops at the <file>, so two files may each
// spell `id="1"` and both be conforming — while a block store keys on the
// document, which is why the id space is document-wide.
func TestReader_UnitIDsAreSeparatedAcrossFilesOfOneDocument(t *testing.T) {
	blocks := readBlocks(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="nb">
  <file id="a"><unit id="1"><segment><source>From A</source></segment></unit></file>
  <file id="b"><unit id="1"><segment><source>From B</source></segment></unit></file>
</xliff>`)
	require.Len(t, blocks, 2)
	assert.NotEqual(t, blocks[0].ID, blocks[1].ID,
		"one document, one id space — the store keys on the document")
	assert.Equal(t, "1", model.DocumentID(blocks[1]))
}

// A group is no boundary either: the enclosing <group> does not scope `unit/@id`,
// and the reader that walks into one carries the same id space with it.
func TestReader_UnitIDsAreSeparatedAcrossGroups(t *testing.T) {
	blocks := readBlocks(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="nb">
  <file id="f1">
    <group id="g1"><unit id="1"><segment><source>In g1</source></segment></unit></group>
    <group id="g2"><unit id="1"><segment><source>In g2</source></segment></unit></group>
  </file>
</xliff>`)
	require.Len(t, blocks, 2)
	assert.NotEqual(t, blocks[0].ID, blocks[1].ID)
	assert.Equal(t, "1", model.DocumentID(blocks[1]))
}

// A document whose ids collide with the separated spelling still gets distinct
// block ids: the separator is retried until it is free.
func TestReader_UnitIDSeparatorCollisionStillYieldsDistinctIDs(t *testing.T) {
	blocks := readBlocks(t, `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="nb">
  <file id="odd">
    <unit id="1"><segment><source>One</source></segment></unit>
    <unit id="1#2"><segment><source>Already taken</source></segment></unit>
    <unit id="1"><segment><source>Two</source></segment></unit>
  </file>
</xliff>`)
	require.Len(t, blocks, 3)
	ids := map[string]bool{}
	for _, b := range blocks {
		require.False(t, ids[b.ID], "block id %q is not an identity", b.ID)
		ids[b.ID] = true
	}
	assert.Equal(t, "1", model.DocumentID(blocks[2]))
}

// The streaming reader — the one a `kapi run` takes, because the runner wires a
// skeleton store — has its own unit-building path, and separates ids there too.
func TestStreamingReader_RepeatedUnitIDsGetDistinctBlockIDs(t *testing.T) {
	blocks := testutil.FilterBlocks(readPartsStreaming(t, dupIDMenu))

	require.Len(t, blocks, 4)
	ids := map[string]bool{}
	for _, b := range blocks {
		assert.False(t, ids[b.ID], "block id %q is not an identity", b.ID)
		ids[b.ID] = true
	}
	assert.Equal(t, "1", model.DocumentID(blocks[1]))
}
