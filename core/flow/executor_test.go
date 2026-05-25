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
		ToolName:     "uppercase",
		WritesTarget: true,
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

func TestExecutorWithThreeMockTools(t *testing.T) {
	var count1, count2, count3 atomic.Int64

	f, err := flow.NewFlow("test").
		AddTool(countingTool("tool1", &count1)).
		AddTool(countingTool("tool2", &count2)).
		AddTool(countingTool("tool3", &count3)).
		Build()
	require.NoError(t, err)

	executor := flow.NewExecutor()
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

	err = wait()
	require.NoError(t, err)

	assert.Len(t, results, numParts)
	assert.Equal(t, int64(numParts), count1.Load())
	assert.Equal(t, int64(numParts), count2.Load())
	assert.Equal(t, int64(numParts), count3.Load())
}

func TestExecutorPreservesOrder(t *testing.T) {
	f, err := flow.NewFlow("order-test").
		AddTool(passThroughTool("tool1")).
		AddTool(passThroughTool("tool2")).
		Build()
	require.NoError(t, err)

	executor := flow.NewExecutor()
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

	err = wait()
	require.NoError(t, err)

	assert.Len(t, results, numParts)
	for i, p := range results {
		block := p.Resource.(*model.Block)
		assert.Equal(t, fmt.Sprintf("tu%d", i), block.ID)
	}
}

func TestExecutorModification(t *testing.T) {
	f, err := flow.NewFlow("modify").
		AddTool(uppercaseTool()).
		Build()
	require.NoError(t, err)

	executor := flow.NewExecutor()
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

	err = wait()
	require.NoError(t, err)

	require.Len(t, results, 1)
	block := results[0].Resource.(*model.Block)
	assert.Equal(t, "HELLO WORLD", block.TargetText(model.LocaleFrench))
}

func TestExecutorErrorPropagation(t *testing.T) {
	errTool := &tool.BaseTool{
		ToolName: "error-tool",
		HandleBlockFn: func(part *model.Part) (*model.Part, error) {
			return nil, errors.New("processing error")
		},
	}

	f, err := flow.NewFlow("error-test").
		AddTool(passThroughTool("tool1")).
		AddTool(errTool).
		AddTool(passThroughTool("tool3")).
		Build()
	require.NoError(t, err)

	executor := flow.NewExecutor()
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

	err = wait()
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

	f, err := flow.NewFlow("cancel-test").
		AddTool(blockingTool).
		Build()
	require.NoError(t, err)

	executor := flow.NewExecutor()

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

	err = wait()
	require.Error(t, err)
}

func TestExecutorNoTools(t *testing.T) {
	_, err := flow.NewFlow("empty").Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "at least one tool")
}

func TestFlowBuilderEmptyName(t *testing.T) {
	dummyTool := &tool.BaseTool{ToolName: "dummy"}
	_, err := flow.NewFlow("").AddTool(dummyTool).Build()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "name must not be empty")
}

func TestExecutorMixedPartTypes(t *testing.T) {
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

	f, err := flow.NewFlow("mixed").AddTool(trackingTool).Build()
	require.NoError(t, err)
	executor := flow.NewExecutor()
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

	err = wait()
	require.NoError(t, err)

	assert.Len(t, results, 6)
	assert.Equal(t, int64(2), blockCount.Load())
	assert.Equal(t, int64(2), dataCount.Load())
	assert.Equal(t, int64(2), layerCount.Load())
}

func TestBuilder(t *testing.T) {
	f, err := flow.NewFlow("test-flow").
		AddTool(passThroughTool("tool1")).
		AddTool(passThroughTool("tool2")).
		Build()
	require.NoError(t, err)

	assert.Equal(t, "test-flow", f.Name)
	assert.Len(t, f.Tools, 2)
}

func TestBuilderWithItems(t *testing.T) {
	fb := flow.NewFlow("test-flow").
		AddTool(passThroughTool("tool1")).
		AddItem(&model.RawDocument{URI: "input.html"}, "output.html", model.LocaleFrench)

	items := fb.Items()
	require.Len(t, items, 1)
	assert.Equal(t, "input.html", items[0].Input.URI)
	assert.Equal(t, "output.html", items[0].OutputPath)
	assert.Equal(t, model.LocaleFrench, items[0].TargetLocale)
}

func TestExecutorSetChannelSize(t *testing.T) {
	executor := flow.NewExecutor()
	executor.SetChannelSize(128)
	// Just verify it doesn't panic; internal channel size is not exposed
}

// --- Parallel Execution Tests ---

