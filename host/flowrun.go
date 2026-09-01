package host

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// Flow-run event types — the vocabulary a RunEventSink receives. It mirrors
// the desktop runner's historical RunEvent stream so the Wails adapter is a
// field-for-field mapping.
const (
	// FlowEventState marks a run-state transition: the initial "running"
	// message, then one "Running for <locale> (<n> files)..." per locale pass.
	FlowEventState = "state"
	// FlowEventProgress carries per-file progress (FileIndex/FileCount/
	// FilePath before each file) and live AI block progress (Message).
	FlowEventProgress = "progress"
	// FlowEventPipelineMetrics carries a per-step throughput snapshot (Steps),
	// emitted on a ticker while the run is live and once at the end.
	FlowEventPipelineMetrics = "pipeline_metrics"
	// FlowEventFileDone reports one written output file (FilePath in,
	// OutputPath out) so a UI can refresh its file listings.
	FlowEventFileDone = "file_done"
	// FlowEventComplete closes the stream with totals (DurationMs,
	// FilesProcessed, Message).
	FlowEventComplete = "complete"
)

// FlowRunEvent is one progress event of a multi-locale flow run
// (RunFlowAllLocales). Errors are not events: a failed run returns a typed
// error (FlowToolBuildError, flowFileError) so the embedding surface renders
// it in its own voice.
type FlowRunEvent struct {
	Type string `json:"type"`
	Flow string `json:"flow"`
	// Locale is the pass's target locale (empty for a source-only pass).
	Locale  string `json:"locale,omitempty"`
	Message string `json:"message,omitempty"`

	// Per-file progress (FlowEventProgress / FlowEventFileDone).
	FileIndex  int    `json:"file_index,omitempty"`
	FileCount  int    `json:"file_count,omitempty"`
	FilePath   string `json:"file_path,omitempty"`
	OutputPath string `json:"output_path,omitempty"`

	// Pipeline metrics snapshot (FlowEventPipelineMetrics).
	Steps []flow.StepSnapshot `json:"steps,omitempty"`

	// Run totals (FlowEventComplete).
	DurationMs     int64 `json:"duration_ms,omitempty"`
	FilesProcessed int   `json:"files_processed,omitempty"`
}

// RunEventSink receives the run's event stream. A nil sink is valid and
// discards everything (callers that only want the returned result). Events
// arrive one at a time, but possibly from different goroutines (the metrics
// ticker, AI progress callbacks); a sink must be safe for that.
type RunEventSink func(FlowRunEvent)

// FlowRunOptions configures one multi-locale flow run.
type FlowRunOptions struct {
	// FlowName names the flow (for events and error messages).
	FlowName string
	// Spec is the flow's steps. When nil it is looked up on the project's
	// `flows:` map under FlowName.
	Spec *flow.StepsSpec
	// Project is the loaded recipe; when nil it is loaded from ProjectPath.
	Project *project.KapiProject
	// ProjectPath is the recipe path. It anchors the project context (content
	// targets, format config) and binds resource resolution (project content memory,
	// terms, voice profile) to this recipe rather than a cwd discovery.
	ProjectPath string
	// InputPaths are the source files to process. Required.
	InputPaths []string
	// TargetLocales overrides the project's defaults.target_languages as the
	// candidate set bilingual tools without a default locale iterate over.
	// Empty → the project's target languages.
	TargetLocales []string
	// MetricsInterval throttles the pipeline-metrics snapshots; <= 0 uses the
	// 200ms default. Snapshots are only produced when a sink is set.
	MetricsInterval time.Duration
}

// FlowRunResult is the structured outcome of a completed run.
type FlowRunResult struct {
	Flow string
	// LocalePasses is the resolved pass set (flow.ResolveFlowLocales
	// semantics): each element is one execution pass's locale set. A
	// source-only flow reports one pass holding just the source locale.
	LocalePasses [][]string
	// FilesProcessed counts input×pass executions that completed.
	FilesProcessed int
	// OutputPaths lists the written output files in execution order.
	OutputPaths []string
	// Duration is the wall-clock run time.
	Duration time.Duration
}

// FlowToolBuildError reports a tool that could not be assembled for a locale
// pass. Error() is transparent (the underlying error's text) so CLI wrapping
// stays byte-identical; embedders (the desktop) use the fields to render
// venue-appropriate guidance (e.g. ambiguous-credential hints).
type FlowToolBuildError struct {
	Tool   string
	Locale string
	Err    error
}

func (e *FlowToolBuildError) Error() string { return e.Err.Error() }
func (e *FlowToolBuildError) Unwrap() error { return e.Err }

