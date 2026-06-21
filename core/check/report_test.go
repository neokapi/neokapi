package check

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiagnosticFrom_RuleAndLocation(t *testing.T) {
	f := Finding{
		Category:     "max-chars-exceeded",
		Severity:     SeverityMajor,
		Message:      "too long",
		Suggestion:   "shorten it",
		Position:     model.RunRange{StartRun: 1, EndRun: 2},
		OriginalText: "the offending text",
		Metadata:     map[string]string{"limit": "60"},
	}
	d := DiagnosticFrom(f, "length", Location{File: "a.json", Block: "greeting"})

	assert.Equal(t, "length.max-chars-exceeded", d.Rule)
	assert.Equal(t, "length", d.Check)
	assert.Equal(t, SeverityMajor, d.Severity)
	assert.Equal(t, "greeting", d.Location.Block)
	require.NotNil(t, d.Location.RunRange, "a non-zero position must carry a run_range")
	assert.Equal(t, "the offending text", d.Location.Snippet)
	assert.Equal(t, "60", d.Metadata["limit"])
}

func TestDiagnosticFrom_ZeroPositionNoRunRange(t *testing.T) {
	d := DiagnosticFrom(Finding{Category: "x", Severity: SeverityMinor}, "hygiene", Location{Block: "b1"})
	assert.Nil(t, d.Location.RunRange, "a zero position must not synthesize a run_range")
}

func TestGate_Evaluate(t *testing.T) {
	tests := []struct {
		name       string
		gate       Gate
		summary    Summary
		wantFailed int
	}{
		{"default passes on majors", DefaultGate(), Summary{Major: 5}, 0},
		{"default fails on a critical", DefaultGate(), Summary{Critical: 1}, 1},
		{"max-major trips", Gate{MaxCritical: 0, MaxMajor: 0, MaxMinor: -1}, Summary{Major: 2}, 1},
		{"min-score trips", Gate{MaxCritical: 0, MaxMajor: -1, MaxMinor: -1, MinScore: 90}, Summary{Score: 80}, 1},
		{"multiple trip", Gate{MaxCritical: 0, MaxMajor: 0, MinScore: 90}, Summary{Critical: 1, Major: 1, Score: 50}, 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := tc.gate.Evaluate(tc.summary)
			assert.Len(t, r.Failed, tc.wantFailed)
		})
	}
}

func TestBuildReport_SummaryGateAndSort(t *testing.T) {
	diags := []Diagnostic{
		{Rule: "hygiene.double-spaces", Check: "hygiene", Severity: SeverityMinor, Location: Location{Block: "b2"}},
		{Rule: "structure.xml-well-formedness", Check: "structure", Severity: SeverityCritical, Location: Location{Block: "b1"}},
		{Rule: "length.max-chars-exceeded", Check: "length", Severity: SeverityMajor, Location: Location{Block: "b3"}},
	}
	r := BuildReport(Target{Kind: "file", File: "a.json", Blocks: 3}, diags, DefaultGate())

	assert.Equal(t, ReportSchema, r.Schema)
	assert.Equal(t, 3, r.Summary.Findings)
	assert.Equal(t, 1, r.Summary.Critical)
	assert.Equal(t, 1, r.Summary.Major)
	assert.Equal(t, 1, r.Summary.Minor)
	assert.False(t, r.Pass, "a critical must fail the default gate")
	require.Len(t, r.Gate.Failed, 1)

	// Sorted severity → rule: critical first, then major, then minor.
	require.Len(t, r.Findings, 3)
	assert.Equal(t, SeverityCritical, r.Findings[0].Severity)
	assert.Equal(t, SeverityMajor, r.Findings[1].Severity)
	assert.Equal(t, SeverityMinor, r.Findings[2].Severity)
}

func TestBuildReport_CleanPasses(t *testing.T) {
	r := BuildReport(Target{Kind: "text", Blocks: 1}, nil, DefaultGate())
	assert.True(t, r.Pass)
	assert.Equal(t, 100, r.Summary.Score)
	assert.Empty(t, r.Findings)
	assert.NotNil(t, r.Gate.Failed) // serializes as [] not null
}
