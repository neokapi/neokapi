package tools_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
)

// Pseudo output is a placeholder, so every target it writes, a new one
// included, is stamped draft with the tool's origin. The handler sets the
// target and stamps it in two writes; each applies at once, so the stamp
// finds the target the first write created. Every path pseudo takes stamps
// it: the dispatched handler, the session path and the session's cache hit.
func TestPseudo_ANewTargetKeepsItsDraftStamp(t *testing.T) {
	cfg := &tools.PseudoConfig{Prefix: "[", Suffix: "]", TargetLocale: "qps"}
	newBlock := func() *model.Block {
		b := model.NewRunsBlock("h1", []model.Run{model.TextR("Hello "), model.PcOpenR(model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<b>"}),
			model.TextR("world"), model.PcCloseR(model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"})})
		b.SourceLocale = "en"
		return b
	}
	stamped := func(t *testing.T, b *model.Block) {
		t.Helper()
		tg, ok := b.Edition(model.Variant("qps"))
		require.True(t, ok, "the target exists")
		assert.Equal(t, model.Status(model.TargetStatusDraft), tg.Status)
		assert.Equal(t, "pseudo-translate", tg.Origin.Tool)
	}

	t.Run("dispatched handler", func(t *testing.T) {
		b := newBlock()
		_, err := tools.NewPseudoTranslateTool(cfg).ApplyContext(context.Background(), &model.Part{Type: model.PartBlock, Resource: b})
		require.NoError(t, err)
		stamped(t, b)
		var open *model.PcOpenRun
		for _, r := range b.TargetRuns("qps") {
			if r.PcOpen != nil {
				open = r.PcOpen
			}
		}
		require.NotNil(t, open, "the markup is kept")
		assert.Equal(t, "<b>", open.Data)
	})

	t.Run("session path and its cache hit", func(t *testing.T) {
		ctx := context.Background()
		store := blockstore.NewMemoryStore()
		defer store.Close()
		for range 2 {
			sess, err := store.Begin(ctx)
			require.NoError(t, err)
			b := newBlock()
			in, out := make(chan *model.Part, 1), make(chan *model.Part, 1)
			in <- &model.Part{Type: model.PartBlock, Resource: b}
			close(in)
			require.NoError(t, tools.NewPseudoTranslateTool(cfg).SessionProcess(ctx, sess, in, out))
			require.NoError(t, sess.Commit())
			stamped(t, b)
		}
	})
}
