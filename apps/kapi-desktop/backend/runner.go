package backend

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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
	Type    string `json:"type"` // "state", "progress", "trace", "pipeline_metrics", "error", "complete", "converge_event"
	FlowID  string `json:"flow_id"`
	Message string `json:"message,omitempty"`
	// Locale is the locale pass the event belongs to (state, progress and
	// trace events of a multi-locale run); empty on a source-only pass.
	Locale string `json:"locale,omitempty"`

	// Error carries the failure as structure (when type == "error"): a
	// headline, the remediation as actions, the affected file/locale, and the
	// raw chain for a details disclosure. Message stays populated with the raw
	// text so nothing regresses for a consumer that only reads it.
	Error *RunError `json:"error,omitempty"`

	// Progress fields. A "trace" event names in FilePath the input file whose
	// recording the run has just retained; GetLastTrace(FilePath, Locale)
	// returns it and ListRunTraces lists every file that has one.
	FileIndex int    `json:"file_index,omitempty"`
	FileCount int    `json:"file_count,omitempty"`
	FilePath  string `json:"file_path,omitempty"`

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

// RunTraces is the last run's replayable record: the flow that ran, the steps
// it ran with (so a reader can tell a trace from an edited flow apart), and
// the files whose traces the desktop kept, in completion order.
type RunTraces struct {
	FlowName string          `json:"flow_name"`
	Steps    []flow.FlowStep `json:"steps"`
	Files    []RunTraceFile  `json:"files"`
	// MaxParts is the recording budget each trace was kept under: a truncated
	// trace holds the first MaxParts parts of its file.
	MaxParts int `json:"max_parts"`
}

// RunTraceFile identifies one retained trace: the input file and locale pass
// it recorded, and whether the recording budget cut it short.
type RunTraceFile struct {
	FilePath   string `json:"file_path"`
	Locale     string `json:"locale,omitempty"`
	OutputPath string `json:"output_path,omitempty"`
	Truncated  bool   `json:"truncated,omitempty"`
}

// retainedTrace is one kept recording and its size as JSON, the size that
// crosses to the frontend.
type retainedTrace struct {
	file  RunTraceFile
	trace *flow.FlowTrace
	size  int
}

const (
	// maxRetainedTraces is how many files' traces the desktop keeps from the
	// last run; the oldest completion is evicted first.
	maxRetainedTraces = 8
	// maxRetainedTraceBytes bounds the retained set as JSON. A single trace
	// over it is not kept at all, so the cap holds whatever the file was.
	maxRetainedTraceBytes = 32 << 20
)

// desktopTraceLimits bounds each file's recording: the first parts of the
// file are traced in full and the rest of the run is not recorded, so a run
// over a large document costs a fixed amount of memory and a trace of a fixed
// size to send. The event cap only bites on a flow of more than twenty steps.
var desktopTraceLimits = flow.TraceLimits{MaxParts: 500, MaxEvents: 20_000}

