package sqlitestore

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// UpdateBlock stores what the update leaves on the block, as in the Postgres
// store.
func TestUpdateBlock_StoresTheChange(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "main", []*model.Block{model.NewBlock("b1", "Hello")}))

	got, err := s.UpdateBlock(ctx, p.ID, "main", "b1", func(sb *venue.StoredBlock) error {
		sb.Block.SetTargetText("nb", "Hei")
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, "Hei", got.Block.TargetText("nb"))
	stored, err := s.GetBlock(ctx, p.ID, "main", "b1")
	require.NoError(t, err)
	assert.Equal(t, "Hei", stored.Block.TargetText("nb"))
}

// An update that returns an error writes nothing, and the error comes back with
// the block as it stands.
func TestUpdateBlock_ARefusedUpdateWritesNothing(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	b := model.NewBlock("b1", "Hello")
	b.SetTargetText("nb", "Hei")
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "main", []*model.Block{b}))

	got, err := s.UpdateBlock(ctx, p.ID, "main", "b1", func(sb *venue.StoredBlock) error {
		return platstore.ErrBlockChanged
	})
	require.ErrorIs(t, err, platstore.ErrBlockChanged)
	require.NotNil(t, got)
	assert.Equal(t, "Hei", got.Block.TargetText("nb"))
	stored, err := s.GetBlock(ctx, p.ID, "main", "b1")
	require.NoError(t, err)
	assert.Equal(t, "Hei", stored.Block.TargetText("nb"))
}

// A block that is not stored is not found, and no row is made for it.
func TestUpdateBlock_AMissingBlockIsNotCreated(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	called := false
	got, err := s.UpdateBlock(ctx, p.ID, "main", "no-such-block", func(*venue.StoredBlock) error {
		called = true
		return nil
	})
	require.Error(t, err)
	assert.Nil(t, got)
	assert.False(t, called)
	_, err = s.GetBlock(ctx, p.ID, "main", "no-such-block")
	require.Error(t, err, "no row is made")
}
