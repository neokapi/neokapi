package store

import (
	"testing"

	bowsync "github.com/neokapi/neokapi/bowrain/core/sync"
	"github.com/neokapi/neokapi/bowrain/core/synctest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSyncOverlayFullChainRoundTrip proves the whole kapi→wire→store→pull chain
// is lossless for the content the store persists — the headline being that
// stand-off overlays (a term/entity/segmentation marked in the kapi CLI) survive
// push→store→pull, not just the model→proto conversion in isolation.
//
// The chain, exactly as production runs it:
//
//	kapi:   model.Block  --BlockToProto-->  SyncBlock        (client push encode)
//	server: SyncBlock    --ProtoToBlock-->  model.Block      (worker ingest)
//	store:               --StoreBlocks-->   Postgres
//	                     --GetBlock-->      model.Block
//	server: model.Block  --BlockToProto-->  SyncBlock        (pull encode)
//	kapi:   SyncBlock    --ProtoToBlock-->  model.Block      (client pull decode)
//
// Requires a Postgres testcontainer (skipped in -short without
// BOWRAIN_TEST_POSTGRES_URL — see pgtest.NewTestDB).
func TestSyncOverlayFullChainRoundTrip(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	const item = "locales/en.json"
	orig := synctest.KitchenSinkBlock()

	// 1. Client push encode → 2. server ingest decode.
	ingested, err := bowsync.ProtoToBlock(bowsync.BlockToProto(orig, item))
	require.NoError(t, err)

	// 3. Store the ingested block. Use the item-less StoreBlocks path so the
	// block keeps its authored ID (the item-scoped path re-mints internal IDs);
	// both share the same overlay-persistence column, which is what we exercise.
	require.NoError(t, s.StoreBlocks(ctx, p.ID, "", []*model.Block{ingested}))

	// 4. Pull it back out of the store.
	stored, err := s.GetBlock(ctx, p.ID, "", orig.ID)
	require.NoError(t, err)
	require.NotNil(t, stored)

	// 5. Server pull encode → 6. client pull decode.
	pulled, err := bowsync.ProtoToBlock(bowsync.BlockToProto(stored.Block, item))
	require.NoError(t, err)

	// Headline: every overlay kind survives the whole chain with anchors, props,
	// variant, and typed span values intact.
	require.Equal(t, orig.Overlays, pulled.Overlays,
		"every overlay kind must survive kapi→wire→store→pull losslessly")

	// The entity/term typed values must still rehydrate to their concrete type
	// after the whole chain (the entity→concept promote path depends on this).
	ent := pulled.OverlayOf(model.OverlayEntity)
	require.NotNil(t, ent)
	ea, ok := ent.Spans[0].Value.(*model.EntityAnnotation)
	require.True(t, ok, "entity span value must rehydrate to *EntityAnnotation after the full chain")
	assert.Equal(t, "Acme", ea.Text)
	assert.True(t, ea.DNT)

	term := pulled.OverlayOf(model.OverlayTerm)
	require.NotNil(t, term)
	ta, ok := term.Spans[0].Value.(*model.TermAnnotation)
	require.True(t, ok, "term span value must rehydrate to *TermAnnotation after the full chain")
	assert.Equal(t, "c-42", ta.ConceptID)
	require.Len(t, ta.TargetTerms, 1)
	assert.Equal(t, "monde", ta.TargetTerms[0].Text)

	// Core content the store persists also survives the chain.
	assert.Equal(t, orig.Source, pulled.Source, "source runs (incl. every kind) survive the chain")
	assert.Equal(t, orig.Properties, pulled.Properties, "properties survive the chain")
	assert.Equal(t, orig.TargetText(model.LocaleFrench), pulled.TargetText(model.LocaleFrench), "fr target survives")
	assert.Equal(t, orig.TargetText(model.LocaleGerman), pulled.TargetText(model.LocaleGerman), "de target survives")
}
