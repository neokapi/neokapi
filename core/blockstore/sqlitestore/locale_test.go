package sqlitestore_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The cache store canonicalizes a `targets/<locale>` kind on every write, read
// and list, so an overlay a producer wrote under one spelling of a locale is the
// overlay every reader finds under any other.
func TestOverlayKindIsCanonical(t *testing.T) {
	s := newStore(t, true)
	ctx := context.Background()
	sess, err := s.Begin(ctx)
	require.NoError(t, err)
	defer sess.Close()

	require.NoError(t, sess.PutOverlay(blockstore.Overlay{
		Kind: "targets/nb_NO", BlockHash: "b1", Payload: []byte(`{"text":"Kai"}`),
	}))

	got, err := sess.GetOverlay("targets/nb-NO", "b1")
	require.NoError(t, err)
	assert.Equal(t, "targets/nb-NO", got.Kind, "the row holds the canonical kind")
	got, err = sess.GetOverlay(blockstore.TargetOverlayKind("NB-no"), "b1")
	require.NoError(t, err)
	assert.Equal(t, []byte(`{"text":"Kai"}`), got.Payload)

	n := 0
	for _, lerr := range sess.ListOverlays("targets/nb_no") {
		require.NoError(t, lerr)
		n++
	}
	assert.Equal(t, 1, n)

	// The canonical spelling replaces the row rather than sitting beside it.
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{
		Kind: "targets/nb-NO", BlockHash: "b1", Payload: []byte(`{"text":"Kaiplass"}`),
	}))
	n = 0
	for _, lerr := range sess.ListOverlays("targets/nb-NO") {
		require.NoError(t, lerr)
		n++
	}
	assert.Equal(t, 1, n)

	// Other namespaces are left exactly as written.
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: "annotations/qa", BlockHash: "b1", Payload: []byte(`{}`)}))
	_, err = sess.GetOverlay("annotations/qa", "b1")
	require.NoError(t, err)
}

// The text index files a target's text under its canonical locale, and a
// locale filter asks in the same form.
func TestSearchBlockText_LocaleFilterIsCanonical(t *testing.T) {
	s, err := sqlitestore.New(filepath.Join(t.TempDir(), "store.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close() })
	ctx := context.Background()

	sess, err := s.Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, sess.PutBlock("app", &blockstore.Block{
		ID: "tu1", Hash: "h1", Translatable: true,
		Source:  []model.Run{model.TextR("Berth")},
		Targets: map[string][]model.Run{"nb_NO": {model.TextR("Kai")}},
	}))
	require.NoError(t, sess.Commit())

	hits, err := blockstore.SearchText(ctx, s, "kai", blockstore.TextSearchOptions{Locales: []string{"NB-no"}})
	require.NoError(t, err)
	require.Len(t, hits, 1)
	assert.Equal(t, "nb-NO", hits[0].Locale)

	hits, err = blockstore.SearchText(ctx, s, "kai", blockstore.TextSearchOptions{Locales: []string{"de"}})
	require.NoError(t, err)
	assert.Empty(t, hits)
}
