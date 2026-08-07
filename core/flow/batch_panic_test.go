package flow_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBatchExecutor_RecoversToolPanic verifies that a panicking tool fails the
// batch instead of crashing the process. Each tool runs on its own errgroup
// goroutine, where no recover up the caller's stack can catch it.
func TestBatchExecutor_RecoversToolPanic(t *testing.T) {
	tests := []struct {
		name    string
		factory flow.ToolFactory
	}{
		{
			name:    "panic in the block handler",
			factory: func() (tool.Tool, error) { return panickingTool("panicky"), nil },
		},
		{
			name: "panic in Process",
			factory: func() (tool.Tool, error) {
				p := &processPanicTool{}
				p.ToolName = "panicky"
				return p, nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			executor := flow.NewBatchExecutorWithOptions(flow.WithFileConcurrency(1))

			results, err := executor.Execute(t.Context(),
				[]flow.ToolFactory{tt.factory},
				[]flow.BatchFile{makeBatchFile("doc.json", 2)})

			require.Error(t, err)
			assert.Nil(t, results)
			assert.Contains(t, err.Error(), "tool panicky")
			assert.Contains(t, err.Error(), "panicked")
		})
	}
}
