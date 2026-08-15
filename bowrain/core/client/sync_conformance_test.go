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
