package tools

import (
	"context"
	"testing"

	corememory "github.com/neokapi/neokapi/core/memory"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func srcBlock(text string) *model.Block {
	return &model.Block{
		Translatable: true,
		Source:       []model.Run{{Text: &model.TextRun{Text: text}}},
	}
}

// hold drives one block through the translate-after stage's Process pipeline, as a
// real run does.
func hold(t *testing.T, gt *TranslateAfterTool, b *model.Block) {
	t.Helper()
	require.NoError(t, applyProduce(t, gt, b))
}

// TestTranslateAfterTool_SettlesAndHoldsBelowLevel: the translate-after stage
// settles a clean block to `written` and, at level `established`, holds it (written
// is below established) — stamping the hold marker and counting it.
func TestTranslateAfterTool_SettlesAndHoldsBelowLevel(t *testing.T) {
	b := srcBlock("Hello world")
	gt := NewTranslateAfterTool(model.TranslateAfterEstablished)
	hold(t, gt, b)

	assert.Equal(t, model.SourceStatusWritten, b.SourceStatus, "settled to written")
	assert.True(t, b.SourceHeld(), "written is below established → held")
	held, total := gt.Snapshot()
	assert.Equal(t, 1, held)
	assert.Equal(t, 1, total)
}

// TestTranslateAfterTool_AdmitsAtLevel: a clean block reaches the default
// (written) level — settled to written, not held, any stale marker cleared.
func TestTranslateAfterTool_AdmitsAtLevel(t *testing.T) {
	b := srcBlock("Hello world")
	b.SetSourceHeld(true) // a stale hold from a prior pass
	gt := NewTranslateAfterTool(model.TranslateAfterWritten)
	hold(t, gt, b)

	assert.Equal(t, model.SourceStatusWritten, b.SourceStatus)
	assert.False(t, b.SourceHeld(), "clean source reaches the written level; the stale hold is cleared")
	held, _ := gt.Snapshot()
	assert.Equal(t, 0, held)
}

// TestTranslateAfterTool_NoneIsPassthrough: the `none` opt-out settles nothing and
// holds nothing — no status change, no marker, no count.
func TestTranslateAfterTool_NoneIsPassthrough(t *testing.T) {
	b := srcBlock("Hello world")
	gt := NewTranslateAfterTool(model.TranslateAfterNone)
	hold(t, gt, b)

	assert.Equal(t, model.SourceStatusNew, b.SourceStatus, "none never settles")
	assert.False(t, b.SourceHeld())
	held, total := gt.Snapshot()
	assert.Equal(t, 0, held)
	assert.Equal(t, 0, total)
}

// TestTranslateAfterTool_WhitespaceHeldAtWritten: a whitespace-only source trips
// the major content-lint finding, so it stays unsettled and is held below the
// written level — the partial-hold case.
func TestTranslateAfterTool_WhitespaceHeldAtWritten(t *testing.T) {
	b := srcBlock("   ")
	gt := NewTranslateAfterTool(model.TranslateAfterWritten)
	hold(t, gt, b)

	assert.Equal(t, model.SourceStatusNew, b.SourceStatus, "a major source finding keeps it unsettled")
	assert.True(t, b.SourceHeld())
}

// TestProducersSkipHeldSource: both producers — recycle and AI translate — skip
// a block carrying the translate-after hold marker, leaving no target; a block
// without the marker is produced normally.
func TestProducersSkipHeldSource(t *testing.T) {
	const fr = model.LocaleID("fr")

	newRecycle := func() tool.Tool {
		return NewMemoryLeverageTool(&MemoryLeverageConfig{
			SourceLocale: "en", TargetLocale: fr,
			FuzzyThreshold: 70, Memory: staticMemoryProvider{"Hello": "Bonjour"},
		})
	}

	t.Run("recycle skips held", func(t *testing.T) {
		held := srcBlock("Hello")
		held.SetSourceHeld(true)
		require.NoError(t, applyProduce(t, newRecycle(), held))
		assert.False(t, held.HasTarget(fr), "held block must not be recycled")

		ready := srcBlock("Hello")
		require.NoError(t, applyProduce(t, newRecycle(), ready))
		assert.Equal(t, "Bonjour", ready.TargetText(fr), "an un-held block recycles normally")
	})
}

// applyProduce dispatches a Produce tool over a single block via its Process
// pipeline, so the block flows exactly as it does in a real run.
func applyProduce(t *testing.T, tl tool.Tool, b *model.Block) error {
	t.Helper()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: b}
	close(in)
	return tl.Process(context.Background(), in, out)
}

// staticMemoryProvider is a minimal exact-match content memory for the
// producer-skip test. It answers a block request and a text request the same
// way, by flattening, and has no version chain — which is a real answer rather
// than a missing method.
type staticMemoryProvider map[string]string

func (m staticMemoryProvider) Lookup(_ context.Context, req corememory.Request) (corememory.Match, bool) {
	key := req.Text
	if req.Block != nil {
		key = model.FlattenRuns(req.Block.Source)
	}
	t, ok := m[key]
	if !ok {
		return corememory.Match{}, false
	}
	return corememory.Match{
		TargetRuns: []model.Run{{Text: &model.TextRun{Text: t}}},
		Score:      100,
		Exact:      true,
	}, true
}

func (m staticMemoryProvider) PriorVersion(context.Context, corememory.VersionRequest) (corememory.Version, bool) {
	return corememory.Version{}, false
}

func TestTranslateAfterFromConfig_RefusesAnUnknownLevel(t *testing.T) {
	_, err := NewTranslateAfterFromConfig(map[string]any{"level": "checked"}, "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `level "checked" is not a source level`)

	tl, err := NewTranslateAfterFromConfig(map[string]any{}, "")
	require.NoError(t, err)
	assert.NotNil(t, tl)
}
