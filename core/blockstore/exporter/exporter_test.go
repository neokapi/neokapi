package exporter_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/exporter"
	"github.com/neokapi/neokapi/core/klf"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seed(t *testing.T, store blockstore.Store) {
	t.Helper()
	ctx := context.Background()
	sess, err := store.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, sess.PutBlock("default", &klf.Block{Hash: "h1", ID: "b1", Translatable: true, Source: []klf.Run{{Text: &klf.TextRun{Text: "Hello"}}}}))
	require.NoError(t, sess.PutBlock("default", &klf.Block{Hash: "h2", ID: "b2", Translatable: true, Source: []klf.Run{{Text: &klf.TextRun{Text: "World"}}}}))
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: "targets/fr", BlockHash: "b1", Payload: []byte(`{"text":"Bonjour"}`)}))
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: "targets/fr", BlockHash: "b2", Payload: []byte(`{"text":"Monde"}`)}))
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: "annotations/qa", BlockHash: "b1", Payload: []byte(`{"ok":true}`)}))
	require.NoError(t, sess.Commit())
}

func TestExportLoadRoundTripCache(t *testing.T) {
	ctx := context.Background()
	src, err := blockstore.NewCacheStore(filepath.Join(t.TempDir(), "a.db"))
	require.NoError(t, err)
	defer src.Close()
	seed(t, src)

	snap, err := exporter.Export(ctx, src)
	require.NoError(t, err)
	require.Len(t, snap.Blocks, 2)
	require.Len(t, snap.Overlays, 3)
	// Deterministic order: overlays by (kind, blockHash).
	assert.Equal(t, "annotations/qa", snap.Overlays[0].Kind)
	assert.Equal(t, "targets/fr", snap.Overlays[1].Kind)
	assert.Equal(t, "b1", snap.Overlays[1].BlockHash)
	assert.Zero(t, snap.Overlays[0].UpdatedAt, "export strips timestamps")

	dst, err := blockstore.NewCacheStore(filepath.Join(t.TempDir(), "b.db"))
	require.NoError(t, err)
	defer dst.Close()
	require.NoError(t, exporter.Load(ctx, dst, snap))

	snap2, err := exporter.Export(ctx, dst)
	require.NoError(t, err)
	require.Len(t, snap2.Blocks, 2)
	require.Len(t, snap2.Overlays, 3)
	assert.Equal(t, snap.Overlays, snap2.Overlays)
	// Block payloads survive identically.
	assert.Equal(t, snap.Blocks[0].Block.Source, snap2.Blocks[0].Block.Source)
}

func TestExportMemoryStore(t *testing.T) {
	ctx := context.Background()
	store := blockstore.NewMemoryStore()
	defer store.Close()
	seed(t, store)

	snap, err := exporter.Export(ctx, store)
	require.NoError(t, err)
	assert.Len(t, snap.Blocks, 2)
	assert.Len(t, snap.Overlays, 3)
}
