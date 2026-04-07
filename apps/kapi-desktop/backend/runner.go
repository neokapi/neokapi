package backend

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
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
	mu        sync.Mutex
	state     RunState
	cancel    context.CancelFunc
	running   bool
	lastTrace *flow.FlowTrace // trace from the last completed run
}

func newRunner() *runner {
	return &runner{state: RunStateIdle}
}

// RunFlow executes a flow by name from the current project.
// Events are streamed to the frontend via Wails events.
func (a *App) RunFlow(tabID, flowName string, inputPaths []string, targetLang string) error {
	op := a.getOpenProject(tabID)
	if op == nil {
		return fmt.Errorf("tab %q not found", tabID)
	}

	spec := op.Project.GetFlow(flowName)
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

	// Build tools from steps, applying step config (before marking as running
	// so errors don't leave stale state).
	var tools []tool.Tool
	for _, step := range spec.Steps {
		config := step.Config
		if config == nil {
			config = make(map[string]any)
		}
		t, err := a.toolReg.NewToolWithConfig(step.Tool, config, targetLang)
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

	pctx := project.NewProjectContext(op.Project, op.Path)
	go a.executeFlow(ctx, flowName, tools, inputPaths, targetLang, pctx)
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

// GetLastTrace returns the trace data from the last completed flow execution.
func (a *App) GetLastTrace() *flow.FlowTrace {
	if a.runState == nil {
		return nil
	}
	a.runState.mu.Lock()
	defer a.runState.mu.Unlock()
	return a.runState.lastTrace
}

// PreviewResult contains trace data from a preview flow execution.
type PreviewResult struct {
	Nodes     []flow.TraceNode                 `json:"nodes"`
	Events    []flow.TraceEvent                `json:"events"`
	Parts     map[string]*flow.PartSnapshotSet `json:"parts"`
	NodeOrder []string                         `json:"node_order"`
}

// PreviewFlow runs a flow on a single sample text block and returns trace data.
// This enables the live preview panel in the flow editor.
func (a *App) PreviewFlow(tabID, flowName, sampleText, sourceLang, targetLang string) (*PreviewResult, error) {
	op := a.getOpenProject(tabID)
	if op == nil {
		return nil, fmt.Errorf("tab %q not found", tabID)
	}

	spec := op.Project.GetFlow(flowName)
	if spec == nil {
		return nil, fmt.Errorf("flow %q not found", flowName)
	}

	if sampleText == "" {
		return nil, fmt.Errorf("sample text is required")
	}

	// Build tools from steps with config.
	var tools []tool.Tool
	for _, step := range spec.Steps {
		config := step.Config
		if config == nil {
			config = make(map[string]any)
		}
		t, err := a.toolReg.NewToolWithConfig(step.Tool, config, targetLang)
		if err != nil {
			return nil, fmt.Errorf("tool %q: %w", step.Tool, err)
		}
		tools = append(tools, t)
	}

	recorder := flow.NewTraceRecorder()

	// Build trace node metadata and wrap tools.
	traceNodes := make([]flow.TraceNode, len(tools))
	tracedTools := make([]tool.Tool, len(tools))
	nodeOrder := make([]string, len(tools))
	for i, t := range tools {
		nodeID := fmt.Sprintf("tool-%d", i)
		traceNodes[i] = flow.TraceNode{
			ID:   nodeID,
			Type: "tool",
			Name: t.Name(),
		}
		tracedTools[i] = flow.NewTracingTool(t, nodeID, recorder)
		nodeOrder[i] = nodeID
	}

	// Create a temporary file with sample text to use as input.
	tmpFile, err := os.CreateTemp("", "kapi-preview-*.txt")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(sampleText); err != nil {
		tmpFile.Close()
		return nil, fmt.Errorf("write sample text: %w", err)
	}
	tmpFile.Close()

	// Build flow and execute with the sample text file.
	fb := flow.NewFlow(flowName)
	for _, t := range tracedTools {
		fb.AddTool(t)
	}
	f, err := fb.Build()
	if err != nil {
		return nil, fmt.Errorf("build flow: %w", err)
	}

	executor := flow.NewExecutor()
	item := &flow.Item{
		Input: &model.RawDocument{
			URI:          tmpFile.Name(),
			SourceLocale: model.LocaleID(sourceLang),
			TargetLocale: model.LocaleID(targetLang),
		},
	}

	ctx := context.Background()
	if err := executor.Execute(ctx, f, []*flow.Item{item}); err != nil {
		return nil, fmt.Errorf("preview: %w", err)
	}

	return &PreviewResult{
		Nodes:     traceNodes,
		Events:    recorder.Events(),
		Parts:     recorder.Snapshots(),
		NodeOrder: nodeOrder,
	}, nil
}

func (a *App) executeFlow(ctx context.Context, flowName string, tools []tool.Tool, inputPaths []string, targetLang string, pctx *project.ProjectContext) {
	defer func() {
		a.runState.mu.Lock()
		a.runState.running = false
		a.runState.mu.Unlock()
	}()

	start := time.Now()
	recorder := flow.NewTraceRecorder()

	traceNodes := make([]flow.TraceNode, len(tools))
	tracedTools := make([]tool.Tool, len(tools))
	for i, t := range tools {
		nodeID := fmt.Sprintf("tool-%d", i)
		traceNodes[i] = flow.TraceNode{ID: nodeID, Type: "tool", Name: t.Name()}
		tracedTools[i] = flow.NewTracingTool(t, nodeID, recorder)
	}

	a.emitEvent("flow:event", RunEvent{
		Type: "state", FlowID: flowName, Message: "running",
	})

	// Stream trace events in the background.
	stopStreaming := make(chan struct{})
	var streamWg sync.WaitGroup
	streamWg.Add(1)
	go func() {
		defer streamWg.Done()
		lastSeen := 0
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-stopStreaming:
				events := recorder.Events()
				for i := lastSeen; i < len(events); i++ {
					a.emitEvent("flow:event", RunEvent{Type: "trace", FlowID: flowName, TraceEvent: &events[i]})
				}
				return
			case <-ticker.C:
				events := recorder.Events()
				for i := lastSeen; i < len(events); i++ {
					a.emitEvent("flow:event", RunEvent{Type: "trace", FlowID: flowName, TraceEvent: &events[i]})
				}
				lastSeen = len(events)
			}
		}
	}()

	emitErr := func(msg string) {
		a.emitEvent("flow:event", RunEvent{Type: "error", FlowID: flowName, Message: msg})
		a.runState.mu.Lock()
		a.runState.state = RunStateError
		a.runState.mu.Unlock()
		close(stopStreaming)
		streamWg.Wait()
	}

	filesProcessed := 0

	for fileIdx, inputPath := range inputPaths {
		if ctx.Err() != nil {
			break
		}

		a.emitEvent("flow:event", RunEvent{
			Type: "progress", FlowID: flowName,
			FileIndex: fileIdx, FileCount: len(inputPaths), FilePath: inputPath,
		})

		outputPath := a.resolveOutputPath(inputPath, targetLang)
		runner := flow.NewFileRunner(flow.FileRunnerConfig{
			FormatReg:    a.formatReg,
			SourceLocale: pctx.SourceLocale,
			Encoding:     pctx.Encoding,
			DetectFormat: func(path string) string {
				return pctx.DetectFormat(a.formatReg, path)
			},
		})
		if err := runner.RunFile(ctx, flowName, tracedTools, inputPath, outputPath, targetLang); err != nil {
			emitErr(fmt.Sprintf("file %s: %v", filepath.Base(inputPath), err))
			return
		}

		filesProcessed++
	}

	close(stopStreaming)
	streamWg.Wait()

	duration := time.Since(start)

	trace := &flow.FlowTrace{
		Name: flowName, Nodes: traceNodes,
		Events: recorder.Events(), Parts: recorder.Snapshots(),
		DurationUs: recorder.DurationUs(),
	}

	a.runState.mu.Lock()
	a.runState.lastTrace = trace
	if a.runState.state == RunStateRunning {
		a.runState.state = RunStateComplete
	}
	a.runState.mu.Unlock()

	a.emitEvent("flow:event", RunEvent{
		Type: "complete", FlowID: flowName,
		DurationMs: duration.Milliseconds(), FilesProcessed: filesProcessed,
		Message: fmt.Sprintf("Completed %d files in %s", filesProcessed, duration.Round(time.Millisecond)),
	})
}

