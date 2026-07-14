package tools

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/ai/prompt"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

// batchSize 0 means "pack from the model's output ceiling". Only an explicit pin
// or a local provider produces a fixed count.
func TestBatchingIntent(t *testing.T) {
	// Cloud provider, default intent → packed, not a fixed count.
	auto := NewAITranslateTool(nil, AITranslateConfig{Provider: "anthropic"})
	assert.Equal(t, 0, auto.batchSize, "auto batching packs; it does not pin a count")

	// The user asked for one block per call.
	single := NewAITranslateTool(nil, AITranslateConfig{Provider: "anthropic", Batching: BatchingSingle})
	assert.Equal(t, 1, single.batchSize)

	// Local providers translate per-block regardless: a small on-device model
	// ignores the "return JSON" instruction on a large batch and emits prose.
	local := NewAITranslateTool(nil, AITranslateConfig{Provider: "demo"})
	assert.Equal(t, 1, local.batchSize, "local provider should default to per-block")

	// An explicit pin wins over everything — it is the eval sweep's knob, and a
	// recipe's way to reproduce a historical run.
	explicit := NewAITranslateTool(nil, AITranslateConfig{Provider: "demo", BatchSize: 8})
	assert.Equal(t, 8, explicit.batchSize)
}

// Packing is bounded by what the model can emit, not by a block count. A fixed
// count is the wrong unit: the same 100 blocks are 300 tokens of UI strings or
// 200k tokens of prose, and only one of those fits in a reply.
func TestPackBlocksRespectsOutputBudget(t *testing.T) {
	long := make([]blockEntry, 8)
	for i := range long {
		// ~250 estimated tokens each.
		long[i] = blockEntry{sourceText: string(make([]byte, 1000))}
	}

	batches := packBlocks(long, 512, MaxBlocksPerCall)
	require.NotEmpty(t, batches)
	for _, b := range batches {
		assert.LessOrEqual(t, len(b), 2, "a batch must not exceed the output budget")
	}
	assert.Equal(t, 8, countEntries(batches), "every block lands in exactly one batch")

	// Short blocks pack up to the ceiling, and no further.
	short := make([]blockEntry, 40)
	for i := range short {
		short[i] = blockEntry{sourceText: "Save"}
	}
	batches = packBlocks(short, 100_000, MaxBlocksPerCall)
	require.NotEmpty(t, batches)
	for _, b := range batches {
		assert.LessOrEqual(t, len(b), MaxBlocksPerCall,
			"batch size is capped independently of length: quality degrades with N")
	}
	assert.Equal(t, 40, countEntries(batches), "every block lands in exactly one batch")
}

// A single block larger than the whole budget still gets translated — in a batch
// of one. Dropping it would be worse than asking for a reply we may have to
// retry smaller.
func TestPackBlocksNeverDropsAnOversizedBlock(t *testing.T) {
	huge := blockEntry{sourceText: string(make([]byte, 100_000))}
	batches := packBlocks([]blockEntry{huge}, 512, MaxBlocksPerCall)

	require.Len(t, batches, 1)
	assert.Len(t, batches[0], 1)
}

// An unknown model must pack conservatively rather than optimistically: a plugin
// provider, or a model newer than this build, should produce smaller batches
// rather than truncated replies.
func TestOutputBudgetIsConservativeForUnknownProviders(t *testing.T) {
	budget := outputBudget(aiprovider.NewMockProvider())

	assert.Positive(t, budget)
	assert.LessOrEqual(t, budget, aiprovider.ConservativeMaxOutputTokens,
		"an unknown model must not be assumed to have a large output ceiling")
}

func countEntries(batches [][]blockEntry) int {
	var n int
	for _, b := range batches {
		n += len(b)
	}
	return n
}

// The cap requested for a batch's reply has to pay for the reply's shape, not
// just its words. Every item comes back wrapped in `{"id":"s123","text":"…"},`,
// and on a batch of many short strings that scaffolding is most of the reply.
//
// The old budget (source tokens x2 + 512) ignored it, and the eval caught the
// consequence on a real model: a 600-block batch asked for 31.5k output tokens
// when the reply needed ~33k, got truncated, and was re-translated in halves —
// 2.4x the tokens and 6x the wall-clock, at an unchanged 100% integrity. Nothing
// was corrupted, which is exactly why it went unnoticed.
func TestBatchOutputBudgetPaysForTheReplyScaffolding(t *testing.T) {
	t.Parallel()

	// 600 short UI strings: the reply is dominated by ids and JSON syntax.
	texts := make([]string, 600)
	for i := range texts {
		texts[i] = "Save"
	}
	segments := prompt.BatchSegments(texts)

	textOnly := 0
	for _, s := range segments {
		textOnly += estimateTokens(s.Text) * 2
	}
	textOnly += 512 // what the old budget would have asked for

	got := batchOutputBudget(segments)
	assert.Greater(t, got, textOnly+len(segments)*ReplyItemOverheadTokens-1,
		"the budget must charge for every item's scaffolding, not only its text")

	// And it must still scale with length, not just count.
	long := prompt.BatchSegments([]string{strings.Repeat("word ", 2000)})
	assert.Greater(t, batchOutputBudget(long), 2000,
		"a single long segment still needs room for a longer translation")
}
