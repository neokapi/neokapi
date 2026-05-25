package flow_test

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeBatchFile(uri string, blockCount int) flow.BatchFile {
	parts := make([]*model.Part, blockCount)
	for i := range blockCount {
		parts[i] = &model.Part{
			Type:     model.PartBlock,
			Resource: model.NewBlock(fmt.Sprintf("%s-b%d", uri, i), fmt.Sprintf("text %d", i)),
		}
	}
	return flow.BatchFile{
		URI:          uri,
		Parts:        parts,
		TargetLocale: "fr",
	}
}

func passthroughFactory() (tool.Tool, error) {
	return &tool.BaseTool{
		ToolName: "passthrough",
		Annotate: func(v tool.BlockView) error {
			v.SetProperty("processed", "true")
			return nil
		},
	}, nil
}

func TestBatchExecutor_BasicExecution(t *testing.T) {
	executor := flow.NewBatchExecutorWithOptions(
		flow.WithFileConcurrency(1),
	)

	files := []flow.BatchFile{
		makeBatchFile("file1.json", 3),
		makeBatchFile("file2.json", 2),
	}

	results, err := executor.Execute(
		t.Context(),
		[]flow.ToolFactory{passthroughFactory},
		files,
	)
	require.NoError(t, err)
	require.Len(t, results, 2)

	assert.Equal(t, "file1.json", results[0].URI)
	assert.Len(t, results[0].Parts, 3)

	assert.Equal(t, "file2.json", results[1].URI)
	assert.Len(t, results[1].Parts, 2)

	// Verify blocks were processed.
	for _, r := range results {
		for _, p := range r.Parts {
			block := p.Resource.(*model.Block)
			assert.Equal(t, "true", block.Properties["processed"])
		}
	}
}

func TestBatchExecutor_Concurrency(t *testing.T) {
	var active atomic.Int32
	var maxActive atomic.Int32

	factory := func() (tool.Tool, error) {
		return &tool.BaseTool{
			ToolName: "slow-tool",
			Annotate: func(v tool.BlockView) error {
				cur := active.Add(1)
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
		}, nil
	}

	executor := flow.NewBatchExecutorWithOptions(
		flow.WithFileConcurrency(4),
	)

	files := make([]flow.BatchFile, 8)
	for i := range files {
		files[i] = makeBatchFile(fmt.Sprintf("file%d.json", i), 1)
	}

	results, err := executor.Execute(
		t.Context(),
		[]flow.ToolFactory{factory},
		files,
	)
	require.NoError(t, err)
	assert.Len(t, results, 8)

	// Should have achieved some parallelism.
	assert.Greater(t, int(maxActive.Load()), 1, "expected parallel file processing")
	assert.LessOrEqual(t, int(maxActive.Load()), 4, "should not exceed file concurrency")
}

func TestBatchExecutor_ErrorPropagation(t *testing.T) {
	factory := func() (tool.Tool, error) {
		return &tool.BaseTool{
			ToolName: "error-tool",
			Annotate: func(v tool.BlockView) error {
				if v.ID() == "file2.json-b0" {
					return errors.New("intentional error")
				}
				return nil
			},
		}, nil
	}

	executor := flow.NewBatchExecutorWithOptions(
		flow.WithFileConcurrency(1),
		flow.WithBatchFailFast(true),
	)

	files := []flow.BatchFile{
		makeBatchFile("file1.json", 2),
		makeBatchFile("file2.json", 2),
		makeBatchFile("file3.json", 2),
	}

	_, err := executor.Execute(
		t.Context(),
		[]flow.ToolFactory{factory},
		files,
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "intentional error")
}

func TestBatchExecutor_SharedResources(t *testing.T) {
	var closed atomic.Bool
	resource := &testCloser{closeFn: func() error {
		closed.Store(true)
		return nil
	}}

	executor := flow.NewBatchExecutorWithOptions(
		flow.WithFileConcurrency(1),
		flow.WithSharedResources(resource),
	)

	files := []flow.BatchFile{makeBatchFile("file1.json", 1)}

	_, err := executor.Execute(
		t.Context(),
		[]flow.ToolFactory{passthroughFactory},
		files,
	)
	require.NoError(t, err)
	assert.True(t, closed.Load(), "shared resource should be closed")
}

func TestBatchExecutor_WithCollectors(t *testing.T) {
	collector := &countCollector{}

	executor := flow.NewBatchExecutorWithOptions(
		flow.WithFileConcurrency(2),
	)

	files := []flow.BatchFile{
		makeBatchFile("file1.json", 3),
		makeBatchFile("file2.json", 2),
	}

	_, err := executor.Execute(
		t.Context(),
		[]flow.ToolFactory{passthroughFactory},
		files,
		collector,
	)
	require.NoError(t, err)

	result, err := collector.Result()
	require.NoError(t, err)
	assert.Equal(t, 5, result.Data.(int)) // 3 + 2 = 5 parts total
}

func TestBatchExecutor_EmptyFiles(t *testing.T) {
	executor := flow.NewBatchExecutorWithOptions()

	results, err := executor.Execute(
		t.Context(),
		[]flow.ToolFactory{passthroughFactory},
		nil,
	)
	require.NoError(t, err)
	assert.Nil(t, results)
}

func TestBatchExecutor_NoTools(t *testing.T) {
	executor := flow.NewBatchExecutorWithOptions()

	files := []flow.BatchFile{makeBatchFile("file1.json", 3)}

	results, err := executor.Execute(
		t.Context(),
		nil,
		files,
	)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Len(t, results[0].Parts, 3)
}

// testCloser implements io.Closer for testing.
type testCloser struct {
	closeFn func() error
}

func (c *testCloser) Close() error { return c.closeFn() }

var _ io.Closer = (*testCloser)(nil)

// countCollector counts total parts across all files.
type countCollector struct {
	count atomic.Int32
}

func (c *countCollector) Collect(_ context.Context, _ *flow.Item, parts []*model.Part) error {
	c.count.Add(int32(len(parts)))
	return nil
}

func (c *countCollector) Result() (flow.CollectorResult, error) {
	return flow.CollectorResult{
		Name: "count",
		Data: int(c.count.Load()),
	}, nil
}