// flowFileError reports one input file that failed during a locale pass. Its
// text is the run-feed line the desktop historically emitted.
type flowFileError struct {
	Path   string
	Locale string
	Err    error
}

func (e *flowFileError) Error() string {
	return fmt.Sprintf("%s [%s]: %v", filepath.Base(e.Path), e.Locale, e.Err)
}
func (e *flowFileError) Unwrap() error { return e.Err }

// flowMetricsIntervalDefault paces the pipeline-metrics snapshots.
const flowMetricsIntervalDefault = 200 * time.Millisecond

// RunFlowAllLocales executes a project flow across every locale pass it
// applies to — the one multi-locale flow orchestrator shared by the kapi
// CLI and Kapi Desktop (the desktop adapts the sink to Wails run events).
// It owns the four concerns the desktop used to hand-roll:
//
//   - Locale-pass selection: flow.ResolveFlowLocales — the single
//     applicability-based answer to "which locales does this flow run for"
//     (tool cardinality + tool default locales + project targets). This is
//     deliberately NOT convergence's need-based selection ("which locales
//     still have work", cli/converge.go localesNeedingPass) — a flow run
//     applies the flow; convergence reconciles remaining work.
//   - Per-pass tool assembly: the CLI's project tool-building semantics
//     (buildProjectFlowTools) — resource-ref resolution, project bindings,
//     content memory injection, the AD-006 placement gate — including the per-tool
//     cleanup contract the desktop's former copy leaked.
//   - Per-file execution: sequential, file-writing (output paths resolved
//     through the canonical project target resolver). The CLI's in-project
//     process-only default (AD-026) is a different sink binding and stays
//     with `kapi run`'s cobra path.
//   - Event emission: the FlowRunEvent stream on the sink (nil = discard).
//
// A source-only flow (all tools monolingual) runs once with no target. The
// run stops at the first error, returned as a typed FlowToolBuildError or
// flowFileError; context cancellation is not an error — the run completes
// early with the files processed so far.
func (a *App) RunFlowAllLocales(ctx context.Context, opts FlowRunOptions, sink RunEventSink) (*FlowRunResult, error) {
	ctx = ctxOrBackground(ctx)
	a.InitRegistries()

	if opts.FlowName == "" {
		return nil, errors.New("flow name is required")
	}
	if len(opts.InputPaths) == 0 {
		return nil, errors.New("no input files specified")
	}
	proj := opts.Project
	if proj == nil {
		if opts.ProjectPath == "" {
			return nil, errors.New("a project is required (set Project or ProjectPath)")
		}
		p, err := project.LoadWithOptions(opts.ProjectPath, project.LoadOptions{SkipRequiresCheck: true})
		if err != nil {
			return nil, fmt.Errorf("load project: %w", err)
		}
		proj = p
	}
	spec := opts.Spec
	if spec == nil {
		spec = proj.Flow(opts.FlowName)
	}
	if spec == nil {
		return nil, fmt.Errorf("flow %q not found", opts.FlowName)
	}

	pctx := project.NewProjectContext(proj, opts.ProjectPath)
	source := string(pctx.SourceLocale)
	targets := opts.TargetLocales
	if len(targets) == 0 {
		targets = localeStrings(pctx.TargetLocales)
	}

	// THE locale-pass selection (shared with the CLI's plain project
	// flow-run in RunFromProject). nil → source-only: run once, no target.
	passes := flow.ResolveFlowLocales(spec, flow.BuildToolInfoMap(a.ToolReg), source, targets)
	if passes == nil {
		passes = [][]string{{source}}
	}

	// The tool builders speak cobra (flag lookups, project discovery); an
	// embedded run drives them through a synthetic command bound to this
	// recipe — the RunUp pattern — so resource resolution (project content memory,
	// terms, voice profile) binds to THIS project, never a cwd walk.
	cmd := NewEnvCommand(ctx, "flow-run")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	AddProjectFlag(cmd)
	if opts.ProjectPath != "" {
		if err := cmd.Flags().Set(projectFlagName, opts.ProjectPath); err != nil {
			return nil, err
		}
	}

	// Standing project context + bindings for the run's tool assembly and
	// output-path resolution; restored on exit so an embedding App can be
	// reused.
	savedPctx, savedBindings := a.ProjectContext, a.ProjectBindings
	savedSource, savedTarget := a.SourceLang, a.TargetLang
	defer func() {
		a.ProjectContext, a.ProjectBindings = savedPctx, savedBindings
		a.SourceLang, a.TargetLang = savedSource, savedTarget
	}()
	a.ProjectContext = pctx
	if source != "" {
		a.SourceLang = source
	}

	// The venue caveat goes to the process's own stderr rather than to cmd: the
	// synthetic command above discards its error stream on purpose, because it
	// carries tool-assembly noise an embedder should not see. This is not that —
	// it says the recipe's coordinates govern here and would not govern a server
	// run — so discarding it would be the one thing worse than printing it.
	a.WarnUnsyncedCoordinates(os.Stderr, proj)

	// Governance is per collection: the input set splits into one group per
	// distinct (voice profile, terms) binding, and each group runs the flow with
	// its own bindings and its own tool chain. A recipe where no collection
	// overrides anything yields exactly one group holding every input, so the
	// pass structure, the event stream and the tool assembly are unchanged.
	groups, err := a.groupInputsByBinding(cmd, proj, pctx.ProjectDir, opts.InputPaths)
	if err != nil {
		return nil, err
	}
	// Without a recipe path there is nothing to resolve bindings from, and an
	// embedder's own standing context on the App is left alone.
	//
	// Cancellation is not a binding failure. Resolution reads the project store,
	// so a run cancelled before it starts turns every query into
	// "context canceled" — and this function's contract is that cancellation
	// ends the run early with a terminal event, never with an error. Checked
	// here rather than swallowed inside each reader, so a genuine store fault
	// still surfaces.
	bound := opts.ProjectPath != "" && ctx.Err() == nil
	if bound {

		if err := a.resolveGroupBindings(cmd, proj, opts.ProjectPath, groups); err != nil {
			return nil, err
		}
	}

	// The run's own project store: the files it writes are half the deliverable,
	// and the `targets/<locale>` overlays it commits are the half a later merge
	// replays and the status panel counts. A run that records nothing leaves the
	// store reading "never translated" over content that is translated.
	projStore := a.openProjectBlockStore(ctx)

	emit := func(ev FlowRunEvent) {
		if sink == nil {
			return
		}
		ev.Flow = opts.FlowName
		sink(ev)
	}

	start := time.Now()
	totalFiles := len(opts.InputPaths) * len(passes)
	var filesDone atomic.Int64
	var outputs []string

	// Pipeline metrics from the spec's step names (the labels a UI renders),
	// snapshotted on a ticker while the run is live.
	stepNames := make([]string, len(spec.Steps))
	for i, s := range spec.Steps {
		stepNames[i] = s.Tool
	}
	metrics := flow.NewPipelineMetrics(stepNames)
	if sink != nil {
		interval := opts.MetricsInterval
		if interval <= 0 {
			interval = flowMetricsIntervalDefault
		}
		ticker := time.NewTicker(interval)
		stopTick := make(chan struct{})
		var wg sync.WaitGroup
		wg.Go(func() {
			for {
				select {
				case <-ticker.C:
					emit(FlowRunEvent{Type: FlowEventPipelineMetrics, Steps: metrics.Snapshot()})
				case <-stopTick:
					return
				}
			}
		})
		defer func() {
			ticker.Stop()
			close(stopTick)
			wg.Wait()
		}()
	}

	emit(FlowRunEvent{Type: FlowEventState, Message: "running"})

	// Live block progress from AI tools, forwarded onto the sink.
	var onProgress func(aiprovider.ProgressEvent)
	if sink != nil {
		onProgress = func(e aiprovider.ProgressEvent) {
			var msg string
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
			emit(FlowRunEvent{
				Type: FlowEventProgress, Message: msg,
				FileIndex: int(filesDone.Load()), FileCount: totalFiles,
			})
		}
	}

	// Committed source approvals, read once for the whole run and stamped onto
	// each block as it leaves the reader. Without this the in-flow source gate
	// re-derives readiness from the checks alone, so a run would hold a unit
	// `kapi status` reports as approved.
	seedSourceState, seedErr := a.SourceStateSeeder(ctx, pctx.ProjectDir, string(pctx.SourceLocale))
	if seedErr != nil && !errors.Is(seedErr, context.Canceled) {
		// A real read failure is fatal rather than silent: without the approvals
		// the source gate would hold units the project has already signed off,
		// and a run that quietly translated nothing would look like a converged
		// one. Cancellation is not that — the run is being torn down, and the
		// same convention governs the feeder's error below.
		return nil, fmt.Errorf("read source approvals: %w", seedErr)
	}

	runPass := func(pass []string, group bindingGroup) error {
		// Target locale is the second element of the pass (if present).
		lang := ""
		if len(pass) > 1 {
			lang = pass[1]
		}
		emit(FlowRunEvent{
			Type: FlowEventState, Locale: lang,
			Message: fmt.Sprintf("Running for %s (%d files)...", lang, len(group.Inputs)),
		})

		// Tools are rebuilt per pass: the target locale is baked into tool
		// config, and each pass's cleanup releases what its assembly opened.
		// The group's bindings are on the App before assembly, because that is
		// where they reach the steps (buildProjectFlowTools → applyBindings).
		a.TargetLang = lang
		if bound {
			a.ProjectBindings = group.bindings
		}
		rCtx := flow.ResourceContext{ProjectDir: pctx.ProjectDir, SourceLocale: source, TargetLocale: lang}
		tools, cleanup, err := a.buildProjectFlowTools(cmd, opts.FlowName, spec, &rCtx, onProgress)
		if err != nil {
			return err
		}
		defer cleanup()
		tools = flow.WrapWithMetrics(tools, metrics)
		// Outside the metrics wrapper, so the span covers the whole run.
		tools = flow.WrapWithSpans(tools)

		for _, inputPath := range group.Inputs {
			if ctx.Err() != nil {
				return nil
			}
			metrics.Reset()
			emit(FlowRunEvent{
				Type: FlowEventProgress, Locale: lang,
				FileIndex: int(filesDone.Load()), FileCount: totalFiles, FilePath: inputPath,
			})

			// Canonical output resolution: the matched project content
			// item's target template, else the <base>_<lang><ext> sibling.
			outputPath, oerr := a.resolveOutputPath(inputPath, "")
			if oerr != nil {
				return oerr
			}
			// The recipe's word on this file: the item whose glob claims it names
			// both the format and the format.config the run must read it under,
			// exactly as every other derivation over the project resolves them.
			item := a.projectItemFor(inputPath)
			runner := flow.NewFileRunner(flow.FileRunnerConfig{
				FormatReg:    a.FormatReg,
				SourceLocale: pctx.SourceLocale,
				Encoding:     pctx.Encoding,
				Store:        projStore,
				ProjectRoot:  pctx.ProjectDir,
				DetectFormat: func(path string) registry.FormatID {
					if declared := itemFormatName(item); declared != "" {
						return registry.FormatID(declared)
					}
					return registry.FormatID(pctx.DetectFormat(a.FormatReg, path))
				},
				ConfigureReader: func(reader format.DataFormatReader, fmtName registry.FormatID) error {
					return pctx.ConfigureReaderFor(reader, string(fmtName), item)
				},
				ConfigureWriter: func(writer format.DataFormatWriter, fmtName registry.FormatID) error {
					return pctx.ConfigureWriterFor(writer, string(fmtName), item)
				},
				SeedBlockState: seedSourceState,
			})
			if err := runner.RunFile(ctx, opts.FlowName, tools, inputPath, outputPath, lang); err != nil {
				// Final metrics snapshot so a UI preserves counts at failure.
				emit(FlowRunEvent{Type: FlowEventPipelineMetrics, Steps: metrics.Snapshot()})
				return &flowFileError{Path: inputPath, Locale: lang, Err: err}
			}
			filesDone.Add(1)
			outputs = append(outputs, outputPath)
			emit(FlowRunEvent{
				Type: FlowEventFileDone, Locale: lang,
				FilePath: inputPath, OutputPath: outputPath,
			})
		}
		return nil
	}

passLoop:
	for _, pass := range passes {
		if ctx.Err() != nil {
			break
		}
		for _, group := range groups {
			if err := runPass(pass, group); err != nil {
				// Cancellation mid-file surfaces as a wrapped context error from
				// the runner; per the documented contract it is not a run error —
				// stop the loop so the complete event and totals still emit.
				if ctx.Err() != nil && errors.Is(err, ctx.Err()) {
					break passLoop
				}
				return nil, err
			}
		}
	}

	duration := time.Since(start)
	done := int(filesDone.Load())
	emit(FlowRunEvent{Type: FlowEventPipelineMetrics, Steps: metrics.Snapshot()})
	emit(FlowRunEvent{
		Type:       FlowEventComplete,
		DurationMs: duration.Milliseconds(), FilesProcessed: done,
		Message: fmt.Sprintf("Completed %d files for %d locale passes in %s",
			done, len(passes), duration.Round(time.Millisecond)),
	})

	return &FlowRunResult{
		Flow:           opts.FlowName,
		LocalePasses:   passes,
		FilesProcessed: done,
		OutputPaths:    outputs,
		Duration:       duration,
	}, nil
}

