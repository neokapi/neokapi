package flow

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// passThroughTool is a minimal tool that forwards all parts unchanged.
type passThroughTool struct{ tool.BaseTool }

func newPassThroughTool(name string) *passThroughTool {
	t := &passThroughTool{}
	t.BaseTool.ToolName = name
	return t
}

func TestPipelineMetrics_Snapshot(t *testing.T) {
	pm := NewPipelineMetrics([]string{"step-a", "step-b"})
	require.Len(t, pm.Steps, 2)

	snap := pm.Snapshot()
	assert.Equal(t, "step-a", snap[0].Name)
	assert.Equal(t, "step-b", snap[1].Name)
	assert.Equal(t, int64(0), snap[0].PartsIn)
	assert.Equal(t, int64(0), snap[0].PartsOut)
}

func TestPipelineMetrics_Reset(t *testing.T) {
	pm := NewPipelineMetrics([]string{"x"})
	pm.Steps[0].PartsIn.Store(10)
	pm.Steps[0].PartsOut.Store(5)
	pm.Reset()

	snap := pm.Snapshot()
	assert.Equal(t, int64(0), snap[0].PartsIn)
	assert.Equal(t, int64(0), snap[0].PartsOut)
}

func TestMetricsTool_CountsParts(t *testing.T) {
	pm := NewPipelineMetrics([]string{"echo"})
	inner := newPassThroughTool("echo")
	wrapped := WrapWithMetrics([]tool.Tool{inner}, pm)

	in := make(chan *model.Part, 3)
	out := make(chan *model.Part, 3)

	// Feed 3 parts.
	for i := range 3 {
		in <- &model.Part{Type: model.PartData, Resource: &model.Data{ID: string(rune('a' + i))}}
	}
	close(in)

	err := wrapped[0].Process(context.Background(), in, out)
	require.NoError(t, err)
	close(out)

	// Drain output.
	var received int
	for range out {
		received++
	}
	assert.Equal(t, 3, received)

	snap := pm.Snapshot()
	assert.Equal(t, int64(3), snap[0].PartsIn)
	assert.Equal(t, int64(3), snap[0].PartsOut)
}

func TestMetricsTool_MultiToolChain(t *testing.T) {
	pm := NewPipelineMetrics([]string{"first", "second"})
	tools := WrapWithMetrics([]tool.Tool{
		newPassThroughTool("first"),
		newPassThroughTool("second"),
	}, pm)

	// Wire up: in → tool0 → mid → tool1 → out
	in := make(chan *model.Part, 5)
	mid := make(chan *model.Part, 5)
	out := make(chan *model.Part, 5)

	for i := range 5 {
		in <- &model.Part{Type: model.PartData, Resource: &model.Data{ID: string(rune('a' + i))}}
	}
	close(in)

	// Run first tool.
	err := tools[0].Process(context.Background(), in, mid)
	require.NoError(t, err)
	close(mid)

	// Run second tool.
	err = tools[1].Process(context.Background(), mid, out)
	require.NoError(t, err)
	close(out)

	// Drain.
	var count int
	for range out {
		count++
	}
	assert.Equal(t, 5, count)

	snap := pm.Snapshot()
	assert.Equal(t, int64(5), snap[0].PartsIn)
	assert.Equal(t, int64(5), snap[0].PartsOut)
	assert.Equal(t, int64(5), snap[1].PartsIn)
	assert.Equal(t, int64(5), snap[1].PartsOut)
}

func TestMetricsTool_DelegatesName(t *testing.T) {
	pm := NewPipelineMetrics([]string{"myTool"})
	inner := newPassThroughTool("myTool")
	wrapped := WrapWithMetrics([]tool.Tool{inner}, pm)

	assert.Equal(t, "myTool", wrapped[0].Name())
}

func TestWrapWithMetrics_PanicsOnMismatch(t *testing.T) {
	pm := NewPipelineMetrics([]string{"a"})
	assert.Panics(t, func() {
		WrapWithMetrics([]tool.Tool{newPassThroughTool("a"), newPassThroughTool("b")}, pm)
	})
}
