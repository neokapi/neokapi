package venue_test

import (
	"reflect"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/core/venue/venuetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file is the durable content-model parity guard for the kapi↔bowrain sync
// PUSH wire (the typed proto path, core/venue). It round-trips the shared
// kitchen-sink fixture (core/venue/venuetest) — every Run kind, every Overlay
// kind, multiple target variants, annotations, provenance, and every
// scalar/struct field on model.Block — and asserts model → proto → model is
// deep-equal to the original. A reflect-based completeness guard then fails
// loudly if model.Block grows a field the kitchen sink doesn't cover, and
// enumerated kind tables fail if a new Run/Overlay kind is added without wiring
// it through the converter + fixture.
//
// The invariant this protects: a Block kapi has locally survives
// push (BlockToProto) → wire → decode (ProtoToBlock) losslessly. See
// web/docs/contribute/implementation/foundations/content-parity.md.

// TestKitchenSinkRoundTrip is the parity gate: model → proto → model must be
// deep-equal to the original for a Block with every field, run kind, and overlay
// kind populated.
func TestKitchenSinkRoundTrip(t *testing.T) {
	orig := venuetest.KitchenSinkBlock()

	proto := venue.BlockToProto(orig, "kitchen.json")
	require.NotNil(t, proto)

	got, err := venue.ProtoToBlock(proto)
	require.NoError(t, err)
	require.NotNil(t, got)

	// Identity is derived (not carried); compare against a fresh fixture with it
	// left nil on both sides.
	want := venuetest.KitchenSinkBlock()
	want.Identity = nil
	got.Identity = nil

	require.Equal(t, want, got, "model → proto → model must be lossless for a fully-populated Block")
}

// TestKitchenSinkCoversEveryRunKind proves the fixture's source really exercises
// every Run kind — so the round-trip above is a real per-kind guarantee, and a
// newly-added Run kind trips this until it is added to the fixture.
func TestKitchenSinkCoversEveryRunKind(t *testing.T) {
	present := map[model.RunKind]bool{}
	for _, r := range venuetest.KitchenSinkBlock().Source {
		present[r.Kind()] = true
	}
	for _, k := range venuetest.AllRunKinds {
		assert.Truef(t, present[k], "kitchen-sink source must contain a %q run (add it + wire the converter)", k)
	}
	// Cross-check the enumerated table itself covers the model's discriminators:
	// if this count drifts, model.RunKind gained a constant not in AllRunKinds.
	assert.Len(t, venuetest.AllRunKinds, 7, "AllRunKinds must enumerate every model.RunKind (add the new kind + converter arm)")
}

// TestKitchenSinkCoversEveryOverlayKind proves the fixture carries every overlay
// kind and that each survives the round-trip with its type, anchors, props,
// variant, and typed value intact.
func TestKitchenSinkCoversEveryOverlayKind(t *testing.T) {
	orig := venuetest.KitchenSinkBlock()

	got, err := venue.ProtoToBlock(venue.BlockToProto(orig, "kitchen.json"))
	require.NoError(t, err)

	for _, kind := range venuetest.AllOverlayKinds {
		var origOverlay, gotOverlay *model.Overlay
		for i := range orig.Overlays {
			if orig.Overlays[i].Type == kind {
				origOverlay = &orig.Overlays[i]
			}
		}
		for i := range got.Overlays {
			if got.Overlays[i].Type == kind {
				gotOverlay = &got.Overlays[i]
			}
		}
		require.NotNilf(t, origOverlay, "kitchen-sink must include a %q overlay (add it + wire the converter)", kind)
		require.NotNilf(t, gotOverlay, "%q overlay must survive the sync round-trip", kind)
		assert.Equalf(t, origOverlay, gotOverlay, "%q overlay must round-trip losslessly", kind)
	}

	// Typed span values must rehydrate to their concrete type (not a generic map),
	// which downstream consumers (e.g. the entity→concept promote path) depend on.
	ent := got.OverlayOf(model.OverlayEntity)
	require.NotNil(t, ent)
	_, ok := ent.Spans[0].Value.(*model.EntityAnnotation)
	assert.True(t, ok, "entity span value must rehydrate to *EntityAnnotation")

	term := got.OverlayOf(model.OverlayTerm)
	require.NotNil(t, term)
	_, ok = term.Spans[0].Value.(*model.TermAnnotation)
	assert.True(t, ok, "term span value must rehydrate to *TermAnnotation")
}

// TestUnknownOverlayKindDegradesGracefully proves a plugin-defined / future
// overlay kind carrying an unregistered payload round-trips by type name + JSON
// (as a GenericAnnotation) rather than being dropped or panicking.
func TestUnknownOverlayKindDegradesGracefully(t *testing.T) {
	b := model.NewBlock("b1", "John visited Paris")
	b.AddOverlaySpan(model.OverlayType("x-plugin-marks"), model.Span{
		ID:    "m0",
		Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 1}),
		Value: &model.GenericAnnotation{Kind: "vendor:thing", Fields: map[string]any{"weight": "high"}},
	})

	got, err := venue.ProtoToBlock(venue.BlockToProto(b, "item.json"))
	require.NoError(t, err)

	o := got.OverlayOf(model.OverlayType("x-plugin-marks"))
	require.NotNil(t, o, "plugin-defined overlay must survive")
	require.Len(t, o.Spans, 1)
	require.Equal(t, model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 1}), o.Spans[0].Range, "span anchor survives")
	ga, ok := o.Spans[0].Value.(*model.GenericAnnotation)
	require.True(t, ok, "unregistered payload round-trips as GenericAnnotation, not dropped")
	assert.Equal(t, "vendor:thing", ga.Kind, "type name preserved")
	// On the proto wire the canonical protoconvert codec captures an unregistered
	// payload's whole JSON into Fields (nested under "fields") rather than
	// dropping it — graceful degradation, no data loss. The value is recoverable.
	assert.NotEmpty(t, ga.Fields, "unregistered payload data is preserved, not dropped")
}

