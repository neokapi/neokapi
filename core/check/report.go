package check

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// ReportSchema is the stable contract id for the check Report shape. Consumers
// (an AI fix-loop, a CI gate) pin this version so the report format can evolve
// without silently breaking them. Bump it only on a breaking shape change.
const ReportSchema = "kapi.check/v1"

// Report is the canonical, machine-consumable result of a `kapi check` run — the
// unit an AI assistant or CI reads, acts on, and re-runs against, the way a test
// runner reports. The CLI and MCP check entry points produce this shape;
// embedded surfaces may retain their own DTOs over the shared findings.
type Report struct {
	// Schema is the stable contract id (ReportSchema). Always set.
	Schema string `json:"schema"`
	// Pass is true exactly when Verdict is VerdictPassed.
	Pass bool `json:"pass"`
	// Verdict is passed, failed or did_not_run. A check that examined no content,
	// or whose analyzers could not show they were able to fail, did not run, and
	// never reports passed. See Decide.
	Verdict Verdict `json:"verdict"`
	// DidNotRun says why the verdict is did_not_run, and is empty otherwise.
	DidNotRun []string `json:"did_not_run,omitempty"`
	// DidNotRunCause is the code a program branches on when the verdict is
	// did_not_run: CauseCheckerInvalid, CauseNothingToCheck or
	// CauseContentNotChecked. The first means a checker is broken; the second
	// means there was nothing to do. Empty otherwise.
	DidNotRunCause string `json:"did_not_run_cause,omitempty"`
	// Target describes what was checked.
	Target Target `json:"target"`
	// Summary is the roll-up (counts + score).
	Summary Summary `json:"summary"`
	// Gate echoes the thresholds and records which ones tripped, so a consumer
	// knows the bar it must clear (e.g. min_score 90), not just pass/fail.
	Gate GateResult `json:"gate"`
	// Findings are the substantive output, sorted severity → rule for stable
	// diffs between loop iterations.
	Findings []Diagnostic `json:"findings"`
	// Warnings name problems in the configuration the check ran under, such as
	// a key a voice profile carries that the profile model does not define.
	// Decide never reads them, so they leave Summary, Gate and Verdict as they
	// would be without them. See Warning.
	Warnings []Warning `json:"warnings,omitempty"`
	// Execution names the analyses that ran and their measured scope. Absent
	// means coverage is unreported, not that all checks completed.
	Execution *Execution `json:"execution,omitempty"`
	// Scope is present when the check was scoped to a diff. It names every file
	// the diff touched and what became of it.
	Scope *Scope `json:"scope,omitempty"`
	// Evaluation says what this run was evaluated against: the project and the
	// state of its context, the build and plugins that ran it, and what each
	// analyzer covered. Decide never reads it, so it reports and gates nothing.
	// Absent means the producer reports no evaluation record.
	Evaluation *Evaluation `json:"evaluation,omitempty"`
}

// Scope records what a diff-scoped check covered.
type Scope struct {
	// Diff says where the diff came from: a file, "-" for standard input, or the
	// git command that produced it.
	Diff  string      `json:"diff"`
	Files []ScopeFile `json:"files"`
}

// ScopeStatus is what a diff-scoped check did with one file the diff names.
type ScopeStatus string

const (
	// ScopeChecked means the blocks the change touched were checked.
	ScopeChecked ScopeStatus = "checked"
	// ScopeUntouched means the change touched no content block: markup only, a
	// rename or a mode change.
	ScopeUntouched ScopeStatus = "untouched"
	// ScopeNoContent means the file holds no content blocks.
	ScopeNoContent ScopeStatus = "no_content"
	// ScopeOutOfScope means the file is not content the check covers: outside
	// the project's declared content, or not among the files named.
	ScopeOutOfScope ScopeStatus = "out_of_scope"
	// ScopeNoReader means no format reads the file.
	ScopeNoReader ScopeStatus = "no_reader"
	// ScopeDeleted means the change removed the file, leaving nothing to check.
	ScopeDeleted ScopeStatus = "deleted"
	// ScopeDidNotRun means the change touched content whose blocks could not be
	// located, so that content was not checked. The reason names it.
	ScopeDidNotRun ScopeStatus = "did_not_run"
)

