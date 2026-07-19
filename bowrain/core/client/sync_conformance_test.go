package client

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/synctest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This is the parity guard for the kapi↔bowrain sync PULL wire (the JSON
// projection: StoredBlock → SyncBlock JSON → model.Block). It round-trips the
// shared kitchen-sink fixture and asserts the JSON pull path carries the same
// content the proto push path does — overlays, skeleton parts, provenance, and
// the new source-locale / is-referent fields included — so a term/entity/
// segmentation marked in kapi survives the pull, not just the push.
//
// See web/docs/contribute/notes-internal/content-parity.md.

func TestSyncBlockJSONKitchenSinkRoundTrip(t *testing.T) {
	orig := synctest.KitchenSinkBlock()

	wire := BlockToSyncBlock(orig, "kitchen.json")

	// The overlay + skeleton blobs must actually be populated on the wire.
	require.NotEmpty(t, wire.Overlays, "overlays must ride the JSON pull wire")
	require.NotEmpty(t, wire.Skeleton, "skeleton must ride the JSON pull wire")
	assert.Equal(t, string(model.LocaleEnglish), wire.SourceLocale, "source locale rides the wire")
	assert.True(t, wire.IsReferent, "is-referent rides the wire")

	got := SyncBlockToBlock(wire)

	// Identity is derived (recomputed), not carried; the JSON path never sets it.
	want := synctest.KitchenSinkBlock()
	want.Identity = nil
	got.Identity = nil

	require.Equal(t, want, got, "model → JSON SyncBlock → model must be lossless for a fully-populated Block")
}

// TestSyncBlockJSONOverlaysRoundTrip pins the overlay-specific guarantee the
// push side already has: every overlay kind survives the JSON pull, with typed
// span values rehydrated to their concrete type.
func TestSyncBlockJSONOverlaysRoundTrip(t *testing.T) {
	orig := synctest.KitchenSinkBlock()

	got := SyncBlockToBlock(BlockToSyncBlock(orig, "kitchen.json"))

	require.Equal(t, orig.Overlays, got.Overlays, "every overlay kind must survive the JSON pull losslessly")

	ent := got.OverlayOf(model.OverlayEntity)
	require.NotNil(t, ent)
	_, ok := ent.Spans[0].Value.(*model.EntityAnnotation)
	assert.True(t, ok, "entity span value must rehydrate to *EntityAnnotation on pull")
}