// buildProjectFlowTools assembles the tool chain for one locale pass of a
// project flow spec — the single implementation behind both the CLI's project
// flow-run (runProjectStepsOver) and the multi-locale orchestrator
// (RunFlowAllLocales). It runs the AD-006 placement gate over the compiled
// steps graph, resolves each step's config (resource references + project
// bindings/presets, via toolFromStep), injects the project content memory into
// content memory-requiring steps, and optionally wires a live AI progress callback into
// every step. The returned cleanup releases every resource the assembly
// opened (e.g. SQLite content memory handles); callers must run it after the pass — the
// desktop's former hand-rolled copy leaked exactly this contract.
//
// Step failures are returned wrapped in a *FlowToolBuildError (transparent
// Error(), so CLI messages are unchanged) carrying the step's tool name and
// the pass's target locale for venue-specific rendering.
func (a *App) buildProjectFlowTools(cmd Command, flowName string, spec *flow.StepsSpec, rCtx *flow.ResourceContext, onProgress func(aiprovider.ProgressEvent)) ([]tool.Tool, func(), error) {
	if len(spec.SourceTransforms) > 0 {
		return nil, nil, fmt.Errorf("flow %q uses the removed source_transforms stage (AD-006): list transformers as ordered steps", flowName)
	}
	// Transformer placement gate (AD-006) over the compiled steps graph —
	// unconditional at load, like the built-in flow path. The gate sees each
	// node's preset-merged config (defaults.tools), so a preset that enables
	// entity detection drives the same contract resolution the runtime uses.
	if nodes, edges, err := flow.StepsToGraph(spec); err == nil {
		if b := a.ProjectBindings; b != nil && len(b.ToolPresets) > 0 {
			for i := range nodes {
				nodes[i].Config = MergeToolPreset(b.ToolPresets[nodes[i].Name], nodes[i].Config)
			}
		}
		def := &flow.FlowDefinition{ID: flowName, Name: flowName, Nodes: nodes, Edges: edges}
		if err := a.checkFlowPlacement(def); err != nil {
			return nil, nil, err
		}
	}

	var cleanups []func()
	cleanup := func() {
		for _, fn := range cleanups {
			fn()
		}
	}

	var tools []tool.Tool
	for _, step := range spec.Steps {
		if onProgress != nil {
			// Live progress for AI tools; non-marshalable config values are
			// skipped by schema decoding, so tools that don't read the key
			// ignore it (the desktop runner's historical injection).
			step.Config = mergeFlowNodeConfig(step.Config, map[string]any{"onProgress": onProgress})
		}
		t, stepCleanup, err := a.toolFromStep(step, cmd, rCtx)
		if stepCleanup != nil {
			cleanups = append(cleanups, stepCleanup)
		}
		if err != nil {
			cleanup()
			return nil, nil, fmt.Errorf("flow %q: %w", flowName, &FlowToolBuildError{Tool: step.Tool, Locale: a.TargetLang, Err: err})
		}
		tools = append(tools, t)
	}
	return tools, cleanup, nil
}

