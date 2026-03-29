package backend

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// RunState represents the current state of a flow execution.
type RunState string

const (
	RunStateIdle     RunState = "idle"
	RunStateRunning  RunState = "running"
	RunStateComplete RunState = "complete"
	RunStateError    RunState = "error"
	RunStateCanceled RunState = "canceled"
)

// RunEvent is emitted to the frontend during flow execution.
type RunEvent struct {
	Type    string `json:"type"` // "state", "progress", "trace", "error", "complete"
	FlowID  string `json:"flow_id"`
	Message string `json:"message,omitempty"`

	// Progress fields
	FileIndex int    `json:"file_index,omitempty"`
	FileCount int    `json:"file_count,omitempty"`
	FilePath  string `json:"file_path,omitempty"`

	// Trace event (when type == "trace")
	TraceEvent *flow.TraceEvent `json:"trace_event,omitempty"`

	// Stats (when type == "complete")
	DurationMs     int64 `json:"duration_ms,omitempty"`
	FilesProcessed int   `json:"files_processed,omitempty"`
}

// runner manages flow execution state with proper synchronization.
// All fields are guarded by mu.
type runner struct {
	mu      sync.Mutex
	state   RunState
	cancel  context.CancelFunc
	running bool
}

func newRunner() *runner {
	return &runner{state: RunStateIdle}
}

// RunFlow executes a flow by name from the current project.
// Events are streamed to the frontend via Wails events.
func (a *App) RunFlow(flowName string, inputPaths []string, targetLang string) error {
	a.mu.RLock()
	proj := a.project
	a.mu.RUnlock()

	if proj == nil {
		return fmt.Errorf("no project open")
	}

	spec := proj.GetFlow(flowName)
	if spec == nil {
		return fmt.Errorf("flow %q not found", flowName)
	}

	if len(inputPaths) == 0 {
		return fmt.Errorf("no input files specified")
	}

	if a.runState == nil {
		a.runState = newRunner()
	}

	// Atomically check and set running state under a single lock.
	a.runState.mu.Lock()
	if a.runState.running {
		a.runState.mu.Unlock()
		return fmt.Errorf("a flow is already running")
	}

	// Build tools from steps (before marking as running so errors don't leave stale state).
	var tools []tool.Tool
	for _, step := range spec.Steps {
		t, err := a.toolReg.NewTool(step.Tool)
		if err != nil {
			a.runState.mu.Unlock()
			return fmt.Errorf("tool %q: %w", step.Tool, err)
		}
		tools = append(tools, t)
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.runState.state = RunStateRunning
	a.runState.cancel = cancel
	a.runState.running = true
	a.runState.mu.Unlock()

	go a.executeFlow(ctx, flowName, tools, inputPaths, targetLang)
	return nil
}

// CancelRun cancels the currently running flow.
func (a *App) CancelRun() {
	if a.runState == nil {
		return
	}
	a.runState.mu.Lock()
	if a.runState.cancel != nil {
		a.runState.cancel()
		a.runState.state = RunStateCanceled
	}
	a.runState.mu.Unlock()
}

// GetRunState returns the current run state.
func (a *App) GetRunState() string {
	if a.runState == nil {
		return string(RunStateIdle)
	}
	a.runState.mu.Lock()
	defer a.runState.mu.Unlock()
	return string(a.runState.state)
}

func (a *App) executeFlow(ctx context.Context, flowName string, tools []tool.Tool, inputPaths []string, targetLang string) {
	defer func() {
		a.runState.mu.Lock()
		a.runState.running = false
		a.runState.mu.Unlock()
	}()

	start := time.Now()
	recorder := flow.NewTraceRecorder()

	// Wrap tools with tracing.
	tracedTools := make([]tool.Tool, len(tools))
	for i, t := range tools {
		nodeID := fmt.Sprintf("tool-%d", i)
		tracedTools[i] = flow.NewTracingTool(t, nodeID, recorder)
	}

	a.emitEvent("flow:event", RunEvent{
		Type:    "state",
		FlowID:  flowName,
		Message: "running",
	})

	filesProcessed := 0

	for fileIdx, inputPath := range inputPaths {
		if ctx.Err() != nil {
			break
		}

		a.emitEvent("flow:event", RunEvent{
			Type:      "progress",
			FlowID:    flowName,
			FileIndex: fileIdx,
			FileCount: len(inputPaths),
			FilePath:  inputPath,
		})

		fb := flow.NewFlow(flowName)
		for _, t := range tracedTools {
			fb.AddTool(t)
		}
		f := fb.Build()

		executor := flow.NewFlowExecutor()
		item := &flow.FlowItem{
			Input: &model.RawDocument{
				URI:          inputPath,
				TargetLocale: model.LocaleID(targetLang),
			},
		}

		if err := executor.Execute(ctx, f, []*flow.FlowItem{item}); err != nil {
			a.emitEvent("flow:event", RunEvent{
				Type:    "error",
				FlowID:  flowName,
				Message: fmt.Sprintf("file %s: %v", inputPath, err),
			})
			a.runState.mu.Lock()
			a.runState.state = RunStateError
			a.runState.mu.Unlock()
			return
		}

		filesProcessed++
	}

	duration := time.Since(start)

	a.runState.mu.Lock()
	if a.runState.state == RunStateRunning {
		a.runState.state = RunStateComplete
	}
	a.runState.mu.Unlock()

	a.emitEvent("flow:event", RunEvent{
		Type:           "complete",
		FlowID:         flowName,
		DurationMs:     duration.Milliseconds(),
		FilesProcessed: filesProcessed,
		Message:        fmt.Sprintf("Completed %d files in %s", filesProcessed, duration.Round(time.Millisecond)),
	})
}
