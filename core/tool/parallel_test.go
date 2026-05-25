package tool_test

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeBlock(id, text string) *model.Part {
	return &model.Part{
		Type:     model.PartBlock,
		Resource: model.NewBlock(id, text),
	}
}

func makeData(id string) *model.Part {
	return &model.Part{
		Type:     model.PartData,
		Resource: &model.Data{ID: id, Name: id},
	}
}

func TestParallelBlockTool_OrderPreserved(t *testing.T) {
	// Create a tool that appends " [processed]" to block source text.
	inner := &tool.BaseTool{
		ToolName: "test-tool",
		Annotate: func(v tool.BlockView) error {
			v.SetProperty("processed", "true")
			return nil
		},
	}

	pt := tool.NewParallelBlockTool(inner, 4)

	parts := []*model.Part{
		makeBlock("b1", "Hello"),
		makeData("d1"),
		makeBlock("b2", "World"),
		makeData("d2"),
		makeBlock("b3", "Foo"),
		makeBlock("b4", "Bar"),
		makeData("d3"),
	}

	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))

	for _, p := range parts {
		in <- p
	}
	close(in)

	err := pt.Process(t.Context(), in, out)
	require.NoError(t, err)
	close(out)

	var results []*model.Part
	for p := range out {
		results = append(results, p)
	}

	// Verify order is preserved.
	require.Len(t, results, 7)

	assert.Equal(t, model.PartBlock, results[0].Type)
	assert.Equal(t, "b1", results[0].Resource.ResourceID())

	assert.Equal(t, model.PartData, results[1].Type)
	assert.Equal(t, "d1", results[1].Resource.ResourceID())

	assert.Equal(t, model.PartBlock, results[2].Type)
	assert.Equal(t, "b2", results[2].Resource.ResourceID())

	assert.Equal(t, model.PartData, results[3].Type)
	assert.Equal(t, "d2", results[3].Resource.ResourceID())

	assert.Equal(t, model.PartBlock, results[4].Type)
	assert.Equal(t, "b3", results[4].Resource.ResourceID())

	assert.Equal(t, model.PartBlock, results[5].Type)
	assert.Equal(t, "b4", results[5].Resource.ResourceID())

	assert.Equal(t, model.PartData, results[6].Type)
	assert.Equal(t, "d3", results[6].Resource.ResourceID())

	// Verify blocks were processed.
	for _, r := range results {
		if r.Type == model.PartBlock {
			block := r.Resource.(*model.Block)
			assert.Equal(t, "true", block.Properties["processed"])
		}
	}
}

func TestParallelBlockTool_Concurrency(t *testing.T) {
	var active atomic.Int32
	var maxActive atomic.Int32

	inner := &tool.BaseTool{
		ToolName: "slow-tool",
		Annotate: func(v tool.BlockView) error {
			cur := active.Add(1)
			// Track max concurrency.
			for {
				old := maxActive.Load()
				if cur <= old || maxActive.CompareAndSwap(old, cur) {
					break
				}
			}
			time.Sleep(10 * time.Millisecond)
			active.Add(-1)
			return nil
		},
	}

	concurrency := 4
	pt := tool.NewParallelBlockTool(inner, concurrency)

	// Send 20 blocks to see concurrency in action.
	numParts := 20
	in := make(chan *model.Part, numParts)
	out := make(chan *model.Part, numParts)

	for i := range numParts {
		in <- makeBlock(fmt.Sprintf("b%d", i), fmt.Sprintf("text %d", i))
	}
	close(in)

	err := pt.Process(t.Context(), in, out)
	require.NoError(t, err)
	close(out)

	var count int
	for range out {
		count++
	}
	assert.Equal(t, numParts, count)

	// Max active should be > 1 (parallelism achieved).
	assert.Greater(t, int(maxActive.Load()), 1, "expected parallel execution")
	// Max active should not exceed concurrency limit.
	assert.LessOrEqual(t, int(maxActive.Load()), concurrency, "exceeded concurrency limit")
}

func TestParallelBlockTool_ErrorPropagation(t *testing.T) {
	inner := &tool.BaseTool{
		ToolName: "error-tool",
		Annotate: func(v tool.BlockView) error {
			if v.ID() == "b3" {
				return errors.New("processing error on b3")
			}
			return nil
		},
	}

	pt := tool.NewParallelBlockTool(inner, 2)

	parts := []*model.Part{
		makeBlock("b1", "ok"),
		makeBlock("b2", "ok"),
		makeBlock("b3", "will fail"),
		makeBlock("b4", "ok"),
	}

	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))

	for _, p := range parts {
		in <- p
	}
	close(in)

	err := pt.Process(t.Context(), in, out)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "processing error on b3")
}

func TestParallelBlockTool_Cancellation(t *testing.T) {
	inner := &tool.BaseTool{
		ToolName: "slow-cancel-tool",
		Annotate: func(v tool.BlockView) error {
			time.Sleep(100 * time.Millisecond)
			return nil
		},
	}

	pt := tool.NewParallelBlockTool(inner, 2)

	ctx, cancel := context.WithCancel(t.Context())

	in := make(chan *model.Part, 10)
	out := make(chan *model.Part, 10)

	for i := range 10 {
		in <- makeBlock(fmt.Sprintf("b%d", i), "text")
	}
	close(in)

	// Cancel quickly.
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()

	err := pt.Process(ctx, in, out)
	require.Error(t, err)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestParallelBlockTool_FallbackToSequential(t *testing.T) {
	// When concurrency is 1, should fall back to inner tool.
	var called bool
	inner := &tool.BaseTool{
		ToolName: "seq-tool",
		Annotate: func(v tool.BlockView) error {
			called = true
			return nil
		},
	}

	pt := tool.NewParallelBlockTool(inner, 1)

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- makeBlock("b1", "hello")
	close(in)

	err := pt.Process(t.Context(), in, out)
	require.NoError(t, err)
	close(out)

	assert.True(t, called)
	assert.Len(t, collectParts(out), 1)
}

func TestParallelBlockTool_EmptyInput(t *testing.T) {
	inner := &tool.BaseTool{ToolName: "empty-test"}
	pt := tool.NewParallelBlockTool(inner, 4)

	in := make(chan *model.Part)
	out := make(chan *model.Part, 1)
	close(in)

	err := pt.Process(t.Context(), in, out)
	require.NoError(t, err)
	close(out)

	assert.Empty(t, collectParts(out))
}

func TestParallelBlockTool_NonBlockOnly(t *testing.T) {
	inner := &tool.BaseTool{
		ToolName: "data-only",
		HandleDataFn: func(part *model.Part) (*model.Part, error) {
			return part, nil
		},
		Annotate: func(v tool.BlockView) error {
			t.Fatal("Annotate should not be called for non-block parts")
			return nil
		},
	}

	pt := tool.NewParallelBlockTool(inner, 4)

	parts := []*model.Part{
		makeData("d1"),
		makeData("d2"),
		makeData("d3"),
	}

	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))

	for _, p := range parts {
		in <- p
	}
	close(in)

	err := pt.Process(t.Context(), in, out)
	require.NoError(t, err)
	close(out)

	results := collectParts(out)
	require.Len(t, results, 3)
	for i, r := range results {
		assert.Equal(t, fmt.Sprintf("d%d", i+1), r.Resource.ResourceID())
	}
}

func collectParts(ch <-chan *model.Part) []*model.Part {
	var parts []*model.Part
	for p := range ch {
		parts = append(parts, p)
	}
	return parts
}
