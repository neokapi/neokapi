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
		Fails:        true,
		Message:      "too long",
		Suggestion:   "shorten it",
		Position:     model.SpanAnchor(model.RunPos{Run: 1}, model.RunPos{Run: 2}),
		OriginalText: "the offending text",
		Metadata:     map[string]string{"limit": "60"},
	}
	d := DiagnosticFrom(f, "length", Location{File: "a.json", Block: "greeting"})

	assert.Equal(t, "length.max-chars-exceeded", d.Rule)
	assert.Equal(t, "length", d.Check)
	assert.True(t, d.Fails)
	assert.Equal(t, "greeting", d.Location.Block)
	require.NotNil(t, d.Location.Anchor, "a non-zero position must carry an anchor")
	assert.Equal(t, "the offending text", d.Location.Snippet)
	assert.Equal(t, "60", d.Metadata["limit"])
}

func TestDiagnosticFrom_ZeroPositionNoAnchor(t *testing.T) {
	d := DiagnosticFrom(Finding{Category: "x"}, "hygiene", Location{Block: "b1"})
	assert.Nil(t, d.Location.Anchor, "a zero position must not synthesize an anchor")
}

func TestBuildReport_ReportingFindingsPass(t *testing.T) {
	diags := []Diagnostic{
		{Rule: "hygiene.double-spaces", Check: "hygiene", Location: Location{Block: "b1"}},
		{Rule: "voice.vocabulary", Check: "voice", Suggested: true, Location: Location{Block: "b2"}},
	}
	r := BuildReport(Target{Kind: "file", Blocks: 2}, diags)
	assert.True(t, r.Pass, "findings that only report never fail a check")
	assert.Equal(t, 0, r.Summary.Failing)
	assert.Equal(t, 2, r.Summary.Reporting)
	assert.Equal(t, 99, r.Summary.Score, "a suggested finding weighs nothing and a reported one a point")
}

func TestBuildReport_SummaryAndSort(t *testing.T) {
	diags := []Diagnostic{
		{Rule: "hygiene.double-spaces", Check: "hygiene", Location: Location{Block: "b2"}},
		{Rule: "structure.xml-well-formedness", Check: "structure", Fails: true, Location: Location{Block: "b1"}},
		{Rule: "length.max-chars-exceeded", Check: "length", Fails: true, Location: Location{Block: "b3"}},
	}
	r := BuildReport(Target{Kind: "file", File: "a.json", Blocks: 3}, diags)

	assert.Equal(t, ReportSchema, r.Schema)
	assert.Equal(t, 3, r.Summary.Findings)
	assert.Equal(t, 2, r.Summary.Failing)
	assert.Equal(t, 1, r.Summary.Reporting)
	assert.False(t, r.Pass, "a failing finding fails the check")

	// Failing findings first, then by rule.
	require.Len(t, r.Findings, 3)
	assert.True(t, r.Findings[0].Fails)
	assert.True(t, r.Findings[1].Fails)
	assert.False(t, r.Findings[2].Fails)
}

func TestBuildReport_CleanPasses(t *testing.T) {
	r := BuildReport(Target{Kind: "text", Blocks: 1}, nil)
	assert.True(t, r.Pass)
	assert.Equal(t, 100, r.Summary.Score)
	assert.Empty(t, r.Findings)
}
