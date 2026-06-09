package model_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// textSpan is the [start,end) rune span a term overlay's span covers, for terse
// assertions.
func termSpanText(t *testing.T, b *model.Block, id string) string {
	t.Helper()
	sp := b.OverlaySpan(model.OverlayTerm, id)
	require.NotNil(t, sp, "term span %q", id)
	return model.RunsText(sp.Range.ExtractRuns(b.Source))
}

func TestRemapOverlays(t *testing.T) {
	// Source "Alice met Bob in Paris"; terms over Alice (0..5), Bob (10..13),
	// Paris (17..22). A redaction deletes "Bob" (edit {10,13} → NewLen 0).
	const oldText = "Alice met Bob in Paris"
	const newText = "Alice met  in Paris" // "Bob" removed (double space remains)

	b := model.NewBlock("b1", oldText)
	old := b.Source
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "alice", Range: model.RunRangeFor(old, 0, 5)})
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "bob", Range: model.RunRangeFor(old, 10, 13)})
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "paris", Range: model.RunRangeFor(old, 17, 22)})

	b.SetSourceText(newText)
	dropped := model.RemapOverlays(b, old, []model.RunEdit{{Start: 10, End: 13, NewLen: 0}})

	assert.Equal(t, 1, dropped, "the Bob span overlaps the edit and is dropped")
	// Alice is before the edit → unchanged; Paris is after → shifted by -3.
	assert.Equal(t, "Alice", termSpanText(t, b, "alice"))
	assert.Equal(t, "Paris", termSpanText(t, b, "paris"))
	assert.Nil(t, b.OverlaySpan(model.OverlayTerm, "bob"))
}

func TestRemapOverlays_DropsEmptyOverlayAndKeepsTargetSide(t *testing.T) {
	b := model.NewBlock("b1", "secret only")
	old := b.Source
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "s", Range: model.RunRangeFor(old, 0, 6)})
	// A target-side overlay must be left untouched by source remapping.
	tv := model.Variant("fr")
	b.Overlays = append(b.Overlays, model.Overlay{Type: model.OverlayQA, Variant: &tv, Spans: []model.Span{{ID: "q"}}})

	b.SetSourceText("only")
	dropped := model.RemapOverlays(b, old, []model.RunEdit{{Start: 0, End: 7, NewLen: 0}})

	assert.Equal(t, 1, dropped)
	assert.Nil(t, b.OverlayOf(model.OverlayTerm), "now-empty source overlay is removed")
	// The target-side QA overlay survives.
	var qaKept bool
	for _, o := range b.Overlays {
		if o.Type == model.OverlayQA && !o.OnSource() {
			qaKept = true
		}
	}
	assert.True(t, qaKept, "target-side overlay untouched")
}

func TestRemapOverlays_NoEditsIsNoop(t *testing.T) {
	b := model.NewBlock("b1", "unchanged")
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "s", Range: model.RunRangeFor(b.Source, 0, 9)})
	assert.Equal(t, 0, model.RemapOverlays(b, b.Source, nil))
	assert.Equal(t, "unchanged", termSpanText(t, b, "s"))
}

func TestRunRange_InBounds(t *testing.T) {
	runs := model.NewBlock("b", "hello").Source // single run, 5 runes
	assert.True(t, model.RunRange{EndRun: 0, EndOffset: 5}.InBounds(runs))
	assert.True(t, model.RunRange{EndRun: 1}.InBounds(runs), "end boundary just past the last run")
	assert.False(t, model.RunRange{EndRun: 0, EndOffset: 6}.InBounds(runs), "offset past the run text")
	assert.False(t, model.RunRange{EndRun: 2}.InBounds(runs), "run index past end")
	assert.False(t, model.RunRange{StartRun: 1, EndRun: 0}.InBounds(runs), "start past end")
}
