package xliff

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

// The target segment views of a translation filed under no language divide
// it by its own segmentation, never by the source's: with none of its own it
// is one segment.
func TestTargetSegViewsOfATranslationUnderNoLanguage(t *testing.T) {
	b := model.NewRunsBlock("b1", []model.Run{model.TextR("One. "), model.TextR("Two.")})
	b.SetSegmentation(model.EditionKey{}, []model.Span{
		{ID: "1", Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 1})},
		{ID: "2", Range: model.SpanAnchor(model.RunPos{Run: 1}, model.RunPos{Run: 2})},
	})
	b.SetTargetRuns("", []model.Run{model.TextR("Eins. Zwei.")})

	views := targetSegViews(b, "")
	require.Len(t, views, 1)
	assert.Equal(t, "s1", views[0].ID)
	assert.Equal(t, "Eins. Zwei.", model.RunsText(views[0].Runs))
}
