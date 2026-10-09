package host

import (
	"context"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/project"
)

// checkExecution belongs to one synchronous check operation, never to App: MCP
// requests must not share mutable execution history.
type checkExecution struct {
	started time.Time
	check.Execution
	// warnings collects the configuration warnings of the voice profiles the
	// operation loads, and report hands them to the Report.
	warnings voiceWarnings
}

// pluginLedger records the plugins that served one operation: each plugin and
// what it did, such as `format:pdf`, with the version its manifest declares.
// The evaluation record reads it (host/check_evaluation.go).
//
// The ledger travels in the operation's context rather than on an execution.
// A gate run holds one execution per gate and reads files through paths that
// hold none, and every one of those reads can reach a plugin. The read sites
// record into whatever ledger their context carries, so a surface that
// attaches one at its start lists every plugin its run reached, and a context
// carrying none records nothing. One ledger belongs to one operation, never to
// App: MCP requests must not share it.
type pluginLedger struct {
	serves   map[string]map[string]bool
	versions map[string]string
}

func newPluginLedger() *pluginLedger {
	return &pluginLedger{serves: map[string]map[string]bool{}, versions: map[string]string{}}
}

// served records that a plugin read a format, located a language's comments or
// ran an analysis. A nil ledger records nothing.
func (l *pluginLedger) served(plugin, pluginVersion, capability string) {
	if l == nil || plugin == "" {
		return
	}
	if l.serves[plugin] == nil {
		l.serves[plugin] = map[string]bool{}
	}
	l.serves[plugin][capability] = true
	if pluginVersion != "" {
		l.versions[plugin] = pluginVersion
	}
}

type pluginLedgerKey struct{}

// withPluginLedger returns ctx carrying ledger, so every read made under it
// records the plugin it reached.
func withPluginLedger(ctx context.Context, ledger *pluginLedger) context.Context {
	return context.WithValue(ctx, pluginLedgerKey{}, ledger)
}

// observePlugins returns ctx carrying a fresh ledger: the start of one check
// operation, whose record names the plugins reached from here on.
func observePlugins(ctx context.Context) context.Context {
	return withPluginLedger(ctx, newPluginLedger())
}

// pluginLedgerFrom returns the ledger ctx carries, or nil, which records
// nothing.
func pluginLedgerFrom(ctx context.Context) *pluginLedger {
	if ctx == nil {
		return nil
	}
	l, _ := ctx.Value(pluginLedgerKey{}).(*pluginLedger)
	return l
}

// termMatching records how a terminology check matched terms for its target
// language. A nil execution records nothing.
func (e *checkExecution) termMatching(m check.TermMatching) {
	if e == nil {
		return
	}
	e.TermMatching = check.MergeTermMatching(e.TermMatching, m)
}

// warningSink is where a resolver working for this operation sends the
// warnings of the profiles it loads. A nil execution collects none.
func (e *checkExecution) warningSink() *voiceWarnings {
	if e == nil {
		return nil
	}
	return &e.warnings
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

// notApplicable records an analyzer the configuration gives no rule for the
// file, because other analyzers check the rules in force.
func (e *checkExecution) notApplicable(id, file, reason string) {
	if e == nil {
		return
	}
	e.Analyzers = append(e.Analyzers, check.AnalyzerExecution{
		ID: id, File: DisplayName(file), Status: check.AnalyzerNotApplicable, Reason: reason,
	})
}

// notChecked records an analyzer the gate required that could not evaluate a
// file's content, so the gate's result is not verified.
func (e *checkExecution) notChecked(id, file, reason string) {
	if e == nil {
		return
	}
	e.Analyzers = append(e.Analyzers, check.AnalyzerExecution{
		ID: id, File: DisplayName(file), Status: check.AnalyzerDidNotRun, Required: true, Reason: reason,
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

// probed gives an analyzer its canaries through probe and records it as
// completed. A nil execution records nothing, so it probes nothing: the
// canaries prove an analyzer to whoever reads the record, and a check that
// keeps none (the commit check) holds content to the analyzers `kapi check`
// proves.
func (e *checkExecution) probed(id, file string, findings int, start time.Time, required bool, probe func() (check.CanaryOutcome, error)) error {
	if e == nil {
		return nil
	}
	canary, err := probe()
	if err != nil {
		return err
	}
	e.completed(id, file, findings, start, canary, required)
	return nil
}

// report assembles the Report one operation produces. Every check surface goes
// through it, so the evaluation record is attached here rather than at each
// caller: a surface added later carries it without being told to.
func (e *checkExecution) report(ctx context.Context, a *App, cmd Command, target check.Target, diags []check.Diagnostic) check.Report {
	// Before the report's own clock: naming the context files a checkout holds
	// opens the project store, which is not time spent assembling a report.
	unread := a.contextUnreadWarning(ctx, cmd)

	start := time.Now()
	report := check.BuildReport(target, diags)
	if e != nil {
		report.Warnings = check.MergeWarnings(e.warnings.merged(), unread)
		e.Timings.ReportMS = elapsedMS(start)
		e.Timings.TotalMS = elapsedMS(e.started)
		report.Execution = &e.Execution
		report.Decide()
	} else {
		report.Warnings = unread
	}
	report.Evaluation = a.checkEvaluation(ctx, cmd, e)
	return report
}

// contextUnreadWarning notes a checkout carrying context files whose project
// store has never held context. The check ran against what the store holds,
// which is none of what those files say.
//
// It is a warning rather than a finding: nothing about the content is wrong,
// and the score, the gate and the verdict are what they would be without it.
func (a *App) contextUnreadWarning(ctx context.Context, cmd Command) []check.Warning {
	projectPath, err := ResolveProjectPath(cmd)
	if err != nil || projectPath == "" {
		return nil
	}
	notice, unread := a.ContextFilesUnread(ctx, projectPath)
	if !unread {
		return nil
	}
	return []check.Warning{{
		Code:    check.WarningContextUnread,
		Source:  project.StateDirName,
		Message: notice.Message(),
	}}
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
		Point: clonePoint(opts.point),
	})
}
