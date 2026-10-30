package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/mt/tools"
	"github.com/neokapi/neokapi/core/redaction"
	coretools "github.com/neokapi/neokapi/core/tools"
)

const stampSentence = "Mr Bean is the new King of England"

// The MT producer stamps the source as read: a block redact rewrote carries
// the stamp of the sentence the file holds, and a later read of the same
// block, redacted the same way, is served the stored answer.
func TestMTTranslate_StampsTheSourceAsReadOfARedactedBlock(t *testing.T) {
	redact, err := coretools.NewRedactTool(&coretools.RedactConfig{
		Detectors: []string{coretools.DetectRules},
		Rules:     []redaction.Rule{{Term: "Mr Bean", Category: "person"}},
	})
	require.NoError(t, err)
	redacted := func() *model.Block {
		b := model.NewBlock("tu1", stampSentence)
		b.SourceLocale = model.LocaleEnglish
		b.Translatable = true
		got := processPart(t, redact, &model.Part{Type: model.PartBlock, Resource: b}).Resource.(*model.Block)
		require.NotContains(t, got.SourceText(), "Mr Bean")
		return got
	}

	ctx := context.Background()
	sess, err := blockstore.NewMemoryStore().Begin(ctx)
	require.NoError(t, err)
	mock := newMock("test-mt")
	tl := tools.NewMTTranslateTool(mock, tools.MTTranslateConfig{SourceLocale: model.LocaleEnglish, TargetLocale: model.LocaleFrench})
	run := func(b *model.Block) {
		in := make(chan *model.Part, 1)
		out := make(chan *model.Part, 1)
		in <- &model.Part{Type: model.PartBlock, Resource: b}
		close(in)
		require.NoError(t, tl.SessionProcess(ctx, sess, in, out))
	}

	run(redacted())
	require.Equal(t, 1, mock.calls)
	o, err := sess.GetOverlay(blockstore.TargetOverlayKind(model.LocaleFrench), "tu1")
	require.NoError(t, err)
	var stored blockstore.TargetOverlay
	require.NoError(t, json.Unmarshal(o.Payload, &stored))
	assert.Equal(t, blockstore.SourceStamp(stampSentence), stored.Source)

	run(redacted())
	assert.Equal(t, 1, mock.calls, "the same rewrite of the same source is served from the store")
}
