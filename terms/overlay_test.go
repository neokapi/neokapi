package terms_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLocateOverlay_MarksEachOccurrenceByAnchor(t *testing.T) {
	ctx := context.Background()
	store := terms.NewInMemoryStore()
	require.NoError(t, store.AddConcept(ctx, terms.Concept{
		ID:     "vessel",
		Domain: "shipping",
		Terms: []terms.Term{
			{Text: "vessel", Locale: model.LocaleEnglish, Status: model.TermPreferred},
			{Text: "fartøy", Locale: "nb", Status: model.TermPreferred},
		},
	}))
	runs := []model.Run{
		{Text: &model.TextRun{Text: "The "}},
		{Ph: &model.PlaceholderRun{ID: "1", Type: "var", Data: "{vessel}"}},
		{Text: &model.TextRun{Text: " vessel docks"}},
	}

	ov, err := terms.LocateOverlay(ctx, store, runs, model.LocaleEnglish, "nb")
	require.NoError(t, err)
	require.NotNil(t, ov)
	assert.Equal(t, model.OverlayTerm, ov.Type)
	require.Len(t, ov.Spans, 1, "a placeholder's name is not an occurrence")
	span := ov.Spans[0]
	assert.Equal(t, "vessel", model.RunsText(span.Range.ExtractRuns(runs)))
	assert.Equal(t, map[string]string{
		"term": "vessel", "target": "fartøy", "concept_id": "vessel", "domain": "shipping",
	}, span.Props)
	ann, ok := span.Value.(*model.TermAnnotation)
	require.True(t, ok)
	assert.Equal(t, "fartøy", ann.TargetTerms[0].Text)
}

func TestLocateOverlay_NothingOccursIsNil(t *testing.T) {
	ov, err := terms.LocateOverlay(context.Background(), terms.NewInMemoryStore(),
		[]model.Run{{Text: &model.TextRun{Text: "No terms here"}}}, model.LocaleEnglish, "")
	require.NoError(t, err)
	assert.Nil(t, ov)
}