// ScopeFile is one file a diff names.
type ScopeFile struct {
	Path   string      `json:"path"`
	Status ScopeStatus `json:"status"`
	Reason string      `json:"reason,omitempty"`
	// Blocks are the blocks checked, each whole, with the lines it spans. A
	// did_not_run file lists the touched blocks it could locate and check.
	Blocks []ScopeBlock `json:"blocks,omitempty"`
}

// ScopeBlock is one block a diff-scoped check covered.
type ScopeBlock struct {
	Block string           `json:"block"`
	Lines format.LineRange `json:"lines"`
}

// Target describes the thing a Report was produced for.
type Target struct {
	Kind   string `json:"kind"`             // "file" | "text" | "diff"
	File   string `json:"file,omitempty"`   // path, for kind=="file"
	Format string `json:"format,omitempty"` // detected/declared format
	Blocks int    `json:"blocks"`           // content blocks checked
	// ContextPath selects the project-relative guidance for a text draft. It
	// does not identify an extracted file or assert that the file exists.
	ContextPath string `json:"context_path,omitempty"`
}

// Summary is the count + score roll-up over a Report's findings.
type Summary struct {
	Findings int `json:"findings"`
	Critical int `json:"critical"`
	Major    int `json:"major"`
	Minor    int `json:"minor"`
	Neutral  int `json:"neutral"`
	Score    int `json:"score"` // 0-100 roll-up (length-normalized when word count is known)
}

// Diagnostic is one finding in a Report. It enriches the producer-agnostic
// Finding with a STABLE rule id (the loop's primary key — an AI tracks it across
// iterations to confirm a fix and avoid regressions) and a block-level location
// (so the AI knows exactly which block to revise).
type Diagnostic struct {
	// Rule is the stable id "<check>.<category>" (e.g. "length.max-chars-exceeded",
	// "structure.xml-well-formedness", "voice.vocabulary"). The dedupe/track key.
	Rule string `json:"rule"`
	// Check is the producing check family (length|pattern|chars|structure|
	// hygiene|voice and the target-gated l10n families).
	Check string `json:"check"`
	// Severity drives the gate and the score penalty (MQM weights 25/5/1/0).
	Severity Severity `json:"severity"`
	// Message is the human-readable explanation.
	Message string `json:"message"`
	// Suggestion is an optional remediation hint.
	Suggestion string `json:"suggestion,omitempty"`
	// Location anchors the finding to a block (and anchor/snippet when known).
	Location Location `json:"location"`
	// Point is the governance point the finding's block was checked at. It is
	// set when a project resolved the governance, and omitted otherwise.
	Point *Point `json:"point,omitempty"`
	// Metadata carries checker-specific detail (limit, count, matched rule id).
	Metadata map[string]string `json:"metadata,omitempty"`
	// Advisory marks a diagnostic raised against a rule nobody has confirmed.
	// It is always SeverityNeutral, so it lands in Summary.Neutral, weighs
	// nothing in the score and trips no gate limit. A surface reads it to show
	// the finding as a proposal awaiting a decision.
	Advisory bool `json:"advisory,omitempty"`
}

// Point is a governance point a project resolved for checked blocks: the
// profile and channel it resolved to, both empty at the project's default
// point. Comments marks the point a file's comments sit at when the project
// places them apart from the file's other content.
type Point struct {
	Profile  string `json:"profile,omitempty"`
	Channel  string `json:"channel,omitempty"`
	Comments bool   `json:"comments,omitempty"`
}

// Location anchors a Diagnostic. Block is the primary handle an AI uses to find
// the content to revise; anchor/snippet refine it when the checker populated
// a position.
type Location struct {
	File   string        `json:"file,omitempty"`
	Block  string        `json:"block,omitempty"`
	Anchor *model.Anchor `json:"anchor,omitempty"`
	// Lines are the lines of File the whole block spans, when the block's
	// position in the file is known. They locate the block, not the finding
	// inside it, which Anchor does.
	Lines *format.LineRange `json:"lines,omitempty"`
	// CommentSHA256 is the hex SHA-256 of the bytes of the comment a finding
	// sits on (comment.Fingerprint), set when its block is a comment. A comment
	// edit carries it back, so a comment that changed since is refused.
	CommentSHA256 string `json:"comment_sha256,omitempty"`
	Snippet       string `json:"snippet,omitempty"`
}

