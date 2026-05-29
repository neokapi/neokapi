package server

import (
	"testing"

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
		Properties:  map[string]string{},
		Annotations: map[string]model.Annotation{},
	}
	block.SetTargetText(model.LocaleFrench, "Bonjour  le monde")

	issues := runQAOnBlock(block, model.LocaleFrench)
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

// TestRunQAOnBlock_CleanBlock returns an empty slice (not nil) when there is
// nothing to report, so the JSON encodes as [] for the frontend.
func TestRunQAOnBlock_CleanBlock(t *testing.T) {
	block := model.NewBlock("b1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")

	issues := runQAOnBlock(block, model.LocaleFrench)
	assert.Empty(t, issues)
	assert.NotNil(t, issues, "must be a non-nil empty slice so JSON encodes as []")
}
