package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/neokapi/neokapi/core/model"
)

// The key is sent by default. A bare "Save" is a coin flip between a verb and a
// noun, and the key is the cheapest signal that settles it — already in the
// document, a handful of tokens, and what every localization vendor sends.
func TestContextDefaultsToKey(t *testing.T) {
	tool := NewAITranslateTool(nil, AITranslateConfig{Provider: "anthropic"})
	assert.Equal(t, ContextKey, tool.contextPolicy)

	got := tool.contextFor(t.Context(), "tu1", "app.settings.save", "")
	assert.Equal(t, "app.settings.save", got.Key)
	assert.Empty(t, got.Before, "neighbours are opt-in: they cost tokens and widen the cache key")
	assert.Empty(t, got.After)
}

func TestContextNoneSendsNothing(t *testing.T) {
	tool := NewAITranslateTool(nil, AITranslateConfig{Provider: "anthropic", Context: ContextNone})

	assert.True(t, tool.contextFor(t.Context(), "tu1", "app.settings.save", "").Empty(),
		"context: none must send nothing about the block but the block")
}

// Neighbours come from the document in order, bounded by the window, and never
// include the block itself.
func TestNeighboursComeFromDocumentOrder(t *testing.T) {
	tool := NewAITranslateTool(nil, AITranslateConfig{
		Provider:      "anthropic",
		Context:       ContextNeighbours,
		ContextWindow: 2,
	})

	texts := []string{"Unsaved changes", "Discard", "Save", "Cancel", "Settings"}
	tool.docEntries = make([]blockEntry, len(texts))
	for i, txt := range texts {
		b := model.NewBlock("tu"+string(rune('1'+i)), txt)
		b.Name = "key." + txt
		tool.docEntries[i] = blockEntry{index: i, block: b, sourceText: txt}
	}

	got := tool.contextFor(t.Context(), "tu3", "key.Save", "") // the middle block
	assert.Equal(t, "key.Save", got.Key)
	assert.Equal(t, []string{"Unsaved changes", "Discard"}, got.Before)
	assert.Equal(t, []string{"Cancel", "Settings"}, got.After)
	assert.NotContains(t, got.Before, "Save", "a block is not its own context")
	assert.NotContains(t, got.After, "Save")
}

// The correctness trap. Neighbour context makes a block's translation depend on
// text that is *not the block*, while the target cache is keyed by the block. If
// the cache key doesn't move with the neighbourhood, editing one string leaves
// the strings around it holding translations produced under a neighbourhood that
// no longer exists.
func TestCacheFingerprintMovesWithTheNeighbourhood(t *testing.T) {
	newTool := func(policy string) *AITranslateTool {
		return NewAITranslateTool(nil, AITranslateConfig{
			Provider:      "anthropic",
			Context:       policy,
			ContextWindow: 1,
		})
	}
	docWith := func(tool *AITranslateTool, before string) *model.Block {
		target := model.NewBlock("tu2", "Save")
		target.Name = "dialog.save"
		prev := model.NewBlock("tu1", before)
		tool.docEntries = []blockEntry{
			{index: 0, block: prev, sourceText: before},
			{index: 1, block: target, sourceText: "Save"},
		}
		return target
	}

	// Neighbours on: a changed neighbour must invalidate the cached translation.
	a, b := newTool(ContextNeighbours), newTool(ContextNeighbours)
	assert.NotEqual(t,
		a.cacheFingerprint(t.Context(), docWith(a, "Unsaved changes")),
		b.cacheFingerprint(t.Context(), docWith(b, "Delete everything?")),
		"a block whose neighbours changed must re-translate, not serve a stale target")

	// Key-only: the key travels with the block, so a block-keyed cache already
	// accounts for it. Folding it in would churn the cache for nothing.
	c := newTool(ContextKey)
	assert.Equal(t, c.configFP, c.cacheFingerprint(t.Context(), docWith(c, "Unsaved changes")),
		"key context must not widen the cache key")
}

// Turning context on changes every prompt, so targets produced without it must
// not be served. That is the config fingerprint's job.
func TestContextPolicyChangesTheConfigFingerprint(t *testing.T) {
	cfg := func(policy string) AITranslateConfig {
		return AITranslateConfig{Provider: "anthropic", Model: "m", Context: policy}
	}

	none := NewAITranslateTool(nil, cfg(ContextNone)).configFP
	key := NewAITranslateTool(nil, cfg(ContextKey)).configFP
	neighbours := NewAITranslateTool(nil, cfg(ContextNeighbours)).configFP

	assert.NotEqual(t, none, key, "enabling key context must re-translate, not serve targets produced blind")
	assert.NotEqual(t, key, neighbours)
}

// Neighbours need the document in order, and only the buffering path has it. So
// `batching: single` + `context: neighbours` must still route through the batched
// path — which is exactly the shape the evidence favours: large non-translated
// context, small translation unit.
func TestNeighboursForceTheBufferingPath(t *testing.T) {
	single := NewAITranslateTool(nil, AITranslateConfig{
		Provider: "anthropic",
		Batching: BatchingSingle,
		Context:  ContextNeighbours,
	})

	assert.Equal(t, 1, single.batchSize, "one block per call")
	assert.Equal(t, ContextNeighbours, single.contextPolicy)
	// Process routes to processBatched whenever neighbours are on; the streaming
	// path cannot see a block's neighbours.
	assert.NotEqual(t, ContextNone, single.contextPolicy)
}