// RuleID builds the stable "<check>.<category>" rule id.
func RuleID(checkFamily, category string) string {
	return checkFamily + "." + category
}

// DiagnosticFrom maps a producer-agnostic Finding into a Diagnostic, given the
// check family that produced it and the block location. The run-range and
// snippet are carried through only when the checker populated them.
//
// An empty checkFamily means the caller does not know the family and is reading
// findings the checkers left behind — the finding's own Check stamp answers
// instead. A caller that passes a family keeps it: `kapi check` groups findings
// into families of its own naming, and its rule ids are a stable contract.
func DiagnosticFrom(f Finding, checkFamily string, loc Location) Diagnostic {
	if checkFamily == "" {
		checkFamily = f.Check
	}
	d := Diagnostic{
		Rule:       RuleID(checkFamily, f.Category),
		Check:      checkFamily,
		Severity:   f.Severity,
		Message:    f.Message,
		Suggestion: f.Suggestion,
		Location:   loc,
		Metadata:   f.Metadata,
		Advisory:   f.Advisory,
	}
	if !f.Position.IsZero() {
		rr := f.Position
		d.Location.Anchor = &rr
	}
	if f.OriginalText != "" && d.Location.Snippet == "" {
		d.Location.Snippet = f.OriginalText
	}
	return d
}

// DiagnosticFromReader maps a format reader's validation Diagnostic (RVM) into a
// check Diagnostic, so `kapi check --validate` folds reader-surfaced structure
// and encoding problems into the same kapi.check/v1 Report as content findings.
// The check family is the prefix before the first dot in the category
// ("structure" | "encoding"); the rule is the full category (already in
// "<check>.<category>" form). The reader's line/column/byte offset ride in
// Metadata so the AI/CI can pinpoint the byte without a Report shape change.
func DiagnosticFromReader(fd format.Diagnostic, file string) Diagnostic {
	family := fd.Category
	if i := strings.IndexByte(family, '.'); i >= 0 {
		family = family[:i]
	}
	d := Diagnostic{
		Rule:     fd.Category,
		Check:    family,
		Severity: severityFromFormat(fd.Severity),
		Message:  fd.Message,
		Location: Location{File: file, Snippet: fd.Snippet},
	}
	meta := map[string]string{}
	if fd.Line > 0 {
		meta["line"] = strconv.Itoa(fd.Line)
	}
	if fd.Column > 0 {
		meta["column"] = strconv.Itoa(fd.Column)
	}
	if fd.ByteOffset > 0 {
		meta["byte_offset"] = strconv.Itoa(fd.ByteOffset)
	}
	if len(meta) > 0 {
		d.Metadata = meta
	}
	return d
}

// severityFromFormat maps a format.Severity onto the check Severity scale.
func severityFromFormat(s format.Severity) Severity {
	switch s {
	case format.SeverityCritical:
		return SeverityCritical
	case format.SeverityMajor:
		return SeverityMajor
	case format.SeverityMinor:
		return SeverityMinor
	default:
		return SeverityNeutral
	}
}

// Gate is the set of severity/score thresholds a Report is judged against.
// MaxMajor/MaxMinor of -1 disable that limit; MinScore of 0 disables the score
// gate; MaxCritical defaults to 0 (any critical fails).
type Gate struct {
	MaxCritical int
	MaxMajor    int
	MaxMinor    int
	MinScore    int
}

// DefaultGate is the conservative default: any critical fails, majors/minors
// unlimited, no score floor.
func DefaultGate() Gate { return Gate{MaxCritical: 0, MaxMajor: -1, MaxMinor: -1, MinScore: 0} }

