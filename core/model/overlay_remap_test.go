package model_test

import (
	"strconv"
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
	return model.RunsText(sp.Range.ExtractRuns(b.SourceRuns()))
}

func TestRemapOverlays(t *testing.T) {
	// Source "Alice met Bob in Paris"; terms over Alice (0..5), Bob (10..13),
	// Paris (17..22). A redaction deletes "Bob" (edit {10,13} → NewLen 0).
	const oldText = "Alice met Bob in Paris"
	const newText = "Alice met  in Paris" // "Bob" removed (double space remains)

	b := model.NewBlock("b1", oldText)
	old := b.SourceRuns()
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "alice", Range: model.RangeAnchor(old, 0, 5)})
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "bob", Range: model.RangeAnchor(old, 10, 13)})
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "paris", Range: model.RangeAnchor(old, 17, 22)})

	b.SetSourceText(newText)
	dropped := model.RemapOverlays(b, model.EditionKey{}, old, b.SourceRuns(), []model.RunEdit{{Start: 10, End: 13, NewLen: 0}})

	assert.Equal(t, 1, dropped, "the Bob span overlaps the edit and is dropped")
	// Alice is before the edit → unchanged; Paris is after → shifted by -3.
	assert.Equal(t, "Alice", termSpanText(t, b, "alice"))
	assert.Equal(t, "Paris", termSpanText(t, b, "paris"))
	assert.Nil(t, b.OverlaySpan(model.OverlayTerm, "bob"))
}

func TestRemapOverlays_DropsEmptyOverlayAndKeepsOtherEditions(t *testing.T) {
	b := model.NewBlock("b1", "secret only")
	old := b.SourceRuns()
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "s", Range: model.RangeAnchor(old, 0, 6)})
	// An overlay on another edition is left untouched by a source remap.
	tv := model.Variant("fr")
	b.Overlays = append(b.Overlays, model.Overlay{Type: model.OverlayCheck, Edition: tv, Spans: []model.Span{{ID: "q"}}})

	b.SetSourceText("only")
	dropped := model.RemapOverlays(b, model.EditionKey{}, old, b.SourceRuns(), []model.RunEdit{{Start: 0, End: 7, NewLen: 0}})

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
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "s", Range: model.RangeAnchor(b.SourceRuns(), 0, 9)})
	assert.Equal(t, 0, model.RemapOverlays(b, model.EditionKey{}, b.SourceRuns(), b.SourceRuns(), nil))
	assert.Equal(t, "unchanged", termSpanText(t, b, "s"))
}

// A derived edition's overlays follow a rewrite of that edition, and the
// source's overlays are left alone.
func TestRemapOverlays_RebasesADerivedEdition(t *testing.T) {
	b := model.NewBlock("b1", "Hello world")
	fr := model.Variant("fr")
	oldFr := []model.Run{model.TextR("Bonjour le monde entier")}
	b.SetTargetRuns("fr", oldFr)
	b.Overlays = append(b.Overlays, model.Overlay{Type: model.OverlayTerm, Edition: fr, Spans: []model.Span{
		{ID: "bonjour", Range: model.RangeAnchor(oldFr, 0, 7)},
		{ID: "monde", Range: model.RangeAnchor(oldFr, 11, 16)},
	}})
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "w", Range: model.RangeAnchor(b.SourceRuns(), 6, 11)})

	// "Bonjour" becomes "Salut": [0,7) → 5 code points.
	newFr := []model.Run{model.TextR("Salut le monde entier")}
	b.SetTargetRuns("fr", newFr)
	dropped := model.RemapOverlays(b, fr, oldFr, newFr, []model.RunEdit{{Start: 0, End: 7, NewLen: 5}})

	assert.Equal(t, 0, dropped)
	var frTerms *model.Overlay
	for i := range b.Overlays {
		if b.Overlays[i].Type == model.OverlayTerm && !b.Overlays[i].OnSource() {
			frTerms = &b.Overlays[i]
		}
	}
	require.NotNil(t, frTerms)
	require.Len(t, frTerms.Spans, 2)
	assert.Equal(t, "Salut", model.RunsText(frTerms.Spans[0].Range.ExtractRuns(newFr)), "the term over the replaced word covers its replacement")
	assert.Equal(t, "monde", model.RunsText(frTerms.Spans[1].Range.ExtractRuns(newFr)))
	assert.Equal(t, "world", termSpanText(t, b, "w"), "the source overlay is untouched")
	_, ok := b.OverlaysInBounds(fr, newFr)
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
			b.SetSegmentation(fr, []model.Span{
				{ID: "s1", Range: model.RangeAnchor(oldFr, 0, 8)},
				{ID: "s2", Range: model.RangeAnchor(oldFr, 9, 18)},
			})
			newFr := []model.Run{model.TextR(tt.newText)}
			b.SetTargetRuns("fr", newFr)

			dropped := model.RemapOverlays(b, fr, oldFr, newFr, tt.edits)

			assert.Equal(t, tt.dropped, dropped)
			seg := b.SegmentationFor(fr)
			if tt.want == nil {
				assert.Nil(t, seg, "the layer is dropped whole")
				return
			}
			require.NotNil(t, seg)
			assert.Equal(t, tt.want, segText(seg, newFr))
		})
	}
}

