package check

import (
	"fmt"
	"sort"

	"github.com/neokapi/neokapi/core/model"
)

// ReportSchema is the stable contract id for the check Report shape. Consumers
// (an AI fix-loop, a CI gate) pin this version so the report format can evolve
// without silently breaking them. Bump it only on a breaking shape change.
const ReportSchema = "kapi.check/v1"

// Report is the canonical, machine-consumable result of a `kapi check` run — the
// unit an AI assistant or CI reads, acts on, and re-runs against, the way a test
// runner reports. It is platform-agnostic (no brand/target/locale types leak in)
// so the CLI, the MCP tools, the desktop app, and bowrain all read one shape.
type Report struct {
	// Schema is the stable contract id (ReportSchema). Always set.
	Schema string `json:"schema"`
	// Pass is true when the gate did not trip (Gate.Failed is empty).
	Pass bool `json:"pass"`
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
}

// Target describes the thing a Report was produced for.
type Target struct {
	Kind   string `json:"kind"`             // "file" | "text"
	File   string `json:"file,omitempty"`   // path, for kind=="file"
	Format string `json:"format,omitempty"` // detected/declared format
	Blocks int    `json:"blocks"`           // content blocks checked
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
	// "structure.xml-well-formedness", "brand.vocabulary"). The dedupe/track key.
	Rule string `json:"rule"`
	// Check is the producing check family (length|pattern|chars|structure|
	// hygiene|brand|voice and the target-gated l10n families).
	Check string `json:"check"`
	// Severity drives the gate and the score penalty (MQM weights 25/5/1/0).
	Severity Severity `json:"severity"`
	// Message is the human-readable explanation.
	Message string `json:"message"`
	// Suggestion is an optional remediation hint.
	Suggestion string `json:"suggestion,omitempty"`
	// Location anchors the finding to a block (and run-range/snippet when known).
	Location Location `json:"location"`
	// Metadata carries checker-specific detail (limit, count, matched rule id).
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Location anchors a Diagnostic. Block is the primary handle an AI uses to find
// the content to revise; run_range/snippet refine it when the checker populated
// a position.
type Location struct {
	File     string          `json:"file,omitempty"`
	Block    string          `json:"block,omitempty"`
	RunRange *model.RunRange `json:"run_range,omitempty"`
	Snippet  string          `json:"snippet,omitempty"`
}

// RuleID builds the stable "<check>.<category>" rule id.
func RuleID(checkFamily, category string) string {
	return checkFamily + "." + category
}

// DiagnosticFrom maps a producer-agnostic Finding into a Diagnostic, given the
// check family that produced it and the block location. The run-range and
// snippet are carried through only when the checker populated them.
func DiagnosticFrom(f Finding, checkFamily string, loc Location) Diagnostic {
	d := Diagnostic{
		Rule:       RuleID(checkFamily, f.Category),
		Check:      checkFamily,
		Severity:   f.Severity,
		Message:    f.Message,
		Suggestion: f.Suggestion,
		Location:   loc,
		Metadata:   f.Metadata,
	}
	if !f.Position.IsZero() {
		rr := f.Position
		d.Location.RunRange = &rr
	}
	if f.OriginalText != "" && d.Location.Snippet == "" {
		d.Location.Snippet = f.OriginalText
	}
	return d
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
	if s.Critical > g.MaxCritical {
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

	gr := gate.Evaluate(sum)
	return Report{
		Schema:   ReportSchema,
		Pass:     len(gr.Failed) == 0,
		Target:   target,
		Summary:  sum,
		Gate:     gr,
		Findings: sorted,
	}
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