// GateResult echoes the thresholds and lists which ones tripped (empty = pass).
type GateResult struct {
	MaxCritical int      `json:"max_critical"`
	MaxMajor    int      `json:"max_major"`
	MaxMinor    int      `json:"max_minor"`
	MinScore    int      `json:"min_score"`
	Failed      []string `json:"failed"`
}

// Evaluate judges a Summary against the gate, returning the echoed thresholds
// plus the human-readable reasons any limit tripped.
func (g Gate) Evaluate(s Summary) GateResult {
	r := GateResult{
		MaxCritical: g.MaxCritical,
		MaxMajor:    g.MaxMajor,
		MaxMinor:    g.MaxMinor,
		MinScore:    g.MinScore,
		Failed:      []string{},
	}
	if g.MaxCritical >= 0 && s.Critical > g.MaxCritical {
		r.Failed = append(r.Failed, fmt.Sprintf("critical findings %d exceed limit %d", s.Critical, g.MaxCritical))
	}
	if g.MaxMajor >= 0 && s.Major > g.MaxMajor {
		r.Failed = append(r.Failed, fmt.Sprintf("major findings %d exceed limit %d", s.Major, g.MaxMajor))
	}
	if g.MaxMinor >= 0 && s.Minor > g.MaxMinor {
		r.Failed = append(r.Failed, fmt.Sprintf("minor findings %d exceed limit %d", s.Minor, g.MaxMinor))
	}
	if g.MinScore > 0 && s.Score < g.MinScore {
		r.Failed = append(r.Failed, fmt.Sprintf("score %d below minimum %d", s.Score, g.MinScore))
	}
	return r
}

// BuildReport assembles a Report from diagnostics, a target, and a gate. It
// computes the count summary and the length-normalizable score (pass
// WithWordCount to normalize), evaluates the gate, and sorts findings
// deterministically (severity → rule).
func BuildReport(target Target, diags []Diagnostic, gate Gate, scoreOpts ...ScoreOption) Report {
	sum := Summary{Findings: len(diags)}
	scoreFindings := make([]Finding, 0, len(diags))
	for _, d := range diags {
		switch d.Severity {
		case SeverityCritical:
			sum.Critical++
		case SeverityMajor:
			sum.Major++
		case SeverityMinor:
			sum.Minor++
		case SeverityNeutral:
			sum.Neutral++
		}
		// Reuse the score kernel: rule as the category key, severity for weight.
		scoreFindings = append(scoreFindings, Finding{Category: d.Rule, Severity: d.Severity})
	}
	sum.Score = CalculateScore(scoreFindings, scoreOpts...).Overall

	sorted := make([]Diagnostic, len(diags))
	copy(sorted, diags)
	SortDiagnostics(sorted)

	report := Report{
		Schema:   ReportSchema,
		Target:   target,
		Summary:  sum,
		Gate:     gate.Evaluate(sum),
		Findings: sorted,
	}
	report.Decide()
	return report
}

// The causes of a did_not_run verdict. They share an exit code and must never
// be read as one another: a loop that took a broken checker for an empty scope
// would carry on past it.
const (
	// CauseCheckerInvalid means an analyzer reported nothing on its canary. A
	// checker is broken, and no result of this run can be trusted.
	CauseCheckerInvalid = "checker_invalid"
	// CauseNothingToCheck means no content was in scope: no blocks, or a diff
	// that touches none.
	CauseNothingToCheck = "nothing_to_check"
	// CauseContentNotChecked means content in scope was not checked: a changed
	// file whose blocks could not be located, an analyzer that was asked for and
	// had nothing to catch, or one that completed without a canary.
	CauseContentNotChecked = "content_not_checked"
)

// CauseSummary is the sentence a person reads for a did_not_run cause.
func CauseSummary(cause string) string {
	switch cause {
	case CauseCheckerInvalid:
		return "a checker failed its canary, so this run's result cannot be trusted"
	case CauseNothingToCheck:
		return "there was nothing in scope to check"
	case CauseContentNotChecked:
		return "content in scope was not checked"
	}
	return ""
}

// Verdict is the outcome of a check.
type Verdict string

