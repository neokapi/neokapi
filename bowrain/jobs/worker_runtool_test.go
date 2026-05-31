package jobs

import (
	"context"
	"testing"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/require"
)

// fanOutTool emits `factor` output parts for every input part. With factor > 1
// it produces more parts than it consumes, which would deadlock any
// implementation of runToolOnParts that drains the output channel only after
// Process returns (because the output buffer is sized to len(input)).
type fanOutTool struct {
	tool.BaseTool
	factor int
}

func (f *fanOutTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
	for p := range in {
		for i := 0; i < f.factor; i++ {
			select {
			case out <- p:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
	return nil
}

func TestRunToolOnParts_FanOutDoesNotDeadlock(t *testing.T) {
	const (
		inputParts = 4
		factor     = 8 // each input emits 8 outputs => 32 outputs > 4-slot buffer
	)

	parts := make([]*model.Part, 0, inputParts)
	for i := 0; i < inputParts; i++ {
		parts = append(parts, &model.Part{Type: model.PartBlock})
	}

	ft := &fanOutTool{factor: factor}
	ft.ToolName = "fan-out"

	done := make(chan struct{})
	var (
		result []*model.Part
		err    error
	)
	go func() {
		result, err = runToolOnParts(context.Background(), ft, parts)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("runToolOnParts deadlocked on a fan-out tool")
	}

	require.NoError(t, err)
	require.Len(t, result, inputParts*factor)
}
