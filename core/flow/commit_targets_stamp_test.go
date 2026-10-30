package flow

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/tools"
)

const stampSentence = "Mr Bean is the new King of England"

// passThrough pushes one part through a tool and returns it.
func passThrough(t *testing.T, tl interface {
	Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error
}, b *model.Block) *model.Block {
	t.Helper()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: b}
	close(in)
	require.NoError(t, tl.Process(t.Context(), in, out))
	close(out)
	return (<-out).Resource.(*model.Block)
}

// committedStamp runs commit-targets over b for fr and returns the source
// stamp of the overlay it wrote.
func committedStamp(t *testing.T, b *model.Block) string {
	t.Helper()
	ctx := context.Background()
	sess, err := blockstore.NewMemoryStore().Begin(ctx)
	require.NoError(t, err)
	kind := blockstore.TargetOverlayKind(model.LocaleFrench)
	require.NoError(t, newCommitTargetsTool(model.LocaleFrench).commitOne(ctx, sess, kind, &model.Part{Type: model.PartBlock, Resource: b}))
	o, err := sess.GetOverlay(kind, b.ID)
	require.NoError(t, err)
	var p blockstore.TargetOverlay
	require.NoError(t, json.Unmarshal(o.Payload, &p))
	return p.Source
}

// commit-targets stamps the source as read: a block redact rewrote, and one a
// script edited, carry the stamp of the text the file holds, which is the
// stamp a merge computes for the block it reads.
func TestCommitTargets_StampsTheSourceAsRead(t *testing.T) {
	want := blockstore.SourceStamp(stampSentence)

	t.Run("redacted", func(t *testing.T) {
		redact, err := tools.NewRedactTool(&tools.RedactConfig{
			Detectors: []string{tools.DetectRules},
			Rules:     []redaction.Rule{{Term: "Mr Bean", Category: "person"}},
		})
		require.NoError(t, err)
		b := model.NewBlock("b1", stampSentence)
		b.SourceLocale = model.LocaleEnglish
		b.Translatable = true
		b = passThrough(t, redact, b)
		require.NotContains(t, b.SourceText(), "Mr Bean")
		b.SetTargetRuns(model.LocaleFrench, b.SourceRuns())

		assert.Equal(t, want, committedStamp(t, b))
	})

	t.Run("redacted then unredacted", func(t *testing.T) {
		redact, err := tools.NewRedactTool(&tools.RedactConfig{
			Detectors: []string{tools.DetectRules},
			Rules:     []redaction.Rule{{Term: "Mr Bean", Category: "person"}},
		})
		require.NoError(t, err)
		unredact, err := tools.NewUnredactTool(&tools.UnredactConfig{})
		require.NoError(t, err)
		b := model.NewBlock("b1", stampSentence)
		b.SourceLocale = model.LocaleEnglish
		b.Translatable = true
		b = passThrough(t, redact, b)
		b.SetTargetRuns(model.LocaleFrench, b.SourceRuns())
		b = passThrough(t, unredact, b)
		require.Equal(t, stampSentence, b.SourceText())

		assert.Equal(t, want, committedStamp(t, b))
	})

	t.Run("script-edited", func(t *testing.T) {
		script := tools.NewScriptTool(&tools.ScriptConfig{
			Code:                `if (part.type === "block") { part.block.source[0].content.text = part.block.source[0].content.text.toUpperCase(); } emit(part);`,
			AllowSourceMutation: true,
		})
		b := model.NewBlock("b1", stampSentence)
		b.SourceLocale = model.LocaleEnglish
		b.Translatable = true
		b = passThrough(t, script, b)
		require.Equal(t, "MR BEAN IS THE NEW KING OF ENGLAND", b.SourceText())
		b.SetTargetText(model.LocaleFrench, "M. BEAN EST LE NOUVEAU ROI D'ANGLETERRE")

		assert.Equal(t, want, committedStamp(t, b))
	})
}