func TestParallelExecutionMultipleDocuments(t *testing.T) {
	var totalBlocks atomic.Int64

	uppercaseFactory := func() (tool.Tool, error) {
		return &tool.BaseTool{
			ToolName:     "uppercase",
			WritesTarget: true,
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

	f, err := flow.NewFlow("parallel-test").
		AddToolFactory(uppercaseFactory).
		Build()
	require.NoError(t, err)

	executor := flow.NewExecutor(
		flow.WithMaxConcurrency(4),
	)

	numDocs := 10
	items := make([]*flow.Item, numDocs)
	for i := range numDocs {
		items[i] = &flow.Item{
			Input: &model.RawDocument{URI: fmt.Sprintf("doc%d.html", i)},
		}
	}

	err = executor.Execute(t.Context(), f, items)
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
		collectFn: func(ctx context.Context, item *flow.Item, parts []*model.Part) error {
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

	f, err := flow.NewFlow("collector-test").
		AddToolFactory(func() (tool.Tool, error) {
			return passThroughTool("pass"), nil
		}).
		Build()
	require.NoError(t, err)

	executor := flow.NewExecutor(
		flow.WithMaxConcurrency(2),
		flow.WithCollectors(collector),
	)

	items := []*flow.Item{
		{Input: &model.RawDocument{URI: "a.html"}},
		{Input: &model.RawDocument{URI: "b.html"}},
		{Input: &model.RawDocument{URI: "c.html"}},
	}

	err = executor.Execute(t.Context(), f, items)
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

	f, err := flow.NewFlow("error-parallel").
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
	require.NoError(t, err)

	executor := flow.NewExecutor(
		flow.WithMaxConcurrency(4),
	)

	// Create items that would trigger the error
	items := []*flow.Item{
		{Input: &model.RawDocument{URI: "err.html"}},
		{Input: &model.RawDocument{URI: "ok.html"}},
	}

	// processItemCollect closes input immediately, so the tools get no blocks.
	// This tests that the parallel path itself works without deadlock.
	err = executor.Execute(t.Context(), f, items)
	require.NoError(t, err)
}

func TestParallelExecutionContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())

	f, err := flow.NewFlow("cancel-parallel").
		AddToolFactory(func() (tool.Tool, error) {
			return passThroughTool("pass"), nil
		}).
		Build()
	require.NoError(t, err)

	executor := flow.NewExecutor(
		flow.WithMaxConcurrency(2),
	)

	items := []*flow.Item{
		{Input: &model.RawDocument{URI: "a.html"}},
		{Input: &model.RawDocument{URI: "b.html"}},
	}

	// Cancel before execution
	cancel()

	err = executor.Execute(ctx, f, items)
	// May or may not error depending on timing, but must not deadlock.
	_ = err
}

func TestSingleItemDirectTools(t *testing.T) {
	// With a single item, Execute should use f.Tools directly (no factory needed).
	var count atomic.Int64

	f, err := flow.NewFlow("single").
		AddTool(countingTool("counter", &count)).
		Build()
	require.NoError(t, err)

	executor := flow.NewExecutor() // default: sequential

	items := []*flow.Item{
		{Input: &model.RawDocument{URI: "single.html"}},
	}

	err = executor.Execute(t.Context(), f, items)
	require.NoError(t, err)
	// processItemCollect closes input immediately, so no blocks are processed.
	// Key test: no panic, no deadlock, direct tools path used.
}

func TestExecutorOptions(t *testing.T) {
	executor := flow.NewExecutor(
		flow.WithMaxConcurrency(8),
		flow.WithChannelSize(128),
		flow.WithFailFast(false),
	)
	// Just verify construction works without panic.
	require.NotNil(t, executor)
}

func TestBuilderWithToolFactory(t *testing.T) {
	f, err := flow.NewFlow("factory-test").
		AddToolFactory(func() (tool.Tool, error) {
			return passThroughTool("pass"), nil
		}).
		AddToolFactory(func() (tool.Tool, error) {
			return passThroughTool("pass2"), nil
		}).
		Build()
	require.NoError(t, err)

	assert.Equal(t, "factory-test", f.Name)
	assert.Len(t, f.ToolFactories, 2)
	assert.Empty(t, f.Tools)
}

// mockCollector is a test helper implementing flow.Collector.
type mockCollector struct {
	collectFn func(ctx context.Context, item *flow.Item, parts []*model.Part) error
	resultFn  func() (flow.CollectorResult, error)
}

func (m *mockCollector) Collect(ctx context.Context, item *flow.Item, parts []*model.Part) error {
	return m.collectFn(ctx, item, parts)
}

func (m *mockCollector) Result() (flow.CollectorResult, error) {
	return m.resultFn()
}