// A code has no width in the flattened text, so a flat offset cannot say which
// side of it a segment boundary sits on. A rewrite keeps every code in the
// segment it was in: a boundary after a code stays after it, wherever the
// edits move the text.
func TestRemapOverlays_KeepsEachCodeInItsSegment(t *testing.T) {
	ph := func(id string) model.Run { return model.PhR(model.PlaceholderRun{ID: id, Type: "code:variable"}) }
	tests := []struct {
		name  string
		old   []model.Run
		spans [][2]model.RunPos
		next  []model.Run
		edits []model.RunEdit
		want  []string
	}{
		{
			name:  "a code that ends a segment stays in it after an edit before it",
			old:   []model.Run{model.TextR("Nous employons "), ph("1"), model.TextR("Puis cela.")},
			spans: [][2]model.RunPos{{{Run: 0}, {Run: 2}}, {{Run: 2}, {Run: 3}}},
			next:  []model.Run{model.TextR("Nous utilisons "), ph("1"), model.TextR("Puis cela.")},
			edits: []model.RunEdit{{Start: 5, End: 14, NewLen: 9}},
			want:  []string{`s1=Nous utilisons <x id="1/"/>`, "s2=Puis cela."},
		},
		{
			name:  "a code that ends a segment stays in it after an edit in the next",
			old:   []model.Run{model.TextR("First."), ph("1"), model.TextR("Second.")},
			spans: [][2]model.RunPos{{{Run: 0}, {Run: 2}}, {{Run: 2}, {Run: 3}}},
			next:  []model.Run{model.TextR("First."), ph("1"), model.TextR("The second.")},
			edits: []model.RunEdit{{Start: 6, End: 7, NewLen: 5}},
			want:  []string{`s1=First.<x id="1/"/>`, "s2=The second."},
		},
		{
			name:  "a code that starts a segment stays in it",
			old:   []model.Run{model.TextR("First."), ph("1"), model.TextR("Second.")},
			spans: [][2]model.RunPos{{{Run: 0}, {Run: 1}}, {{Run: 1}, {Run: 3}}},
			next:  []model.Run{model.TextR("The first."), ph("1"), model.TextR("Second.")},
			edits: []model.RunEdit{{Start: 0, End: 1, NewLen: 5}},
			want:  []string{"s1=The first.", `s2=<x id="1/"/>Second.`},
		},
		{
			name:  "two codes at one boundary stay on their sides",
			old:   []model.Run{model.TextR("First."), ph("1"), ph("2"), model.TextR("Second.")},
			spans: [][2]model.RunPos{{{Run: 0}, {Run: 2}}, {{Run: 2}, {Run: 4}}},
			next:  []model.Run{model.TextR("The first."), ph("1"), ph("2"), model.TextR("Second.")},
			edits: []model.RunEdit{{Start: 0, End: 1, NewLen: 5}},
			want:  []string{`s1=The first.<x id="1/"/>`, `s2=<x id="2/"/>Second.`},
		},
		{
			name:  "a segment whose text is deleted keeps its code",
			old:   []model.Run{model.TextR("One."), ph("1"), model.TextR("Two."), ph("2"), model.TextR("Three.")},
			spans: [][2]model.RunPos{{{Run: 0}, {Run: 2}}, {{Run: 2}, {Run: 4}}, {{Run: 4}, {Run: 5}}},
			next:  []model.Run{model.TextR("One."), ph("1"), ph("2"), model.TextR("Three.")},
			edits: []model.RunEdit{{Start: 4, End: 8, NewLen: 0}},
			want:  []string{`s1=One.<x id="1/"/>`, `s2=<x id="2/"/>`, "s3=Three."},
		},
		{
			name:  "a code an edit removed shifts no later boundary",
			old:   []model.Run{model.TextR("A "), ph("0"), model.TextR("b."), ph("1"), model.TextR("Two.")},
			spans: [][2]model.RunPos{{{Run: 0}, {Run: 4}}, {{Run: 4}, {Run: 5}}},
			next:  []model.Run{model.TextR("Ab."), ph("1"), model.TextR("Two.")},
			edits: []model.RunEdit{{Start: 1, End: 2, NewLen: 0}},
			want:  []string{`s1=Ab.<x id="1/"/>`, "s2=Two."},
		},
		{
			name:  "a code that ends the last segment stays in it",
			old:   []model.Run{model.TextR("First. "), model.TextR("Second."), ph("1")},
			spans: [][2]model.RunPos{{{Run: 0}, {Run: 1}}, {{Run: 1}, {Run: 3}}},
			next:  []model.Run{model.TextR("First. The second."), ph("1")},
			edits: []model.RunEdit{{Start: 7, End: 8, NewLen: 5}},
			want:  []string{"s1=First. ", `s2=The second.<x id="1/"/>`},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fr := model.Variant("fr")
			b := model.NewBlock("b1", "Source.")
			b.SetTargetRuns("fr", tt.old)
			var spans []model.Span
			for i, s := range tt.spans {
				spans = append(spans, model.Span{ID: "s" + strconv.Itoa(i+1), Range: model.SpanAnchor(s[0], s[1])})
			}
			b.SetSegmentation(fr, spans)
			b.SetTargetRuns("fr", tt.next)

			dropped := model.RemapOverlays(b, fr, tt.old, tt.next, tt.edits)

			assert.Equal(t, 0, dropped)
			seg := b.SegmentationFor(fr)
			require.NotNil(t, seg)
			var got []string
			for _, s := range seg.Spans {
				got = append(got, s.ID+"="+model.RunsEditText(s.Range.ExtractRuns(tt.next)))
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// A term that starts after a code still starts after it once the text before
// the code is edited.
func TestRemapOverlays_KeepsATermStartAfterACode(t *testing.T) {
	ph := model.PhR(model.PlaceholderRun{ID: "1", Type: "code:variable"})
	old := []model.Run{model.TextR("Use "), ph, model.TextR("kapi now")}
	b := model.NewRunsBlock("b1", old)
	b.AddOverlaySpan(model.OverlayTerm, model.Span{ID: "kapi", Range: model.SpanAnchor(model.RunPos{Run: 2}, model.RunPos{Run: 2, Offset: 4})})
	next := []model.Run{model.TextR("Please use "), ph, model.TextR("kapi now")}
	b.SetSourceRuns(next)

	dropped := model.RemapOverlays(b, model.EditionKey{}, old, next, []model.RunEdit{{Start: 0, End: 1, NewLen: 8}})

	assert.Equal(t, 0, dropped)
	sp := b.OverlaySpan(model.OverlayTerm, "kapi")
	require.NotNil(t, sp)
	assert.Equal(t, "kapi", model.RunsEditText(sp.Range.ExtractRuns(next)), "the term does not take in the code before it")
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

	dropped := model.RemapOverlays(b, model.EditionKey{}, old, next, []model.RunEdit{{Start: 0, End: 1, NewLen: 5}})

	assert.Equal(t, 1, dropped)
	_, ok := b.OverlaysInBounds(model.EditionKey{}, next)
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
	dropped := model.RemapOverlays(b, model.EditionKey{}, old, next, []model.RunEdit{{Start: 0, End: 2, NewLen: 5}})

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
		{Type: model.OverlaySegmentation, Edition: fr},
		{Type: model.OverlaySegmentation, Edition: de},
		{Type: model.OverlayTerm},
	}
	nbNO := model.VariantKey{Locale: "nb_NO"}
	assert.Equal(t, 0, model.DropOverlays(b, nbNO))
	assert.Equal(t, 1, model.DropOverlays(b, fr))
	assert.Len(t, b.Overlays, 2)
	assert.Equal(t, 1, model.DropOverlays(b, model.EditionKey{}))
	require.Len(t, b.Overlays, 1)
	assert.Equal(t, de, b.Overlays[0].Edition)
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
	runs := model.NewBlock("b", "hello").SourceRuns() // single run, 5 runes
	assert.True(t, model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 0, Offset: 5}).InBounds(runs))
	assert.True(t, model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 1}).InBounds(runs), "end boundary just past the last run")
	assert.False(t, model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 0, Offset: 6}).InBounds(runs), "offset past the run text")
	assert.False(t, model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 2}).InBounds(runs), "run index past end")
	assert.False(t, model.SpanAnchor(model.RunPos{Run: 1}, model.RunPos{Run: 0}).InBounds(runs), "start past end")
}

