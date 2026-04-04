package tools

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBatchTool_CollectsBlocks(t *testing.T) {
	bt := NewBatchTool(&BatchConfig{Size: 3})

	in := make(chan *model.Part, 10)
	out := make(chan *model.Part, 10)

	// Send 5 blocks
	for range 5 {
		in <- &model.Part{
			Type:     model.PartBlock,
			Resource: &model.Block{},
		}
	}
	close(in)

	err := bt.Process(t.Context(), in, out)
	require.NoError(t, err)
	close(out)

	var received []*model.Part
	for p := range out {
		received = append(received, p)
	}
	// All 5 blocks should come through (3 in first batch, 2 flushed at end)
	assert.Len(t, received, 5)
}

func TestBatchTool_NonBlocksPassThrough(t *testing.T) {
	bt := NewBatchTool(&BatchConfig{Size: 10})

	in := make(chan *model.Part, 10)
	out := make(chan *model.Part, 10)

	// Mix of blocks and data parts
	in <- &model.Part{Type: model.PartData}
	in <- &model.Part{Type: model.PartBlock, Resource: &model.Block{}}
	in <- &model.Part{Type: model.PartLayerStart}
	in <- &model.Part{Type: model.PartBlock, Resource: &model.Block{}}
	in <- &model.Part{Type: model.PartData}
	close(in)

	err := bt.Process(t.Context(), in, out)
	require.NoError(t, err)
	close(out)

	var received []*model.Part
	for p := range out {
		received = append(received, p)
	}
	assert.Len(t, received, 5)

	// Non-block parts should come first (they pass through immediately)
	assert.Equal(t, model.PartData, received[0].Type)
	assert.Equal(t, model.PartLayerStart, received[1].Type)
	assert.Equal(t, model.PartData, received[2].Type)
	// Blocks are flushed at end (batch size 10 not reached)
	assert.Equal(t, model.PartBlock, received[3].Type)
	assert.Equal(t, model.PartBlock, received[4].Type)
}

func TestBatchTool_EmptyStream(t *testing.T) {
	bt := NewBatchTool(&BatchConfig{Size: 5})

	in := make(chan *model.Part)
	out := make(chan *model.Part, 10)
	close(in)

	err := bt.Process(t.Context(), in, out)
	require.NoError(t, err)
	close(out)

	var received []*model.Part
	for p := range out {
		received = append(received, p)
	}
	assert.Empty(t, received)
}

func TestBatchTool_CancelContext(t *testing.T) {
	bt := NewBatchTool(&BatchConfig{Size: 100})

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)

	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Cancel immediately

	in <- &model.Part{Type: model.PartBlock, Resource: &model.Block{}}

	err := bt.Process(ctx, in, out)
	require.Error(t, err)
}

func TestBatchConfig_Validate(t *testing.T) {
	cfg := &BatchConfig{Size: 0}
	require.Error(t, cfg.Validate())

	cfg.Size = 1
	require.NoError(t, cfg.Validate())
}
