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

// gate drives one block through the source-gate stage's Process pipeline, as a
// real run does.
func gate(t *testing.T, gt *SourceGateTool, b *model.Block) {
	t.Helper()
	require.NoError(t, applyProduce(t, gt, b))
}

// TestSourceGateTool_SettlesAndHoldsBelowGate: the source-gate stage settles a
// clean block to `checked` and, under the `approved` gate, holds it (checked is
// below approved) — stamping the hold marker and counting it.
func TestSourceGateTool_SettlesAndHoldsBelowGate(t *testing.T) {
	b := srcBlock("Hello world")
	gt := NewSourceGateTool(model.SourceGateApproved)
	gate(t, gt, b)

	assert.Equal(t, model.SourceStatusChecked, b.SourceStatus, "settled to checked")
	assert.True(t, b.SourceHeld(), "checked is below approved → held")
	held, total := gt.Snapshot()
	assert.Equal(t, 1, held)
	assert.Equal(t, 1, total)
}

// TestSourceGateTool_AdmitsAtGate: a clean block clears the default (checked)
// gate — settled to checked, not held, any stale marker cleared.
func TestSourceGateTool_AdmitsAtGate(t *testing.T) {
	b := srcBlock("Hello world")
	b.SetSourceHeld(true) // a stale hold from a prior pass
	gt := NewSourceGateTool(model.SourceGateChecked)
	gate(t, gt, b)

	assert.Equal(t, model.SourceStatusChecked, b.SourceStatus)
	assert.False(t, b.SourceHeld(), "clean source clears the checked gate; the stale hold is cleared")
	held, _ := gt.Snapshot()
	assert.Equal(t, 0, held)
}

// TestSourceGateTool_NoneIsPassthrough: the `none` opt-out settles nothing and
// holds nothing — no status change, no marker, no count.
func TestSourceGateTool_NoneIsPassthrough(t *testing.T) {
	b := srcBlock("Hello world")
	gt := NewSourceGateTool(model.SourceGateNone)
	gate(t, gt, b)

	assert.Equal(t, model.SourceStatusNew, b.SourceStatus, "none never settles")
	assert.False(t, b.SourceHeld())
	held, total := gt.Snapshot()
	assert.Equal(t, 0, held)
	assert.Equal(t, 0, total)
}

// TestSourceGateTool_WhitespaceHeldAtChecked: a whitespace-only source trips the
// major content-lint finding, so it stays at the authored baseline and is held
// below the checked gate — the partial-hold case.
func TestSourceGateTool_WhitespaceHeldAtChecked(t *testing.T) {
	b := srcBlock("   ")
	gt := NewSourceGateTool(model.SourceGateChecked)
	gate(t, gt, b)

	assert.Equal(t, model.SourceStatusNew, b.SourceStatus, "a major source finding keeps it at authored")
	assert.True(t, b.SourceHeld())
}

// TestProducersSkipHeldSource: both producers — recycle and AI translate — skip
// a block carrying the source-gate hold marker, leaving no target; a block
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
