package flow_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// panickingTool panics in its block handler — the fixture for a crafted
// document that trips a bug in tool code.
func panickingTool(name string) *tool.BaseTool {
	return &tool.BaseTool{
		ToolName: name,
		Annotate: func(v tool.BlockView) error {
			panic("boom: crafted input")
		},
	}
}

// processPanicTool panics as soon as the executor invokes its Process,
// covering tools that override Process rather than set a block handler.
type processPanicTool struct {
	tool.BaseTool
}

func (p *processPanicTool) Process(context.Context, <-chan *model.Part, chan<- *model.Part) error {
	panic("boom: process")
}

// TestExecutorRecoversToolPanic verifies that a panic in a tool goroutine is
// recovered into a flow error (carrying the tool name and panic value)
// instead of crashing the process. Goroutine cleanup is enforced by the
// package's goleak TestMain.
func TestExecutorRecoversToolPanic(t *testing.T) {
	f, err := flow.NewFlow("panic-flow").
		AddTool(passThroughTool("pre")).
		AddTool(panickingTool("panicky")).
		AddTool(passThroughTool("post")).
		Build()
	require.NoError(t, err)

	executor := flow.NewExecutor()
	in, out, wait := executor.ExecuteWithChannels(t.Context(), f)

	go func() {
		in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("b1", "Hello")}
		close(in)
	}()
	for range out { //nolint:revive // drain so the pipeline can finish
	}

	err = wait()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool panicky")
	assert.Contains(t, err.Error(), "panicked")
	assert.Contains(t, err.Error(), "boom: crafted input")
}

// TestExecutorRecoversProcessPanic covers the Execute (batch) path with a
// tool that panics directly in Process.
func TestExecutorRecoversProcessPanic(t *testing.T) {
	f, err := flow.NewFlow("panic-flow").
		AddTool(&processPanicTool{tool.BaseTool{ToolName: "exploder"}}).
		Build()
	require.NoError(t, err)

	executor := flow.NewExecutor()
	items := []*flow.Item{{Input: &model.RawDocument{URI: "test://doc"}}}

	err = executor.Execute(t.Context(), f, items)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "tool exploder")
	assert.Contains(t, err.Error(), "panicked")
	assert.Contains(t, err.Error(), "boom: process")
}
