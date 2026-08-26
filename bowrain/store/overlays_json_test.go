package store

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// overlaysFixture builds a block's worth of stand-off overlays covering every
// kind that must survive a store round-trip: segmentation (incl. an ignorable
// span), an entity overlay carrying a typed *EntityAnnotation Value (the
// entity→concept promote path), a term overlay carrying a typed
// *TermAnnotation Value, a term-candidate overlay, a QA overlay carrying only
// props, and a target-side (Variant-bearing) alignment overlay. Anchors use
// real run-index ranges.
func overlaysFixture() []model.Overlay {
	frVariant := model.VariantKey{Locale: model.LocaleFrench}
	return []model.Overlay{
		{
			Type:  model.OverlaySegmentation,
			Layer: model.LayerPrimary,
			Spans: []model.Span{
				{ID: "s1", Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 0, Offset: 4})},
				{
					ID:    "s2",
					Range: model.SpanAnchor(model.RunPos{Run: 0, Offset: 5}, model.RunPos{Run: 0, Offset: 11}),
					Props: map[string]string{model.SpanPropIgnorable: "true"},
				},
			},
		},
		{
			Type: model.OverlayEntity,
			Spans: []model.Span{{
				ID:    "entity:0",
				Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 0, Offset: 4}),
				Value: &model.EntityAnnotation{
					Text:   "Acme",
					Type:   model.EntityOrganization,
					Locale: model.LocaleEnglish,
					DNT:    true,
					Source: "ner",
				},
			}},
		},
		{
			Type: model.OverlayTerm,
			Spans: []model.Span{{
				ID:    "term:0",
				Range: model.SpanAnchor(model.RunPos{Run: 0, Offset: 5}, model.RunPos{Run: 0, Offset: 11}),
				Value: &model.TermAnnotation{
					SourceTerm:  "widget",
					ConceptID:   "c-42",
					Status:      model.TermPreferred,
					Score:       1,
					MatchType:   model.MatchStrategyExact,
					TargetTerms: []model.TermRef{{Text: "gadget", Locale: model.LocaleFrench, Status: model.TermPreferred}},
				},
			}},
		},
		{
			Type: model.OverlayTermCandidate,
			Spans: []model.Span{{
				ID:    "term-candidate:0",
				Range: model.SpanAnchor(model.RunPos{Run: 0, Offset: 5}, model.RunPos{Run: 0, Offset: 11}),
				Value: &model.TermCandidateAnnotation{Text: "widget", Locale: model.LocaleEnglish},
			}},
		},
		{
			Type: model.OverlayQA,
			Spans: []model.Span{{
				ID:    "qa:0",
				Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 0, Offset: 11}),
				Props: map[string]string{"rule": "length", "severity": "warn"},
			}},
		},
		{
			Type:    model.OverlayAlignment,
			Variant: &frVariant,
			Spans: []model.Span{{
				ID:    "a0",
				Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 0, Offset: 4}),
				Props: map[string]string{"target": "0:0-0:6"},
			}},
		},
	}
}

// TestOverlaysCodec_RoundTripsEveryKind asserts the JSON codec faithfully
// round-trips every overlay kind — anchors, props, variant, and typed span
// values — with no loss.
func TestOverlaysCodec_RoundTripsEveryKind(t *testing.T) {
	want := overlaysFixture()

	data, err := MarshalOverlays(want)
	require.NoError(t, err)

	got, err := UnmarshalOverlays(data)
	require.NoError(t, err)

	require.Equal(t, want, got, "overlays must survive marshal→unmarshal identical to input")

	// Entity case specifically: the value must rehydrate to the concrete
	// *EntityAnnotation the entity handler / entity→concept promote path asserts
	// on — not a generic map — or the promote path breaks.
	var entity *model.Overlay
	for i := range got {
		if got[i].Type == model.OverlayEntity {
			entity = &got[i]
		}
	}
	require.NotNil(t, entity)
	require.Len(t, entity.Spans, 1)
	ea, ok := entity.Spans[0].Value.(*model.EntityAnnotation)
	require.True(t, ok, "entity span value must rehydrate to *EntityAnnotation")
	assert.Equal(t, "Acme", ea.Text)
	assert.Equal(t, model.EntityOrganization, ea.Type)
	assert.True(t, ea.DNT)

	// Term case: rehydrates to *TermAnnotation with its target terms intact.
	term := got[2]
	require.Equal(t, model.OverlayTerm, term.Type)
	ta, ok := term.Spans[0].Value.(*model.TermAnnotation)
	require.True(t, ok, "term span value must rehydrate to *TermAnnotation")
	assert.Equal(t, "c-42", ta.ConceptID)
	require.Len(t, ta.TargetTerms, 1)
	assert.Equal(t, "gadget", ta.TargetTerms[0].Text)
}

// TestOverlaysCodec_EmptyIsByteStable asserts nil/empty overlays encode to the
// byte-stable "[]" default and decode back to nil — an unset round-trip is a
// no-op.
func TestOverlaysCodec_EmptyIsByteStable(t *testing.T) {
	for _, in := range [][]model.Overlay{nil, {}} {
		data, err := MarshalOverlays(in)
		require.NoError(t, err)
		assert.Equal(t, "[]", string(data), "empty overlays must encode to the stable [] default")

		got, err := UnmarshalOverlays(data)
		require.NoError(t, err)
		assert.Nil(t, got)
	}

	// The column default / empty / null forms all decode to nil.
	for _, raw := range []string{"", "[]", "null", "  []  "} {
		got, err := UnmarshalOverlays([]byte(raw))
		require.NoError(t, err)
		assert.Nil(t, got, "raw %q must decode to nil overlays", raw)
	}
}

// TestOverlaysCodec_UnknownKindDegradesGracefully asserts an unregistered span
// value type is preserved as a GenericAnnotation rather than dropped.
func TestOverlaysCodec_UnknownKindDegradesGracefully(t *testing.T) {
	in := []model.Overlay{{
		Type: model.OverlayType("plugin-custom"),
		Spans: []model.Span{{
			ID:    "x0",
			Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 0, Offset: 3}),
			Value: &model.GenericAnnotation{Kind: "vendor:thing", Fields: map[string]any{"k": "v"}},
		}},
	}}

	data, err := MarshalOverlays(in)
	require.NoError(t, err)
	got, err := UnmarshalOverlays(data)
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Len(t, got[0].Spans, 1)
	ga, ok := got[0].Spans[0].Value.(*model.GenericAnnotation)
	require.True(t, ok)
	assert.Equal(t, "vendor:thing", ga.Kind)
	assert.Equal(t, "v", ga.Fields["k"])
}
