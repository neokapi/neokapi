package check

import (
	"context"
	"errors"
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

// runTool evaluates a block through a check tool the way a producer does, and
// returns the findings the tool recorded on it.
func runTool(t tool.Tool) func(*model.Block) ([]Finding, error) {
	return func(b *model.Block) ([]Finding, error) {
		in := make(chan *model.Part, 1)
		out := make(chan *model.Part, 1)
		in <- &model.Part{Type: model.PartBlock, Resource: b}
		close(in)
		errc := make(chan error, 1)
		go func() {
			defer close(out)
			errc <- t.Process(context.Background(), in, out)
		}()
		for range out { //nolint:revive // drain
		}
		if err := <-errc; err != nil {
			return nil, err
		}
		ann, ok := model.AnnoAs[*FindingsAnnotation](b, AnnotationKey)
		if !ok {
			return nil, nil
		}
		return ann.Findings, nil
	}
}

// inert returns no finding for any input: the checker the canary exists to
// expose.
func inert(*model.Block) ([]Finding, error) { return nil, nil }

func TestProbe(t *testing.T) {
	canaries := HygieneCanaries()

	caught, err := Probe(canaries, "", runTool(NewContentLintTool()))
	require.NoError(t, err)
	assert.Equal(t, CanaryOutcome{Status: CanaryCaught, Probes: 1}, caught)

	missed, err := Probe(canaries, "", inert)
	require.NoError(t, err)
	assert.Equal(t, CanaryMissed, missed.Status)
	assert.Equal(t, "doubled word", missed.Missed)
	assert.Equal(t, "A canary with a doubled doubled word.", missed.Input)

	impossible, err := Probe(nil, "the profile declares no rules", inert)
	require.NoError(t, err)
	assert.Equal(t, CanaryOutcome{Status: CanaryImpossible, Reason: "the profile declares no rules"}, impossible)

	_, err = Probe(canaries, "", func(*model.Block) ([]Finding, error) { return nil, errors.New("plugin gone") })
	require.ErrorContains(t, err, "plugin gone")
}

func TestProbe_ExpectNamesTheCategory(t *testing.T) {
	canary := []Canary{{Name: "x", Block: CanaryBlock("x"), Expect: "doubled-word"}}
	other := func(*model.Block) ([]Finding, error) {
		return []Finding{{Category: "double-spaces"}}, nil
	}
	outcome, err := Probe(canary, "", other)
	require.NoError(t, err)
	assert.Equal(t, CanaryMissed, outcome.Status, "a finding of another category does not catch the canary")
}

func TestCanaryOutcome_Merge(t *testing.T) {
	caught := CanaryOutcome{Status: CanaryCaught, Probes: 2}
	missed := CanaryOutcome{Status: CanaryMissed, Probes: 1, Missed: "store term"}
	impossible := CanaryOutcome{Status: CanaryImpossible, Reason: "no required patterns"}

	assert.Equal(t, missed, caught.Merge(missed))
	assert.Equal(t, missed, missed.Merge(caught))
	assert.Equal(t, CanaryOutcome{Status: CanaryCaught, Probes: 2}, caught.Merge(impossible))
	assert.Equal(t, CanaryOutcome{Status: CanaryCaught, Probes: 2}, impossible.Merge(caught))
	assert.Equal(t, CanaryImpossible, impossible.Merge(impossible).Status)
}

func TestCanaryOutcome_StatusFor(t *testing.T) {
	assert.Equal(t, AnalyzerPassed, CanaryOutcome{Status: CanaryCaught}.StatusFor(0))
	assert.Equal(t, AnalyzerFindings, CanaryOutcome{Status: CanaryCaught}.StatusFor(3))
	assert.Equal(t, AnalyzerInvalid, CanaryOutcome{Status: CanaryMissed}.StatusFor(3), "findings do not redeem a missed canary")
	assert.Equal(t, AnalyzerDidNotRun, CanaryOutcome{Status: CanaryImpossible}.StatusFor(0))
}

// TestSourceCanaries_AreCaughtByTheirCheckers gives each source checker its own
// canaries, and an inert checker the same canaries, which it must miss.
func TestSourceCanaries_AreCaughtByTheirCheckers(t *testing.T) {
	length, err := NewSourceLengthTool(10, 3)
	require.NoError(t, err)
	rules := []PatternRule{
		{Name: "forbidden-1", Pattern: `(?i)\btodo\b`, MustNotMatch: true},
		{Name: "required-1", Pattern: `©`, MustMatch: true},
	}
	pattern, err := NewSourcePatternTool(rules)
	require.NoError(t, err)
	patternCanaries, uncheckable := PatternCanaries(rules)
	require.Empty(t, uncheckable)

	tests := []struct {
		name     string
		canaries []Canary
		checker  tool.Tool
		probes   int
	}{
		{"hygiene", HygieneCanaries(), NewContentLintTool(), 1},
		{"length", LengthCanaries(10, 3), length, 2},
		{"pattern", patternCanaries, pattern, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outcome, err := Probe(tt.canaries, "", runTool(tt.checker))
			require.NoError(t, err)
			assert.Equal(t, CanaryCaught, outcome.Status)
			assert.Equal(t, tt.probes, outcome.Probes)

			outcome, err = Probe(tt.canaries, "", inert)
			require.NoError(t, err)
			assert.Equal(t, CanaryMissed, outcome.Status)
		})
	}
}

