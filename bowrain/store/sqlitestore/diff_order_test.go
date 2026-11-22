package sqlitestore

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	var firstJSON []byte
	for i := range 20 {
		diff, err := s.Diff(ctx, v1.ID, v2.ID)
		require.NoError(t, err)
		b, err := json.Marshal(diff)
		require.NoError(t, err)
		if i == 0 {
			firstJSON = b
			var ids []string
			for _, c := range diff.Changes {
				ids = append(ids, c.BlockID)
			}
			assert.Equal(t, []string{"b1", "b2", "b4", "b5", "b7", "b8"}, ids)
			continue
		}
		assert.Equal(t, string(firstJSON), string(b))
	}
}
