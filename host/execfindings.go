package host

import (
	"context"
	"io"
	"sync"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/schema"
)

// A check tool writes its verdict into a stand-off annotation, never into the
// content, so a `kapi exec <check>` run had nowhere to say what it found: it
// annotated the blocks in memory, exited 0, and printed nothing. `kapi exec qa`
// and `kapi exec term-check` — both named in the exec help — were silent this
// way, and #1476 was about to add three more checks to that surface. A check that
// cannot report is the same defect as a check that cannot fail.
//
// findingsCollector closes it through the extension point the tool runner
// already has: a tool whose IO contract declares a findings port gets one, and
// RunToolOnFiles renders the aggregate as the same kapi.check/v1 report
// `kapi check` prints — same table, same `--json` shape, same consumers. The
// rule ids differ in their first segment by design: `kapi check` groups findings
// into its own families (`dnt`, `placeholder`, `hygiene`), while an exec run
// names the single tool that produced them (`dnt-check.do-not-translate`),
// because that is what the caller asked to run.
//
// Gating stays with `kapi check`. `kapi exec` is the raw layer — one tool, one
// pass, no guardrails — and its exit code is a stable contract (`kapi check` is
// the verb that exits 3 on an unmet bar), so reporting findings does not make an
// exec run fail. The report's gate is therefore the open one.

// findingsPorts are the produced ports that carry a check verdict.
//
// The first two write findings under check.AnnotationKey. AnnoVoice does not:
// the voice checks write a profile.VoiceAnnotation under their own key, which
// is why they were absent from this map and why `kapi exec voice-check` wrote
// zero bytes and exited 0 under every provider and profile tried (#2225). The
// tool ran, annotated the blocks, and the run had nowhere to say so — exactly
// the defect the comment above describes, in the one family it did not reach.
var findingsPorts = map[string]bool{
	string(model.OverlayCheck): true,
	string(model.OverlayTerm):  true,
	model.AnnoVoice:            true,
}

// ProducesFindings reports whether a tool's schema declares a findings port.
func ProducesFindings(s *schema.ComponentSchema) bool {
	if s == nil || s.ToolMeta == nil {
		return false
	}
	for _, p := range s.ToolMeta.Produces {
		if findingsPorts[p.Type] {
			return true
		}
	}
	return false
}

// NewFindingsCollectorFor returns a collector factory for a tool that produces
// check findings, or nil for a tool that does not. Callers pass it as
// ToolRunConfig.NewCollector; a tool with a bespoke entry in CollectorFactories
// keeps that one.
func NewFindingsCollectorFor(s *schema.ComponentSchema) func() flow.Collector {
	if !ProducesFindings(s) {
		return nil
	}
	return func() flow.Collector { return &findingsCollector{} }
}

// findingsCollector accumulates the findings every processed block carries, over
// all the files of one run. Safe for concurrent use: RunToolOnFiles processes
// files in parallel and feeds the collector from each worker.
type findingsCollector struct {
	mu     sync.Mutex
	diags  []check.Diagnostic
	blocks int
}

// Collect reads the findings off each block in one document's output parts.
func (c *findingsCollector) Collect(_ context.Context, item *flow.Item, parts []*model.Part) error {
	file := ""
	if item != nil && item.Input != nil {
		file = DisplayName(item.Input.URI)
	}
	var diags []check.Diagnostic
	blocks := 0
	for _, p := range parts {
		if p == nil || p.Type != model.PartBlock {
			continue
		}
		b, ok := p.Resource.(*model.Block)
		if !ok {
			continue
		}
		blocks++
		loc := check.Location{File: file, Block: blockKey(b)}
		// clear=false: the blocks may still be on their way to a writer, and a
		// reporting read must not consume what the content carries.
		if ann, ok := model.AnnoAs[*check.FindingsAnnotation](b, check.AnnotationKey); ok {
			for _, f := range ann.Findings {
				diags = append(diags, check.DiagnosticFrom(f, ann.Source, loc))
			}
		}
		// The voice checks write their own annotation shape. `kapi check`
		// already converts it (host/check.go, collectFileDiagnostics); an exec
		// run needs the same conversion or the tool reports nothing.
		if ann, ok := model.AnnoAs[*coreprofile.VoiceAnnotation](b, model.AnnoVoice); ok {
			for _, f := range ann.Findings {
				diags = append(diags, check.DiagnosticFrom(f, model.AnnoVoice, loc))
			}
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.blocks += blocks
	c.diags = append(c.diags, diags...)
	return nil
}

// Result builds the run's report. check.BuildReport does the counting, scoring
// and the stable severity ordering — files are processed in parallel, so the
// report must not depend on which finished first.
func (c *findingsCollector) Result() (flow.CollectorResult, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	// The gate is open (every limit -1) because exec reports and `kapi check`
	// gates; the resulting verdict is therefore meaningless and is not emitted.
	gate := check.Gate{MaxCritical: -1, MaxMajor: -1, MaxMinor: -1}
	report := check.BuildReport(check.Target{Kind: "file", Blocks: c.blocks}, c.diags, gate)
	return flow.CollectorResult{Name: "findings", Data: findingsReport{
		Target:   report.Target,
		Summary:  report.Summary,
		Findings: report.Findings,
	}}, nil
}

// findingsReport is what a run that is not the gate reports: the findings and
// their roll-up, field-for-field as the kapi.check/v1 Report carries them, so
// `.findings[]` and `.summary` read the same whichever command produced them.
// `kapi exec <check>` emits it on its own; `kapi run <flow>` nests it under the
// run's own output so one run stays one document.
//
// Deliberately without `pass` and `gate`. Neither verb holds the content to a
// bar, and a `"pass": true` next to a critical finding is precisely the
// reassurance this whole class of defect hands out. The verdict belongs to
// `kapi check`, which owns the gate and the exit code.
type findingsReport struct {
	Target   check.Target       `json:"target"`
	Summary  check.Summary      `json:"summary"`
	Findings []check.Diagnostic `json:"findings"`
}

// FormatTable renders the findings the way `kapi check` renders them, minus the
// PASS/FAIL verdict.
func (r findingsReport) FormatTable(w io.Writer) {
	renderFindingsTable(w, r.Findings)
	if r.Summary.Findings > 0 {
		writeFindingsCounts(w, r.Summary)
	}
}

// FormatText is the same rendering under the interface output.Print asks for,
// so a report nested in another command's output renders with it.
func (r findingsReport) FormatText(w io.Writer) error {
	r.FormatTable(w)
	return nil
}
