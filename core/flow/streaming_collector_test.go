package flow_test

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/gokapi/gokapi/core/flow"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testStreamingCollector counts observed parts.
type testStreamingCollector struct {
	observed atomic.Int32
}

func (c *testStreamingCollector) Observe(part *model.Part) {
	c.observed.Add(1)
}

func (c *testStreamingCollector) Collect(_ context.Context, _ *flow.FlowItem, _ []*model.Part) error {
	return nil
}

func (c *testStreamingCollector) Result() (flow.CollectorResult, error) {
	return flow.CollectorResult{
		Name: "test-streaming",
		Data: int(c.observed.Load()),
	}, nil
}

func TestTappingTool_ObservesAllParts(t *testing.T) {
	inner := &tool.BaseTool{
		ToolName: "passthrough",
		HandleBlockFn: func(part *model.Part) (*model.Part, error) {
			return part, nil
		},
	}

	collector := &testStreamingCollector{}
	tapper := flow.NewTappingTool(inner, collector)

	parts := []*model.Part{
		{Type: model.PartBlock, Resource: model.NewBlock("b1", "Hello")},
		{Type: model.PartData, Resource: &model.Data{ID: "d1", Name: "d1"}},
		{Type: model.PartBlock, Resource: model.NewBlock("b2", "World")},
	}

	in := make(chan *model.Part, len(parts))
	out := make(chan *model.Part, len(parts))

	for _, p := range parts {
		in <- p
	}
	close(in)

	err := tapper.Process(context.Background(), in, out)
	require.NoError(t, err)
	close(out)

	// All 3 parts should be observed.
	assert.Equal(t, int32(3), collector.observed.Load())

	// All 3 parts should be forwarded.
	var results []*model.Part
	for p := range out {
		results = append(results, p)
	}
	assert.Len(t, results, 3)
}

func TestTappingTool_DoesNotMutateParts(t *testing.T) {
	inner := &tool.BaseTool{
		ToolName: "uppercase-tool",
		HandleBlockFn: func(part *model.Part) (*model.Part, error) {
			// Tool modifies the block (this is normal).
			block := part.Resource.(*model.Block)
			block.Properties = map[string]string{"touched": "true"}
			return part, nil
		},
	}

	collector := &testStreamingCollector{}
	tapper := flow.NewTappingTool(inner, collector)

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("b1", "test")}
	close(in)

	err := tapper.Process(context.Background(), in, out)
	require.NoError(t, err)
	close(out)

	// The tapper should observe the modified part (after inner processes it).
	assert.Equal(t, int32(1), collector.observed.Load())

	result := <-out
	block := result.Resource.(*model.Block)
	assert.Equal(t, "true", block.Properties["touched"])
}

func TestTappingTool_EmptyInput(t *testing.T) {
	inner := &tool.BaseTool{ToolName: "empty"}
	collector := &testStreamingCollector{}
	tapper := flow.NewTappingTool(inner, collector)

	in := make(chan *model.Part)
	out := make(chan *model.Part, 1)
	close(in)

	err := tapper.Process(context.Background(), in, out)
	require.NoError(t, err)
	close(out)

	assert.Equal(t, int32(0), collector.observed.Load())
}

func TestTappingTool_ImplementsTool(t *testing.T) {
	inner := &tool.BaseTool{ToolName: "test", ToolDescription: "desc"}
	collector := &testStreamingCollector{}
	tapper := flow.NewTappingTool(inner, collector)

	assert.Equal(t, "test", tapper.Name())
	assert.Equal(t, "desc", tapper.Description())
	assert.Nil(t, tapper.Config())
}
