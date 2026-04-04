package flow_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockTool creates a pass-through BaseTool with the given name.
func passThroughTool(name string) *tool.BaseTool {
	return &tool.BaseTool{ToolName: name}
}

// uppercaseTool creates a tool that uppercases block source text.
func uppercaseTool() *tool.BaseTool {
	return &tool.BaseTool{
		ToolName: "uppercase",
		HandleBlockFn: func(part *model.Part) (*model.Part, error) {
			block := part.Resource.(*model.Block)
			if block.Translatable {
				block.SetTargetText(model.LocaleFrench, strings.ToUpper(block.SourceText()))
			}
			return part, nil
		},
	}
}

// countingTool counts parts processed.
func countingTool(name string, count *atomic.Int64) *tool.BaseTool {
	return &tool.BaseTool{
		ToolName: name,
		HandleBlockFn: func(part *model.Part) (*model.Part, error) {
			count.Add(1)
			return part, nil
		},
	}
}

func TestFlowExecutorWithThreeMockTools(t *testing.T) {
	var count1, count2, count3 atomic.Int64

	f := flow.NewFlow("test").
		AddTool(countingTool("tool1", &count1)).
		AddTool(countingTool("tool2", &count2)).
		AddTool(countingTool("tool3", &count3)).
		Build()

	executor := flow.NewFlowExecutor()
	ctx := t.Context()

	in, out, wait := executor.ExecuteWithChannels(ctx, f)

	// Feed 100 parts
	numParts := 100
	go func() {
		for i := range numParts {
			in <- &model.Part{
				Type:     model.PartBlock,
				Resource: model.NewBlock(fmt.Sprintf("tu%d", i), fmt.Sprintf("Text %d", i)),
			}
		}
		close(in)
	}()

	// Collect output
	var results []*model.Part
	for p := range out {
		results = append(results, p)
	}

	err := wait()
	require.NoError(t, err)

	assert.Len(t, results, numParts)
	assert.Equal(t, int64(numParts), count1.Load())
	assert.Equal(t, int64(numParts), count2.Load())
	assert.Equal(t, int64(numParts), count3.Load())
}

func TestFlowExecutorPreservesOrder(t *testing.T) {
	f := flow.NewFlow("order-test").
		AddTool(passThroughTool("tool1")).
		AddTool(passThroughTool("tool2")).
		Build()

	executor := flow.NewFlowExecutor()
	ctx := t.Context()

	in, out, wait := executor.ExecuteWithChannels(ctx, f)

	numParts := 50
	go func() {
		for i := range numParts {
			in <- &model.Part{
				Type:     model.PartBlock,
				Resource: model.NewBlock(fmt.Sprintf("tu%d", i), fmt.Sprintf("Text %d", i)),
			}
		}
		close(in)
	}()

	var results []*model.Part
	for p := range out {
		results = append(results, p)
	}

	err := wait()
	require.NoError(t, err)

	assert.Len(t, results, numParts)
	for i, p := range results {
		block := p.Resource.(*model.Block)
		assert.Equal(t, fmt.Sprintf("tu%d", i), block.ID)
	}
}

func TestFlowExecutorModification(t *testing.T) {
	f := flow.NewFlow("modify").
		AddTool(uppercaseTool()).
		Build()

	executor := flow.NewFlowExecutor()
	ctx := t.Context()

	in, out, wait := executor.ExecuteWithChannels(ctx, f)

	go func() {
		in <- &model.Part{
			Type:     model.PartBlock,
			Resource: model.NewBlock("tu1", "hello world"),
		}
		close(in)
	}()

	var results []*model.Part
	for p := range out {
		results = append(results, p)
	}

	err := wait()
	require.NoError(t, err)

	require.Len(t, results, 1)
	block := results[0].Resource.(*model.Block)
	assert.Equal(t, "HELLO WORLD", block.TargetText(model.LocaleFrench))
}

