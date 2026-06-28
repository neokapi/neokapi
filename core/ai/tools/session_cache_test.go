package tools_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAITranslate_SessionCacheIsConfigAware verifies the config-aware overlay
// cache: an unchanged-config re-run is served from the cache (no LLM call), but a
// changed model or glossary re-translates rather than serving the stale cached
// target — the "never run unneeded processing, never serve stale processing"
// contract.
func TestAITranslate_SessionCacheIsConfigAware(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.New(filepath.Join(t.TempDir(), "blocks.db"))
	require.NoError(t, err)
	defer store.Close()
	require.True(t, store.Capabilities().RandomAccess, "overlay skip needs random access")

	calls := 0
	mock := aiprovider.NewMockProvider()
	mock.TranslateFunc = func(context.Context, aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		calls++
		return &aiprovider.TranslateResponse{Translation: "T", Model: "m"}, nil
	}

	run := func(cfg tools.AITranslateConfig) {
		tl := tools.NewAITranslateTool(mock, cfg)
		sess, err := store.Begin(ctx)
		require.NoError(t, err)
		in := make(chan *model.Part, 1)
		out := make(chan *model.Part, 1)
		b := model.NewBlock("h1", "Hello")
		b.Translatable = true
		in <- &model.Part{Type: model.PartBlock, Resource: b}
		close(in)
		require.NoError(t, tl.SessionProcess(ctx, sess, in, out))
		close(out)
		<-out
		require.NoError(t, sess.Commit())
	}

	// Single-block session path (BatchSize 1) so the per-block overlay gate runs.
	base := tools.AITranslateConfig{
		SourceLocale: "en", TargetLocale: "fr",
		Provider: "anthropic", Model: "claude-x",
		BatchSize: 1, BatchConcurrency: 1,
	}

	run(base)
	assert.Equal(t, 1, calls, "first run calls the LLM")

	run(base)
	assert.Equal(t, 1, calls, "an unchanged-config re-run is served from the overlay cache")

	changedModel := base
	changedModel.Model = "claude-y"
	run(changedModel)
	assert.Equal(t, 2, calls, "a changed model re-translates instead of serving the stale target")

	changedGlossary := base
	changedGlossary.Glossary = map[string]string{"Hello": "Salut"}
	run(changedGlossary)
	assert.Equal(t, 3, calls, "a changed glossary re-translates")
}
