package sqlitestore_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/storage"
)

// schema1Payload is a block row as a store wrote it while blocks carried
// schema 1 of the bundle: `source` runs beside `targets` keyed by locale.
const schema1Payload = `{"id":"tu1","hash":"h1","translatable":true,"type":"","source":[{"text":"Berth"}],"targets":{"nb_NO":[{"text":"Kai"}]},"placeholders":null,"properties":{"file":"docs/a.md","line":0,"component":"","jsxPath":"","element":""}}`

// A project store written before the bundle's blocks carried editions still
// reads: each stored block comes back as the editions it describes, by key and
// in a scan, and its text is found by a search.
func TestAStoreWrittenInSchema1Reads(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "store.db")

	s, err := sqlitestore.New(path)
	require.NoError(t, err)
	require.NoError(t, s.Close())

	db, err := storage.Open(path)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx,
		`INSERT INTO blocks (hash, collection, translatable, payload) VALUES (?, ?, ?, ?)`,
		"h1", "docs", 1, []byte(schema1Payload))
	require.NoError(t, err)
	require.NoError(t, db.Close())

	s, err = sqlitestore.New(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	sess, err := s.Begin(ctx)
	require.NoError(t, err)
	defer sess.Close()

	got, err := sess.GetBlock("h1")
	require.NoError(t, err)
	assert.Equal(t, "tu1", got.ID)
	assert.Equal(t, "Berth", model.RunsText(got.SourceRuns()))
	nb, ok := got.Edition("nb_NO")
	require.True(t, ok, "the schema 1 target is an edition under its locale")
	assert.Equal(t, "Kai", model.RunsText(nb.Runs))
	assert.Equal(t, "docs/a.md", got.Properties.File)

	n := 0
	for b, berr := range sess.Blocks(blockstore.BlockFilter{Collection: "docs"}) {
		require.NoError(t, berr)
		assert.Equal(t, []string{"nb_NO"}, b.TargetKeys())
		n++
	}
	assert.Equal(t, 1, n)

	hits, err := blockstore.SearchText(ctx, s, "kai", blockstore.TextSearchOptions{Locales: []string{"nb-NO"}})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, "nb-NO", hits[0].Locale)
}
