package host

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/stretchr/testify/assert"
)

// TestWriteCoverage_NamesOnlyWhatWasRequested pins the coverage line: it counts
// the analyzer runs that completed and names, by id, the analyzers that were
// requested and did not run. Analyzers nobody asked for are left out, and the
// score caveat appears only when a requested analyzer is missing.
func TestWriteCoverage_NamesOnlyWhatWasRequested(t *testing.T) {
	tests := []struct {
		name string
		runs []check.AnalyzerExecution
		want string
	}{
		{
			name: "analyzers nobody requested are not a gap",
			runs: []check.AnalyzerExecution{
				{ID: "hygiene", Status: check.AnalyzerPassed},
				{ID: "voice.rules", Status: check.AnalyzerFindings},
				{ID: "length", Status: check.AnalyzerNotRequested},
				{ID: "pattern", Status: check.AnalyzerNotRequested},
				{ID: "voice.similarity", Status: check.AnalyzerNotRequested},
				{ID: "voice.llm", Status: check.AnalyzerNotRequested},
				{ID: "comments", Status: check.AnalyzerNotApplicable},
			},
			want: "  Coverage: 2 analyzer runs completed.\n",
		},
		{
			name: "a requested analyzer that did not run is named with its reason",
			runs: []check.AnalyzerExecution{
				{ID: "hygiene", Status: check.AnalyzerPassed},
				{ID: "pattern", Status: check.AnalyzerDidNotRun, Reason: "The profile declares no patterns."},
				{ID: "pattern", Status: check.AnalyzerDidNotRun, Reason: "The profile declares no patterns."},
				{ID: "voice.llm", Status: check.AnalyzerUnsupported},
				{ID: "length", Status: check.AnalyzerNotRequested},
			},
			want: "  Coverage: 1 analyzer run completed; requested and not run: " +
				"pattern (The profile declares no patterns), voice.llm. Score covers reported findings only.\n",
		},
		{
			name: "a missed canary is counted",
			runs: []check.AnalyzerExecution{
				{ID: "hygiene", Status: check.AnalyzerPassed},
				{ID: "voice.rules", Status: check.AnalyzerInvalid},
			},
			want: "  Coverage: 1 analyzer run completed, 1 missed a canary.\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var b bytes.Buffer
			writeCoverage(&b, tt.runs)
			assert.Equal(t, tt.want, b.String())
		})
	}
}
