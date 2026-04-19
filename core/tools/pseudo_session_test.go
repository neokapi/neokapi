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

// Verify SessionTool: a block that already has a `targets/qps`
// sidecar is hydrated from the cache instead of re-translated.
func TestPseudo_SessionSkipsCachedTarget(t *testing.T) {
	ctx := context.Background()
	store := blockstore.NewMemoryStore()
	defer store.Close()

	// Prime the store with a cached target for block ID "h1".
	sess1, err := store.Begin(ctx)
	require.NoError(t, err)
	payload, _ := json.Marshal(map[string]string{"target": "[pre-computed]"})
	require.NoError(t, sess1.PutSidecar(blockstore.Sidecar{
		Kind:      "targets/qps",
		BlockHash: "h1",
		Payload:   payload,
	}))
	require.NoError(t, sess1.Commit())

	// Run the tool against a second session.
	tl := tools.NewPseudoTranslateTool(&tools.PseudoConfig{
		Prefix: "[", Suffix: "]", TargetLocale: "qps",
	})

	sess2, err := store.Begin(ctx)
	require.NoError(t, err)
	defer sess2.Close()

	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	block := model.NewRunsBlock("h1", []model.Run{
		{Text: &model.TextRun{Text: "Hello"}},
	})
	block.Translatable = true
	in <- &model.Part{Type: model.PartBlock, Resource: block}
	close(in)
	require.NoError(t, tl.SessionProcess(ctx, sess2, in, out))
	close(out)

	got := <-out
	blk := got.Resource.(*model.Block)
	// Target must be the cached value, NOT the default "[Hello]"
	// the translator would produce.
	assert.Equal(t, "[pre-computed]", blk.TargetText("qps"))
}

// Verify SessionTool: fresh run writes a targets/qps sidecar so the
// next run can skip.
func TestPseudo_SessionWritesSidecar(t *testing.T) {
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

	// Confirm the sidecar was written.
	sess2, err := store.Begin(ctx)
	require.NoError(t, err)
	defer sess2.Close()
	sc, err := sess2.GetSidecar("targets/qps", "h1")
	require.NoError(t, err)
	var cached map[string]string
	require.NoError(t, json.Unmarshal(sc.Payload, &cached))
	assert.NotEmpty(t, cached["target"], "sidecar carries the translated text")
}
