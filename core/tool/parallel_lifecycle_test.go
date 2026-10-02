package tool_test

import (
	"context"
	"errors"
	"strconv"
	"sync/atomic"
	"testing"
	"testing/synctest"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParallelBlockTool_BoundedLookahead(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const concurrency = 4
		const count = 1000
		release := make(chan struct{})
		inner := &tool.BaseTool{
			ToolName: "delayed-first-block",
			Annotate: func(v tool.BlockView) error {
				if v.ID() == "0" {
					<-release
				}
				return nil
			},
		}
		in := make(chan *model.Part)
		out := make(chan *model.Part, count)
		var consumed atomic.Int32
		go func() {
			defer close(in)
			for i := range count {
				in <- makeBlock(strconv.Itoa(i), "content")
				consumed.Add(1)
			}
		}()
		done := make(chan error, 1)
		go func() {
			done <- tool.NewParallelBlockTool(inner, concurrency).Process(t.Context(), in, out)
			close(out)
		}()

		synctest.Wait()
		assert.Equal(t, int32(concurrency), consumed.Load(), "a blocked first result must backpressure input")
		close(release)
		require.NoError(t, <-done)
		parts := collectParts(out)
		require.Len(t, parts, count)
		for i, p := range parts {
			assert.Equal(t, strconv.Itoa(i), p.Resource.ResourceID())
		}
	})
}

func TestParallelBlockTool_NonBlockHandlersAreBarriers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		dataHandled := make(chan struct{})
		inner := &tool.BaseTool{
			ToolName: "structural-state",
			Annotate: func(v tool.BlockView) error {
				<-release
				return nil
			},
			HandleDataFn: func(p *model.Part) (*model.Part, error) {
				close(dataHandled)
				return p, nil
			},
		}
		in := make(chan *model.Part, 2)
		out := make(chan *model.Part, 2)
		in <- makeBlock("b1", "content")
		in <- makeData("d1")
		close(in)
		done := make(chan error, 1)
		go func() {
			done <- tool.NewParallelBlockTool(inner, 4).Process(t.Context(), in, out)
		}()

		synctest.Wait()
		select {
		case <-dataHandled:
			t.Error("non-block handler ran before the preceding block completed")
		default:
		}
		close(release)
		require.NoError(t, <-done)
		<-dataHandled
	})
}

func TestParallelBlockTool_JoinsWorkersBeforeReturn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := make(chan struct{})
		releaseCleanup := make(chan struct{})
		workerDone := make(chan struct{})
		failure := errors.New("failed block")
		inner := &tool.BaseTool{
			ToolName: "worker-lifetime",
			Annotate: func(v tool.BlockView) error {
				if v.ID() == "first" {
					close(started)
					<-v.Context().Done()
					<-releaseCleanup
					close(workerDone)
					return v.Context().Err()
				}
				<-started
				return failure
			},
		}
		in := make(chan *model.Part, 2)
		out := make(chan *model.Part, 2)
		in <- makeBlock("first", "content")
		in <- makeBlock("second", "content")
		close(in)
		done := make(chan error, 1)
		go func() {
			done <- tool.NewParallelBlockTool(inner, 2).Process(t.Context(), in, out)
		}()

		synctest.Wait()
		select {
		case err := <-done:
			t.Errorf("Process returned while a worker was still running: %v", err)
			done <- err
		default:
		}
		close(releaseCleanup)
		require.ErrorIs(t, <-done, failure)
		<-workerDone
	})
}

func TestBlockTool_DroppedPartsAreOmitted(t *testing.T) {
	for _, concurrency := range []int{1, 4} {
		t.Run(strconv.Itoa(concurrency), func(t *testing.T) {
			inner := &tool.BaseTool{
				ToolName: "drop",
				Annotate: func(v tool.BlockView) error {
					if v.ID() == "drop" {
						v.Drop()
					}
					return nil
				},
				HandleDataFn: func(*model.Part) (*model.Part, error) { return nil, nil },
			}
			parts := []*model.Part{makeBlock("drop", "content"), makeData("drop-data"), makeBlock("keep", "content")}
			got, err := tool.RunOnParts(t.Context(), tool.NewParallelBlockTool(inner, concurrency), parts)
			require.NoError(t, err)
			require.Len(t, got, 1)
			assert.Equal(t, "keep", got[0].Resource.ResourceID())
		})
	}
}

func TestParallelBlockTool_CancelIdleInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		inner := &tool.BaseTool{ToolName: "idle", Annotate: func(tool.BlockView) error { return nil }}
		done := make(chan error, 1)
		go func() {
			done <- tool.NewParallelBlockTool(inner, 4).Process(ctx, make(chan *model.Part), make(chan *model.Part))
		}()
		synctest.Wait()
		cancel()
		require.ErrorIs(t, <-done, context.Canceled)
	})
}

func TestParallelBlockTool_BoundedWhenOutputStops(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		const concurrency = 4
		ctx, cancel := context.WithCancel(t.Context())
		inner := &tool.BaseTool{ToolName: "backpressure", Annotate: func(tool.BlockView) error { return nil }}
		in := make(chan *model.Part)
		out := make(chan *model.Part)
		var consumed atomic.Int32
		go func() {
			defer close(in)
			for i := range 1000 {
				select {
				case in <- makeBlock(strconv.Itoa(i), "content"):
					consumed.Add(1)
				case <-ctx.Done():
					return
				}
			}
		}()
		done := make(chan error, 1)
		go func() {
			done <- tool.NewParallelBlockTool(inner, concurrency).Process(ctx, in, out)
		}()
		synctest.Wait()
		assert.Equal(t, int32(concurrency), consumed.Load())
		cancel()
		require.ErrorIs(t, <-done, context.Canceled)
	})
}

func TestParallelBlockTool_DroppedPartsReleaseWindow(t *testing.T) {
	inner := &tool.BaseTool{
		ToolName: "drop-all",
		Annotate: func(v tool.BlockView) error {
			v.Drop()
			return nil
		},
	}
	parts := make([]*model.Part, 100)
	for i := range parts {
		parts[i] = makeBlock(strconv.Itoa(i), "content")
	}
	got, err := tool.RunOnParts(t.Context(), tool.NewParallelBlockTool(inner, 4), parts)
	require.NoError(t, err)
	assert.Empty(t, got)
}
