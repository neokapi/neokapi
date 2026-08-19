package store_test

import (
	"context"
	"fmt"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pagedStore is a ContentStore that answers block reads out of an ordered slice,
// honouring the keyset cursor and the limit the way both real stores do, and
// recording the largest page it was ever asked to build.
type pagedStore struct {
	platstore.ContentStore // nil: every other method would panic if reached

	blocks    []*venue.StoredBlock
	calls     int
	widestRow int
	lastQuery platstore.BlockQuery
}

func (p *pagedStore) GetBlocks(_ context.Context, q platstore.BlockQuery) ([]*venue.StoredBlock, error) {
	p.calls++
	p.lastQuery = q

	start := 0
	if q.AfterID != "" {
		for i, b := range p.blocks {
			if b.Block.ID == q.AfterID {
				start = i + 1
				break
			}
		}
	}
	end := len(p.blocks)
	if q.Limit > 0 && start+q.Limit < end {
		end = start + q.Limit
	}
	page := p.blocks[start:end]
	if len(page) > p.widestRow {
		p.widestRow = len(page)
	}
	return page, nil
}

func corpus(n int) []*venue.StoredBlock {
	out := make([]*venue.StoredBlock, 0, n)
	for i := range n {
		b := &model.Block{ID: fmt.Sprintf("b%05d", i), Translatable: true}
		out = append(out, &venue.StoredBlock{Block: b, ItemName: "en.json"})
	}
	return out
}

// The property the walk exists for: memory is the batch, not the corpus.
//
// Reading a project in one query is what OOM-killed the production server — it
// idles at ~16 MiB and one such read drove it past 934 MiB of a 1 GiB task.
func TestEachBlockBatch_HoldsABatchNotTheCorpus(t *testing.T) {
	cs := &pagedStore{blocks: corpus(2500)}

	seen := 0
	require.NoError(t, platstore.EachBlockBatch(t.Context(), cs,
		platstore.BlockQuery{ProjectID: "p1", Stream: "main"}, 100,
		func(blocks []*venue.StoredBlock) error {
			assert.LessOrEqual(t, len(blocks), 100, "a batch must never exceed what was asked for")
			seen += len(blocks)
			return nil
		}))

	assert.Equal(t, 2500, seen, "every block is visited exactly once")
	assert.Equal(t, 100, cs.widestRow, "the store is never asked for more than a batch")
	// 25 full pages, and one more that comes back empty: a page that fills the
	// batch cannot be known to be the last without asking again.
	assert.Equal(t, 26, cs.calls)
}

// The walk carries its own paging: a caller passes the scope and nothing else,
// so a scope that happens to carry a stale Limit or Offset cannot truncate it.
func TestEachBlockBatch_OwnsThePaging(t *testing.T) {
	cs := &pagedStore{blocks: corpus(30)}

	seen := 0
	require.NoError(t, platstore.EachBlockBatch(t.Context(), cs,
		platstore.BlockQuery{ProjectID: "p1", Limit: 3, Offset: 27}, 10,
		func(blocks []*venue.StoredBlock) error {
			seen += len(blocks)
			return nil
		}))

	assert.Equal(t, 30, seen)
	assert.Zero(t, cs.lastQuery.Offset, "the walk is a keyset scan, not an offset one")
}

// An error from the visitor stops the walk where it happened, so a caller that
// fails on batch three does not pay for batch four.
func TestEachBlockBatch_StopsOnTheVisitorsError(t *testing.T) {
	cs := &pagedStore{blocks: corpus(1000)}

	boom := fmt.Errorf("persist failed")
	err := platstore.EachBlockBatch(t.Context(), cs,
		platstore.BlockQuery{ProjectID: "p1"}, 100,
		func(blocks []*venue.StoredBlock) error { return boom })

	require.ErrorIs(t, err, boom)
	assert.Equal(t, 1, cs.calls, "the walk stops at the batch that failed")
}

// A corpus that divides evenly must not ask for a page beyond the end and must
// not visit an empty batch.
func TestEachBlockBatch_ExactMultipleEndsCleanly(t *testing.T) {
	cs := &pagedStore{blocks: corpus(200)}

	batches := 0
	require.NoError(t, platstore.EachBlockBatch(t.Context(), cs,
		platstore.BlockQuery{ProjectID: "p1"}, 100,
		func(blocks []*venue.StoredBlock) error {
			assert.NotEmpty(t, blocks, "an empty batch is never visited")
			batches++
			return nil
		}))

	assert.Equal(t, 2, batches)
	assert.Equal(t, 3, cs.calls, "the third asks past the end and gets nothing")
}

func TestEachBlockBatch_EmptyProjectVisitsNothing(t *testing.T) {
	cs := &pagedStore{}
	batches := 0
	require.NoError(t, platstore.EachBlockBatch(t.Context(), cs,
		platstore.BlockQuery{ProjectID: "p1"}, 100,
		func([]*venue.StoredBlock) error { batches++; return nil }))
	assert.Zero(t, batches)
}

// A store that hands back rows it cannot name would make the cursor stand
// still, and the walk would read the same page forever. It stops and says so.
func TestEachBlockBatch_RefusesToSpinOnAnUnnamedBlock(t *testing.T) {
	cs := &pagedStore{blocks: []*venue.StoredBlock{
		{Block: &model.Block{ID: ""}},
		{Block: &model.Block{ID: ""}},
	}}

	err := platstore.EachBlockBatch(t.Context(), cs,
		platstore.BlockQuery{ProjectID: "p1"}, 2,
		func([]*venue.StoredBlock) error { return nil })

	require.ErrorContains(t, err, "stalled")
}