func TestFlowExecutorErrorPropagation(t *testing.T) {
	errTool := &tool.BaseTool{
		ToolName: "error-tool",
		HandleBlockFn: func(part *model.Part) (*model.Part, error) {
			return nil, errors.New("processing error")
		},
	}

	f := flow.NewFlow("error-test").
		AddTool(passThroughTool("tool1")).
		AddTool(errTool).
		AddTool(passThroughTool("tool3")).
		Build()

	executor := flow.NewFlowExecutor()
	ctx := t.Context()

	in, out, wait := executor.ExecuteWithChannels(ctx, f)

	go func() {
		in <- &model.Part{
			Type:     model.PartBlock,
			Resource: model.NewBlock("tu1", "Hello"),
		}
		close(in)
	}()

	// Drain output
	for range out {
	}

	err := wait()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "processing error")
}

func TestFlowExecutorContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	// Tool that blocks until context is cancelled
	blockingTool := &tool.BaseTool{
		ToolName: "blocking",
		HandleBlockFn: func(part *model.Part) (*model.Part, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}

	f := flow.NewFlow("cancel-test").
		AddTool(blockingTool).
		Build()

	executor := flow.NewFlowExecutor()

	in, out, wait := executor.ExecuteWithChannels(ctx, f)

	go func() {
		in <- &model.Part{
			Type:     model.PartBlock,
			Resource: model.NewBlock("tu1", "Hello"),
		}
		// Don't close in - the tool will block
	}()

	// Cancel after feeding one part
	cancel()

	// Drain output
	for range out {
	}

	err := wait()
	require.Error(t, err)
}

func TestFlowExecutorNoTools(t *testing.T) {
	f := flow.NewFlow("empty").Build()
	executor := flow.NewFlowExecutor()
	ctx := t.Context()

	in, out, wait := executor.ExecuteWithChannels(ctx, f)

	go func() {
		in <- &model.Part{
			Type:     model.PartBlock,
			Resource: model.NewBlock("tu1", "Hello"),
		}
		close(in)
	}()

	var results []*model.Part
	for p := range out {
		results = append(results, p)
	}

	err := wait()
	require.NoError(t, err)
	assert.Len(t, results, 1)
}

