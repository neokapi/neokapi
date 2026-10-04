package model_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
)

func twoSpans() []model.Span {
	return []model.Span{
		{ID: "a", Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 1})},
		{ID: "b", Range: model.SpanAnchor(model.RunPos{Run: 1}, model.RunPos{Run: 2})},
	}
}

// The segmentation of a translation filed under no language sits with that
// translation: the empty locale never reaches the source's segmentation, and
// the source's never reaches it.
func TestATranslationUnderNoLanguageKeepsItsSegmentationApart(t *testing.T) {
	b := model.NewRunsBlock("b1", []model.Run{model.TextR("One. "), model.TextR("Two.")})
	b.SetTargetRuns("", []model.Run{model.TextR("Eins. "), model.TextR("Zwei.")})
	b.SetTargetSegmentation("", twoSpans())

	assert.Nil(t, b.SourceSegmentation(), "the source carries no segmentation")
	assert.Equal(t, 1, b.SourceSegmentCount())
	assert.Nil(t, b.SegmentationFor(model.EditionKey{}))
	assert.Empty(t, b.Overlays, "Overlays holds only overlays on editions with a key or the source")

	seg := b.TargetSegmentation("")
	require.NotNil(t, seg)
	assert.Equal(t, twoSpans(), seg.Spans)
	assert.Len(t, b.UnlabelledOverlays(), 1)

	// The source's segmentation stays the source's.
	b.SetSegmentation(model.EditionKey{}, twoSpans()[:1])
	assert.Len(t, b.TargetSegmentation("").Spans, 2)
	assert.Len(t, b.SourceSegmentation().Spans, 1)

	// Empty spans remove the layer.
	b.SetTargetSegmentation("", nil)
	assert.Nil(t, b.TargetSegmentation(""))
	assert.Empty(t, b.UnlabelledOverlays())
	assert.NotNil(t, b.SourceSegmentation())
}

// A locale reaches the segmentation filed under its key, as SegmentationFor
// does, and a named layer is held beside the primary one.
func TestTargetSegmentationOfALanguage(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SetTargetText("fr-FR", "Bonjour")
	b.SetTargetSegmentation("fr-FR", twoSpans())
	b.SetTargetSegmentationLayer("fr-FR", "clause", twoSpans()[:1])

	assert.Same(t, b.SegmentationFor(model.Variant("fr-FR")), b.TargetSegmentation("fr-FR"))
	assert.Same(t, b.SegmentationLayerFor(model.Variant("fr-FR"), "clause"), b.TargetSegmentationLayer("fr-FR", "clause"))
	assert.Nil(t, b.TargetSegmentation(""))
	assert.Nil(t, b.SourceSegmentation())
	assert.Empty(t, b.UnlabelledOverlays())
}

// SetUnlabelledOverlays files every overlay under the zero key and takes an
// empty list as none.
func TestSetUnlabelledOverlays(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SetUnlabelledOverlays([]model.Overlay{{Type: model.OverlayTerm, Edition: model.Variant("de"), Spans: twoSpans()[:1]}})
	require.Len(t, b.UnlabelledOverlays(), 1)
	assert.True(t, b.UnlabelledOverlays()[0].Edition.IsZero())
	assert.Empty(t, b.Overlays)

	b.SetUnlabelledOverlays([]model.Overlay{})
	assert.Nil(t, b.UnlabelledOverlays())
}
