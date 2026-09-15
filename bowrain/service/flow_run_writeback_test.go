package service

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/core/venue"
)

// removeItemAfterRead removes an item once, right after the flow reads it and
// before the flow writes its output back.
type removeItemAfterRead struct {
	store.ContentStore
	item   string
	once   sync.Once
	change func(ctx context.Context)
}

func (s *removeItemAfterRead) GetBlocks(ctx context.Context, q store.BlockQuery) ([]*venue.StoredBlock, error) {
	blocks, err := s.ContentStore.GetBlocks(ctx, q)
	if q.ItemName == s.item {
		s.once.Do(func() { s.change(ctx) })
	}
	return blocks, err
}

// An item removed while a flow runs over it stays removed: the flow's output
// lands only on the rows it read.
func TestFlowServiceRunFlow_AnItemRemovedDuringTheRunStaysRemoved(t *testing.T) {
	_, cs, _ := newFlowRunFixture(t)
	racing := &removeItemAfterRead{ContentStore: cs, item: "a.json", change: func(ctx context.Context) {
		assert.NoError(t, cs.DeleteItem(ctx, "p1", "main", "a.json"))
	}}
	reg := registry.NewToolRegistry()
	tools.RegisterAll(reg)
	fs := NewFlowService(racing, nil, reg)
	fs.tracker = &recordingTracker{}

	_, err := fs.RunFlow(context.Background(), FlowRun{
		Definition:    builtInFlow(t, "pseudo-translate"),
		ProjectID:     "p1",
		Items:         []string{"a.json"},
		TargetLocales: []string{"fr"},
	})
	require.NoError(t, err)
	assert.Empty(t, itemBlocks(t, cs, "a.json"), "the removed item's blocks are not stored again")
	all, err := cs.GetBlocks(context.Background(), store.BlockQuery{ProjectID: "p1", Stream: "main"})
	require.NoError(t, err)
	for _, sb := range all {
		assert.Equal(t, "b.json", sb.ItemName, "block %s is stored again outside any item it was read from", sb.Block.ID)
	}
}
