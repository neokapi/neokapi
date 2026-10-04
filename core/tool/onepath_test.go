package tool_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
)

func dispatch(t *testing.T, bt *tool.BaseTool, b *model.Block) error {
	t.Helper()
	_, err := bt.ApplyContext(context.Background(), &model.Part{Type: model.PartBlock, Resource: b})
	return err
}

// A view applies each write at once, so a handler reads what it wrote: a new
// target it stamps keeps the stamp, and a write it reads back is there.
func TestView_ReadsItsOwnWrites(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	var seen string
	bt := &tool.BaseTool{ToolName: "probe"}
	bt.Produce = func(v tool.VariantView) error {
		v.SetTargetRuns("fr", []model.Run{model.TextR("Bonjour")})
		seen = v.TargetText("fr")
		v.StampTargetProvenance("fr", model.TargetStatusDraft, model.Origin{Tool: "probe"})
		return nil
	}
	require.NoError(t, dispatch(t, bt, b))
	assert.Equal(t, "Bonjour", seen)
	tg, ok := b.Edition(model.Variant("fr"))
	require.True(t, ok)
	assert.Equal(t, model.Status(model.TargetStatusDraft), tg.Status, "the stamp lands on the target the handler just created")
	assert.Equal(t, "probe", tg.Origin.Tool)
}

// A tool that rewrites a target makes it a draft, since nobody has read its
// wording, and keeps the origin that names who translated it; its provenance
// stamp is how the tool records what it produced.
func TestView_TargetWritesTakeTheToolConsequence(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SetEdition(model.Variant("fr"), model.Edition{Runs: []model.Run{model.TextR("Salut")}, Status: model.Status(model.TargetStatusEstablished), Origin: model.Origin{Kind: model.OriginHuman}})
	bt := &tool.BaseTool{ToolName: "case"}
	bt.Produce = func(v tool.VariantView) error {
		v.SetTargetText("fr", "Salut")
		return nil
	}
	require.NoError(t, dispatch(t, bt, b))
	assert.Equal(t, model.Status(model.TargetStatusEstablished), target(t, b, "fr").Status, "a write that changes nothing keeps the approval")

	bt.Produce = func(v tool.VariantView) error {
		v.SetTargetText("fr", "SALUT")
		return nil
	}
	require.NoError(t, dispatch(t, bt, b))
	assert.Equal(t, "SALUT", b.TargetText("fr"))
	assert.Equal(t, model.Status(model.TargetStatusDraft), target(t, b, "fr").Status)
	assert.Equal(t, model.OriginHuman, target(t, b, "fr").Origin.Kind)

	bt.Produce = func(v tool.VariantView) error {
		v.SetEdition(model.Variant("de"), model.Edition{Runs: []model.Run{model.TextR("Hallo")}, Status: model.Status(model.TargetStatusDraft), Score: 0.5})
		v.RemoveTarget("fr")
		return nil
	}
	require.NoError(t, dispatch(t, bt, b))
	assert.False(t, b.HasTarget("fr"))
	de := target(t, b, "de")
	assert.Equal(t, model.Status(model.TargetStatusDraft), de.Status)
	assert.InDelta(t, 0.5, de.Score, 0)

	formal := model.EditionKey{Locale: "de", Tone: "formal"}
	bt.Produce = func(v tool.VariantView) error {
		v.SetEdition(formal, model.Edition{Runs: []model.Run{model.TextR("Guten Tag")}})
		return nil
	}
	require.NoError(t, dispatch(t, bt, b))
	_, ok := b.Edition(formal)
	require.True(t, ok)
	bt.Produce = func(v tool.VariantView) error {
		v.RemoveEdition(formal)
		return nil
	}
	require.NoError(t, dispatch(t, bt, b))
	_, ok = b.Edition(formal)
	assert.False(t, ok, "RemoveEdition removes a tone edition")
	assert.True(t, b.HasTarget("de"), "and leaves the language's own target")
}

// A write the applier refuses is the handler's error, not a silent loss.
func TestView_ARefusedWriteFailsTheHandler(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	bt := &tool.BaseTool{ToolName: "broken"}
	bt.Produce = func(v tool.VariantView) error {
		v.SetTargetRuns("fr", []model.Run{{}})
		return nil
	}
	err := dispatch(t, bt, b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), `tool "broken"`)
	assert.Contains(t, err.Error(), "refused")
	assert.False(t, b.HasTarget("fr"), "nothing is written")

	err = tool.WriteAs(context.Background(), b, "broken", func(v tool.VariantView) error {
		v.SetTargetRuns("fr", []model.Run{{}})
		return nil
	})
	require.Error(t, err, "a Process override writing through WriteAs sees the refusal too")
}

