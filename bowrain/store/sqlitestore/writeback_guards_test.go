package sqlitestore

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The change log stamps target_modified for every target the caller carried,
// asking only whether that locale already had a row, never whether its text
// moved. The comment at the site calls the payload diff a best-effort check;
// recordTargetHistory, two calls earlier, already makes the real comparison.
func TestWriteBackBlocks_AnUnchangedTargetIsNotLogged(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	require.NoError(t, s.StoreItem(ctx, p.ID, "main", &platstore.Item{Name: "en.json", Format: "json"}))
	seed := model.NewBlock("k", "Hello")
	seed.SetTargetText("nb", "Uendret")
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{seed}))

	sb := readItemByKey(t, s, p.ID, "en.json")["k"]
	require.NotNil(t, sb)
	modified := func() int {
		return countRows(t, s, "change_log",
			`project_id=? AND block_id=? AND change_type='target_modified'`, p.ID, sb.Block.ID)
	}
	before := modified()

	// The caller records a server stamp and leaves nb exactly as it read it.
	if sb.Block.Properties == nil {
		sb.Block.Properties = map[string]string{}
	}
	sb.Block.Properties["__source_settled_hash"] = sb.ContentHash
	res, err := s.WriteBackBlocks(ctx, p.ID, "main", []*venue.StoredBlock{sb})
	require.NoError(t, err)
	require.Equal(t, 1, res.Written)

	assert.Equal(t, before, modified(),
		"a target whose text did not move is not logged as modified")
}
