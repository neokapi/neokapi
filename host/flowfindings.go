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

// A check inside a flow had nowhere to say what it found. The tool wrote its
// verdict into a stand-off annotation, no `core/formats` writer serializes
// overlays, and `kapi run` printed the run line and nothing else — so a recipe
// could declare a guardrail, fire it, and report exit 0 with no output either
// way. `kapi check` had a findings surface and `kapi exec <check>` gained one in
// #1476; the flow verb is where a check actually lives in a recipe.
//
// The surface is the same kapi.check/v1 findings the other two print — same
// table, same `.findings[]` and `.summary` under `--json` — nested inside the
// run's own output so one run remains one document under every --output-format.
//
// It reports and does not gate, which is `kapi exec`'s contract rather than
// `kapi check`'s. `kapi check` and `kapi up` are the enforcement points and
// their exit codes are what CI is written against; a flow run that started
// failing because a step it always ran found something would be a new gating
// rule, and this issue is that the finding was invisible, not that it was
// unenforced.

// flowFindings accumulates the findings the blocks of one run carry. One
// instance per reported run, shared by every file's tap — files are processed
// in parallel and a tool may itself be block-parallel, so every field is
// guarded.
type flowFindings struct {
	mu     sync.Mutex
	diags  []check.Diagnostic
	blocks int
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

// report builds the run's findings report, or nil when no block was observed at
// all (a flow whose chain never reached a tap has nothing to say).
func (c *flowFindings) report() *findingsReport {
	c.mu.Lock()
	defer c.mu.Unlock()
	// The gate is open (every limit -1): this report carries findings, not a
	// verdict, and BuildReport's counting, scoring and stable severity ordering
	// are what it is called for. Files finish in whatever order they finish.
	gate := check.Gate{MaxCritical: -1, MaxMajor: -1, MaxMinor: -1}
	built := check.BuildReport(check.Target{Kind: "file", Blocks: c.blocks}, c.diags, gate)
	return &findingsReport{Target: built.Target, Summary: built.Summary, Findings: built.Findings}
}

// reportIfTapped is report for a run that reached at least one tap, and nil
// for one that ran no check step: nothing tapped, so nothing observed, which
// is what keeps an ordinary run's output free of an empty findings table.
func (c *flowFindings) reportIfTapped() *findingsReport {
	c.mu.Lock()
	tapped := c.blocks > 0 || len(c.diags) > 0
	c.mu.Unlock()
	if !tapped {
		return nil
	}
	return c.report()
}

// FlowFindings is what one flow run's check steps reported: the summary and
// the diagnostics the run prints. See RunCmdOptions.OnFindings.
type FlowFindings struct {
	Summary  check.Summary
	Findings []check.Diagnostic
}

// flowFindingsTap is one file's view of the run's collector: a
// flow.StreamingCollector that names the file each observed block belongs to.
// The tool chain is shared across a project run's files, so the file cannot be
// carried on the collector — it is carried on the tap the chain was wrapped with
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

// Collect satisfies flow.Collector. The tap reports through Observe — nothing in
// the flow-run path drives the batch API — so there is nothing to do here.
func (t *flowFindingsTap) Collect(context.Context, *flow.Item, []*model.Part) error { return nil }

// Result satisfies flow.Collector. The run's report is read from the shared
// collector once, after every file, rather than per tap.
func (t *flowFindingsTap) Result() (flow.CollectorResult, error) {
	return flow.CollectorResult{Name: "findings"}, nil
}

var _ flow.StreamingCollector = (*flowFindingsTap)(nil)

// beginFlowFindings arms a run-scoped findings collector and returns the
// function that disarms it. Arming has to happen before the tool chain is built
// — buildFlowTools is what attaches the taps — and every path that prints a run
// result arms exactly once, so a project run that dispatches to a built-in flow
// reports its findings under the output it actually prints.
//
// Disarming hands the run's report to the findings sink when one is set
// (RunCmdOptions.OnFindings), after the run has printed it, so a surface that
// gates on the run reads the same report the run showed.
//
// A run whose output is suppressed and that nobody gates on arms nothing, so
// it also taps nothing: the collector exists to fill a report `--quiet` does
// not print, and a convergence pass — whose workers are silent by construction
// and whose flows do run checks — would otherwise pay a channel hop per part to
// fill one. A sink arms the collector under `--quiet` too: a gate that read
// nothing under `-q` would pass a push its findings should have stopped.
func (a *App) beginFlowFindings() func() {
	sink := a.flowFindingsSink
	if a.Quiet && sink == nil {
		return func() {}
	}
	prev := a.flowFindings
	c := &flowFindings{}
	a.flowFindings = c
	return func() {
		a.flowFindings = prev
		if sink == nil {
			return
		}
		if r := c.reportIfTapped(); r != nil {
			sink(FlowFindings{Summary: r.Summary, Findings: r.Findings})
		}
	}
}

// flowFindingsReport returns the armed collector's report, or nil when no
// collector is armed or the flow ran no check step.
func (a *App) flowFindingsReport() *findingsReport {
	if a.flowFindings == nil {
		return nil
	}
	return a.flowFindings.reportIfTapped()
}

// tapFindings wraps the chain's last tool so every block it emits is observed,
// when a collector is armed and at least one step of the chain declares a
// findings port. The wrap goes on the LAST tool rather than on each check: a
// block passes every step before it leaves, so one observation sees everything
// the flow contributed and counts the block once.
//
// The returned slice is always a copy — a project run's chain is built once and
// shared across its files, and each file needs a tap of its own to name itself.
func (a *App) tapFindings(tools []tool.Tool, inputPath string) []tool.Tool {
	if a.flowFindings == nil || len(tools) == 0 {
		return tools
	}
	if !a.chainProducesFindings(tools) {
		return tools
	}
	tapped := make([]tool.Tool, len(tools))
	copy(tapped, tools)
	last := len(tapped) - 1
	tapped[last] = flow.NewTappingTool(tapped[last], &flowFindingsTap{
		shared: a.flowFindings,
		file:   DisplayName(inputPath),
	})
	return tapped
}

// chainProducesFindings reports whether any step of the chain declares a
// findings port — the same IO-contract question ProducesFindings answers for a
// single tool, which is what decides whether `kapi exec` reports.
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
	return false
}

// flowRunReport is a flow run's own output with the findings its check steps
// produced nested inside it. The embedded FlowRunOutput's fields are promoted,
// so `--json` emits what it always emitted plus a `findings` object, and
// `--output-format yaml` follows because output.Print routes both through the
// same json tags.
type flowRunReport struct {
	output.FlowRunOutput
	Findings *findingsReport `json:"findings,omitempty"`
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
	return output.Print(cmd, flowRunReport{FlowRunOutput: out, Findings: a.flowFindingsReport()})
}
