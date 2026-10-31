package tools_test

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/redaction"
	coretools "github.com/neokapi/neokapi/core/tools"
)

const stampSentence = "Mr Bean is the new King of England"

// rewrite passes a freshly read block through a source-rewriting tool.
func rewrite(t *testing.T, tl interface {
	Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error
}) *model.Block {
	t.Helper()
	b := model.NewBlock("tu1", stampSentence)
	b.SourceLocale = model.LocaleEnglish
	b.Translatable = true
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: b}
	close(in)
	require.NoError(t, tl.Process(t.Context(), in, out))
	close(out)
	got := (<-out).Resource.(*model.Block)
	require.NotEqual(t, stampSentence, got.SourceText(), "the tool rewrites the source")
	return got
}

func storedOverlay(t *testing.T, sess blockstore.Session) blockstore.TargetOverlay {
	t.Helper()
	o, err := sess.GetOverlay(blockstore.TargetOverlayKind(model.LocaleFrench), "tu1")
	require.NoError(t, err)
	var p blockstore.TargetOverlay
	require.NoError(t, json.Unmarshal(o.Payload, &p))
	return p
}

// The AI producer stamps the source as read, so the stamp names the sentence
// the file holds rather than the redacted text the model was sent; a later
// read of the same block, redacted the same way, is served the stored answer.
// The block is redacted into a sidecar vault, as extract and ingest redact it,
// so it carries no in-process entry: a block that does is never served from
// the store (TestAITranslate_ARedactedBlockIsNeverServedFromTheCache).
func TestAITranslate_StampsTheSourceAsReadOfARedactedBlock(t *testing.T) {
	redact, err := coretools.NewRedactTool(&coretools.RedactConfig{
		Detectors: []string{coretools.DetectRules},
		Rules:     []redaction.Rule{{Term: "Mr Bean", Category: "person"}},
		VaultPath: filepath.Join(t.TempDir(), "vault.json"),
	})
	require.NoError(t, err)

	ctx := context.Background()
	sess, err := blockstore.NewMemoryStore().Begin(ctx)
	require.NoError(t, err)
	mock, calls := newTranslateMock(t)
	tl := tools.NewAITranslateTool(mock, singleBlockConfig())

	b := runSession(t, ctx, tl, sess, rewrite(t, redact))
	require.Equal(t, 1, *calls)
	require.NotContains(t, b.TargetText(model.LocaleFrench), "Mr Bean")
	stored := storedOverlay(t, sess)
	assert.Equal(t, blockstore.SourceStamp(stampSentence), stored.Source)

	runSession(t, ctx, tl, sess, rewrite(t, redact))
	assert.Equal(t, 1, *calls, "the same rewrite of the same source is served from the store")
}

// A script edit is rewriting the source the model is sent: the stamp names
// the source as read, and the text sent goes into the configuration, so a
// different edit of the same source is translated again.
func TestAITranslate_StampsTheSourceAsReadOfAScriptEditedBlock(t *testing.T) {
	upper := coretools.NewScriptTool(&coretools.ScriptConfig{
		Code:                `if (part.type === "block") { part.block.source[0].content.text = part.block.source[0].content.text.toUpperCase(); } emit(part);`,
		AllowSourceMutation: true,
	})
	lower := coretools.NewScriptTool(&coretools.ScriptConfig{
		Code:                `if (part.type === "block") { part.block.source[0].content.text = part.block.source[0].content.text.toLowerCase(); } emit(part);`,
		AllowSourceMutation: true,
	})

	ctx := context.Background()
	sess, err := blockstore.NewMemoryStore().Begin(ctx)
	require.NoError(t, err)
	mock, calls := newTranslateMock(t)
	tl := tools.NewAITranslateTool(mock, singleBlockConfig())

	b := runSession(t, ctx, tl, sess, rewrite(t, upper))
	require.Equal(t, 1, *calls)
	assert.Equal(t, "[fr] MR BEAN IS THE NEW KING OF ENGLAND", b.TargetText(model.LocaleFrench))
	assert.Equal(t, blockstore.SourceStamp(stampSentence), storedOverlay(t, sess).Source)

	runSession(t, ctx, tl, sess, rewrite(t, upper))
	assert.Equal(t, 1, *calls, "the same edit is served from the store")

	b = runSession(t, ctx, tl, sess, rewrite(t, lower))
	assert.Equal(t, 2, *calls, "another edit of the same source is translated again")
	assert.Equal(t, "[fr] mr bean is the new king of england", b.TargetText(model.LocaleFrench))
}
