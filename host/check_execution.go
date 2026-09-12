package host

import (
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/check"
)

// checkExecution belongs to one synchronous check operation, never to App: MCP
// requests must not share mutable execution history.
type checkExecution struct {
	started time.Time
	check.Execution
}

func newCheckExecution() *checkExecution {
	execution := &checkExecution{started: time.Now()}
	execution.Analyzers = []check.AnalyzerExecution{}
	return execution
}

func elapsedMS(start time.Time) float64 {
	return float64(time.Since(start)) / float64(time.Millisecond)
}

func (e *checkExecution) skipped(id, file, reason string) {
	if e == nil {
		return
	}
	e.Analyzers = append(e.Analyzers, check.AnalyzerExecution{
		ID: id, File: DisplayName(file), Status: check.AnalyzerNotRequested, Reason: reason,
	})
}

func (e *checkExecution) unsupported(id, file, reason string) {
	if e == nil {
		return
	}
	e.Analyzers = append(e.Analyzers, check.AnalyzerExecution{
		ID: id, File: DisplayName(file), Status: check.AnalyzerUnsupported, Reason: reason,
	})
}

// completed records an analyzer that evaluated the content and its canaries.
// required says the invocation asked for the analyzer, so that one with nothing
// to catch leaves the run unverified rather than only itself.
func (e *checkExecution) completed(id, file string, findings int, start time.Time, canary check.CanaryOutcome, required bool) {
	if e == nil {
		return
	}
	duration := elapsedMS(start)
	run := check.AnalyzerExecution{
		ID: id, File: DisplayName(file), Status: canary.StatusFor(findings), Required: required,
		Findings: findings, DurationMS: &duration, Canary: &canary,
	}
	switch run.Status {
	case check.AnalyzerInvalid:
		run.Reason = fmt.Sprintf("Reported no finding on its canary, %s (%q).", canary.Missed, canary.Input)
	case check.AnalyzerDidNotRun:
		run.Reason = canary.Reason
	}
	e.Analyzers = append(e.Analyzers, run)
	if id != "reader.validation" {
		e.Timings.AnalyzersMS += duration
	}
}

func (e *checkExecution) report(target check.Target, diags []check.Diagnostic, gate check.Gate) check.Report {
	start := time.Now()
	report := check.BuildReport(target, diags, gate)
	if e != nil {
		e.Timings.ReportMS = elapsedMS(start)
		e.Timings.TotalMS = elapsedMS(e.started)
		report.Execution = &e.Execution
		report.Decide()
	}
	return report
}

func (e *checkExecution) recordContext(file, destination string, opts checkRunOptions) {
	if e == nil {
		return
	}
	if file != "" {
		file = DisplayName(file)
	}
	e.Contexts = append(e.Contexts, check.CheckContext{
		File: file, ContextPath: destination, Voice: opts.voiceContext, TermsApplied: opts.terms != nil,
	})
}
