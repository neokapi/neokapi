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

// A tool's write keeps the target's status and provenance; its provenance
// stamp is how the tool records what it produced.
func TestView_TargetWritesKeepProvenance(t *testing.T) {
	b := model.NewBlock("b1", "Hello")
	b.SetTarget("fr", &model.Target{Runs: []model.Run{model.TextR("Salut")}, Status: model.TargetStatusEstablished, Origin: model.Origin{Kind: model.OriginHuman}})
	bt := &tool.BaseTool{ToolName: "case"}
	bt.Produce = func(v tool.VariantView) error {
		v.SetTargetText("fr", "SALUT")
		return nil
	}
	require.NoError(t, dispatch(t, bt, b))
	assert.Equal(t, "SALUT", b.TargetText("fr"))
	assert.Equal(t, model.TargetStatusEstablished, b.Target("fr").Status)
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
