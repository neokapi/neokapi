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
	b.Overlays = append(b.Overlays, model.Overlay{Type: model.OverlayTerm, Variant: &fr, Spans: []model.Span{
		{ID: "bonjour", Range: model.RangeAnchor(oldFr, 0, 7)},
		{ID: "monde", Range: model.RangeAnchor(oldFr, 11, 16)},
	}})
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "w", Range: model.RangeAnchor(b.Source, 6, 11)})

	// "Bonjour" becomes "Salut": [0,7) → 5 code points.
	newFr := []model.Run{model.TextR("Salut le monde entier")}
	b.SetTargetRuns("fr", newFr)
	dropped := model.RemapOverlays(b, &fr, oldFr, newFr, []model.RunEdit{{Start: 0, End: 7, NewLen: 5}})

	assert.Equal(t, 1, dropped, "the term over the replaced word is dropped")
	var frTerms *model.Overlay
	for i := range b.Overlays {
		if b.Overlays[i].Type == model.OverlayTerm && !b.Overlays[i].OnSource() {
			frTerms = &b.Overlays[i]
		}
	}
	require.NotNil(t, frTerms)
	require.Len(t, frTerms.Spans, 1)
	assert.Equal(t, "monde", model.RunsText(frTerms.Spans[0].Range.ExtractRuns(newFr)))
	assert.Equal(t, "world", termSpanText(t, b, "w"), "the source overlay is untouched")
	_, ok := b.OverlaysInBounds(&fr, newFr)
	assert.True(t, ok)
}

// A segmentation layer is the edition's whole segment list: the bilingual
// writers build one segment per span. A rewrite keeps it whole, the segment
// holding an edit resized around it, or drops it whole. It never leaves some
// of its segments, which would write the edited segment's text nowhere.
func TestRemapOverlays_KeepsASegmentationLayerWhole(t *testing.T) {
	segText := func(seg *model.Overlay, runs []model.Run) []string {
		var out []string
		for _, s := range seg.Spans {
			out = append(out, s.ID+"="+model.RunsText(s.Range.ExtractRuns(runs)))
		}
		return out
	}
	tests := []struct {
		name    string
		newText string
		edits   []model.RunEdit
		want    []string // nil: the layer is dropped
		dropped int
	}{
		{
			name:    "an edit inside the second segment resizes it",
			newText: "Premier. Second.",
			edits:   []model.RunEdit{{Start: 9, End: 17, NewLen: 6}},
			want:    []string{"s1=Premier.", "s2=Second."},
		},
		{
			name:    "an edit inside the first segment shifts the second",
			newText: "Le premier. Deuxieme.",
			edits:   []model.RunEdit{{Start: 0, End: 1, NewLen: 4}},
			want:    []string{"s1=Le premier.", "s2=Deuxieme."},
		},
		{
			name:    "an insertion at a segment's end extends that segment",
			newText: "Premier.!! Deuxieme.",
			edits:   []model.RunEdit{{Start: 8, End: 8, NewLen: 2}},
			want:    []string{"s1=Premier.!!", "s2=Deuxieme."},
		},
		{
			name:    "an insertion at the start of the first segment extends it",
			newText: ">> Premier. Deuxieme.",
			edits:   []model.RunEdit{{Start: 0, End: 0, NewLen: 3}},
			want:    []string{"s1=>> Premier.", "s2=Deuxieme."},
		},
		{
			name:    "an edit across the boundary drops the layer",
			newText: "Premier et deuxieme.",
			edits:   []model.RunEdit{{Start: 7, End: 10, NewLen: 4}},
			dropped: 2,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := model.NewBlock("b1", "First. Second.")
			fr := model.Variant("fr")
			oldFr := []model.Run{model.TextR("Premier. Deuxieme.")}
			b.SetTargetRuns("fr", oldFr)
			b.SetSegmentation(&fr, []model.Span{
				{ID: "s1", Range: model.RangeAnchor(oldFr, 0, 8)},
				{ID: "s2", Range: model.RangeAnchor(oldFr, 9, 18)},
			})
			newFr := []model.Run{model.TextR(tt.newText)}
			b.SetTargetRuns("fr", newFr)

			dropped := model.RemapOverlays(b, &fr, oldFr, newFr, tt.edits)

			assert.Equal(t, tt.dropped, dropped)
			seg := b.SegmentationFor(&fr)
			if tt.want == nil {
				assert.Nil(t, seg, "the layer is dropped whole")
				return
			}
			require.NotNil(t, seg)
			assert.Equal(t, tt.want, segText(seg, newFr))
		})
	}
}

// A span a detector anchored over the flattened text can end inside a
// plural's other branch, where no run position addresses it. A rewrite drops
// such a span rather than carry it, so the edition's overlays stay in bounds
// and the rewrite is not refused over them.
func TestRemapOverlays_DropsASpanThatEndsInsideAPlural(t *testing.T) {
	plural := model.Run{Plural: &model.PluralRun{Pivot: "n", Forms: map[model.PluralForm][]model.Run{
		model.PluralOne:   {model.TextR("one item")},
		model.PluralOther: {model.TextR("many items")},
	}}}
	old := []model.Run{model.TextR("You have "), plural, model.TextR(" now")}
	b := model.NewRunsBlock("b1", old)
	// "have many" ends inside the plural's other branch.
	b.AddOverlaySpan(model.OverlayEntity, model.Span{ID: "x", Range: model.RangeAnchor(old, 4, 13)})
	b.AddOverlaySpan(model.OverlayEntity, model.Span{ID: "now", Range: model.RangeAnchor(old, 20, 23)})
	require.False(t, model.RangeAnchor(old, 4, 13).Resolves(old))
	next := []model.Run{model.TextR("Now you have "), plural, model.TextR(" now")}
	b.SetSourceRuns(next)

	dropped := model.RemapOverlays(b, nil, old, next, []model.RunEdit{{Start: 0, End: 1, NewLen: 5}})

	assert.Equal(t, 1, dropped)
	_, ok := b.OverlaysInBounds(nil, next)
	assert.True(t, ok)
	sp := b.OverlaySpan(model.OverlayEntity, "now")
	require.NotNil(t, sp)
	assert.Equal(t, "now", model.RunsText(sp.Range.ExtractRuns(next)))
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
