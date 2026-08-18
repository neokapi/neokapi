package model_test

import (
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A conforming document passes through untouched: every id is its own, so
// nothing is separated and nothing records a separation that did not happen.
func TestIDBuilder_DistinctIDsAreLeftAlone(t *testing.T) {
	var ids model.IDBuilder
	for _, id := range []string{"1", "2", "greeting", "tu7"} {
		block := model.NewBlock(id, "text")
		ids.Assign(block)
		assert.Equal(t, id, block.ID)
		assert.Empty(t, block.Properties[model.PropDocumentID],
			"an id that did not have to change records nothing")
		assert.Equal(t, id, model.DocumentID(block))
	}
}

// The repeats are what move, and only the repeats: the first block to claim an
// id keeps it, so the document's own numbering survives wherever it was already
// an identity.
func TestIDBuilder_RepeatsAreSeparatedAndRememberTheirDocumentID(t *testing.T) {
	var ids model.IDBuilder
	var got []string
	for range 3 {
		block := model.NewBlock("1", "text")
		ids.Assign(block)
		got = append(got, block.ID)
	}
	assert.Equal(t, []string{"1", "1#2", "1#3"}, got)

	separated := model.NewBlock("1", "text")
	ids.Assign(separated)
	assert.Equal(t, "1#4", separated.ID)
	assert.Equal(t, "1", separated.Properties[model.PropDocumentID])
	assert.Equal(t, "1", model.DocumentID(separated),
		"a separated id is a store identity, never something the document carries")
}

// A document may itself spell the id a separation would have produced, so the
// ordinal is retried until it is free.
func TestIDBuilder_SeparatorCollisionStillYieldsDistinctIDs(t *testing.T) {
	var ids model.IDBuilder
	seen := map[string]bool{}
	for _, id := range []string{"1", "1#2", "1", "1#3", "1"} {
		block := model.NewBlock(id, "text")
		ids.Assign(block)
		require.False(t, seen[block.ID], "block id %q is not an identity", block.ID)
		seen[block.ID] = true
	}
	assert.Len(t, seen, 5)
}

// An empty id is not a claim on the id space — blockstore.StoreKey falls back to
// the source text for one — so a reader that leaves some blocks unnamed does not
// find them all separated into `#2`, `#3`, `#4`.
func TestIDBuilder_EmptyIDsAreNotSeparated(t *testing.T) {
	var ids model.IDBuilder
	for range 3 {
		block := model.NewBlock("", "text")
		ids.Assign(block)
		assert.Empty(t, block.ID)
		assert.Empty(t, block.Properties[model.PropDocumentID])
	}
}

// One builder per document read. Reset is what a reader reused across files
// calls, so a block's id never depends on which file was read first.
func TestIDBuilder_ResetStartsANewDocument(t *testing.T) {
	var ids model.IDBuilder
	first := model.NewBlock("1", "text")
	ids.Assign(first)
	ids.Reset()
	second := model.NewBlock("1", "text")
	ids.Assign(second)
	assert.Equal(t, "1", second.ID, "a new document starts with an empty id space")
}

// DocumentID reads through for a block that was never separated, including one
// with no properties map at all.
func TestDocumentID_ReadsThroughWhenNothingWasSeparated(t *testing.T) {
	assert.Empty(t, model.DocumentID(nil))
	assert.Equal(t, "tu1", model.DocumentID(&model.Block{ID: "tu1"}))
	assert.Equal(t, "tu1", model.DocumentID(&model.Block{
		ID: "tu1", Properties: map[string]string{model.PropDocumentID: ""},
	}), "an empty recorded id is not an id")
}

// The digest table is the part that keeps a streaming reader's memory bound, so
// it is exercised past several growth doublings — the sizes where an off-by-one
// in the probe or the load factor stops being invisible.
func TestIDBuilder_SeparatesAtScale(t *testing.T) {
	var ids model.IDBuilder
	seen := map[string]bool{}
	const n = 20_000
	for i := range n {
		block := model.NewBlock(fmt.Sprintf("u%d", i), "text")
		ids.Assign(block)
		require.False(t, seen[block.ID], "block id %q is not an identity", block.ID)
		require.Equal(t, fmt.Sprintf("u%d", i), block.ID,
			"distinct ids must survive a table that has grown")
		seen[block.ID] = true
	}

	// And the same ids again, all of which now have to be separated.
	for i := range n {
		block := model.NewBlock(fmt.Sprintf("u%d", i), "text")
		ids.Assign(block)
		require.Equal(t, fmt.Sprintf("u%d#2", i), block.ID)
	}
}

// Two members of one container each hand back the id their own reader minted
// from one, and the container makes both its own.
func TestQualifyMemberID_MembersDoNotShareTheIDsTheyMint(t *testing.T) {
	first := model.NewBlock("tu1", "File")
	second := model.NewBlock("tu1", "Edit")
	model.QualifyMemberID(first, "sf1")
	model.QualifyMemberID(second, "sf2")

	assert.Equal(t, "sf1_tu1", first.ID)
	assert.Equal(t, "sf2_tu1", second.ID)
	assert.NotEqual(t, first.ID, second.ID)
}

// A qualified id is the block's identity and the only one: unlike a repeat the
// document itself spelled, there is no member-local spelling for a writer to put
// back, so an export carries the identity rather than the collision.
func TestQualifyMemberID_RecordsNoDocumentSpelling(t *testing.T) {
	block := model.NewBlock("tu1", "File")
	model.QualifyMemberID(block, "sf1")

	assert.Empty(t, block.Properties[model.PropDocumentID])
	assert.Equal(t, "sf1_tu1", model.DocumentID(block))
}

// Containers nest — a JSON value handed to an XML subfilter that hands an
// element to HTML — and each layer of container qualifies what it carries.
func TestQualifyMemberID_Nests(t *testing.T) {
	block := model.NewBlock("tu1", "File")
	model.QualifyMemberID(block, "sf1") // the inner container's member
	model.QualifyMemberID(block, "sf3") // the outer container's member

	assert.Equal(t, "sf3_sf1_tu1", block.ID)
}

// Nothing to qualify, or nothing to qualify it with, leaves the block as it is:
// an empty id is not a claim on the id space, and a container with no name for
// one of its members has nothing to separate by.
func TestQualifyMemberID_LeavesWhatItCannotQualify(t *testing.T) {
	empty := model.NewBlock("", "File")
	model.QualifyMemberID(empty, "sf1")
	assert.Empty(t, empty.ID)

	unnamed := model.NewBlock("tu1", "File")
	model.QualifyMemberID(unnamed, "")
	assert.Equal(t, "tu1", unnamed.ID)

	model.QualifyMemberID(nil, "sf1")
}

// A block id is written into documents that constrain its characters — an XLIFF
// 2 `unit/@id` is an xs:NMTOKEN — so the qualification stays inside that set.
// This is why the member is the container's layer id and not the entry path or
// key path it stands for.
func TestQualifyMemberID_StaysWithinTheCharactersAnIDMaySpell(t *testing.T) {
	block := model.NewBlock("tu1", "File")
	model.QualifyMemberID(block, "sf1")
	for _, r := range block.ID {
		assert.True(t,
			r == '.' || r == '-' || r == '_' || r == ':' ||
				(r >= '0' && r <= '9') || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'),
			"%q is not a character an NMTOKEN may hold", r)
	}
}