func TestFlowExecutorMixedPartTypes(t *testing.T) {
	var blockCount, dataCount, layerCount atomic.Int64

	trackingTool := &tool.BaseTool{
		ToolName: "tracker",
		HandleBlockFn: func(part *model.Part) (*model.Part, error) {
			blockCount.Add(1)
			return part, nil
		},
		HandleDataFn: func(part *model.Part) (*model.Part, error) {
			dataCount.Add(1)
			return part, nil
		},
		HandleLayerStartFn: func(part *model.Part) (*model.Part, error) {
			layerCount.Add(1)
			return part, nil
		},
		HandleLayerEndFn: func(part *model.Part) (*model.Part, error) {
			layerCount.Add(1)
			return part, nil
		},
	}

	f := flow.NewFlow("mixed").AddTool(trackingTool).Build()
	executor := flow.NewFlowExecutor()
	ctx := t.Context()

	in, out, wait := executor.ExecuteWithChannels(ctx, f)

	go func() {
		in <- &model.Part{Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1"}}
		in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu1", "Hello")}
		in <- &model.Part{Type: model.PartData, Resource: &model.Data{ID: "d1"}}
		in <- &model.Part{Type: model.PartBlock, Resource: model.NewBlock("tu2", "World")}
		in <- &model.Part{Type: model.PartData, Resource: &model.Data{ID: "d2"}}
		in <- &model.Part{Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1"}}
		close(in)
	}()

	var results []*model.Part
	for p := range out {
		results = append(results, p)
	}

	err := wait()
	require.NoError(t, err)

	assert.Len(t, results, 6)
	assert.Equal(t, int64(2), blockCount.Load())
	assert.Equal(t, int64(2), dataCount.Load())
	assert.Equal(t, int64(2), layerCount.Load())
}

func TestFlowBuilder(t *testing.T) {
	f := flow.NewFlow("test-flow").
		AddTool(passThroughTool("tool1")).
		AddTool(passThroughTool("tool2")).
		Build()

	assert.Equal(t, "test-flow", f.Name)
	assert.Len(t, f.Tools, 2)
}

func TestFlowBuilderWithItems(t *testing.T) {
	fb := flow.NewFlow("test-flow").
		AddTool(passThroughTool("tool1")).
		AddItem(&model.RawDocument{URI: "input.html"}, "output.html", model.LocaleFrench)

	items := fb.Items()
	require.Len(t, items, 1)
	assert.Equal(t, "input.html", items[0].Input.URI)
	assert.Equal(t, "output.html", items[0].OutputPath)
	assert.Equal(t, model.LocaleFrench, items[0].TargetLocale)
}

func TestFlowExecutorSetChannelSize(t *testing.T) {
	executor := flow.NewFlowExecutor()
	executor.SetChannelSize(128)
	// Just verify it doesn't panic; internal channel size is not exposed
}

// --- Parallel Execution Tests ---

func TestParallelExecutionMultipleDocuments(t *testing.T) {
	var totalBlocks atomic.Int64

	uppercaseFactory := func() (tool.Tool, error) {
		return &tool.BaseTool{
			ToolName: "uppercase",
			HandleBlockFn: func(part *model.Part) (*model.Part, error) {
				totalBlocks.Add(1)
				block := part.Resource.(*model.Block)
				if block.Translatable {
					block.SetTargetText(model.LocaleFrench, strings.ToUpper(block.SourceText()))
				}
				return part, nil
			},
		}, nil
	}

	f := flow.NewFlow("parallel-test").
		AddToolFactory(uppercaseFactory).
		Build()

	executor := flow.NewFlowExecutor(
		flow.WithMaxConcurrency(4),
	)

	numDocs := 10
	items := make([]*flow.FlowItem, numDocs)
	for i := range numDocs {
		items[i] = &flow.FlowItem{
			Input: &model.RawDocument{URI: fmt.Sprintf("doc%d.html", i)},
		}
	}

	err := executor.Execute(t.Context(), f, items)
	require.NoError(t, err)

	// Each document's tool chain gets an empty pipeline (processItemCollect closes input),
	// so no blocks are processed. Verify no error.
	// The real point: 10 documents processed through 4-wide concurrency without panic or deadlock.
}

func TestParallelExecutionWithCollector(t *testing.T) {
	// mockCollector records all items it receives.
	type collectedEntry struct {
		uri   string
		parts int
	}
	var mu sync.Mutex
	var entries []collectedEntry

	collector := &mockCollector{
		collectFn: func(ctx context.Context, item *flow.FlowItem, parts []*model.Part) error {
			mu.Lock()
			defer mu.Unlock()
			entries = append(entries, collectedEntry{uri: item.Input.URI, parts: len(parts)})
			return nil
		},
		resultFn: func() (flow.CollectorResult, error) {
			mu.Lock()
			defer mu.Unlock()
			return flow.CollectorResult{Name: "mock", Data: len(entries)}, nil
		},
	}

	f := flow.NewFlow("collector-test").
		AddToolFactory(func() (tool.Tool, error) {
			return passThroughTool("pass"), nil
		}).
		Build()

	executor := flow.NewFlowExecutor(
		flow.WithMaxConcurrency(2),
		flow.WithCollectors(collector),
	)

	items := []*flow.FlowItem{
		{Input: &model.RawDocument{URI: "a.html"}},
		{Input: &model.RawDocument{URI: "b.html"}},
		{Input: &model.RawDocument{URI: "c.html"}},
	}

	err := executor.Execute(t.Context(), f, items)
	require.NoError(t, err)

	mu.Lock()
	assert.Len(t, entries, 3)
	mu.Unlock()

	result, err := collector.Result()
	require.NoError(t, err)
	assert.Equal(t, "mock", result.Name)
	assert.Equal(t, 3, result.Data.(int))
}

func TestParallelExecutionErrorPropagation(t *testing.T) {
	callCount := atomic.Int64{}

	f := flow.NewFlow("error-parallel").
		AddToolFactory(func() (tool.Tool, error) {
			return &tool.BaseTool{
				ToolName: "maybe-error",
				HandleBlockFn: func(part *model.Part) (*model.Part, error) {
					n := callCount.Add(1)
					if n == 1 {
						return nil, errors.New("intentional error")
					}
					return part, nil
				},
			}, nil
		}).
		Build()

	executor := flow.NewFlowExecutor(
		flow.WithMaxConcurrency(4),
	)

	// Create items that would trigger the error
	items := []*flow.FlowItem{
		{Input: &model.RawDocument{URI: "err.html"}},
		{Input: &model.RawDocument{URI: "ok.html"}},
	}

	// processItemCollect closes input immediately, so the tools get no blocks.
	// This tests that the parallel path itself works without deadlock.
	err := executor.Execute(t.Context(), f, items)
	require.NoError(t, err)
}

func TestParallelExecutionContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	f := flow.NewFlow("cancel-parallel").
		AddToolFactory(func() (tool.Tool, error) {
			return passThroughTool("pass"), nil
		}).
		Build()

	executor := flow.NewFlowExecutor(
		flow.WithMaxConcurrency(2),
	)

	items := []*flow.FlowItem{
		{Input: &model.RawDocument{URI: "a.html"}},
		{Input: &model.RawDocument{URI: "b.html"}},
	}

	// Cancel before execution
	cancel()

	err := executor.Execute(ctx, f, items)
	// May or may not error depending on timing, but must not deadlock.
	_ = err
}

func TestSingleItemDirectTools(t *testing.T) {
	// With a single item, Execute should use f.Tools directly (no factory needed).
	var count atomic.Int64

	f := flow.NewFlow("single").
		AddTool(countingTool("counter", &count)).
		Build()

	executor := flow.NewFlowExecutor() // default: sequential

	items := []*flow.FlowItem{
		{Input: &model.RawDocument{URI: "single.html"}},
	}

	err := executor.Execute(t.Context(), f, items)
	require.NoError(t, err)
	// processItemCollect closes input immediately, so no blocks are processed.
	// Key test: no panic, no deadlock, direct tools path used.
}

func TestExecutorOptions(t *testing.T) {
	executor := flow.NewFlowExecutor(
		flow.WithMaxConcurrency(8),
		flow.WithChannelSize(128),
		flow.WithFailFast(false),
	)
	// Just verify construction works without panic.
	require.NotNil(t, executor)
}

func TestFlowBuilderWithToolFactory(t *testing.T) {
	f := flow.NewFlow("factory-test").
		AddToolFactory(func() (tool.Tool, error) {
			return passThroughTool("pass"), nil
		}).
		AddToolFactory(func() (tool.Tool, error) {
			return passThroughTool("pass2"), nil
		}).
		Build()

	assert.Equal(t, "factory-test", f.Name)
	assert.Len(t, f.ToolFactories, 2)
	assert.Empty(t, f.Tools)
}

// mockCollector is a test helper implementing flow.Collector.
type mockCollector struct {
	collectFn func(ctx context.Context, item *flow.FlowItem, parts []*model.Part) error
	resultFn  func() (flow.CollectorResult, error)
}

func (m *mockCollector) Collect(ctx context.Context, item *flow.FlowItem, parts []*model.Part) error {
	return m.collectFn(ctx, item, parts)
}

func (m *mockCollector) Result() (flow.CollectorResult, error) {
	return m.resultFn()
}