// resolveOutputPath computes the output file path for a given input and target
// language. It replaces {lang} in the first matching content target pattern.
// Falls back to input_targetLang.ext if no pattern matches.
func (a *App) resolveOutputPath(inputPath, targetLang string) string {
	// Try to find the matching content pattern and use its target template.
	a.mu.RLock()
	defer a.mu.RUnlock()
	for _, op := range a.projects {
		basePath := filepath.Dir(op.Path)
		rel, err := filepath.Rel(basePath, inputPath)
		if err != nil {
			continue
		}
		for _, coll := range op.Project.Content {
			for _, item := range coll.EffectiveItems() {
				if item.Target == "" {
					continue
				}
				// Check if the input matches this pattern.
				matched, _ := filepath.Match(item.Path, rel)
				if !matched {
					continue
				}
				// Resolve {lang} in the target pattern.
				target := strings.ReplaceAll(item.Target, "{lang}", targetLang)
				// Replace the wildcard with the actual filename.
				if strings.Contains(target, "*") {
					target = strings.ReplaceAll(target, "*", filepath.Base(rel))
				}
				return filepath.Join(basePath, target)
			}
		}
	}
	// Fallback: input_targetLang.ext
	ext := filepath.Ext(inputPath)
	base := inputPath[:len(inputPath)-len(ext)]
	return fmt.Sprintf("%s_%s%s", base, targetLang, ext)
}
