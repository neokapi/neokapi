package host

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/kbf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
)

// linkSource is a source sentence whose words link to href, so two of them
// share their text and their hash and differ in an inline code.
func linkSource(href string) []model.Run {
	return []model.Run{
		model.TextR("Read the "),
		model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link", Data: `<a href="` + href + `">`}),
		model.TextR("guide"),
		model.PcCloseR(model.PcCloseRun{ID: "1", Type: "link", Data: "</a>"}),
	}
}

// priorStore is a block store holding one translatable block per source.
func priorStore(t *testing.T, sources ...[]model.Run) blockstore.Store {
	t.Helper()
	store := blockstore.NewMemoryStore()
	sess, err := store.Begin(context.Background())
	require.NoError(t, err)
	for i, runs := range sources {
		id := string(rune('a' + i))
		require.NoError(t, sess.PutBlock("docs", &blockstore.Block{
			ID: id, Hash: "h-" + id, Translatable: true, Editions: kbf.SourceEditions(runs),
			Properties: model.BlockProperties{File: "docs/" + id + ".md"},
		}))
	}
	require.NoError(t, sess.Commit())
	require.NoError(t, sess.Close())
	return store
}

// The absorber recovers the source a record's basis names by its revision,
// under every key the readers of the document give that source: here the
// language an ARB reader declared (en) in a project whose language is en-US.
// A revision the store holds under none of them names content that is gone,
// and is never answered by the text hash, which would hand back another
// source with the same words and another link. A record made before revisions
// is recovered by its hash.
func TestPriorSourceIndex_RecoversTheBasisByRevisionUnderEveryKey(t *testing.T) {
	ctx := context.Background()
	guide, manual := linkSource("https://a.example/guide"), linkSource("https://a.example/manual")
	require.Equal(t, model.RunsText(guide), model.RunsText(manual))
	hash := state.SourceHash(model.RunsText(guide))
	declared := model.RunsRevision(model.Variant("en"), guide)

	// The block in hand, as a project read files it: under en-US, with the
	// language its reader declared kept beside it.
	b := model.NewRunsBlock("a", guide)
	b.SourceLocale = "en-US"
	b.Properties = map[string]string{model.PropReadSourceLocale: "en"}

	prior := newPriorSourceIndex(ctx, priorStore(t, guide), "en-US")
	runs, ok := prior.runsFor(declared, "", b)
	require.True(t, ok, "a basis taken under the declared language is found by its revision")
	assert.Equal(t, guide, runs)
	runs, ok = prior.runsFor(model.RunsRevision(model.Variant("en-US"), guide), "", b)
	require.True(t, ok, "and one taken under the project's language")
	assert.Equal(t, guide, runs)
	runs, ok = prior.runsFor(model.RunsRevision(model.EditionKey{}, guide), "", nil)
	require.True(t, ok, "and one taken under no language")
	assert.Equal(t, guide, runs)

	gone := newPriorSourceIndex(ctx, priorStore(t, manual), "en-US")
	_, ok = gone.runsFor(declared, hash, b)
	assert.False(t, ok, "the store holds the words with another link only, and a revision is not answered by the text")

	runs, ok = gone.runsFor("", hash, b)
	require.True(t, ok, "a record made before revisions is recovered by its hash")
	assert.Equal(t, manual, runs)
}