// A span follows the text it covers. An edit inside it, or one that replaces
// all of its text, keeps it over the new text; an edit across one of its
// boundaries keeps it over the part of its text the edit left; it goes only
// when the edits delete everything it covered. A quality finding states
// something about the exact text under it, so an edit there drops it.
func TestRemapOverlays_ASpanFollowsAnEditToItsText(t *testing.T) {
	const old = "Grind the coffee beans now"
	tests := []struct {
		name  string
		typ   model.OverlayType
		span  [2]int // over old
		edits []model.RunEdit
		next  string
		want  string // "" when the span is dropped
	}{
		{"an edit inside the span", model.OverlayTerm, [2]int{10, 22},
			[]model.RunEdit{{Start: 17, End: 22, NewLen: 7}}, "Grind the coffee grounds now", "coffee grounds"},
		{"an edit replacing all of its text", "xliff2:mrk", [2]int{10, 16},
			[]model.RunEdit{{Start: 10, End: 16, NewLen: 3}}, "Grind the tea beans now", "tea"},
		{"an edit at its start", model.OverlayEntity, [2]int{10, 22},
			[]model.RunEdit{{Start: 10, End: 16, NewLen: 3}}, "Grind the tea beans now", "tea beans"},
		{"an edit across its start", model.OverlayTerm, [2]int{10, 22},
			[]model.RunEdit{{Start: 6, End: 16, NewLen: 5}}, "Grind a tea beans now", " beans"},
		{"an edit across its end", model.OverlayTerm, [2]int{6, 16},
			[]model.RunEdit{{Start: 10, End: 22, NewLen: 3}}, "Grind the tea now", "the "},
		{"a case conversion across its end", model.OverlayTerm, [2]int{6, 13},
			[]model.RunEdit{{Start: 10, End: 16, NewLen: 6}}, "Grind the COFFEE beans now", "the COF"},
		{"an insertion at its start goes before it", model.OverlayTerm, [2]int{10, 16},
			[]model.RunEdit{{Start: 10, End: 10, NewLen: 4}}, "Grind the hot coffee beans now", "coffee"},
		{"an insertion at its end goes after it", model.OverlayTerm, [2]int{10, 16},
			[]model.RunEdit{{Start: 16, End: 16, NewLen: 1}}, "Grind the coffees beans now", "coffee"},
		{"an edit deleting all of its text", model.OverlayTerm, [2]int{10, 17},
			[]model.RunEdit{{Start: 10, End: 17, NewLen: 0}}, "Grind the beans now", ""},
		{"an edit replacing its text from outside", model.OverlayTerm, [2]int{10, 16},
			[]model.RunEdit{{Start: 6, End: 16, NewLen: 3}}, "Grind tea beans now", ""},
		{"a finding under an edit", model.OverlayCheck, [2]int{10, 22},
			[]model.RunEdit{{Start: 17, End: 22, NewLen: 7}}, "Grind the coffee grounds now", ""},
		{"a finding after an edit", model.OverlayCheck, [2]int{17, 22},
			[]model.RunEdit{{Start: 0, End: 5, NewLen: 4}}, "Brew the coffee beans now", "beans"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := model.NewBlock("b1", old)
			before := b.SourceRuns()
			b.Overlays = []model.Overlay{{Type: tt.typ, Spans: []model.Span{{ID: "x", Range: model.RangeAnchor(before, tt.span[0], tt.span[1])}}}}
			b.SetSourceText(tt.next)

			dropped := model.RemapOverlays(b, model.EditionKey{}, before, b.SourceRuns(), tt.edits)

			sp := b.OverlaySpan(tt.typ, "x")
			if tt.want == "" {
				assert.Equal(t, 1, dropped)
				assert.Nil(t, sp)
				return
			}
			assert.Equal(t, 0, dropped)
			require.NotNil(t, sp)
			assert.Equal(t, tt.want, model.RunsText(sp.Range.ExtractRuns(b.SourceRuns())))
		})
	}
}

// One edit per changed character, the shape a conversion edit by edit takes,
// keeps every span over the characters it changed.
func TestRemapOverlays_KeepsSpansAcrossManyEdits(t *testing.T) {
	const old = "grind the coffee"
	b := model.NewBlock("b1", old)
	before := b.SourceRuns()
	b.Overlays = []model.Overlay{{Type: model.OverlayTerm, Spans: []model.Span{
		{ID: "the", Range: model.RangeAnchor(before, 6, 9)},
		{ID: "coffee", Range: model.RangeAnchor(before, 10, 16)},
	}}}
	var edits []model.RunEdit
	for i, r := range old {
		if r != ' ' {
			edits = append(edits, model.RunEdit{Start: i, End: i + 1, NewLen: 1})
		}
	}
	b.SetSourceText("GRIND THE COFFEE")

	assert.Equal(t, 0, model.RemapOverlays(b, model.EditionKey{}, before, b.SourceRuns(), edits))
	assert.Equal(t, "THE", termSpanText(t, b, "the"))
	assert.Equal(t, "COFFEE", termSpanText(t, b, "coffee"))
}
