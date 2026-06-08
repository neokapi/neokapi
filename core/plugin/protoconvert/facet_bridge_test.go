package protoconvert_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/protoconvert"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests pin the facet-vocabulary bridge contract: overlays
// (term, entity, …) and plugin-defined facet types cross the gRPC bridge fully
// — type, span ranges, props and typed values — instead of being dropped.
// block annotations keep crossing as `annotations` and segmentation as the
// segment boundaries, so nothing is double-encoded.

const pluginOverlay model.OverlayType = "x-plugin-marks"

// buildOverlayBlock returns a block carrying one of each facet flavour.
func buildOverlayBlock() *model.Block {
	b := model.NewBlock("b1", "John Smith visited Paris yesterday")

	// Positional, built-in facets with typed values.
	b.AddOverlaySpan(model.OverlayEntity, model.Span{
		ID:    "entity:0",
		Range: model.RunRangeForBytes(b.Source, 0, 10),
		Value: &model.EntityAnnotation{Text: "John Smith", Type: model.EntityPerson, DNT: true},
	})
	b.AddOverlaySpan(model.OverlayTerm, model.Span{
		ID:    "term:0",
		Range: model.RunRangeForBytes(b.Source, 19, 24),
		Props: map[string]string{"strength": "preferred"},
		Value: &model.TermAnnotation{SourceTerm: "Paris", ConceptID: "c-1"},
	})

	// A plugin-defined overlay, carrying an arbitrary (unregistered)
	// payload — it must still survive by type name + JSON.
	b.AddOverlaySpan(pluginOverlay, model.Span{
		ID:    "m0",
		Range: model.RunRangeForBytes(b.Source, 0, 4),
		Value: &model.GenericAnnotation{Kind: "x-mark", Fields: map[string]any{"weight": "high"}},
	})

	// A block annotation (must keep crossing as an annotation, not an overlay).
	b.SetAnno("note", &model.NoteAnnotation{Text: "reviewer note"})

	// A segmentation overlay (must never appear in the overlays field — it is
	// reconstructed from the segment boundaries).
	b.Overlays = append(b.Overlays, model.Overlay{
		Type:  model.OverlaySegmentation,
		Spans: []model.Span{{ID: "s1", Range: model.RunRange{StartRun: 0, EndRun: 1}}},
	})
	return b
}

func assertRoundTripped(t *testing.T, got *model.Block) {
	t.Helper()

	// entity overlay.
	es := got.OverlaySpan(model.OverlayEntity, "entity:0")
	require.NotNil(t, es, "entity overlay span must survive the bridge")
	ea, ok := es.Value.(*model.EntityAnnotation)
	require.True(t, ok)
	assert.Equal(t, "John Smith", ea.Text)
	assert.Equal(t, model.EntityPerson, ea.Type)
	assert.True(t, ea.DNT)
	start, end := es.Range.ByteSpan(got.Source)
	assert.Equal(t, 0, start)
	assert.Equal(t, 10, end)

	// Term facet (with props).
	ts := got.OverlaySpan(model.OverlayTerm, "term:0")
	require.NotNil(t, ts)
	ta, ok := ts.Value.(*model.TermAnnotation)
	require.True(t, ok)
	assert.Equal(t, "Paris", ta.SourceTerm)
	assert.Equal(t, "preferred", ts.Props["strength"])

	// Plugin facet: type preserved; unregistered payload degrades to a generic
	// annotation but keeps its type name (no data loss of identity).
	pf := got.OverlayOf(pluginOverlay)
	require.NotNil(t, pf, "plugin-defined overlay must survive")
	require.Len(t, pf.Spans, 1)
	ga, ok := pf.Spans[0].Value.(*model.GenericAnnotation)
	require.True(t, ok, "unregistered payload round-trips as GenericAnnotation")
	assert.Equal(t, "x-mark", ga.Kind)

	// Block-scoped note still present.
	n, ok := model.AnnoAs[*model.NoteAnnotation](got, "note")
	require.True(t, ok)
	assert.Equal(t, "reviewer note", n.Text)
}

func TestBlockOverlayRoundTrip(t *testing.T) {
	b := buildOverlayBlock()
	proto := protoconvert.BlockToProto(b)
	require.NotNil(t, proto)

	// The overlays field carries exactly the positional, non-segmentation overlays
	// (entity, term, plugin) — not the note and not segmentation.
	require.Len(t, proto.Overlays, 3)
	for _, o := range proto.Overlays {
		assert.NotEqual(t, string(model.OverlaySegmentation), o.Type, "segmentation must not be in overlays")
		assert.NotEqual(t, string(model.AnnoNote), o.Type, "block-scoped note must not be in overlays")
	}

	assertRoundTripped(t, protoconvert.ProtoToBlock(proto))
}

func TestContentBlockOverlayRoundTrip(t *testing.T) {
	b := buildOverlayBlock()
	part := &model.Part{Type: model.PartBlock, Resource: b}

	cb := protoconvert.PartToContentBlock(part)
	require.NotNil(t, cb)
	require.Len(t, cb.Overlays, 3)

	got := protoconvert.ContentBlockToPart(cb)
	require.Equal(t, model.PartBlock, got.Type)
	assertRoundTripped(t, got.Resource.(*model.Block))
}

// No overlays → no overlays emitted (nil, not an empty slice churn).
func TestBlockNoOverlays(t *testing.T) {
	b := model.NewBlock("b1", "plain")
	b.SetAnno("note", &model.NoteAnnotation{Text: "hi"})
	proto := protoconvert.BlockToProto(b)
	assert.Empty(t, proto.Overlays)
	// The block-scoped note still crosses as an annotation.
	got := protoconvert.ProtoToBlock(proto)
	_, ok := model.AnnoAs[*model.NoteAnnotation](got, "note")
	assert.True(t, ok)
}
