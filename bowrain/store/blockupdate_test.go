package store

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
)

// UpdateBlock stores what the update leaves on the block, and returns it.
func TestUpdateBlock_StoresTheChange(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	seedWriteBackItem(t, s, p.ID, "en.json", model.NewBlock("k", "Hello"))
	read := readItemByKey(t, s, p.ID, "en.json")["k"]
	require.NotNil(t, read)

	got, err := s.UpdateBlock(ctx, p.ID, "main", read.Block.ID, func(sb *venue.StoredBlock) error {
		sb.Block.SetTargetText("nb", "Hei")
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, "Hei", got.Block.TargetText("nb"))
	assert.Equal(t, "Hei", readItemByKey(t, s, p.ID, "en.json")["k"].Block.TargetText("nb"))
}

// An update that returns an error writes nothing, and the error comes back with
// the block as it stands.
func TestUpdateBlock_ARefusedUpdateWritesNothing(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	b := model.NewBlock("k", "Hello")
	b.SetTargetText("nb", "Hei")
	seedWriteBackItem(t, s, p.ID, "en.json", b)
	read := readItemByKey(t, s, p.ID, "en.json")["k"]
	require.NotNil(t, read)

	got, err := s.UpdateBlock(ctx, p.ID, "main", read.Block.ID, func(sb *venue.StoredBlock) error {
		return platstore.ErrBlockChanged
	})
	require.ErrorIs(t, err, platstore.ErrBlockChanged)
	require.NotNil(t, got, "the refusal carries the current block")
	assert.Equal(t, "Hei", got.Block.TargetText("nb"))
	assert.Equal(t, "Hei", readItemByKey(t, s, p.ID, "en.json")["k"].Block.TargetText("nb"))
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
	assert.False(t, called, "there is no block to update")
	assert.Zero(t, countRows(t, s, "blocks", `project_id=$1`, p.ID))
}

// Another write to the block waits while an update holds it, so nothing lands
// between the block the update judged and the block it writes.
func TestUpdateBlock_AnotherWriteWaitsForTheUpdate(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	seedWriteBackItem(t, s, p.ID, "en.json", model.NewBlock("k", "Hello"))
	read := readItemByKey(t, s, p.ID, "en.json")["k"]
	require.NotNil(t, read)

	other := make(chan error, 1)
	var otherDoneDuringUpdate bool
	_, err := s.UpdateBlock(ctx, p.ID, "main", read.Block.ID, func(sb *venue.StoredBlock) error {
		go func() {
			b := model.NewBlock("k", "Hello")
			b.ID = read.Block.ID
			b.SetTargetText("nb", "Hallo")
			other <- s.StoreBlocks(context.Background(), p.ID, "main", []*model.Block{b})
		}()
		waited := assert.Eventually(t, func() bool { return sessionsWaitingOnLocks(t, s) > 0 },
			10*time.Second, 20*time.Millisecond, "the other write waits on the held block")
		select {
		case <-other:
			otherDoneDuringUpdate = true
		default:
		}
		if !waited {
			return errors.New("the other write did not wait")
		}
		sb.Block.SetTargetText("nb", "Hei")
		return nil
	})
	require.NoError(t, err)
	assert.False(t, otherDoneDuringUpdate, "the other write did not land during the update")
	require.NoError(t, <-other)
	assert.Equal(t, "Hallo", readItemByKey(t, s, p.ID, "en.json")["k"].Block.TargetText("nb"),
		"the other write lands after the update")
}
