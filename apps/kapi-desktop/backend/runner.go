package backend

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/cli/credentials"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
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

	// Pipeline metrics snapshot (when type == "pipeline_metrics")
	Steps []flow.StepSnapshot `json:"steps,omitempty"`

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
	events    []RunEvent      // accumulated events for reconnection
}

func newRunner() *runner {
	return &runner{state: RunStateIdle}
}

// RunFlow executes a flow by name from the current project. Target locales
// are inferred from the flow's tool chain metadata (Framework AD-006) — the frontend
// passes project target languages as a fallback, but ResolveFlowLocales
// determines the actual locale passes based on tool cardinality.
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
		return errors.New("a flow is already running — cancel it first")
	}

	ctx, cancel := context.WithCancel(context.Background())
	a.runState.state = RunStateRunning
	a.runState.cancel = cancel
	a.runState.running = true
	a.runState.events = nil // clear events from previous run
	a.runState.mu.Unlock()

	pctx := project.NewProjectContext(op.Project, op.Path)

	// Resolve locale passes from tool chain metadata (Framework AD-006).
	// Falls back to project target languages for bilingual tools without defaults.
	toolInfoMap := flow.BuildToolInfoMap(a.toolReg)
	localePasses := flow.ResolveFlowLocales(spec, toolInfoMap, string(pctx.SourceLocale), targetLangs)

	go a.executeFlowAllLangs(ctx, flowName, spec, inputPaths, localePasses, pctx)
	return nil
}

