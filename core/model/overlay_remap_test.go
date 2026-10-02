package model_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// termSpanText is the text a term overlay's span covers, for terse assertions.
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
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "alice", Range: model.RangeAnchor(old, 0, 5)})
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "bob", Range: model.RangeAnchor(old, 10, 13)})
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "paris", Range: model.RangeAnchor(old, 17, 22)})

	b.SetSourceText(newText)
	dropped := model.RemapOverlays(b, nil, old, b.Source, []model.RunEdit{{Start: 10, End: 13, NewLen: 0}})

	assert.Equal(t, 1, dropped, "the Bob span overlaps the edit and is dropped")
	// Alice is before the edit → unchanged; Paris is after → shifted by -3.
	assert.Equal(t, "Alice", termSpanText(t, b, "alice"))
	assert.Equal(t, "Paris", termSpanText(t, b, "paris"))
	assert.Nil(t, b.OverlaySpan(model.OverlayTerm, "bob"))
}

func TestRemapOverlays_DropsEmptyOverlayAndKeepsOtherEditions(t *testing.T) {
	b := model.NewBlock("b1", "secret only")
	old := b.Source
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "s", Range: model.RangeAnchor(old, 0, 6)})
	// An overlay on another edition is left untouched by a source remap.
	tv := model.Variant("fr")
	b.Overlays = append(b.Overlays, model.Overlay{Type: model.OverlayCheck, Variant: &tv, Spans: []model.Span{{ID: "q"}}})

	b.SetSourceText("only")
	dropped := model.RemapOverlays(b, nil, old, b.Source, []model.RunEdit{{Start: 0, End: 7, NewLen: 0}})

	assert.Equal(t, 1, dropped)
	assert.Nil(t, b.OverlayOf(model.OverlayTerm), "now-empty source overlay is removed")
	var checkKept bool
	for _, o := range b.Overlays {
		if o.Type == model.OverlayCheck && !o.OnSource() {
			checkKept = true
		}
	}
	assert.True(t, checkKept, "target-side overlay untouched")
}

func TestRemapOverlays_NoEditsIsNoop(t *testing.T) {
	b := model.NewBlock("b1", "unchanged")
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "s", Range: model.RangeAnchor(b.Source, 0, 9)})
	assert.Equal(t, 0, model.RemapOverlays(b, nil, b.Source, b.Source, nil))
	assert.Equal(t, "unchanged", termSpanText(t, b, "s"))
}

// A derived edition's overlays follow a rewrite of that edition, and the
// source's overlays are left alone.
func TestRemapOverlays_RebasesADerivedEdition(t *testing.T) {
	b := model.NewBlock("b1", "Hello world")
	fr := model.Variant("fr")
	oldFr := []model.Run{model.TextR("Bonjour le monde entier")}
	b.SetTargetRuns("fr", oldFr)
	b.SetSegmentation(&fr, []model.Span{
		{ID: "s1", Range: model.RangeAnchor(oldFr, 0, 7)},
		{ID: "s2", Range: model.RangeAnchor(oldFr, 8, 23)},
	})
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "w", Range: model.RangeAnchor(b.Source, 6, 11)})

	// "Bonjour" becomes "Salut": [0,7) → 5 code points.
	newFr := []model.Run{model.TextR("Salut le monde entier")}
	b.SetTargetRuns("fr", newFr)
	dropped := model.RemapOverlays(b, &fr, oldFr, newFr, []model.RunEdit{{Start: 0, End: 7, NewLen: 5}})

	assert.Equal(t, 1, dropped, "the span over the replaced word is dropped")
	seg := b.SegmentationFor(&fr)
	require.NotNil(t, seg)
	require.Len(t, seg.Spans, 1)
	assert.Equal(t, "le monde entier", model.RunsText(seg.Spans[0].Range.ExtractRuns(newFr)))
	assert.Equal(t, "world", termSpanText(t, b, "w"), "the source overlay is untouched")
	_, ok := b.OverlaysInBounds(&fr, newFr)
	assert.True(t, ok)
}

