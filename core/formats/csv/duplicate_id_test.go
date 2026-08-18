package csv_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	csvfmt "github.com/neokapi/neokapi/core/formats/csv"
	"github.com/neokapi/neokapi/core/model"
)

// #2067. A declared key column addresses a row, and it holds ordinary data
// cells: nothing in CSV forbids two rows carrying the same key, and a table
// exported twice, a category label pressed into service as a key, or a repeated
// SKU produces it immediately. Everything downstream treats (source document,
// block id) as a block's identity, so those rows became one — the writer joins
// its skeleton refs on the same id, so both cells were filled from one block.
//
// The reader will not INFER a key column whose values repeat
// (TestCSVIdentity_RepeatedKeyIsNotInferred) precisely because a repeated key is
// not an address. A key column the recipe declares is taken at its word, and
// this is what makes it an identity anyway.

// duplicateKeyBlocks reads a table whose key column repeats, through both read
// paths: the buffered one and the skeleton one a `kapi run` takes.
func duplicateKeyBlocks(t *testing.T, input string) []*model.Block {
	t.Helper()
	return collectBlocks(readCSVWithConfig(t, input, func(c *csvfmt.Config) {
		c.KeyColumns = []int{0}
		c.TranslatableColumns = []int{1}
	}))
}

func TestReader_RepeatedKeyColumnValuesGetDistinctBlockIDs(t *testing.T) {
	t.Parallel()

	blocks := duplicateKeyBlocks(t, "id,text\nk,File\nk,Edit\nother,Cancel\n")
	require.Len(t, blocks, 3)

	ids := map[string]bool{}
	for _, b := range blocks {
		assert.False(t, ids[b.ID], "block id %q is not an identity", b.ID)
		ids[b.ID] = true
	}

	// The first row of a repeated key keeps it, so a table whose keys already
	// address its rows is untouched.
	assert.Equal(t, "k", blocks[0].ID)
	assert.Equal(t, "other", blocks[2].ID)
	assert.Empty(t, blocks[0].Properties[model.PropDocumentID],
		"an id that did not have to change records nothing")
	assert.Equal(t, "k", blocks[1].Properties[model.PropDocumentID])

	var texts []string
	for _, b := range blocks {
		texts = append(texts, b.SourceText())
	}
	assert.Equal(t, []string{"File", "Edit", "Cancel"}, texts)
}

// The skeleton path joins a ref to its block by the block's id, so the ref has
// to carry the separated one — otherwise the writer resolves both refs to one
// block and a row's content is written over its neighbour's.
func TestWriter_RepeatedKeyColumnValuesRoundTripUnchanged(t *testing.T) {
	t.Parallel()

	const input = "id,text\nk,File\nk,Edit\nother,Cancel\n"
	assert.Equal(t, input, skeletonRoundtrip(t, input, func(c *csvfmt.Config) {
		c.KeyColumns = []int{0}
		c.TranslatableColumns = []int{1}
	}))
}

// A bilingual table's unit is the row, and its id comes from the same key
// columns.
func TestReader_BilingualRepeatedKeyGetsDistinctBlockIDs(t *testing.T) {
	t.Parallel()

	blocks := collectBlocks(readCSVWithConfig(t,
		"id\tsource\ttarget\nk\tFile\t\nk\tEdit\t\nother\tCancel\t\n",
		func(c *csvfmt.Config) {
			c.Separator = '\t'
			c.KeyColumns = []int{0}
			c.SourceColumn = 1
			c.TargetColumn = 2
		}))
	require.Len(t, blocks, 3)

	ids := map[string]bool{}
	for _, b := range blocks {
		assert.False(t, ids[b.ID], "block id %q is not an identity", b.ID)
		ids[b.ID] = true
	}
	assert.Equal(t, "k", model.DocumentID(blocks[1]))
}