func TestPatternCanaries_Uncheckable(t *testing.T) {
	tests := []struct {
		name string
		rule PatternRule
	}{
		{"a required pattern every text satisfies", PatternRule{Name: "r", Pattern: `.*`, MustMatch: true}},
		{"a forbidden pattern that matches only the empty string", PatternRule{Name: "f", Pattern: `^$`, MustNotMatch: true}},
		{"a forbidden pattern that matches nothing", PatternRule{Name: "f", Pattern: `a\bb`, MustNotMatch: true}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canaries, uncheckable := PatternCanaries([]PatternRule{{Name: "ok", Pattern: "TODO", MustNotMatch: true}, tt.rule})
			assert.Nil(t, canaries)
			assert.NotEmpty(t, uncheckable)
		})
	}
}

func TestTextMatching(t *testing.T) {
	patterns := []string{
		`TODO`, `(?i)\bfixme\b`, `\Bing`, `^Note:`, `colou?r`, `[0-9]{3}-[0-9]{4}`,
		`(foo|bar)baz`, `a{900}`, `\p{Greek}+`, `[^a-z]+`, `(?m)^\s*$\n?x`, `.+`,
		`\$\{[A-Z_]+\}`, `(?:very|really) (?:unique|perfect)`,
	}
	for _, p := range patterns {
		t.Run(p, func(t *testing.T) {
			re := regexp.MustCompile(p)
			text, ok := TextMatching(re)
			require.True(t, ok)
			loc := re.FindStringIndex(text)
			require.NotNil(t, loc)
			assert.Greater(t, loc[1], loc[0], "the match must be non-empty")
			assert.LessOrEqual(t, len(text), 4096)
		})
	}
	for _, p := range []string{`^$`, `\b`, `a\bb`, `x{0}`} {
		t.Run("uncheckable "+p, func(t *testing.T) {
			_, ok := TextMatching(regexp.MustCompile(p))
			assert.False(t, ok)
		})
	}
}

func TestTextNotMatching(t *testing.T) {
	text, ok := TextNotMatching(regexp.MustCompile(`©`))
	require.True(t, ok)
	assert.NotContains(t, text, "©")
	_, ok = TextNotMatching(regexp.MustCompile(`.*`))
	assert.False(t, ok)
}

func caughtRun(id string, status AnalyzerStatus) AnalyzerExecution {
	return AnalyzerExecution{ID: id, Status: status, Required: true, Canary: &CanaryOutcome{Status: CanaryCaught, Probes: 1}}
}

