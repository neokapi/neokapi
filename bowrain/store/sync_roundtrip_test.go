package store

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/neokapi/neokapi/core/venue/venuetest"
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
	orig := venuetest.KitchenSinkBlock()

	// 1. Client push encode → 2. server ingest decode.
	ingested, err := venue.ProtoToBlock(venue.BlockToProto(orig, item))
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
	pulled, err := venue.ProtoToBlock(venue.BlockToProto(stored.Block, item))
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
	origSrc, _ := orig.Edition(model.EditionKey{})
	pulledSrc, _ := pulled.Edition(model.EditionKey{})
	assert.Equal(t, origSrc.Runs, pulledSrc.Runs, "source runs (incl. every kind) survive the chain")
	assert.Equal(t, orig.Properties, pulled.Properties, "properties survive the chain")
	assert.Equal(t, orig.TargetText(model.LocaleFrench), pulled.TargetText(model.LocaleFrench), "fr target survives")
	assert.Equal(t, orig.TargetText(model.LocaleGerman), pulled.TargetText(model.LocaleGerman), "de target survives")

	// Every edition survives under its own key with its status, the
	// same-language channel edition among them, and none folds into the
	// edition the block was read in.
	type held struct {
		text   string
		status model.Status
	}
	// The item-less store path keeps no source locale, so the edition the
	// block was read in is compared under the zero key, which names it on
	// either side.
	editions := func(b *model.Block) map[model.EditionKey]held {
		out := map[model.EditionKey]held{}
		for k, e := range b.EachEdition {
			if b.IsSourceEdition(k) {
				k = model.EditionKey{}
			}
			out[k] = held{model.RunsText(e.Runs), e.Status}
		}
		return out
	}
	assert.Equal(t, editions(orig), editions(pulled), "every edition survives the chain under its key")
	short, ok := pulled.Edition(model.EditionKey{Locale: model.LocaleEnglish, Channel: "short"})
	require.True(t, ok, "the same-language channel edition survives as an edition of its own")
	assert.Equal(t, "Hi", model.RunsText(short.Runs))
	assert.Equal(t, model.RunsText(origSrc.Runs), model.RunsText(pulledSrc.Runs), "the channel edition never replaces the source")
}
