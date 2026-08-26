package server

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRunQAOnBlock_MapsFindingsToWireShape locks the QA handler boundary: the QA
// tools emit core/check.Finding internally, but the HTTP response must keep the
// stable {type, severity ("error"|"warning"), message} shape the editor's
// Problems panel consumes. A dropped non-deletable inline code is a major
// finding → "error"; a double space in the target is a minor finding →
// "warning". The category survives verbatim as the response "type".
func TestRunQAOnBlock_MapsFindingsToWireShape(t *testing.T) {
	// Source has a non-deletable break placeholder; the target drops it AND
	// introduces a double space.
	block := &model.Block{
		ID:           "b1",
		Translatable: true,
		Source: []model.Run{
			{Text: &model.TextRun{Text: "Hello"}},
			{Ph: &model.PlaceholderRun{
				ID: "1", Type: "struct:break", Data: "<br/>",
				Constraints: &model.RunConstraints{Deletable: false},
			}},
			{Text: &model.TextRun{Text: "world"}},
		},
		Properties: map[string]string{},
	}
	block.SetTargetText(model.LocaleFrench, "Bonjour  le monde")

	issues := runQAOnBlock(t.Context(), block, model.LocaleFrench)
	require.NotEmpty(t, issues)

	byType := map[string]string{} // category -> wire severity
	for _, iss := range issues {
		assert.NotEmpty(t, iss.Type, "every issue carries a category (type)")
		assert.NotEmpty(t, iss.Message, "every issue carries a message")
		assert.Contains(t, []string{"error", "warning"}, iss.Severity,
			"wire severity stays two-valued for the Problems panel")
		byType[iss.Type] = iss.Severity
	}

	// The dropped non-deletable span is release-blocking → "error".
	sev, ok := byType["non-deletable-span-missing"]
	require.True(t, ok, "dropped non-deletable inline code must be flagged")
	assert.Equal(t, "error", sev, "a major finding maps to wire severity error")

	// The double space is cosmetic → "warning".
	sev, ok = byType["double-spaces"]
	require.True(t, ok, "double space must be flagged")
	assert.Equal(t, "warning", sev, "a minor finding maps to wire severity warning")
}

// TestQAIssuesFromFindings_KeepsWhatTheFindingLocated: the boundary used to
// keep only {type, severity, message}, so a caller received an issue it could
// name but not point at — a run-native consumer (preview/toContentTree) could
// record it as a block annotation and nothing more. Position, suggestion and
// the offending snippet now ride along.
//
// A position that was never reported is ABSENT, not a zero range: model.Anchor
// zero is a legitimate reading ("the checker located nothing"), and serialized
// as {0,0,0,0} it is indistinguishable from a real span at the start of the
// first run.
func TestQAIssuesFromFindings_KeepsWhatTheFindingLocated(t *testing.T) {
	located := check.Finding{
		Category:     "doubled-word",
		Severity:     check.SeverityMinor,
		Message:      `Target contains doubled word: "le"`,
		Suggestion:   "Remove the repetition",
		OriginalText: "le le",
		Position:     model.SpanAnchor(model.RunPos{Run: 0, Offset: 8}, model.RunPos{Run: 0, Offset: 13}),
	}
	unlocated := check.Finding{
		Category: "empty-target",
		Severity: check.SeverityMajor,
		Message:  "Target is empty but source has content",
	}

	issues := qaIssuesFromFindings([]check.Finding{located, unlocated})
	require.Len(t, issues, 2)

	require.NotNil(t, issues[0].Position)
	assert.Equal(t, located.Position, *issues[0].Position)
	assert.Equal(t, "Remove the repetition", issues[0].Suggestion)
	assert.Equal(t, "le le", issues[0].OriginalText)
	assert.Equal(t, "warning", issues[0].Severity, "the two-valued wire severity is unchanged")

	assert.Nil(t, issues[1].Position)
	raw, err := json.Marshal(issues[1])
	require.NoError(t, err)
	assert.JSONEq(t,
		`{"type":"empty-target","severity":"error","message":"Target is empty but source has content"}`,
		string(raw))
}

// No QA checker populates Position yet — core/tools/qacheck.go judges whole
// texts and the shape flattenings it works over are not run offsets — so the
// endpoint is honest about locating nothing rather than inventing a range.
// This pins that: when the tools start locating, this test says so.
func TestRunQAOnBlock_ReportsNoPositionYet(t *testing.T) {
	block := model.NewBlock("b1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour  le monde")

	issues := runQAOnBlock(t.Context(), block, model.LocaleFrench)
	require.NotEmpty(t, issues)
	for _, iss := range issues {
		assert.Nil(t, iss.Position, "%s: qacheck does not locate its findings yet", iss.Type)
	}
}

// TestRunQAOnBlock_CleanBlock returns an empty slice (not nil) when there is
// nothing to report, so the JSON encodes as [] for the frontend.
func TestRunQAOnBlock_CleanBlock(t *testing.T) {
	block := model.NewBlock("b1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")

	issues := runQAOnBlock(t.Context(), block, model.LocaleFrench)
	assert.Empty(t, issues)
	assert.NotNil(t, issues, "must be a non-nil empty slice so JSON encodes as []")
}
