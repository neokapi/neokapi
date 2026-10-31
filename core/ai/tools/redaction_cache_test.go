package tools_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/redaction"
	libtools "github.com/neokapi/neokapi/core/tools"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// A block a redact step read is never served from the session's target cache,
// nor kept in it. The translation of such a block can hold a token for a value
// met only beside the block, in the instruction here, and only the request
// that produced it fills the entry unredact restores it from. A served target
// makes no request, so its token would stay in the translation.
func TestAITranslate_ARedactedBlockIsNeverServedFromTheCache(t *testing.T) {
	ctx := context.Background()
	store, err := sqlitestore.New(filepath.Join(t.TempDir(), "blocks.db"))
	require.NoError(t, err)
	defer store.Close()

	calls := 0
	mock := aiprovider.NewMockProvider()
	mock.TranslateFunc = func(_ context.Context, req aiprovider.TranslateRequest) (*aiprovider.TranslateResponse, error) {
		calls++
		// The model copies the instruction's token into its answer.
		return &aiprovider.TranslateResponse{Translation: "FR " + req.Source + " " + req.Instruction, Model: "m"}, nil
	}
	cfg := tools.AITranslateConfig{
		SourceLocale: "en", TargetLocale: "fr", Provider: "anthropic", Model: "claude-x",
		BatchSize: 1, BatchConcurrency: 1, Instruction: "Contact Ada Lovelace",
	}

	run := func() string {
		redact, err := libtools.NewRedactTool(&libtools.RedactConfig{
			Detectors: []string{libtools.DetectRules},
			Rules:     []redaction.Rule{{Term: "Ada Lovelace", Category: redaction.CategoryPerson}},
		})
		require.NoError(t, err)
		unredact, err := libtools.NewUnredactTool(&libtools.UnredactConfig{})
		require.NoError(t, err)

		b := model.NewBlock("h1", "Thank her.")
		b.Translatable = true
		part := &model.Part{Type: model.PartBlock, Resource: b}
		_, err = redact.ApplyContext(ctx, part)
		require.NoError(t, err)

		sess, err := store.Begin(ctx)
		require.NoError(t, err)
		in := make(chan *model.Part, 1)
		out := make(chan *model.Part, 1)
		in <- part
		close(in)
		require.NoError(t, tools.NewAITranslateTool(mock, cfg).SessionProcess(ctx, sess, in, out))
		close(out)
		<-out
		require.NoError(t, sess.Commit())

		_, err = unredact.ApplyContext(ctx, part)
		require.NoError(t, err)
		return b.TargetText("fr")
	}

	assert.Equal(t, "FR Thank her. Contact Ada Lovelace", run())
	assert.Equal(t, "FR Thank her. Contact Ada Lovelace", run(), "the second run asks again and restores the name")
	assert.Equal(t, 2, calls, "no run is served from the cache")
}
