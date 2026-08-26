package leverage_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/leverage"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The gate through the WHOLE tool, which is a different question from the gate
// in the provider.
//
// The provider classifies and the block path refuses, and for a while that was
// all this was tested at — while the tool went on to try its plain-text path,
// which asks the same corpus keyed differently, gets back a translation with no
// source to compare, and fills on the score alone. Every refusal above 95 was
// answered by the same match coming in through another door.
//
// Nothing failed. The target was written, stamped a reviewable draft, and the
// only evidence was the Norwegian saying the opposite of the English.

// runRecycle puts one block through the real tool and returns the target it
// wrote, or "" when it left the block for a translator.
func runRecycle(t *testing.T, tm memory.ContentMemory, source string, fillFloor int) string {
	t.Helper()

	cfg := &tools.MemoryLeverageConfig{}
	cfg.Reset()
	cfg.SourceLocale = "en"
	cfg.TargetLocale = "nb"
	cfg.FillTargetThreshold = fillFloor
	cfg.Memory = leverage.NewProvider(tm)

	tl := tools.NewMemoryLeverageTool(cfg)
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: blockOf(source)}
	close(in)
	require.NoError(t, tl.Process(t.Context(), in, out))
	close(out)

	part := <-out
	require.NotNil(t, part)
	block, ok := part.Resource.(*model.Block)
	require.True(t, ok)
	return block.TargetText("nb")
}

func pairCorpus(t *testing.T, source, target string) *memory.InMemoryStore {
	t.Helper()
	tm := memory.NewInMemoryStore()
	require.NoError(t, tm.Add(t.Context(), memory.Entry{
		ID:          "approved",
		HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: source}}},
			"nb": {{Text: &model.TextRun{Text: target}}},
		},
	}))
	return tm
}

// The sentence the /coordinate ladder is built on, so the numbers in the test
// and the numbers on the page are the same numbers.
const (
	ladderSource  = "Click the button below when you're ready to continue with your account setup"
	ladderTarget  = "Klikk på knappen nedenfor når du er klar til å fortsette med kontooppsettet"
	ladderNegated = "Click the button below when you're not ready to continue with your account setup"
)

func TestASubstantiveEditIsRefusedByTheWholeTool(t *testing.T) {
	t.Parallel()

	tm := pairCorpus(t, ladderSource, ladderTarget)

	// Scores 95, and the fill floor is 95, so the score says yes twice: once in
	// the block path and once in the text path behind it.
	got := runRecycle(t, tm, ladderNegated, 95)
	assert.Empty(t, got, "a meaning inversion must reach a translator, not a reviewer's rubber stamp")
}

func TestACosmeticEditStillFillsThroughTheWholeTool(t *testing.T) {
	t.Parallel()

	// The mirror, and the reason the refusal above cannot be implemented by
	// raising the floor: this scores 91, below any floor that would have
	// refused the inversion.
	tm := pairCorpus(t, "Get started", "Kom i gang")
	assert.Equal(t, "Kom i gang", runRecycle(t, tm, "Get started.", 95),
		"the words did not move, so the approved answer stands")
}

func TestTheRecycleToolRecordsWhatItRefusesToFill(t *testing.T) {
	t.Parallel()

	// Refusing to fill is not throwing the match away: it stays recorded, so a
	// translator sees what was approved for the old wording.
	tm := pairCorpus(t, ladderSource, ladderTarget)
	cfg := &tools.MemoryLeverageConfig{}
	cfg.Reset()
	cfg.SourceLocale = "en"
	cfg.TargetLocale = "nb"
	cfg.Memory = leverage.NewProvider(tm)

	tl := tools.NewMemoryLeverageTool(cfg)
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- &model.Part{
		Type:     model.PartBlock,
		Resource: blockOf(ladderNegated),
	}
	close(in)
	require.NoError(t, tl.Process(t.Context(), in, out))
	close(out)

	block, ok := (<-out).Resource.(*model.Block)
	require.True(t, ok)
	assert.Empty(t, block.TargetText("nb"))

	m, recorded := model.AnnoAs[*tools.MemoryMatchAnnotation](block, string(model.AnnoMemoryMatch))
	require.True(t, recorded, "the match is still the best reference this block has")
	assert.Equal(t, 95, m.Score)
}
