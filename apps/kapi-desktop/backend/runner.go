package backend

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/host"
	"github.com/neokapi/neokapi/host/credentials"
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

	// Error carries the failure as structure (when type == "error"): a
	// headline, the remediation as actions, the affected file/locale, and the
	// raw chain for a details disclosure. Message stays populated with the raw
	// text so nothing regresses for a consumer that only reads it.
	Error *RunError `json:"error,omitempty"`

	// Progress fields
	FileIndex int    `json:"file_index,omitempty"`
	FileCount int    `json:"file_count,omitempty"`
	FilePath  string `json:"file_path,omitempty"`

	// Trace event (when type == "trace")
	TraceEvent *flow.TraceEvent `json:"trace_event,omitempty"`

	// Pipeline metrics snapshot (when type == "pipeline_metrics")
	Steps []flow.StepSnapshot `json:"steps,omitempty"`

	// Stats (when type == "complete")
	DurationMs     int64 `json:"duration_ms,omitempty"`
	FilesProcessed int   `json:"files_processed,omitempty"`

	// Convergence-run fields (BringUpToDate — the shared `kapi up` engine):
	// ConvergeEvent carries one typed progress event of the run (when type ==
	// "converge_event") — the per-pass/per-locale stream (pass_start,
	// locale_start, unit_progress, locale_done, pass_done, materialized, done)
	// the live run view renders locale rows from, the same protocol the CLI's
	// live renderer and the server's SSE stream speak. ConvergeResult carries
	// the final structured outcome (on the run's "complete").
	ConvergeEvent  *convergence.Event   `json:"converge_event,omitempty"`
	ConvergeResult *host.ConvergeOutput `json:"converge_result,omitempty"`
}

// runner manages flow execution state with proper synchronization.
// All fields are guarded by mu.
type runner struct {
	mu        sync.Mutex
	state     RunState
	cancel    context.CancelFunc
	running   bool
	lastTrace *flow.FlowTrace // trace from the last completed run
	events    []RunEvent      // accumulated events for reconnection
	// lastFailure is the classified failure of the most recent run, kept until
	// a run succeeds. Derived state (coverage, the up plan) is computed from
	// files and cannot express "the last attempt to produce these files
	// failed", so without this the UI has no way to tell a converged project
	// from one whose catch-up run died on its first locale.
	lastFailure *RunError
}

func newRunner() *runner {
	return &runner{state: RunStateIdle}
}

// recordRunFailure stores (or, with nil, clears) the classified failure of the
// most recent run. Safe on a nil runner so callers need no guard.
func (a *App) recordRunFailure(re *RunError) {
	if a.runState == nil {
		return
	}
	a.runState.mu.Lock()
	a.runState.lastFailure = re
	a.runState.mu.Unlock()
}

// GetLastRunError returns the classified failure of the most recent run, or nil
// when the last run succeeded (or none has been attempted). The home surfaces
// read it so a failed catch-up run can never be presented as convergence.
func (a *App) GetLastRunError() *RunError {
	if a.runState == nil {
		return nil
	}
	a.runState.mu.Lock()
	defer a.runState.mu.Unlock()
	return a.runState.lastFailure
}

