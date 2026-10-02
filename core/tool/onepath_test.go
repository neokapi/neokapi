package tool_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	tg := b.Target("fr")
	require.NotNil(t, tg)
	assert.Equal(t, model.TargetStatusDraft, tg.Status, "the stamp lands on the target the handler just created")
	assert.Equal(t, "probe", tg.Origin.Tool)
}

// A tool that rewrites a target makes it a draft, since nobody has read its
// wording, and keeps the origin that names who translated it; its provenance
// stamp is how the tool records what it produced.
func TestView_TargetWritesTakeTheToolConsequence(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SetTarget("fr", &model.Target{Runs: []model.Run{model.TextR("Salut")}, Status: model.TargetStatusEstablished, Origin: model.Origin{Kind: model.OriginHuman}})
	bt := &tool.BaseTool{ToolName: "case"}
	bt.Produce = func(v tool.VariantView) error {
		v.SetTargetText("fr", "Salut")
		return nil
	}
	require.NoError(t, dispatch(t, bt, b))
	assert.Equal(t, model.TargetStatusEstablished, b.Target("fr").Status, "a write that changes nothing keeps the approval")

	bt.Produce = func(v tool.VariantView) error {
		v.SetTargetText("fr", "SALUT")
		return nil
	}
	require.NoError(t, dispatch(t, bt, b))
	assert.Equal(t, "SALUT", b.TargetText("fr"))
	assert.Equal(t, model.TargetStatusDraft, b.Target("fr").Status)
	assert.Equal(t, model.OriginHuman, b.Target("fr").Origin.Kind)

	bt.Produce = func(v tool.VariantView) error {
		v.SetTarget("de", &model.Target{Runs: []model.Run{model.TextR("Hallo")}, Status: model.TargetStatusDraft, Score: 0.5})
		v.RemoveTarget("fr")
		return nil
	}
	require.NoError(t, dispatch(t, bt, b))
	assert.False(t, b.HasTarget("fr"))
	require.NotNil(t, b.Target("de"))
	assert.Equal(t, model.TargetStatusDraft, b.Target("de").Status)
	assert.InDelta(t, 0.5, b.Target("de").Score, 0)
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
	_, raw := b.Targets[model.VariantKey{Locale: "nb_NO"}]
	assert.False(t, raw, "no target is filed under the spelling the plan used")
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
		b.SourceStatus = model.SourceStatusWritten
		b.SetTarget("en-US", &model.Target{Runs: []model.Run{model.TextR("colour target")}, Status: model.TargetStatusEstablished})
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
	assert.Equal(t, model.SourceStatusWritten, b.SourceStatus)
	assert.Equal(t, "[colour]", b.TargetText("en-US"))
	assert.Equal(t, model.TargetStatusDraft, b.Target("en-US").Status)

	b = model.NewBlock("b2", "colour source")
	b.SourceLocale = "en"
	b.SourceStatus = model.SourceStatusWritten
	err = tool.WriteAs(context.Background(), b, "pseudo", func(v tool.VariantView) error {
		v.SetTargetText("en", "[colour]")
		v.StampTargetProvenance("en", model.TargetStatusDraft, model.Origin{Tool: "pseudo"})
		return nil
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "source language")
	assert.Equal(t, "colour source", b.SourceText())
	assert.Equal(t, model.SourceStatusWritten, b.SourceStatus)
	_, hasOrigin := b.SourceOrigin()
	assert.False(t, hasOrigin, "no tool origin reaches the source")

	err = tool.WriteAs(context.Background(), b, "pseudo", func(v tool.VariantView) error {
		v.StampTargetProvenance("en", model.TargetStatusDraft, model.Origin{Tool: "pseudo"})
		return nil
	})
	require.NoError(t, err, "a stamp on a target the block lacks does nothing")
	assert.Equal(t, model.SourceStatusWritten, b.SourceStatus)
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
