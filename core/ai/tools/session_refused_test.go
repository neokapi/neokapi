package tools_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A stored target the applier refuses is not served: the block is translated
// afresh, on the per-block session path as on the batched one, rather than
// failing.
func TestAITranslate_ARefusedStoredTargetIsTranslatedAfresh(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.New(filepath.Join(t.TempDir(), "blocks.db"))
	require.NoError(t, err)
	defer store.Close()

	calls := 0
	mock := aiprovider.NewMockProvider()
	mock.TranslateFunc = func(_ context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		calls++
		return &aiprovider.TranslateResponse{Translation: `Bonjour <x id="1/"/>`, Model: "m"}, nil
	}
	tl := tools.NewAITranslateTool(mock, singleBlockConfig())
	newBlock := func() *model.Block {
		b := model.NewRunsBlock("h1", []model.Run{model.TextR("Hello "), model.PhR(model.PlaceholderRun{ID: "1", Type: "code:variable", Data: "{name}"})})
		b.Translatable = true
		return b
	}
	run := func() *model.Block {
		sess, err := store.Begin(ctx)
		require.NoError(t, err)
		b := runSession(t, ctx, tl, sess, newBlock())
		require.NoError(t, sess.Commit())
		return b
	}

	run()
	require.Equal(t, 1, calls)

	// The stored target now names code 1 as a bold code, which the source holds
	// as a variable: writing it would change the code, so the applier refuses.
	sess, err := store.Begin(ctx)
	require.NoError(t, err)
	key := blockstore.OverlayKey(ctx, "h1", newBlock().SourceText())
	ov, err := sess.GetOverlay("targets/fr", key)
	require.NoError(t, err)
	var stored map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(ov.Payload, &stored))
	stored["runs"] = json.RawMessage(`[{"text":"Bonjour "},{"ph":{"id":"1","type":"fmt:bold"}}]`)
	payload, err := json.Marshal(stored)
	require.NoError(t, err)
	require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: "targets/fr", BlockHash: key, Payload: payload}))
	require.NoError(t, sess.Commit())

	got := run()
	assert.Equal(t, 2, calls, "the refused stored target is translated afresh")
	runs := got.TargetRuns(model.LocaleFrench)
	require.Len(t, runs, 2)
	require.NotNil(t, runs[1].Ph)
	assert.Equal(t, "{name}", runs[1].Ph.Data)
}