// A rewrite carries each anchor kind by its own rule: a block anchor stays, a
// run anchor stays while its run does, and a range anchor is remapped.
func TestRemapOverlays_KeepsEachAnchorKindByItsOwnRule(t *testing.T) {
	ph := model.PhR(model.PlaceholderRun{ID: "v", Type: "code:variable", Data: "{n}"})
	old := []model.Run{model.TextR("Hi "), ph, model.TextR(" there")}
	b := model.NewRunsBlock("b1", old)
	b.Overlays = []model.Overlay{{Type: "note", Spans: []model.Span{
		{ID: "whole", Range: model.BlockAnchor()},
		{ID: "var", Range: model.RunAnchor(nil, "v")},
		{ID: "gone", Range: model.RunAnchor(nil, "x")},
	}}}

	next := []model.Run{model.TextR("Hello "), ph, model.TextR(" there")}
	b.SetSourceRuns(next)
	dropped := model.RemapOverlays(b, nil, old, next, []model.RunEdit{{Start: 0, End: 2, NewLen: 5}})

	assert.Equal(t, 1, dropped, "only the run anchor whose run is gone is dropped")
	require.Len(t, b.Overlays, 1)
	got := map[string]model.Anchor{}
	for _, s := range b.Overlays[0].Spans {
		got[s.ID] = s.Range
	}
	assert.Equal(t, model.BlockAnchor(), got["whole"], "a block anchor is not turned into a range")
	assert.Equal(t, model.RunAnchor(nil, "v"), got["var"])
}

func TestDropOverlays_RemovesOneEditionOnly(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	fr, de := model.Variant("fr"), model.Variant("de")
	b.Overlays = []model.Overlay{
		{Type: model.OverlaySegmentation, Variant: &fr},
		{Type: model.OverlaySegmentation, Variant: &de},
		{Type: model.OverlayTerm},
	}
	nbNO := model.VariantKey{Locale: "nb_NO"}
	assert.Equal(t, 0, model.DropOverlays(b, &nbNO))
	assert.Equal(t, 1, model.DropOverlays(b, &fr))
	assert.Len(t, b.Overlays, 2)
	assert.Equal(t, 1, model.DropOverlays(b, nil))
	require.Len(t, b.Overlays, 1)
	assert.Equal(t, &de, b.Overlays[0].Variant)
}

func TestResolveRunPath(t *testing.T) {
	plural := model.PluralR(model.PluralRun{Pivot: "n", Forms: map[model.PluralForm][]model.Run{
		model.PluralOne:   {model.TextR("one item")},
		model.PluralOther: {model.TextR("many items")},
	}})
	runs := []model.Run{model.TextR("You have "), plural}

	tests := []struct {
		name string
		path model.RunPath
		want string
		ok   bool
	}{
		{"empty path is the sequence", nil, "You have many items", true},
		{"index then form", model.RunPath{{Kind: model.StepIndex, Index: 1}, {Kind: model.StepPlural, PluralForm: model.PluralOne}}, "one item", true},
		{"ending on an index", model.RunPath{{Kind: model.StepIndex, Index: 1}}, "", false},
		{"index past the end", model.RunPath{{Kind: model.StepIndex, Index: 5}, {Kind: model.StepPlural, PluralForm: model.PluralOne}}, "", false},
		{"missing form", model.RunPath{{Kind: model.StepIndex, Index: 1}, {Kind: model.StepPlural, PluralForm: model.PluralFew}}, "", false},
		{"form on a text run", model.RunPath{{Kind: model.StepIndex, Index: 0}, {Kind: model.StepPlural, PluralForm: model.PluralOne}}, "", false},
		{"select step on a plural", model.RunPath{{Kind: model.StepIndex, Index: 1}, {Kind: model.StepSelect, SelectValue: "one"}}, "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			seq, ok := model.ResolveRunPath(runs, tc.path)
			assert.Equal(t, tc.ok, ok)
			if ok {
				assert.Equal(t, tc.want, model.RunsText(seq))
			}
		})
	}
}

func TestAnchor_InBounds(t *testing.T) {
	runs := model.NewBlock("b", "hello").Source // single run, 5 runes
	assert.True(t, model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 0, Offset: 5}).InBounds(runs))
	assert.True(t, model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 1}).InBounds(runs), "end boundary just past the last run")
	assert.False(t, model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 0, Offset: 6}).InBounds(runs), "offset past the run text")
	assert.False(t, model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 2}).InBounds(runs), "run index past end")
	assert.False(t, model.SpanAnchor(model.RunPos{Run: 1}, model.RunPos{Run: 0}).InBounds(runs), "start past end")
}
