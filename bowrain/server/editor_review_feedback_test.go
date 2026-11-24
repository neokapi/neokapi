package server

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/jobs"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/venue"
)

// TestEditorAndWorkerGrantOneReviewFeedback pins the review feedback to the one
// assembly: a translation a person starts from the editor and a queued one are
// told the same about a rejected unit, and neither names who rejected it.
func TestEditorAndWorkerGrantOneReviewFeedback(t *testing.T) {
	ctx := t.Context()
	cs, voiceCtx, proj, tm := translatePointFixture(t)

	// "Save" was drafted, and a reviewer turned the draft down.
	save := &model.Block{ID: "b-Save", Translatable: true}
	save.SetSourceText("Save")
	save.SetTargetText("fr", "Sauver")
	require.NoError(t, cs.StoreBlocksForItem(ctx, proj.ID, "main", "hello.txt", []*model.Block{save}))
	stored, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: proj.ID, Stream: "main", ItemName: "hello.txt"})
	require.NoError(t, err)
	var saved *venue.StoredBlock
	for _, sb := range stored {
		if sb.Block.SourceText() == "Save" {
			saved = sb
		}
	}
	require.NotNil(t, saved)
	ds, ok := cs.(platstore.DecisionStore)
	require.True(t, ok)
	_, err = ds.UpsertUnitDecisions(ctx, proj.ID, "main", []venue.UnitDecision{{
		ItemName: "hello.txt", Unit: saved.SourceID, Variant: "fr",
		Status: string(model.TargetStatusDraft), ReviewState: venue.ReviewStateRejected,
		Revision: platstore.VariantRevision("fr", []model.Run{model.TextR("Sauver")}), Basis: saved.SourceRevision,
		Reasons: []string{"wrong_term"}, Note: "Use Enregistrer for saving a file.",
		DecidedBy: "reviewer@example.com", AgentHost: "claude-code",
		Updated: "2026-11-24T10:00:00Z",
	}})
	require.NoError(t, err)

	fromEditor := editorTranslateConfig(ctx, cs, voiceCtx, proj,
		"p1", "main", "hello.txt", "ws-1", "acme", TranslateRequest{TargetLocale: "fr"}, stored)
	fromWorker := jobs.BuildTranslateConfig(ctx, jobs.TranslateBinding{
		Store:            cs,
		Voice:            voiceCtx.Voice,
		WorkspaceDefault: voiceCtx.WorkspaceDefault,
		Terms:            editorTerms(ctx, voiceCtx, "acme", "p1"),
		Memory:           tm,
		Project:          proj,
		WorkspaceID:      "ws-1",
		ProjectID:        "p1",
		Stream:           "main",
		ItemName:         "hello.txt",
		TargetLocale:     "fr",
		Blocks:           stored,
	})

	require.Len(t, fromEditor.ReviewFeedback.Units, 1, "the rejected unit, and only that one")
	assert.Equal(t, fromEditor.ReviewFeedback.Units, fromWorker.ReviewFeedback.Units)
	fb := fromEditor.ReviewFeedback.Units[tool.FeedbackUnitKey("", saved.Block.UnitKey())]
	require.Len(t, fb.Rejections, 1)
	assert.Equal(t, tool.ReviewRejection{
		Text:       "Sauver",
		TargetHash: fb.Rejections[0].TargetHash,
		Reasons:    []string{"wrong_term"},
		Note:       "Use Enregistrer for saving a file.",
	}, fb.Rejections[0])
	assert.NotEmpty(t, fb.Rejections[0].TargetHash, "the hash lets the tool tell a draft that repeats it")
	assert.False(t, fb.Held)

	raw, err := json.Marshal(fromEditor.ReviewFeedback.Units)
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "reviewer@example.com", "who rejected the draft is never sent")
	assert.NotContains(t, string(raw), "claude-code")
}