// TestDecide pairs each rule with the neighbour that must not trigger it.
func TestDecide(t *testing.T) {
	failedGate := GateResult{Failed: []string{"critical findings 1 exceed limit 0"}}
	tests := []struct {
		name      string
		report    Report
		want      Verdict
		didNotRun string
	}{
		{
			name:   "content checked by proven analyzers passes",
			report: Report{Target: Target{Blocks: 3}, Execution: &Execution{Analyzers: []AnalyzerExecution{caughtRun("hygiene", AnalyzerPassed)}}},
			want:   VerdictPassed,
		},
		{
			name:      "zero blocks did not run",
			report:    Report{Target: Target{Blocks: 0}, Execution: &Execution{Analyzers: []AnalyzerExecution{caughtRun("hygiene", AnalyzerPassed)}}},
			want:      VerdictDidNotRun,
			didNotRun: "no content blocks were checked",
		},
		{
			name:   "zero blocks without execution did not run",
			report: Report{Target: Target{Blocks: 0}},
			want:   VerdictDidNotRun,
		},
		{
			name:   "one block without execution passes",
			report: Report{Target: Target{Blocks: 1}},
			want:   VerdictPassed,
		},
		{
			name:   "a tripped gate fails",
			report: Report{Target: Target{Blocks: 3}, Gate: failedGate, Execution: &Execution{Analyzers: []AnalyzerExecution{caughtRun("hygiene", AnalyzerFindings)}}},
			want:   VerdictFailed,
		},
		{
			name: "a missed canary outranks a tripped gate",
			report: Report{Target: Target{Blocks: 3}, Gate: failedGate, Execution: &Execution{Analyzers: []AnalyzerExecution{
				caughtRun("hygiene", AnalyzerFindings),
				{ID: "voice.rules", Status: AnalyzerInvalid, File: "a.md", Canary: &CanaryOutcome{Status: CanaryMissed}},
			}}},
			want:      VerdictDidNotRun,
			didNotRun: "voice.rules reported no finding on its canary on a.md",
		},
		{
			name: "an analyzer without a canary is not proven",
			report: Report{Target: Target{Blocks: 3}, Execution: &Execution{Analyzers: []AnalyzerExecution{
				caughtRun("hygiene", AnalyzerPassed),
				{ID: "length", Status: AnalyzerPassed, Required: true},
			}}},
			want:      VerdictDidNotRun,
			didNotRun: "length reported a result without a caught canary",
		},
		{
			name: "a required analyzer with nothing to catch did not run",
			report: Report{Target: Target{Blocks: 3}, Execution: &Execution{Analyzers: []AnalyzerExecution{
				caughtRun("hygiene", AnalyzerPassed),
				{ID: "voice.rules", Status: AnalyzerDidNotRun, Required: true, Reason: "the profile declares no rules"},
			}}},
			want:      VerdictDidNotRun,
			didNotRun: "voice.rules did not run: the profile declares no rules",
		},
		{
			name: "an optional analyzer with nothing to catch does not stop a pass",
			report: Report{Target: Target{Blocks: 3}, Execution: &Execution{Analyzers: []AnalyzerExecution{
				caughtRun("hygiene", AnalyzerPassed),
				{ID: "voice.rules", Status: AnalyzerDidNotRun, Reason: "the profile declares no rules"},
			}}},
			want: VerdictPassed,
		},
		{
			name: "no proven analyzer did not run",
			report: Report{Target: Target{Blocks: 3}, Execution: &Execution{Analyzers: []AnalyzerExecution{
				{ID: "voice.similarity", Status: AnalyzerNotRequested},
			}}},
			want:      VerdictDidNotRun,
			didNotRun: "no analyzer completed a check",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.report
			r.Decide()
			assert.Equal(t, tt.want, r.Verdict)
			assert.Equal(t, tt.want == VerdictPassed, r.Pass)
			if tt.want == VerdictDidNotRun {
				require.NotEmpty(t, r.DidNotRun)
			} else {
				assert.Empty(t, r.DidNotRun)
			}
			if tt.didNotRun != "" {
				assert.Contains(t, r.DidNotRun[0], tt.didNotRun)
			}
		})
	}
}

func TestBuildReport_ZeroBlocksNeverPasses(t *testing.T) {
	r := BuildReport(Target{Kind: "file", Blocks: 0}, nil, DefaultGate())
	assert.False(t, r.Pass)
	assert.Equal(t, VerdictDidNotRun, r.Verdict)

	r = BuildReport(Target{Kind: "file", Blocks: 1}, nil, DefaultGate())
	assert.True(t, r.Pass)
	assert.Equal(t, VerdictPassed, r.Verdict)
}