// A target rewrite through an edit plan carries the target's overlays across
// the rewrite (main-model P6), and a variant key spelled another way names the
// edition every reader reaches (P7).
func TestEditPlan_TargetOverlaysFollowAndKeysAreCanonical(t *testing.T) {
	b := model.NewBlock("b1", "Hello world")
	fr := model.Variant("fr")
	b.SetTargetRuns("fr", []model.Run{model.TextR("Bonjour le monde entier")})
	b.SetSegmentation(&fr, []model.Span{{ID: "s1", Range: model.RangeAnchor(b.TargetRuns("fr"), 8, 23)}})
	bt := &tool.BaseTool{ToolName: "probe"}
	bt.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
		var p tool.EditPlan
		p.SetTargetVariant(model.VariantKey{Locale: "fr"}, []model.Run{model.TextR("Salut")})
		p.SetTargetVariant(model.VariantKey{Locale: "nb_NO"}, []model.Run{model.TextR("Hei")})
		return p, nil
	}
	require.NoError(t, dispatch(t, bt, b))

	assert.Equal(t, "Salut", b.TargetText("fr"))
	if seg := b.SegmentationFor(&fr); seg != nil {
		for _, s := range seg.Spans {
			assert.True(t, s.Range.InBounds(b.TargetRuns("fr")), "no target span is left out of bounds")
		}
	}
	assert.Equal(t, "Hei", b.TargetText("nb-NO"))
	assert.NotContains(t, b.TargetLocales(), model.LocaleID("nb_NO"), "no target is filed under the spelling the plan used")
}

// A bilingual file whose two languages are one holds a target in the source
// language. A tool's target writes reach that target, through a plan or a
// view, and never the source; its status stays on the target ladder. A block
// with no such target refuses a target write in the source language rather
// than replace the source with it.
func TestTargetWrites_SameLanguageTarget(t *testing.T) {
	sameLanguage := func() *model.Block {
		b := model.NewBlock("b1", "colour source")
		b.SourceLocale = "en-US"
		b.SetEditionStatus(model.EditionKey{}, model.Status(model.SourceStatusWritten))
		b.SetTargetRuns("en-US", []model.Run{model.TextR("colour target")})
		b.SetEditionStatus(model.Variant("en-US"), model.Status(model.TargetStatusEstablished))
		return b
	}

	b := sameLanguage()
	bt := &tool.BaseTool{ToolName: "spell"}
	bt.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
		var p tool.EditPlan
		p.SetTarget("en-US", []model.Run{model.TextR("color target")})
		return p, nil
	}
	require.NoError(t, dispatch(t, bt, b))
	assert.Equal(t, "colour source", b.SourceText())
	assert.Equal(t, "color target", b.TargetText("en-US"))
	assert.Equal(t, []model.EditionKey{{}, {Locale: "en-US"}}, b.Editions())

	b = sameLanguage()
	err := tool.WriteAs(context.Background(), b, "pseudo", func(v tool.VariantView) error {
		v.SetTargetText("en-US", "[colour]")
		v.StampTargetProvenance("en-US", model.TargetStatusDraft, model.Origin{Tool: "pseudo"})
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, "colour source", b.SourceText())
	assert.Equal(t, model.SourceStatusWritten, sourceStatus(b))
	assert.Equal(t, "[colour]", b.TargetText("en-US"))
	assert.Equal(t, model.Status(model.TargetStatusDraft), target(t, b, "en-US").Status)

	b = model.NewBlock("b2", "colour source")
	b.SourceLocale = "en"
	b.SetEditionStatus(model.EditionKey{}, model.Status(model.SourceStatusWritten))
	err = tool.WriteAs(context.Background(), b, "pseudo", func(v tool.VariantView) error {
		v.SetTargetText("en", "[colour]")
		v.StampTargetProvenance("en", model.TargetStatusDraft, model.Origin{Tool: "pseudo"})
		return nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source language")
	assert.Equal(t, "colour source", b.SourceText())
	assert.Equal(t, model.SourceStatusWritten, sourceStatus(b))
	_, hasOrigin := b.SourceOrigin()
	assert.False(t, hasOrigin, "no tool origin reaches the source")

	err = tool.WriteAs(context.Background(), b, "pseudo", func(v tool.VariantView) error {
		v.SetEdition(model.Variant("en"), model.Edition{Runs: []model.Run{model.TextR("[colour]")},
			Status: model.Status(model.TargetStatusDraft), Origin: model.Origin{Tool: "pseudo"}})
		return nil
	})
	require.Error(t, err, "SetEdition writes targets, never the source")
	assert.Contains(t, err.Error(), "source language")
	assert.Equal(t, "colour source", b.SourceText())

	err = tool.WriteAs(context.Background(), b, "pseudo", func(v tool.VariantView) error {
		v.StampTargetProvenance("en", model.TargetStatusDraft, model.Origin{Tool: "pseudo"})
		return nil
	})
	require.NoError(t, err, "a stamp on a target the block lacks does nothing")
	assert.Equal(t, model.SourceStatusWritten, sourceStatus(b))
}

// Overlay writes go through the applier too, and keep every field of a span.
func TestView_OverlayWritesKeepWholeSpans(t *testing.T) {
	b := model.NewBlock("b1", "Alice met Bob")
	bt := &tool.BaseTool{ToolName: "ner"}
	bt.Annotate = func(v tool.BlockView) error {
		v.AddOverlaySpan(model.OverlayEntity, model.Span{ID: "e1", Range: model.RangeAnchor(v.SourceRuns(), 0, 5),
			Props: map[string]string{"k": "v"}, Value: &model.EntityAnnotation{Text: "Alice", Type: "PERSON"}})
		v.SetSegmentation(nil, []model.Span{{ID: "s1", Range: model.RangeAnchor(v.SourceRuns(), 0, 13)}})
		return nil
	}
	require.NoError(t, dispatch(t, bt, b))
	sp := b.OverlaySpan(model.OverlayEntity, "e1")
	require.NotNil(t, sp)
	assert.Equal(t, "v", sp.Props["k"])
	ent, ok := sp.Value.(*model.EntityAnnotation)
	require.True(t, ok)
	assert.Equal(t, "Alice", ent.Text)
	require.NotNil(t, b.SourceSegmentation())
	assert.Len(t, b.SourceSegmentation().Spans, 1)

	bt.Annotate = func(v tool.BlockView) error {
		v.AddOverlaySpan(model.OverlayEntity, model.Span{ID: "e2", Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 0, Offset: 99})})
		return nil
	}
	err := dispatch(t, bt, b)
	require.Error(t, err, "a span outside the content is refused")
	assert.Nil(t, b.OverlaySpan(model.OverlayEntity, "e2"))
}

