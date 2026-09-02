package host

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A stale unit is work, and every surface that answers "what will this run do"
// or "what did it do" has to say so. Reporting the drift while quoting nothing
// for it described a run that would go on to spend tokens the reader was never
// shown, and a summary that named the drift without naming the re-draft left the
// loop looking like it had declined to act on what it had just refused to ship.

func TestLocalesNeedingPass_StaleScopeIsWork(t *testing.T) {
	locales := []model.LocaleID{"de", "nb"}
	tests := []struct {
		name string
		cov  []LocaleCoverage
		want []model.LocaleID
	}{
		{
			name: "an ungated scope holding a stale unit still needs a pass",
			// The trap this pins: staleness tallies at `draft`, which is the top
			// of the ungated rung test, so the scope read as complete and the
			// loop skipped the very drift it was reporting.
			cov: []LocaleCoverage{
				{Locale: "de", Stale: 1, Shippable: false, Pct: map[string]int{"draft": 100}},
				{Locale: "nb", Shippable: true, Pct: map[string]int{"draft": 100}},
			},
			want: []model.LocaleID{"de"},
		},
		{
			name: "a gated scope holding a stale unit needs one too",
			cov: []LocaleCoverage{
				{Locale: "de", Gated: true, Stale: 1, Shippable: false, Pct: map[string]int{"draft": 100}},
				{Locale: "nb", Gated: true, Shippable: true, Pct: map[string]int{"draft": 100}},
			},
			want: []model.LocaleID{"de"},
		},
		{
			name: "nothing stale and nothing short of its gate is nothing to do",
			cov: []LocaleCoverage{
				{Locale: "de", Gated: true, Shippable: true, Pct: map[string]int{"draft": 100}},
				{Locale: "nb", Shippable: true, Pct: map[string]int{"draft": 100}},
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, localesNeedingPass(tc.cov, locales))
		})
	}
}

func TestFormatPlanLine_PricesTheRedraft(t *testing.T) {
	tests := []struct {
		name     string
		plan     UpPlanOutput
		contains []string
		absent   []string
	}{
		{
			name: "stale work alone is still priced",
			plan: UpPlanOutput{Totals: UpPlanScope{Stale: 1, AIRemaining: 1, TokenEstimate: 7}},
			contains: []string{
				"re-drafting 1 stale unit(s)",
				"their source changed since the translation was decided",
				"1 AI", "≈7 tokens",
			},
		},
		{
			name: "missing and stale work are named apart and costed together",
			plan: UpPlanOutput{Totals: UpPlanScope{
				MissingTarget: 3, Stale: 1, MemoryExact: 2, AIRemaining: 2, TokenEstimate: 40,
			}},
			contains: []string{"3 unit(s) missing", "re-drafting 1 stale unit(s)", "2 exact-content memory", "2 AI"},
		},
		{
			// The pass drafts what the corpus does not answer, so a produced unit
			// the record never paired costs a provider call. Named on its own axis
			// and priced with the rest.
			name: "produced units the content memory does not answer are named and priced",
			plan: UpPlanOutput{Totals: UpPlanScope{Unanswered: 3, AIRemaining: 3, TokenEstimate: 12}},
			contains: []string{
				"drafting 3 unit(s) the content memory does not answer",
				"3 AI", "≈12 tokens",
			},
			absent: []string{"missing", "stale"},
		},
		{
			name:     "nothing to do says so",
			plan:     UpPlanOutput{},
			contains: []string{"every unit has a committed target the content memory answers"},
			absent:   []string{"stale", "does not answer"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			line := formatPlanLine(tc.plan)
			for _, want := range tc.contains {
				assert.Contains(t, line, want)
			}
			for _, no := range tc.absent {
				assert.NotContains(t, line, no)
			}
		})
	}
}

func TestConvergeOutput_ReportsRedraftsBesideStale(t *testing.T) {
	out := ConvergeOutput{
		Flow: "default (built-in)", Passes: 1,
		Locales: []ConvergeLocaleResult{{
			Locale: "de", Stale: 2, Redrafted: 2, Pct: map[string]int{"draft": 100, "translated": 95},
		}},
	}
	var buf bytes.Buffer
	require.NoError(t, out.FormatText(&buf))
	text := buf.String()

	assert.Contains(t, text, "Re-drafted 2 stale unit(s) against the current source")
	assert.Contains(t, text, "the earlier approval is not restored")
	assert.Contains(t, text, "2 unit(s) stale")
	assert.Equal(t, 2, out.RedraftedUnits())

	// The two lines are not interchangeable: the loop's half is done, the
	// person's is not, and a summary that printed only one of them would claim
	// either that nothing happened or that everything did.
	assert.Less(t, bytes.Index([]byte(text), []byte("Re-drafted")),
		bytes.Index([]byte(text), []byte("unit(s) stale:")))
}
