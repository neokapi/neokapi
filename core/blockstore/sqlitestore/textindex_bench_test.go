//go:build !wasm

package sqlitestore

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/require"
)

// benchCorpus builds the blocks a purge-and-refill writes.
func benchCorpus(n int) []*blockstore.Block {
	out := make([]*blockstore.Block, n)
	for i := range out {
		out[i] = &blockstore.Block{
			Hash:         fmt.Sprintf("h%08d", i),
			ID:           fmt.Sprintf("doc.section.%d", i),
			Translatable: true,
			Source: []model.Run{model.TextR(fmt.Sprintf(
				"Paragraph %d of the guide explains how the content memory recycles approved wording across a project.", i))},
		}
	}
	return out
}

// BenchmarkPutBlock measures extraction's hot path with the text index in
// place. Its counterpart below writes the same corpus the way the store did
// before the index existed, so the pair is the cost of the index, not the cost
// of SQLite.
func BenchmarkPutBlock(b *testing.B) {
	corpus := benchCorpus(2000)
	for b.Loop() {
		b.StopTimer()
		store, err := New(filepath.Join(b.TempDir(), "store.db"))
		require.NoError(b, err)
		sess, err := store.Begin(b.Context())
		require.NoError(b, err)
		b.StartTimer()

		for _, blk := range corpus {
			if err := sess.PutBlock("docs", blk); err != nil {
				b.Fatal(err)
			}
		}

		b.StopTimer()
		require.NoError(b, sess.Commit())
		require.NoError(b, store.Close())
		b.StartTimer()
	}
}

// BenchmarkPutBlockWithoutTextIndex writes the identical corpus through the
// identical session, issuing the statement PutBlock issued before the text
// index existed. Everything else — the store, the schema, the transaction, the
// JSON encoding — is held constant, so the gap between the two is the whole
// price extraction pays for the index.
func BenchmarkPutBlockWithoutTextIndex(b *testing.B) {
	corpus := benchCorpus(2000)
	for b.Loop() {
		b.StopTimer()
		store, err := New(filepath.Join(b.TempDir(), "store.db"))
		require.NoError(b, err)
		sess, err := store.Begin(b.Context())
		require.NoError(b, err)
		cs := sess.(*cacheSession)
		b.StartTimer()

		for _, blk := range corpus {
			payload, err := json.Marshal(blk)
			if err != nil {
				b.Fatal(err)
			}
			if _, err := cs.q().ExecContext(b.Context(), `
				INSERT INTO blocks (hash, collection, translatable, payload)
				VALUES (?, ?, ?, ?)
				ON CONFLICT(hash) DO UPDATE SET
					collection=excluded.collection,
					translatable=excluded.translatable,
					payload=excluded.payload
			`, blk.Hash, "docs", 1, payload); err != nil {
				b.Fatal(err)
			}
		}

		b.StopTimer()
		require.NoError(b, sess.Commit())
		require.NoError(b, store.Close())
		b.StartTimer()
	}
}

// BenchmarkBuildTextIndex measures what the write path stopped paying: the
// first search after a full extraction, which flattens every changed block and
// files its text. It is the whole cost of the index, charged once, to the
// command that asked for it.
func BenchmarkBuildTextIndex(b *testing.B) {
	corpus := benchCorpus(2000)
	for b.Loop() {
		b.StopTimer()
		store, err := New(filepath.Join(b.TempDir(), "store.db"))
		require.NoError(b, err)
		writeBench(b, store, corpus)
		b.StartTimer()

		require.NoError(b, store.(*cacheStore).ensureTextIndex(b.Context()))

		b.StopTimer()
		require.NoError(b, store.Close())
		b.StartTimer()
	}
}

// BenchmarkSearchBlockTextWarm is the steady state: nothing is stale, so the
// probe finds nothing and the search is a query.
func BenchmarkSearchBlockTextWarm(b *testing.B) {
	store, err := New(filepath.Join(b.TempDir(), "store.db"))
	require.NoError(b, err)
	defer store.Close()
	writeBench(b, store, benchCorpus(2000))
	_, err = blockstore.SearchText(b.Context(), store, "content memory", blockstore.TextSearchOptions{})
	require.NoError(b, err)

	for b.Loop() {
		if _, err := blockstore.SearchText(b.Context(), store, "content memory", blockstore.TextSearchOptions{Limit: 20}); err != nil {
			b.Fatal(err)
		}
	}
}

func writeBench(b *testing.B, store blockstore.Store, corpus []*blockstore.Block) {
	b.Helper()
	sess, err := store.Begin(b.Context())
	require.NoError(b, err)
	for _, blk := range corpus {
		if err := sess.PutBlock("docs", blk); err != nil {
			b.Fatal(err)
		}
	}
	require.NoError(b, sess.Commit())
}