// In-place text edits compile to one replace_text per pass, in order, so the
// second pass reads the text the first left; the codes stay, and a target
// rewritten in place and whole at once is refused.
func TestEditPlan_TextEditsApplyAsReplaceText(t *testing.T) {
	coded := []model.Run{
		model.TextR("Click "),
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<b>"}},
		model.TextR("Save"),
		{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"}},
		model.TextR(" now"),
	}
	edit := func(start, end int, text string) change.TextEdit {
		return change.TextEdit{Start: &start, End: &end, Text: text}
	}
	b := model.NewBlock("b1", "x")
	b.SetSourceRuns(coded)
	b.SetTargetRuns("fr", coded)
	bt := &tool.BaseTool{ToolName: "probe"}
	bt.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
		var p tool.EditPlan
		p.AddTextEdits(model.VariantKey{}, []change.TextEdit{edit(0, 5, "Press")})
		p.AddTextEdits(model.VariantKey{}, []change.TextEdit{edit(11, 14, "later")})
		p.AddTextEdits(model.Variant("fr"), []change.TextEdit{edit(6, 10, "Keep")})
		p.AddTextEdits(model.Variant("fr"), nil)
		return p, nil
	}
	require.NoError(t, dispatch(t, bt, b))
	assert.Equal(t, `Press <x id="1"/>Save<x id="/1"/> later`, model.RunsPlaceholderText(b.SourceRuns()))
	assert.Equal(t, `Click <x id="1"/>Keep<x id="/1"/> now`, model.RunsPlaceholderText(b.TargetRuns("fr")))

	var both tool.EditPlan
	both.SetTarget("fr", []model.Run{model.TextR("Salut")})
	both.AddTextEdits(model.Variant("fr"), []change.TextEdit{edit(0, 1, "c")})
	_, err := both.Ops(b)
	require.Error(t, err)

	var source tool.EditPlan
	text := "Salut"
	source.ReplaceAll = &text
	source.AddTextEdits(model.VariantKey{}, []change.TextEdit{edit(0, 1, "c")})
	_, err = source.Ops(b)
	require.Error(t, err)
}

// A plan built as a struct literal can key a target by any spelling of its
// language. Ops reads the maps by the canonical spelling, so fr-fr replaces
// the fr-FR target with what the plan holds for it, rather than with nothing,
// and two spellings of one edition are refused.
func TestEditPlan_NonCanonicalKeysNameTheirEdition(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SetTargetText("fr-FR", "Bonjour")
	p := tool.EditPlan{Targets: map[model.VariantKey][]model.Run{{Locale: "fr-fr"}: {model.TextR("Salut")}}}

	ops, err := p.Ops(b)
	require.NoError(t, err)
	require.Len(t, ops, 1)
	body, ok := ops[0].Body.(*change.SetContent)
	require.True(t, ok)
	assert.Equal(t, "Salut", model.RunsText(body.Runs))
	for _, r := range change.ApplyBlock(b, ops, change.BlockEnv{Actor: change.Actor{Kind: change.ActorTool, Name: "probe"}}) {
		require.Equal(t, change.OpApplied, r.Status, "%+v", r.Error)
	}
	assert.Equal(t, "Salut", b.TargetText("fr-FR"))

	two := tool.EditPlan{Targets: map[model.VariantKey][]model.Run{
		{Locale: "fr-fr"}: {model.TextR("Salut")},
		{Locale: "fr_FR"}: {model.TextR("Coucou")},
	}}
	_, err = two.Ops(b)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "two spellings")
}

// target returns the target edition b holds for loc, failing the test when it
// holds none.
func target(t *testing.T, b *model.Block, loc model.LocaleID) model.Edition {
	t.Helper()
	key := model.Variant(loc)
	require.False(t, b.IsSourceEdition(key), "%s names the source", loc)
	e, ok := b.Edition(key)
	require.True(t, ok, "no %s target", loc)
	return e
}
