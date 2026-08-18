package ts_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #2067. Qt's id-based workflow expects one `message/@id` per string, and the
// DTD asks for nothing — a catalog merged from several sources, or one whose
// messages were copied with their ids, repeats them. Everything downstream
// treats (source document, block id) as a block's identity, so a repeated id
// made several messages one and the translate tools' overlay cache served the
// first message's translation to every later one.

const dupMessageIDCatalog = `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.1" language="nb">
<context>
    <name>MainWindow</name>
    <message id="m1">
        <source>File</source>
        <translation type="unfinished"></translation>
    </message>
    <message id="m1">
        <source>Edit</source>
        <translation type="unfinished"></translation>
    </message>
    <message id="m2">
        <source>Cancel</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>
`

func TestReader_RepeatedMessageIDsGetDistinctBlockIDs(t *testing.T) {
	blocks := translatableBlocks(readTSBlocks(t, dupMessageIDCatalog))
	require.Len(t, blocks, 3)

	ids := map[string]bool{}
	for _, b := range blocks {
		assert.False(t, ids[b.ID], "block id %q is not an identity", b.ID)
		ids[b.ID] = true
	}

	assert.Equal(t, "m1", blocks[0].ID)
	assert.Equal(t, "m2", blocks[2].ID)
	assert.Empty(t, blocks[0].Properties[model.PropDocumentID],
		"an id that did not have to change records nothing")

	assert.Equal(t, "m1", blocks[1].Properties[model.PropDocumentID])
	assert.Equal(t, "m1", model.DocumentID(blocks[1]))

	var texts []string
	for _, b := range blocks {
		texts = append(texts, b.SourceText())
	}
	assert.Equal(t, []string{"File", "Edit", "Cancel"}, texts)
}

// A separated id is a store identity, never something the catalog is made to
// carry: the writer emits the id the document spelled.
func TestWriter_RepeatedMessageIDsAreWrittenAsTheDocumentSpellsThem(t *testing.T) {
	out := snippetRoundtrip(t, dupMessageIDCatalog)
	assert.Equal(t, 2, strings.Count(out, `id="m1"`),
		"the two messages the catalog calls m1 must be written as m1")
	assert.NotContains(t, out, "m1#2", "a store-side separator must not reach the file")
	for _, want := range []string{"File", "Edit", "Cancel"} {
		assert.Contains(t, out, "<source>"+want+"</source>")
	}
}

// A message with no id is numbered positionally, and the reader's `tu` prefix is
// what tells the writer not to write that number back as an id the document
// never had — including where a message the document DID name has to be
// separated.
func TestWriter_SynthesizedIDsStayOutOfTheFile(t *testing.T) {
	out := snippetRoundtrip(t, `<?xml version="1.0" encoding="utf-8"?>
<!DOCTYPE TS>
<TS version="2.1" language="nb">
<context>
    <name>MainWindow</name>
    <message>
        <source>Unnamed</source>
        <translation type="unfinished"></translation>
    </message>
    <message>
        <source>Also unnamed</source>
        <translation type="unfinished"></translation>
    </message>
</context>
</TS>
`)
	assert.NotContains(t, out, ` id="tu`, "a synthesized id is not something the catalog said")
}