const (
	VerdictPassed    Verdict = "passed"
	VerdictFailed    Verdict = "failed"
	VerdictDidNotRun Verdict = "did_not_run"
)

// Decide sets Verdict, Pass and DidNotRun from the report's target, gate and
// execution. A producer calls it again after changing any of them.
//
// The rules apply in order:
//
//  1. An analyzer that missed its canary makes the run invalid, so the verdict
//     is did_not_run, even when the gate failed: an analyzer that passed a
//     known-bad input cannot be trusted about the rest either.
//  2. A tripped gate is failed.
//  3. A run that checked no blocks did not run.
//  4. A run did not run when a file its diff changed could not be checked,
//     and, with execution reported, when no analyzer completed with its canary
//     caught, when an analyzer completed without a canary, or when a required
//     analyzer had nothing it could catch.
//  5. Anything else passed.
func (r *Report) Decide() {
	var invalid, unproven []string
	proven := false
	if r.Scope != nil {
		for _, f := range r.Scope.Files {
			if f.Status == ScopeDidNotRun {
				unproven = append(unproven, fmt.Sprintf("%s changed but was not checked: %s", f.Path, f.Reason))
			}
		}
	}
	if r.Execution != nil {
		for _, a := range r.Execution.Analyzers {
			switch a.Status {
			case AnalyzerInvalid:
				invalid = append(invalid, fmt.Sprintf("%s reported no finding on its canary%s, so its result cannot be trusted", a.ID, onFile(a.File)))
			case AnalyzerPassed, AnalyzerFindings:
				if a.Canary == nil || a.Canary.Status != CanaryCaught {
					unproven = append(unproven, fmt.Sprintf("%s reported a result%s without a caught canary", a.ID, onFile(a.File)))
					continue
				}
				proven = true
			case AnalyzerDidNotRun:
				if a.Required {
					unproven = append(unproven, fmt.Sprintf("%s did not run%s: %s", a.ID, onFile(a.File), a.Reason))
				}
			}
		}
	}
	switch {
	case len(invalid) > 0:
		r.setVerdict(VerdictDidNotRun, CauseCheckerInvalid, invalid)
	case len(r.Gate.Failed) > 0:
		r.setVerdict(VerdictFailed, "", nil)
	case r.Target.Blocks == 0 && r.Scope != nil && len(unproven) == 0:
		r.setVerdict(VerdictDidNotRun, CauseNothingToCheck, []string{"the diff touches no content block"})
	case r.Target.Blocks == 0 && r.Scope == nil:
		r.setVerdict(VerdictDidNotRun, CauseNothingToCheck, []string{"no content blocks were checked"})
	case len(unproven) > 0:
		r.setVerdict(VerdictDidNotRun, CauseContentNotChecked, unproven)
	case r.Execution != nil && !proven:
		r.setVerdict(VerdictDidNotRun, CauseContentNotChecked, []string{"no analyzer completed a check"})
	default:
		r.setVerdict(VerdictPassed, "", nil)
	}
}

func (r *Report) setVerdict(v Verdict, cause string, reasons []string) {
	r.Verdict = v
	r.Pass = v == VerdictPassed
	r.DidNotRunCause = cause
	r.DidNotRun = reasons
}

func onFile(file string) string {
	if file == "" {
		return ""
	}
	return " on " + file
}

// SortDiagnostics orders diagnostics severity (critical→neutral) then rule, for
// stable output and stable diffs between fix-loop iterations.
func SortDiagnostics(ds []Diagnostic) {
	sort.SliceStable(ds, func(i, j int) bool {
		ri, rj := severityOrder(ds[i].Severity), severityOrder(ds[j].Severity)
		if ri != rj {
			return ri < rj
		}
		if ds[i].Rule != ds[j].Rule {
			return ds[i].Rule < ds[j].Rule
		}
		return ds[i].Location.Block < ds[j].Location.Block
	})
}

func severityOrder(s Severity) int {
	switch s {
	case SeverityCritical:
		return 0
	case SeverityMajor:
		return 1
	case SeverityMinor:
		return 2
	case SeverityNeutral:
		return 3
	default:
		return 4
	}
}
