package host

import (
	"context"
	"io"
	"sync"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/host/output"
)

// A check inside a flow reports what it found here. The tool writes its
// verdict into a stand-off annotation that no writer serializes, so the flow
// run prints the findings itself: the same kapi.check/v2 findings `kapi check`
// and `kapi exec <check>` print, nested inside the run's own output so one run
// remains one document under every --output-format.

// The flow run reports and does not gate, as `kapi exec` does. `kapi check`
// and `kapi up` are the enforcement points whose exit codes CI is written
// against.

// flowFindings accumulates the findings the blocks of one run carry. One
// instance per reported run, shared by every file's tap, files are processed
// in parallel and a tool may itself be block-parallel, so every field is
// guarded.
type flowFindings struct {
	mu     sync.Mutex
	diags  []check.Diagnostic
	blocks int
	// files are the files a chain holding a check step was built for, by
	// display name.
	files map[string]struct{}
	// checks is set when the run's flow holds a check step. Such a run reports
	// even when its check steps read no block, and its report is did_not_run.
	checks bool
}

// tapped records that a chain holding a check step was built for file.
func (c *flowFindings) tapped(file string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.checks = true
	if c.files == nil {
		c.files = map[string]struct{}{}
	}
	c.files[file] = struct{}{}
}

// fileCount is the number of files the run's check steps were run over.
func (c *flowFindings) fileCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.files)
}

// observe records one block's findings under the file it came from. Called once
// per block, at the end of the tool chain, so the annotation carries everything
// every step of the flow contributed.
func (c *flowFindings) observe(file string, b *model.Block) {
	// clear=false: the block is still on its way to a writer or a store commit,
	// and a reporting read must not consume what the content carries.
	ann, ok := model.AnnoAs[*check.FindingsAnnotation](b, check.AnnotationKey)

	c.mu.Lock()
	defer c.mu.Unlock()
	c.blocks++
	if !ok {
		return
	}
	loc := check.Location{File: file, Block: blockKey(b)}
	for _, f := range ann.Findings {
		// An empty family lets each finding name its own producer: a flow can
		// run several checks, and the annotation names only the last of them.
		c.diags = append(c.diags, check.DiagnosticFrom(f, "", loc))
	}
}

// report builds the run's findings report.
func (c *flowFindings) report() *FindingsReport {
	c.mu.Lock()
	defer c.mu.Unlock()
	// This report carries findings, not a verdict, and BuildReport's counting,
	// scoring and stable ordering are what it is called for. Files finish in
	// whatever order they finish. Decide still marks a report over no block
	// did_not_run, which is coverage.
	built := check.BuildReport(check.Target{Kind: "file", Blocks: c.blocks}, c.diags)
	r := newFindingsReport(built)
	return &r
}

// reportIfChecked is report for a run whose flow holds a check step, and nil
// for one that holds none, which keeps an ordinary run's output free of an
// empty findings table. A check step that read no block still reports, and its
// report is did_not_run.
func (c *flowFindings) reportIfChecked() *FindingsReport {
	c.mu.Lock()
	checks := c.checks
	c.mu.Unlock()
	if !checks {
		return nil
	}
	return c.report()
}

// FlowFindings is what one flow run's check steps reported: the summary and
// the diagnostics the run prints, what the check steps read, and whether they
// ran. See RunCmdOptions.OnFindings.
type FlowFindings struct {
	Summary  check.Summary
	Findings []check.Diagnostic
	// Files counts the files the check steps were run over, and Blocks the
	// content blocks they read in them.
	Files  int
	Blocks int
	// DidNotRunCause is check.CauseNothingToCheck when the check steps read no
	// block, and empty when they read any. DidNotRun gives the reason in words.
	// A surface that gates on the run treats such a report the way `kapi check`
	// treats a check that did not run.
	DidNotRunCause string
	DidNotRun      []string
}

// flowFindingsTap is one file's view of the run's collector: a
// flow.StreamingCollector that names the file each observed block belongs to.
// The tool chain is shared across a project run's files, so the file cannot be
// carried on the collector, it is carried on the tap the chain was wrapped with
// for that file.
type flowFindingsTap struct {
	shared *flowFindings
	file   string
}

// Observe is called inline for every part leaving the last tool of the chain.
func (t *flowFindingsTap) Observe(part *model.Part) {
	if part == nil || part.Type != model.PartBlock {
		return
	}
	if b, ok := part.Resource.(*model.Block); ok && b != nil {
		t.shared.observe(t.file, b)
	}
}

// Collect satisfies flow.Collector. The tap reports through Observe, nothing in
// the flow-run path drives the batch API, so there is nothing to do here.
func (t *flowFindingsTap) Collect(context.Context, *flow.Item, []*model.Part) error { return nil }

// Result satisfies flow.Collector. The run's report is read from the shared
// collector once, after every file, rather than per tap.
func (t *flowFindingsTap) Result() (flow.CollectorResult, error) {
	return flow.CollectorResult{Name: "findings"}, nil
}

var _ flow.StreamingCollector = (*flowFindingsTap)(nil)

