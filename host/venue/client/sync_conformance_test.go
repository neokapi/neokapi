package client

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue/venuetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This is the parity guard for the kapi↔bowrain sync JSON converter
// (model.Block → SyncBlock JSON → model.Block). It round-trips the shared
// kitchen-sink fixture and asserts the converter carries the same content the
// proto push path does — overlays, skeleton parts, provenance, and the
// source-locale / is-referent fields included — so a term/entity/segmentation
// marked in kapi survives the projection.
//
// Scope note: this exercises the CONVERTER in memory, not an end-to-end pull.
// A real pull reads a StoredBlock, and the content store has no skeleton column
// (skeleton is a connector-edge artifact that does not persist), so a pulled
// block's Skeleton is always empty regardless of what the converter can carry.
// The assertion below therefore pins converter fidelity, not store fidelity.
//
// See web/docs/contribute/implementation/foundations/content-parity.md.

func TestSyncBlockJSONKitchenSinkRoundTrip(t *testing.T) {
	orig := venuetest.KitchenSinkBlock()

	wire := BlockToSyncBlock(orig, "kitchen.json")

	// The overlay + skeleton blobs must survive the converter losslessly.
	require.NotEmpty(t, wire.Overlays, "overlays must survive the JSON converter")
	require.NotEmpty(t, wire.Skeleton, "skeleton must survive the JSON converter")
	assert.Equal(t, string(model.LocaleEnglish), wire.SourceLocale, "source locale rides the wire")
	assert.True(t, wire.IsReferent, "is-referent rides the wire")

	got := SyncBlockToBlock(wire)

	// Identity is derived (recomputed), not carried; the JSON path never sets it.
	want := venuetest.KitchenSinkBlock()
	want.Identity = nil
	got.Identity = nil

	require.Equal(t, want, got, "model → JSON SyncBlock → model must be lossless for a fully-populated Block")
}

// TestSyncBlockJSONOverlaysRoundTrip pins the overlay-specific guarantee the
// push side already has: every overlay kind survives the JSON pull, with typed
// span values rehydrated to their concrete type.
func TestSyncBlockJSONOverlaysRoundTrip(t *testing.T) {
	orig := venuetest.KitchenSinkBlock()

	got := SyncBlockToBlock(BlockToSyncBlock(orig, "kitchen.json"))

	require.Equal(t, orig.Overlays, got.Overlays, "every overlay kind must survive the JSON pull losslessly")

	ent := got.OverlayOf(model.OverlayEntity)
	require.NotNil(t, ent)
	_, ok := ent.Spans[0].Value.(*model.EntityAnnotation)
	assert.True(t, ok, "entity span value must rehydrate to *EntityAnnotation on pull")
}

// A translation filed under no language rides the JSON wire under the empty
// target key, and its segmentation rides with it: the decoded block holds the
// spans on that translation and none on the source.
func TestSyncBlockJSONCarriesTheOverlaysOfATranslationUnderNoLanguage(t *testing.T) {
	orig := model.NewBlock("b1", "Some files")
	orig.SetTargetRuns("", []model.Run{model.TextR("Eine Datei"), model.TextR("Viele Dateien")})
	spans := []model.Span{
		{ID: "n0", Range: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 1})},
		{ID: "n1", Range: model.SpanAnchor(model.RunPos{Run: 1}, model.RunPos{Run: 2})},
	}
	orig.SetTargetSegmentation("", spans)

	wire := BlockToSyncBlock(orig, "app.ts")
	require.Contains(t, wire.Targets, "")
	got := SyncBlockToBlock(wire)

	assert.Equal(t, "Eine DateiViele Dateien", got.TargetText(""))
	assert.Nil(t, got.SourceSegmentation(), "the spans are the translation's, never the source's")
	assert.Empty(t, got.Overlays)
	seg := got.TargetSegmentation("")
	require.NotNil(t, seg)
	assert.Equal(t, spans, seg.Spans)
}
