package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runPseudoSession runs the pseudo tool once over block "h1" ("Hello") against a
// fresh session on store, commits, and returns the resulting qps target.
func runPseudoSession(t *testing.T, store blockstore.Store, cfg *tools.PseudoConfig) string {
	t.Helper()
	ctx := context.Background()
	tl := tools.NewPseudoTranslateTool(cfg)
	sess, err := store.Begin(ctx)
	require.NoError(t, err)
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	block := model.NewRunsBlock("h1", []model.Run{{Text: &model.TextRun{Text: "Hello"}}})
	block.Translatable = true
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)
	require.NoError(t, tl.SessionProcess(ctx, sess, in, out))
	close(out)
	got := <-out
	require.NoError(t, sess.Commit())
	return got.Resource.(*model.Block).TargetText("qps")
}

// Verify SessionTool: a cached target is reused only when the tool config is
// unchanged; a changed config (here the prefix/suffix style) re-runs instead of
// serving the stale cached target. This is the config-aware overlay cache.
func TestPseudo_SessionSkipsCachedTargetOnlyWhenConfigMatches(t *testing.T) {
	ctx := context.Background()
	store := blockstore.NewMemoryStore()
	defer store.Close()

	cfg := &tools.PseudoConfig{Prefix: "[", Suffix: "]", TargetLocale: "qps"}

	// First run computes the target and writes the overlay (with the config
	// fingerprint). Pseudo accents the source, so "Hello" → "Ĥéļļö".
	assert.Equal(t, "[Ĥéļļö]", runPseudoSession(t, store, cfg))

	// Tamper: replace the cached target text but KEEP the config fingerprint, so a
	// reuse is observable (a recompute would yield "[Hello]" again).
	sess, err := store.Begin(ctx)
	require.NoError(t, err)
	sc, err := sess.GetOverlay("targets/qps", "h1")
	require.NoError(t, err)
	var cached map[string]string
	require.NoError(t, json.Unmarshal(sc.Payload, &cached))
	require.NotEmpty(t, cached["config"], "the fresh overlay must carry a config fingerprint")
	cached["target"] = "[pre-computed]"
	payload, _ := json.Marshal(cached)
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: "targets/qps", BlockHash: "h1", Payload: payload}))
	require.NoError(t, sess.Commit())

	// Same config → the (tampered) cached target is reused, not recomputed.
	assert.Equal(t, "[pre-computed]", runPseudoSession(t, store, cfg),
		"an unchanged config reuses the cached target")

	// Changed config (different style) → re-runs, ignoring the stale cache.
	changed := &tools.PseudoConfig{Prefix: "<", Suffix: ">", TargetLocale: "qps"}
	assert.Equal(t, "<Ĥéļļö>", runPseudoSession(t, store, changed),
		"a changed config re-runs instead of serving the stale cached target")
}

// Verify SessionTool: fresh run writes a targets/qps overlay so the
// next run can skip.
func TestPseudo_SessionWritesOverlay(t *testing.T) {
	ctx := context.Background()
	store := blockstore.NewMemoryStore()
	defer store.Close()

	tl := tools.NewPseudoTranslateTool(&tools.PseudoConfig{
		Prefix: "[", Suffix: "]", TargetLocale: "qps",
	})

	sess, err := store.Begin(ctx)
	require.NoError(t, err)

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	block := model.NewRunsBlock("h1", []model.Run{
		{Text: &model.TextRun{Text: "Hello"}},
	})
	block.Translatable = true
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)
	require.NoError(t, tl.SessionProcess(ctx, sess, in, out))
	close(out)
	<-out // drain
	require.NoError(t, sess.Commit())

	// Confirm the overlay was written.
	sess2, err := store.Begin(ctx)
	require.NoError(t, err)
	defer sess2.Close()
	sc, err := sess2.GetOverlay("targets/qps", "h1")
	require.NoError(t, err)
	var cached map[string]string
	require.NoError(t, json.Unmarshal(sc.Payload, &cached))
	assert.NotEmpty(t, cached["target"], "overlay carries the translated text")
}
