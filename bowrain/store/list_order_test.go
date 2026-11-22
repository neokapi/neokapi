package store

import (
	"encoding/json"
	"slices"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func changeIDs(changes []platstore.BlockChange) []string {
	ids := make([]string, 0, len(changes))
	for _, c := range changes {
		ids = append(ids, c.BlockID)
	}
	return ids
}

// A version diff lists its changes by block ID, the same on every call.
func TestVersionDiffIsSortedByBlockID(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	ctx := t.Context()

	var first []*model.Block
	for _, id := range []string{"b5", "b2", "b8", "b1", "b6", "b3"} {
		first = append(first, model.NewBlock(id, "text "+id))
	}
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", first))
	v1, err := s.CreateVersion(ctx, p.ID, "", "v1", "")
	require.NoError(t, err)

	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{
		model.NewBlock("b5", "changed"), model.NewBlock("b1", "changed"),
		model.NewBlock("b7", "new"), model.NewBlock("b4", "new"),
	}))
	require.NoError(t, s.DeleteBlock(ctx, p.ID, "", "b8"))
	require.NoError(t, s.DeleteBlock(ctx, p.ID, "", "b2"))
	v2, err := s.CreateVersion(ctx, p.ID, "", "v2", "")
	require.NoError(t, err)

	var firstJSON string
	for i := range 20 {
		diff, err := s.Diff(ctx, v1.ID, v2.ID)
		require.NoError(t, err)
		b, err := json.Marshal(diff)
		require.NoError(t, err)
		if i == 0 {
			firstJSON = string(b)
			assert.Equal(t, []string{"b1", "b2", "b4", "b5", "b7", "b8"}, changeIDs(diff.Changes))
			continue
		}
		assert.Equal(t, firstJSON, string(b))
	}
}

// A merge reports its changes by block ID.
func TestMergeStreamListsChangesByBlockID(t *testing.T) {
	s := newTestStore(t)
	p := createTestProject(t, s)
	ctx := t.Context()
	units := []string{"u5", "u2", "u8", "u1", "u6", "u3", "u7", "u4"}
	seedStream(t, s, p.ID, "main", "en.json", units...)
	require.NoError(t, s.CreateStream(ctx, &platstore.Stream{ProjectID: p.ID, Name: "feature", Parent: "main"}))

	var edited []*model.Block
	for _, u := range units {
		b := model.NewBlock(u, "the branch's wording of "+u)
		b.Translatable = true
		edited = append(edited, b)
	}
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "feature", "en.json", edited))

	res, err := s.MergeStream(ctx, p.ID, "feature", platstore.MergeOptions{})
	require.NoError(t, err)
	ids := changeIDs(res.Changes)
	assert.Len(t, ids, len(units))
	assert.True(t, slices.IsSorted(ids), "merge changes are sorted by block ID: %v", ids)
}