// beginFlowFindings arms a run-scoped findings collector and returns the
// function that disarms it. Arming happens before the tool chain is built,
// because buildFlowTools attaches the taps. Every path that prints a run result
// arms exactly once, so a project run that dispatches to a built-in flow
// reports its findings under the output it prints.
//
// Disarming hands the run's report to the findings sink when one is set
// (RunCmdOptions.OnFindings), after the run has printed it.
//
// A run whose output is suppressed and that nobody gates on arms nothing, and
// taps nothing: a convergence pass, whose workers are silent and whose flows
// run checks, would otherwise pay a channel hop per part. A sink arms the
// collector under `--quiet` too, so a gate under `-q` still reads findings.
//
// The collector learns from flowName whether the flow holds a check step. A
// run whose check steps read no block still reports, as did_not_run.
func (a *App) beginFlowFindings(flowName string) func() {
	sink := a.flowFindingsSink
	if a.Quiet && sink == nil {
		return func() {}
	}
	prev := a.flowFindings
	c := &flowFindings{checks: a.flowDeclaresChecks(flowName)}
	a.flowFindings = c
	return func() {
		a.flowFindings = prev
		if sink == nil {
			return
		}
		if r := c.reportIfChecked(); r != nil {
			sink(FlowFindings{
				Summary:        r.Summary,
				Findings:       r.Findings,
				Files:          c.fileCount(),
				Blocks:         r.Target.Blocks,
				DidNotRunCause: r.DidNotRunCause,
				DidNotRun:      r.DidNotRun,
			})
		}
	}
}

// flowDeclaresChecks reports whether the flow a run is about to execute holds a
// check step: the project chain the run was handed, or else the built-in flow
// of that name.
func (a *App) flowDeclaresChecks(flowName string) bool {
	if a.projectFlowTools != nil {
		return a.chainProducesFindings(a.projectFlowTools)
	}
	def := builtInFlow(flowName)
	if def == nil || a.ToolReg == nil {
		return false
	}
	for _, tn := range orderedToolNodes(def) {
		if ProducesFindings(a.ToolReg.Schema(registry.ToolID(tn.Name))) {
			return true
		}
	}
	return false
}

// flowFindingsReport returns the armed collector's report, or nil when no
// collector is armed or the flow holds no check step.
func (a *App) flowFindingsReport() *FindingsReport {
	if a.flowFindings == nil {
		return nil
	}
	return a.flowFindings.reportIfChecked()
}

// tapFindings wraps the chain's last tool so every block it emits is observed,
// when a collector is armed and at least one step of the chain declares a
// findings port. The wrap goes on the LAST tool rather than on each check: a
// block passes every step before it leaves, so one observation sees everything
// the flow contributed and counts the block once.
//
// The returned slice is always a copy, a project run's chain is built once and
// shared across its files, and each file needs a tap of its own to name itself.
func (a *App) tapFindings(tools []tool.Tool, inputPath string) []tool.Tool {
	if a.flowFindings == nil || len(tools) == 0 {
		return tools
	}
	if !a.chainProducesFindings(tools) {
		return tools
	}
	file := DisplayName(inputPath)
	a.flowFindings.tapped(file)
	tapped := make([]tool.Tool, len(tools))
	copy(tapped, tools)
	last := len(tapped) - 1
	tapped[last] = flow.NewTappingTool(tapped[last], &flowFindingsTap{
		shared: a.flowFindings,
		file:   file,
	})
	return tapped
}

// chainProducesFindings reports whether any step of the chain declares a
// findings port. ProducesFindings asks the same IO-contract question of a
// single tool, which decides whether `kapi exec` reports. A chain
// that redacts ahead of a tool that hands blocks to a program reports too:
// that tool records a failing finding on each block whose program output
// changes a redaction placeholder (chainChecksRedactionTokens).
func (a *App) chainProducesFindings(tools []tool.Tool) bool {
	if a.ToolReg == nil {
		return false
	}
	for _, t := range tools {
		if t == nil {
			continue
		}
		if ProducesFindings(a.ToolReg.Schema(registry.ToolID(t.Name()))) {
			return true
		}
	}
	return a.chainChecksRedactionTokens(tools)
}

// flowRunReport is a flow run's own output with the findings its check steps
// produced nested inside it. The embedded FlowRunOutput's fields are promoted,
// so `--json` emits what it always emitted plus a `findings` object, and
// `--output-format yaml` follows because output.Print routes both through the
// same json tags.
type flowRunReport struct {
	output.FlowRunOutput
	Findings *FindingsReport `json:"findings,omitempty"`
}

// FormatText prints the run line the flow always printed, then the findings
// table `kapi check` prints.
func (r flowRunReport) FormatText(w io.Writer) error {
	if err := r.FlowRunOutput.FormatText(w); err != nil {
		return err
	}
	if r.Findings == nil {
		return nil
	}
	return r.Findings.FormatText(w)
}

// printFlowRun prints one flow run's result, carrying the findings its check
// steps produced when the flow declared any.
func (a *App) printFlowRun(cmd Command, out output.FlowRunOutput) error {
	if out.ParseCacheOff == "" {
		out.ParseCacheOff = a.ParseCacheOff()
	}
	return output.Print(cmd, flowRunReport{FlowRunOutput: out, Findings: a.flowFindingsReport()})
}