// RunFlow executes a flow by name from the current project. Target locales
// are inferred from the flow's tool chain metadata (Framework AD-006) — the
// frontend passes project target languages as a fallback, and the shared
// orchestrator (host.App.RunFlowAllLocales → flow.ResolveFlowLocales)
// determines the actual locale passes based on tool cardinality, exactly as
// the CLI's project flow-run does.
func (a *App) RunFlow(tabID, flowName string, inputPaths []string, targetLangs []string) error {
	op := a.getOpenProject(tabID)
	if op == nil {
		return fmt.Errorf("tab %q not found", tabID)
	}

	spec := op.Project.Flow(flowName)
	if spec == nil {
		return fmt.Errorf("flow %q not found", flowName)
	}

	if len(inputPaths) == 0 {
		return errors.New("no input files specified")
	}

	if a.runState == nil {
		a.runState = newRunner()
	}

	a.runState.mu.Lock()
	if a.runState.running {
		a.runState.mu.Unlock()
		return errors.New("a flow is already running. Cancel it first")
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.runState.state = RunStateRunning
	a.runState.cancel = cancel
	a.runState.running = true
	a.runState.events = nil // clear events from previous run
	a.runState.mu.Unlock()

	go a.executeFlowRun(ctx, op, flowName, spec, inputPaths, targetLangs)
	return nil
}

// executeFlowRun drives the shared multi-locale flow orchestrator
// (host.App.RunFlowAllLocales) and adapts its event stream to Wails run
// events. Locale-pass selection, per-pass tool assembly (with the cleanup
// contract the former hand-rolled loop leaked), per-file execution, and
// event emission all live in the cli module now — shared with the CLI's
// project flow-run, so the two surfaces agree about which locales a flow
// runs for. The run uses a fresh host.App carrying the desktop's own
// registries (plugin-provided formats and segmenters stay available) and its
// AI-default + credential-resolving config preprocessor already wired on
// toolReg.
func (a *App) executeFlowRun(ctx context.Context, op *openProject, flowName string, spec *flow.StepsSpec, inputPaths, targetLangs []string) {
	defer func() {
		a.runState.mu.Lock()
		a.runState.running = false
		a.runState.mu.Unlock()
	}()

	// Attribute this flow's model calls to it in the session's AI activity log.
	ctx = withAIScope(ctx, AIActivityScope{Surface: "flow", Action: flowName})

	capp := a.borrowEngine(&host.App{
		FormatReg:   a.formatReg,
		ToolReg:     a.toolReg,
		SchemaReg:   a.schemaReg,
		Credentials: a.credentials,
		Config:      a.aiConfig,
		Quiet:       true,
	})
	_, err := capp.RunFlowAllLocales(ctx, host.FlowRunOptions{
		FlowName:      flowName,
		Spec:          spec,
		Project:       op.Project,
		ProjectPath:   op.Path,
		InputPaths:    inputPaths,
		TargetLocales: targetLangs,
	}, func(ev host.FlowRunEvent) {
		switch ev.Type {
		case host.FlowEventState:
			a.emitRunEvent(RunEvent{Type: "state", FlowID: flowName, Message: ev.Message})
		case host.FlowEventProgress:
			a.emitRunEvent(RunEvent{
				Type: "progress", FlowID: flowName, Message: ev.Message,
				FileIndex: ev.FileIndex, FileCount: ev.FileCount, FilePath: ev.FilePath,
			})
		case host.FlowEventPipelineMetrics:
			a.emitRunEvent(RunEvent{Type: "pipeline_metrics", FlowID: flowName, Steps: ev.Steps})
		case host.FlowEventFileDone:
			// Notify the Content view that a new output file landed so it can
			// refresh the outputs shown beneath each source (issue #5).
			a.emitEvent("outputs-changed", map[string]any{"path": ev.OutputPath})
		case host.FlowEventComplete:
			// State transition before the closing event, preserving the order
			// the frontend has always observed.
			a.runState.mu.Lock()
			if a.runState.state == RunStateRunning {
				a.runState.state = RunStateComplete
			}
			a.runState.mu.Unlock()
			a.emitRunEvent(RunEvent{
				Type: "complete", FlowID: flowName,
				DurationMs: ev.DurationMs, FilesProcessed: ev.FilesProcessed,
				Message: ev.Message,
			})
		}
	})
	if err != nil {
		a.emitRunEvent(RunEvent{
			Type:    "error",
			FlowID:  flowName,
			Message: runErrorMessage(err),
			Error:   classifyRunError(err),
		})
		a.runState.mu.Lock()
		a.runState.state = RunStateError
		a.runState.lastFailure = classifyRunError(err)
		a.runState.mu.Unlock()
		// The failed run may still have written some outputs; re-derive so the
		// home surfaces reflect the real on-disk state rather than the pre-run
		// snapshot (which a partial failure would otherwise present as done).
		a.emitEvent("outputs-changed", map[string]any{})
		return
	}
	a.recordRunFailure(nil)
}

// runErrorMessage renders an orchestrator failure for the run feed: tool
// assembly failures go through toolBuildErrorMessage (GUI-appropriate
// credential guidance); file failures already carry the historical
// "<file> [<lang>]: <cause>" text on the typed error.
func runErrorMessage(err error) string {
	var tb *host.FlowToolBuildError
	if errors.As(err, &tb) {
		return toolBuildErrorMessage(tb.Tool, tb.Locale, tb.Err)
	}
	return err.Error()
}

// toolBuildErrorMessage renders a tool-construction failure for the run feed.
// Ambiguous-credential failures get GUI-appropriate guidance — the shared
// resolver's Error() carries the CLI's "--credential" hint, which is meaningless
// in the desktop, so we catch the typed error and point at the in-app fixes.
func toolBuildErrorMessage(toolName, lang string, err error) string {
	var amb *credentials.AmbiguousCredentialError
	if errors.As(err, &amb) {
		return fmt.Sprintf(
			"%s: multiple AI credentials are configured (%s). Set a default in Settings → AI Models, or choose one on this flow step.",
			toolName, strings.Join(amb.Candidates, ", "),
		)
	}
	return fmt.Sprintf("tool %q for %s: %v", toolName, lang, err)
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

// GetRunEvents returns accumulated events from the current or last run.
// Used by the frontend to reconnect to a running flow after navigation.
func (a *App) GetRunEvents() []RunEvent {
	if a.runState == nil {
		return nil
	}
	a.runState.mu.Lock()
	defer a.runState.mu.Unlock()
	out := make([]RunEvent, len(a.runState.events))
	copy(out, a.runState.events)
	return out
}

// emitRunEvent emits a flow event to the frontend and stores it for reconnection.
// For pipeline_metrics events, the last stored snapshot is replaced instead of
// appending to prevent the reconnection buffer from growing at 5 events/second.
func (a *App) emitRunEvent(event RunEvent) {
	a.runState.mu.Lock()
	if event.Type == "pipeline_metrics" && len(a.runState.events) > 0 {
		last := a.runState.events[len(a.runState.events)-1]
		if last.Type == "pipeline_metrics" {
			a.runState.events[len(a.runState.events)-1] = event
			a.runState.mu.Unlock()
			a.emitEvent("flow:event", event)
			return
		}
	}
	a.runState.events = append(a.runState.events, event)
	a.runState.mu.Unlock()
	a.emitEvent("flow:event", event)
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

	spec := op.Project.Flow(flowName)
	if spec == nil {
		return nil, fmt.Errorf("flow %q not found", flowName)
	}

	if sampleText == "" {
		return nil, errors.New("sample text is required")
	}

	// Build tools from steps with config.
	var tools []tool.Tool
	for _, step := range spec.Steps {
		config := step.Config
		if config == nil {
			config = make(map[string]any)
		}
		t, err := a.toolReg.NewToolWithConfig(registry.ToolID(step.Tool), config, targetLang)
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
			Type: flow.NodeTool,
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

	pctx := project.NewProjectContext(op.Project, op.Path)

	executor := flow.NewExecutor()
	item := &flow.Item{
		Input: &model.RawDocument{
			URI:          tmpFile.Name(),
			SourceLocale: model.LocaleID(sourceLang),
			TargetLocale: model.LocaleID(targetLang),
			Encoding:     pctx.Encoding,
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