// executeFlowAllLangs runs the flow for each locale pass sequentially.
// Each pass is a locale set (e.g., ["en-US", "de-DE"]) determined by
// ResolveFlowLocales. Tools are rebuilt per pass since target locale is
// baked into tool config. If localePasses is nil (source-only flow),
// runs once with no target.
func (a *App) executeFlowAllLangs(ctx context.Context, flowName string, spec *flow.StepsSpec, inputPaths []string, localePasses [][]string, pctx *project.ProjectContext) {
	defer func() {
		a.runState.mu.Lock()
		a.runState.running = false
		a.runState.mu.Unlock()
	}()

	start := time.Now()

	// Source-only flows: run once with no target.
	if localePasses == nil {
		localePasses = [][]string{{string(pctx.SourceLocale)}}
	}

	totalFiles := len(inputPaths) * len(localePasses)
	filesDone := 0

	// Build pipeline metrics from step names.
	stepNames := make([]string, len(spec.Steps))
	for i, s := range spec.Steps {
		stepNames[i] = s.Tool
	}
	metrics := flow.NewPipelineMetrics(stepNames)

	// Start 200ms ticker to emit pipeline metrics snapshots.
	ticker := time.NewTicker(200 * time.Millisecond)
	stopTick := make(chan struct{})
	tickDone := make(chan struct{})
	go func() {
		defer close(tickDone)
		for {
			select {
			case <-ticker.C:
				a.emitRunEvent(RunEvent{
					Type: "pipeline_metrics", FlowID: flowName,
					Steps: metrics.Snapshot(),
				})
			case <-stopTick:
				return
			}
		}
	}()
	defer func() {
		ticker.Stop()
		close(stopTick)
		<-tickDone
	}()

	a.emitRunEvent(RunEvent{
		Type: "state", FlowID: flowName, Message: "running",
	})

	// Progress callback for AI tools — emits live block progress to the frontend.
	onProgress := func(e aiprovider.ProgressEvent) {
		msg := ""
		if e.TotalBlocks > 0 {
			msg = fmt.Sprintf("[%d/%d]", e.Block, e.TotalBlocks)
		} else {
			msg = fmt.Sprintf("[%d]", e.Block)
		}
		if e.Thinking != "" {
			think := e.Thinking
			if len(think) > 80 {
				think = think[:77] + "..."
			}
			msg += " " + think
		}
		a.emitRunEvent(RunEvent{
			Type: "progress", FlowID: flowName, Message: msg,
			FileIndex: filesDone, FileCount: totalFiles,
		})
	}

	for _, pass := range localePasses {
		if ctx.Err() != nil {
			break
		}

		// Target locale is the second element in the pass (if present).
		lang := ""
		if len(pass) > 1 {
			lang = pass[1]
		}

		a.emitRunEvent(RunEvent{
			Type:    "state",
			FlowID:  flowName,
			Message: fmt.Sprintf("Running for %s (%d files)...", lang, len(inputPaths)),
		})

		// Build tools for this locale pass, with metrics and progress callbacks.
		var tools []tool.Tool
		for _, step := range spec.Steps {
			// Copy step config to avoid mutating the original flow spec.
			config := make(map[string]any)
			maps.Copy(config, step.Config)

			// Inject live progress callback for AI tools.
			config["onProgress"] = onProgress

			t, err := a.toolReg.NewToolWithConfig(registry.ToolID(step.Tool), config, lang)
			if err != nil {
				a.emitRunEvent(RunEvent{
					Type: "error", FlowID: flowName,
					Message: toolBuildErrorMessage(step.Tool, lang, err),
				})
				a.runState.mu.Lock()
				a.runState.state = RunStateError
				a.runState.mu.Unlock()
				return
			}

			tools = append(tools, t)
		}

		// Wrap with pipeline metrics (outermost wrapper).
		tools = flow.WrapWithMetrics(tools, metrics)

		// Process each file for this language.
		for fileIdx, inputPath := range inputPaths {
			if ctx.Err() != nil {
				break
			}

			// Reset metrics for the new file and emit a zero snapshot.
			metrics.Reset()

			a.emitRunEvent(RunEvent{
				Type: "progress", FlowID: flowName,
				FileIndex: filesDone, FileCount: totalFiles, FilePath: inputPath,
			})

			outputPath := a.resolveOutputPath(inputPath, lang)
			runner := flow.NewFileRunner(flow.FileRunnerConfig{
				FormatReg:    a.formatReg,
				SourceLocale: pctx.SourceLocale,
				Encoding:     pctx.Encoding,
				DetectFormat: func(path string) registry.FormatID {
					return registry.FormatID(pctx.DetectFormat(a.formatReg, path))
				},
			})

			if err := runner.RunFile(ctx, flowName, tools, inputPath, outputPath, lang); err != nil {
				// Emit final metrics snapshot so the frontend preserves counts at failure.
				a.emitRunEvent(RunEvent{
					Type: "pipeline_metrics", FlowID: flowName,
					Steps: metrics.Snapshot(),
				})
				a.emitRunEvent(RunEvent{
					Type: "error", FlowID: flowName,
					Message: fmt.Sprintf("%s [%s]: %v", filepath.Base(inputPath), lang, err),
				})
				a.runState.mu.Lock()
				a.runState.state = RunStateError
				a.runState.mu.Unlock()
				return
			}

			filesDone++
			_ = fileIdx

			// Notify the Content view that a new output file landed so it can
			// refresh the outputs shown beneath each source (issue #5).
			a.emitEvent("outputs-changed", map[string]any{"path": outputPath})
		}
	}

	duration := time.Since(start)

	// Emit final metrics snapshot so frontend shows completed state.
	a.emitRunEvent(RunEvent{
		Type: "pipeline_metrics", FlowID: flowName,
		Steps: metrics.Snapshot(),
	})

	a.runState.mu.Lock()
	if a.runState.state == RunStateRunning {
		a.runState.state = RunStateComplete
	}
	a.runState.mu.Unlock()

	a.emitRunEvent(RunEvent{
		Type: "complete", FlowID: flowName,
		DurationMs: duration.Milliseconds(), FilesProcessed: filesDone,
		Message: fmt.Sprintf("Completed %d files for %d locale passes in %s",
			filesDone, len(localePasses), duration.Round(time.Millisecond)),
	})
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
		relSlash := filepath.ToSlash(rel)
		for _, coll := range op.Project.Content {
			for _, item := range coll.EffectiveItems() {
				if item.Target == "" {
					continue
				}
				// Match the input against the content glob (doublestar, so `**`
				// and `{a,b}` behave like ExpandGlob / ResolveContent).
				if !project.MatchGlob(item.Path, relSlash) {
					continue
				}
				// Resolve the output via the shared core resolver.
				return filepath.Join(basePath, project.ResolveTargetPath(item.Path, item.Base, item.Target, relSlash, targetLang))
			}
		}
	}
	// Fallback: input_targetLang.ext
	ext := filepath.Ext(inputPath)
	base := inputPath[:len(inputPath)-len(ext)]
	return fmt.Sprintf("%s_%s%s", base, targetLang, ext)
}
