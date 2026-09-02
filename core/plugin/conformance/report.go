package conformance

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Text renders the report as a plain-text transcript suitable for CI logs.
// One line per check, then a summary and — when there are failures — the
// actionable "why it matters" line for each.
func (r *Report) Text() string {
	var b strings.Builder

	fmt.Fprintf(&b, "kapi plugin protocol v%s conformance\n", r.ProtocolVersion)
	name := r.Plugin
	if name == "" {
		name = "(unidentified)"
	}
	fmt.Fprintf(&b, "  plugin:  %s %s\n", name, r.Version)
	fmt.Fprintf(&b, "  dir:     %s\n", r.Dir)
	fmt.Fprintf(&b, "  modes:   %s\n", modeList(r.Modes))
	fmt.Fprintf(&b, "  started: %s\n\n", r.GeneratedAt.Format(time.RFC3339))

	width := 0
	for _, res := range r.Results {
		if len(res.ID) > width {
			width = len(res.ID)
		}
	}
	for _, res := range r.Results {
		mark := statusMark(res.Status)
		req := ""
		if res.Status == Fail && !res.Required {
			req = " (advisory)"
		}
		fmt.Fprintf(&b, "  %s %-*s  %s%s\n", mark, width, res.ID, res.Detail, req)
	}

	fmt.Fprintf(&b, "\n  %d passed, %d failed, %d advisory, %d skipped in %s\n",
		len(r.Passed()), len(r.Failures()), len(r.Advisories()), len(r.Skipped()),
		r.Elapsed.Round(time.Millisecond))

	if fails := r.Failures(); len(fails) > 0 {
		b.WriteString("\n  NOT CONFORMANT: required checks failed:\n")
		for _, f := range fails {
			fmt.Fprintf(&b, "    %s: %s\n      %s\n", f.ID, f.Title, f.Why)
		}
	} else {
		fmt.Fprintf(&b, "\n  CONFORMANT with plugin protocol v%s\n", r.ProtocolVersion)
	}
	if adv := r.Advisories(); len(adv) > 0 {
		b.WriteString("\n  Advisory (does not affect conformance):\n")
		for _, a := range adv {
			fmt.Fprintf(&b, "    %s: %s\n      %s\n", a.ID, a.Detail, a.Why)
		}
	}
	return b.String()
}

func statusMark(s Status) string {
	switch s {
	case Pass:
		return "PASS"
	case Fail:
		return "FAIL"
	case Skip:
		return "SKIP"
	default:
		return "????"
	}
}

// jsonResult is the wire shape of a Result. Result itself embeds Check and
// carries an error, neither of which marshals usefully, so the JSON encoding is
// declared explicitly — it is a published artifact that external CI parses.
type jsonResult struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Mode     Mode   `json:"mode,omitempty"`
	Required bool   `json:"required"`
	Why      string `json:"why"`
	Status   Status `json:"status"`
	Detail   string `json:"detail"`
	Error    string `json:"error,omitempty"`
	ElapsedM int64  `json:"elapsed_ms"`
}

// jsonReport is the wire shape of a Report.
type jsonReport struct {
	ProtocolVersion string       `json:"protocol_version"`
	Plugin          string       `json:"plugin"`
	Version         string       `json:"version"`
	Dir             string       `json:"dir"`
	Modes           []Mode       `json:"modes"`
	Conformant      bool         `json:"conformant"`
	Summary         jsonSummary  `json:"summary"`
	Results         []jsonResult `json:"results"`
	GeneratedAt     string       `json:"generated_at"`
	ElapsedMS       int64        `json:"elapsed_ms"`
}

type jsonSummary struct {
	Passed    int `json:"passed"`
	Failed    int `json:"failed"`
	Advisory  int `json:"advisory"`
	Skipped   int `json:"skipped"`
	TotalRuns int `json:"total"`
}

// MarshalJSON renders the report as a stable machine-readable artifact. The
// shape is part of this package's contract: an external plugin repository can
// publish it from CI and diff it across runs.
func (r *Report) MarshalJSON() ([]byte, error) {
	out := jsonReport{
		ProtocolVersion: r.ProtocolVersion,
		Plugin:          r.Plugin,
		Version:         r.Version,
		Dir:             r.Dir,
		Modes:           r.Modes,
		Conformant:      r.OK(),
		Summary: jsonSummary{
			Passed:    len(r.Passed()),
			Failed:    len(r.Failures()),
			Advisory:  len(r.Advisories()),
			Skipped:   len(r.Skipped()),
			TotalRuns: len(r.Results),
		},
		Results:     make([]jsonResult, 0, len(r.Results)),
		GeneratedAt: r.GeneratedAt.UTC().Format(time.RFC3339),
		ElapsedMS:   r.Elapsed.Milliseconds(),
	}
	if out.Modes == nil {
		out.Modes = []Mode{}
	}
	for _, res := range r.Results {
		jr := jsonResult{
			ID:       res.ID,
			Title:    res.Title,
			Mode:     res.Mode,
			Required: res.Required,
			Why:      res.Why,
			Status:   res.Status,
			Detail:   res.Detail,
			ElapsedM: res.Elapsed.Milliseconds(),
		}
		if res.Err != nil {
			jr.Error = res.Err.Error()
		}
		out.Results = append(out.Results, jr)
	}
	return json.Marshal(out)
}

// JSON returns the report as indented JSON.
func (r *Report) JSON() ([]byte, error) { return json.MarshalIndent(r, "", "  ") }

// TestingT is the subset of *testing.T that [RunT] needs. Declaring it as an
// interface keeps the testing package out of this package's imports, so a
// plugin repository can also call the suite from a plain binary.
type TestingT interface {
	Helper()
	Errorf(format string, args ...any)
	Logf(format string, args ...any)
}

// RunT runs the suite and reports one test failure per failing required check,
// logging the full transcript either way. It returns the report so a caller can
// make further assertions or publish the JSON.
//
//	func TestProtocolConformance(t *testing.T) {
//		conformance.RunT(t, conformance.Suite{Dir: pluginDir})
//	}
func RunT(t TestingT, s Suite) *Report {
	t.Helper()
	return RunTContext(t, context.Background(), s)
}

// RunTContext is [RunT] with a caller-supplied context, for tests that need to
// bound the whole run rather than each check.
func RunTContext(t TestingT, ctx context.Context, s Suite) *Report { //nolint:revive // ctx follows t by testing convention
	t.Helper()
	rep, err := Run(ctx, s)
	if err != nil {
		t.Errorf("plugin conformance suite could not run: %v", err)
		return nil
	}
	t.Logf("\n%s", rep.Text())
	for _, f := range rep.Failures() {
		t.Errorf("plugin protocol v%s: %s failed: %s\n  %s: %s",
			rep.ProtocolVersion, f.ID, f.Detail, f.Title, f.Why)
	}
	return rep
}
