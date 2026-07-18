package tools_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/ai/tools"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// heldBlock builds a translatable block carrying the source-gate hold marker.
func heldBlock(id, text string) *model.Block {
	b := model.NewBlock(id, text)
	b.Translatable = true
	b.SetSourceHeld(true)
	return b
}

func pushBlock(t *testing.T, tl *tools.AITranslateTool, b *model.Block) {
	t.Helper()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: b}
	close(in)
	require.NoError(t, tl.Process(context.Background(), in, out))
}

// TestAITranslate_SkipsSourceHeld_SinglePath: a block held on its un-settled
// source is not translated on the single-block path — the provider is never
// called and no target is written — while an un-held block translates normally
// (epic 019: the local source gate holds an un-ready source).
func TestAITranslate_SkipsSourceHeld_SinglePath(t *testing.T) {
	mock, calls := newTranslateMock(t)
	tl := tools.NewAITranslateTool(mock, singleBlockConfig())

	held := heldBlock("h1", "Hello")
	pushBlock(t, tl, held)
	assert.Equal(t, 0, *calls, "the provider is never called for a held block")
	assert.False(t, held.HasTarget(model.LocaleFrench))

	ready := model.NewBlock("r1", "Hello")
	ready.Translatable = true
	pushBlock(t, tl, ready)
	assert.Equal(t, 1, *calls, "an un-held block translates")
	assert.Equal(t, "[fr] Hello", ready.TargetText(model.LocaleFrench))
}

// TestAITranslate_SkipsSourceHeld_BatchPath: the batched path also drops a held
// block from the batch — a mixed batch translates only its ready blocks.
func TestAITranslate_SkipsSourceHeld_BatchPath(t *testing.T) {
	mock, _ := newTranslateMock(t)
	cfg := singleBlockConfig()
	cfg.BatchSize = 10
	cfg.BatchConcurrency = 1
	tl := tools.NewAITranslateTool(mock, cfg)

	held := heldBlock("h1", "Hello")
	ready := model.NewBlock("r1", "World")
	ready.Translatable = true

	in := make(chan *model.Part, 2)
	out := make(chan *model.Part, 2)
	in <- &model.Part{Type: model.PartBlock, Resource: held}
	in <- &model.Part{Type: model.PartBlock, Resource: ready}
	close(in)
	require.NoError(t, tl.Process(context.Background(), in, out))

	assert.False(t, held.HasTarget(model.LocaleFrench), "held block dropped from the batch")
	assert.True(t, ready.HasTarget(model.LocaleFrench), "ready block translated")
}