// runner manages flow execution state with proper synchronization.
// All fields are guarded by mu.
type runner struct {
	mu      sync.Mutex
	state   RunState
	cancel  context.CancelFunc
	running bool
	events  []RunEvent // accumulated events for reconnection
	// The last run's replayable record: the flow and steps it ran, and the
	// traces retained from it (newest last, bounded by maxRetainedTraces and
	// maxRetainedTraceBytes). A new run replaces all of it.
	traceFlow  string
	traceSteps []flow.FlowStep
	traces     []retainedTrace
	traceBytes int
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
	a.runState.resetTracesLocked(flowName, spec)
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
		// Every desktop run is recorded so the flow's Run view can replay it.
		Trace:       true,
		TraceLimits: desktopTraceLimits,
	}, func(ev host.FlowRunEvent) {
		switch ev.Type {
		case host.FlowEventState:
			a.emitRunEvent(RunEvent{Type: "state", FlowID: flowName, Message: ev.Message, Locale: ev.Locale})
		case host.FlowEventProgress:
			a.emitRunEvent(RunEvent{
				Type: "progress", FlowID: flowName, Message: ev.Message, Locale: ev.Locale,
				FileIndex: ev.FileIndex, FileCount: ev.FileCount, FilePath: ev.FilePath,
			})
		case host.FlowEventPipelineMetrics:
			a.emitRunEvent(RunEvent{Type: "pipeline_metrics", FlowID: flowName, Steps: ev.Steps})
		case host.FlowEventFileDone:
			// Notify the Content view that a new output file landed so it can
			// refresh the outputs shown beneath each source (issue #5).
			a.emitEvent("outputs-changed", map[string]any{"path": ev.OutputPath})
		case host.FlowEventFileTrace:
			if a.runState.retainTrace(ev) {
				a.emitRunEvent(RunEvent{
					Type: "trace", FlowID: flowName, Locale: ev.Locale, FilePath: ev.FilePath,
					Message: traceRetainedMessage(ev),
				})
			}
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

// traceRetainedMessage is the run feed's line for a kept recording.
func traceRetainedMessage(ev host.FlowRunEvent) string {
	name := filepath.Base(ev.FilePath)
	if ev.Locale == "" {
		return "Recorded a trace of " + name
	}
	return "Recorded a trace of " + name + " for " + ev.Locale
}

// resetTracesLocked starts the run record over for a run of flowName with
// spec. The steps are copied so a later edit of the flow leaves the record
// describing what actually ran. Caller holds mu.
func (r *runner) resetTracesLocked(flowName string, spec *flow.StepsSpec) {
	r.traceFlow = flowName
	r.traceSteps = cloneSteps(spec.Steps)
	r.traces = nil
	r.traceBytes = 0
}

// cloneSteps deep-copies a step list (configs are nested maps).
func cloneSteps(steps []flow.FlowStep) []flow.FlowStep {
	if steps == nil {
		return nil
	}
	data, err := json.Marshal(steps)
	if err != nil {
		return nil
	}
	var out []flow.FlowStep
	if err := json.Unmarshal(data, &out); err != nil {
		return nil
	}
	return out
}

// retainTrace keeps the trace on a FlowEventFileTrace as the newest retained
// one, evicting the oldest until both caps hold. It reports false when the
// trace was not kept: it alone exceeds maxRetainedTraceBytes, or it cannot be
// encoded.
func (r *runner) retainTrace(ev host.FlowRunEvent) bool {
	if ev.Trace == nil {
		return false
	}
	data, err := json.Marshal(ev.Trace)
	if err != nil || len(data) > maxRetainedTraceBytes {
		return false
	}
	entry := retainedTrace{
		file: RunTraceFile{
			FilePath:   ev.FilePath,
			Locale:     ev.Locale,
			OutputPath: ev.OutputPath,
			Truncated:  ev.Trace.Truncated,
		},
		trace: ev.Trace,
		size:  len(data),
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	for len(r.traces) > 0 && (len(r.traces) >= maxRetainedTraces || r.traceBytes+entry.size > maxRetainedTraceBytes) {
		r.traceBytes -= r.traces[0].size
		// Shift down and clear the vacated slot, so the evicted trace is not
		// kept alive by the slice's backing array.
		last := len(r.traces) - 1
		copy(r.traces, r.traces[1:])
		r.traces[last] = retainedTrace{}
		r.traces = r.traces[:last]
	}
	r.traces = append(r.traces, entry)
	r.traceBytes += entry.size
	return true
}

// ListRunTraces describes the last run's retained traces: the flow and steps
// that ran and the files with a trace, in completion order. Nil before the
// first run; a run with no completed file lists no files.
func (a *App) ListRunTraces() *RunTraces {
	if a.runState == nil {
		return nil
	}
	a.runState.mu.Lock()
	defer a.runState.mu.Unlock()
	if a.runState.traceFlow == "" {
		return nil
	}
	out := &RunTraces{
		FlowName: a.runState.traceFlow,
		Steps:    a.runState.traceSteps,
		Files:    make([]RunTraceFile, 0, len(a.runState.traces)),
		MaxParts: desktopTraceLimits.MaxParts,
	}
	for _, rt := range a.runState.traces {
		out.Files = append(out.Files, rt.file)
	}
	return out
}

// GetLastTrace returns a trace retained from the last run. With an empty
// filePath it is the trace of the file that completed last; otherwise the
// trace of that input file, and with a locale too the trace of that file's
// pass for the locale (an empty locale picks the file's newest pass). Nil
// when nothing retained matches; ListRunTraces names what would.
func (a *App) GetLastTrace(filePath, locale string) *flow.FlowTrace {
	if a.runState == nil {
		return nil
	}
	a.runState.mu.Lock()
	defer a.runState.mu.Unlock()
	for _, rt := range slices.Backward(a.runState.traces) {
		if filePath != "" && rt.file.FilePath != filePath {
			continue
		}
		if locale != "" && rt.file.Locale != locale {
			continue
		}
		return rt.trace
	}
	return nil
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
