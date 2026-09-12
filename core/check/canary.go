package check

import (
	"fmt"

	"github.com/neokapi/neokapi/core/model"
)

// A check that reports no findings has either examined content that is clean,
// or examined nothing, or run a checker that finds nothing whatever it is given.
// The report cannot tell the three apart from the findings alone. A canary is how
// an analyzer shows it is the first: beside the real content, it is given input
// that it must flag, through the same configured checker, and a run whose
// analyzer passes its canary is invalid whatever else it reports.

// CanaryStatus is what an analyzer did with the known-bad input it was given.
type CanaryStatus string

const (
	// CanaryCaught means the analyzer flagged every canary it was given.
	CanaryCaught CanaryStatus = "caught"
	// CanaryMissed means the analyzer reported no finding on a canary. Its result
	// on the real content cannot be trusted.
	CanaryMissed CanaryStatus = "missed"
	// CanaryImpossible means the analyzer's configuration gives it nothing to
	// catch, so no canary could be built and the analyzer checked nothing.
	CanaryImpossible CanaryStatus = "impossible"
)

// CanaryOutcome records an analyzer's canaries for one input.
type CanaryOutcome struct {
	Status CanaryStatus `json:"status"`
	// Probes is how many canaries the analyzer was given.
	Probes int `json:"probes"`
	// Missed names the canary the analyzer reported nothing on, and Input is
	// that canary's text.
	Missed string `json:"missed,omitempty"`
	Input  string `json:"input,omitempty"`
	// Reason says why no canary could be built, or, beside a caught outcome,
	// which configured rules had none.
	Reason string `json:"reason,omitempty"`
}

// Canary is one known-bad input for a configured analyzer.
type Canary struct {
	// Name says what the canary exercises, such as "doubled word".
	Name string
	// Block is the input the analyzer must flag.
	Block *model.Block
	// Expect, when set, is the finding category the canary must produce. Empty
	// accepts any finding.
	Expect string
}

// CanaryBlock builds a translatable source block holding text.
func CanaryBlock(text string) *model.Block {
	return &model.Block{ID: "canary", Translatable: true, Source: []model.Run{{Text: &model.TextRun{Text: text}}}}
}

// Probe gives each canary to run, which must evaluate the block with the same
// configured checker the real content went through and return the findings it
// produced, and reports what the analyzer made of them.
//
// Every canary must produce a finding (of its Expect category, when set). With
// no canary to give, the outcome is CanaryImpossible and uncheckable says why;
// beside canaries, uncheckable names configured rules that had none, and is kept
// as the outcome's reason. An error from run is returned: a checker that could
// not evaluate its canary is an operational failure, not a missed canary.
func Probe(canaries []Canary, uncheckable string, run func(*model.Block) ([]Finding, error)) (CanaryOutcome, error) {
	if len(canaries) == 0 {
		if uncheckable == "" {
			uncheckable = "the analyzer has no canary for its configuration"
		}
		return CanaryOutcome{Status: CanaryImpossible, Reason: uncheckable}, nil
	}
	for _, c := range canaries {
		findings, err := run(c.Block)
		if err != nil {
			return CanaryOutcome{}, fmt.Errorf("canary %q: %w", c.Name, err)
		}
		if !flags(findings, c.Expect) {
			return CanaryOutcome{
				Status: CanaryMissed,
				Probes: len(canaries),
				Missed: c.Name,
				Input:  model.RunsText(c.Block.SourceRuns()),
			}, nil
		}
	}
	return CanaryOutcome{Status: CanaryCaught, Probes: len(canaries), Reason: uncheckable}, nil
}

func flags(findings []Finding, category string) bool {
	for _, f := range findings {
		if category == "" || f.Category == category {
			return true
		}
	}
	return false
}

// Merge combines the outcomes of two sets of canaries given to one analyzer. A
// miss in either is a miss. Otherwise a catch in either is a catch, and the
// analyzer is impossible only when both are.
func (o CanaryOutcome) Merge(other CanaryOutcome) CanaryOutcome {
	switch {
	case o.Status == CanaryMissed:
		return o
	case other.Status == CanaryMissed:
		return other
	case o.Status == CanaryImpossible && other.Status == CanaryImpossible:
		return CanaryOutcome{Status: CanaryImpossible, Reason: joinReasons(o.Reason, other.Reason)}
	}
	merged := CanaryOutcome{Status: CanaryCaught, Probes: o.Probes + other.Probes}
	for _, x := range []CanaryOutcome{o, other} {
		if x.Status == CanaryCaught {
			merged.Reason = joinReasons(merged.Reason, x.Reason)
		}
	}
	return merged
}

func joinReasons(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + "; " + b
}

// StatusFor is the execution status of an analyzer that reported findings on
// the real content and this outcome on its canaries.
func (o CanaryOutcome) StatusFor(findings int) AnalyzerStatus {
	switch o.Status {
	case CanaryMissed:
		return AnalyzerInvalid
	case CanaryImpossible:
		return AnalyzerDidNotRun
	}
	if findings > 0 {
		return AnalyzerFindings
	}
	return AnalyzerPassed
}