// BuildProjectFlowTools assembles the placement-gated tool chain for one
// (source, target) pass of a project flow spec, scoped to the given project
// context. It owns the synthetic command and resource context the assembly
// needs, so an embedding surface (the MCP run_flow porcelain) gets the same
// AD-006 gate and binding resolution as the CLI without reproducing them. The
// returned cleanup releases resources the assembly opened and must run after
// the pass.
func (a *App) BuildProjectFlowTools(ctx context.Context, flowName string, spec *flow.StepsSpec, pctx *project.ProjectContext, projectPath, src, tgt string) ([]tool.Tool, func(), error) {
	cmd := NewEnvCommand(ctx, "mcp-flow")
	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)
	AddProjectFlag(cmd)
	if projectPath != "" {
		if err := cmd.Flags().Set(projectFlagName, projectPath); err != nil {
			return nil, nil, err
		}
	}
	savedPctx := a.ProjectContext
	savedSrc, savedTgt := a.SourceLang, a.TargetLang
	a.ProjectContext = pctx
	a.SourceLang, a.TargetLang = src, tgt
	defer func() {
		a.ProjectContext = savedPctx
		a.SourceLang, a.TargetLang = savedSrc, savedTgt
	}()
	rCtx := &flow.ResourceContext{ProjectDir: pctx.ProjectDir, SourceLocale: src, TargetLocale: tgt}
	return a.buildProjectFlowTools(cmd, flowName, spec, rCtx, nil)
}