// TestBlockFixtureIsComplete is the reflect-based drift guard: it fails loudly
// if any exported field of model.Block is left zero in the kitchen-sink fixture.
// A new field added to model.Block will be zero here (nobody set it) and trip
// this test, forcing the author to decide whether it must round-trip — and, if
// so, to wire it through the .proto, the converter (both directions), and the
// fixture. Fields that are intentionally derived (not carried) are allow-listed
// in venuetest.BlockDerivedFields with a stated reason.
func TestBlockFixtureIsComplete(t *testing.T) {
	b := venuetest.KitchenSinkBlock()
	v := reflect.ValueOf(*b)
	typ := v.Type()

	for i := range typ.NumField() {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		if reason, ok := venuetest.BlockDerivedFields[field.Name]; ok {
			// Allow-listed: assert it is genuinely derived (left zero) so the
			// list can't silently mask a field that IS being populated.
			assert.Truef(t, v.Field(i).IsZero(),
				"model.Block.%s is allow-listed as %q but the fixture sets it — remove it from BlockDerivedFields or stop setting it",
				field.Name, reason)
			continue
		}
		assert.Falsef(t, v.Field(i).IsZero(),
			"model.Block.%s is zero in the kitchen-sink fixture — a new Block field must be added to venuetest.KitchenSinkBlock() AND carried by the sync converter (or allow-listed in venuetest.BlockDerivedFields if derived). See content-parity.md.",
			field.Name)
	}
}

// TestOriginFixtureIsComplete is the same drift guard one level down, on
// model.Origin. TestBlockFixtureIsComplete only walks Block's own fields, so a
// new Origin field would be zero inside a non-zero Targets map and slip past it
// — and the round-trip above would then "pass" while silently dropping it.
// Provenance is the one record that cannot be reconstructed after the fact, so
// a field added to it must be populated in the fixture (and therefore carried by
// both converters) rather than quietly not carried.
func TestOriginFixtureIsComplete(t *testing.T) {
	b := venuetest.KitchenSinkBlock()

	var populated *model.Origin
	for _, tgt := range b.Targets {
		o := tgt.Origin
		if populated == nil || countSetFields(o) > countSetFields(*populated) {
			cp := o
			populated = &cp
		}
	}
	require.NotNil(t, populated, "kitchen-sink fixture must carry at least one target with an Origin")

	v := reflect.ValueOf(*populated)
	typ := v.Type()
	for i := range typ.NumField() {
		field := typ.Field(i)
		if !field.IsExported() {
			continue
		}
		assert.Falsef(t, v.Field(i).IsZero(),
			"model.Origin.%s is zero on every kitchen-sink target — a new Origin field must be populated in venuetest.KitchenSinkBlock() AND carried by both sync converters (core/venue and host/venue/client). See content-parity.md.",
			field.Name)
	}
}

// countSetFields reports how many exported fields of an Origin are non-zero, so
// the completeness guard picks the fixture's fully-populated target rather than
// the deliberately sparse one.
func countSetFields(o model.Origin) int {
	n := 0
	for _, fv := range reflect.ValueOf(o).Fields() {
		if !fv.IsZero() {
			n++
		}
	}
	return n
}
