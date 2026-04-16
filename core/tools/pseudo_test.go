package tools_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// processPart is a helper that sends a single Part through a tool and returns the result.
func processPart(t *testing.T, tl interface {
	Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error
}, part *model.Part) *model.Part {
	t.Helper()
	in := make(chan *model.Part, 1)
	out := make(chan *model.Part, 1)
	in <- part
	close(in)

	err := tl.Process(t.Context(), in, out)
	close(out)
	require.NoError(t, err)

	result := <-out
	require.NotNil(t, result)
	return result
}

func TestPseudoTranslateTool(t *testing.T) {
	t.Parallel()
	cfg := &tools.PseudoConfig{
		ExpansionPercent: 0,
		Prefix:           "[",
		Suffix:           "]",
		TargetLocale:     "qps",
	}
	tl := tools.NewPseudoTranslateTool(cfg)

	assert.Equal(t, "pseudo-translate", tl.Name())
	assert.Contains(t, tl.Description(), "pseudo")

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	targetText := resultBlock.TargetText("qps")

	// Should be wrapped in brackets.
	assert.True(t, len(targetText) > 0)
	assert.Equal(t, '[', rune(targetText[0]))
	assert.Equal(t, ']', rune(targetText[len(targetText)-1]))

	// Should contain accented characters, not the original ASCII.
	assert.NotContains(t, targetText, "Hello")
	// The 'e' in "Hello" should have been replaced with 'é'.
	assert.Contains(t, targetText, "\u00e9")
	// The 'o' in "Hello" should have been replaced with 'ö'.
	assert.Contains(t, targetText, "\u00f6")
}

func TestPseudoTranslateToolWithExpansion(t *testing.T) {
	t.Parallel()
	cfg := &tools.PseudoConfig{
		ExpansionPercent: 50,
		Prefix:           "[",
		Suffix:           "]",
		TargetLocale:     "qps",
	}
	tl := tools.NewPseudoTranslateTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	targetText := resultBlock.TargetText("qps")

	// With 50% expansion on 5 chars, should add padding of 2 tildes + space.
	// Total should be longer than just accented + brackets.
	assert.Contains(t, targetText, "~~")
	assert.True(t, len([]rune(targetText)) > len([]rune("[Ĥéļļö]")))
}

func TestPseudoTranslateToolSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	cfg := &tools.PseudoConfig{
		TargetLocale: "qps",
		Prefix:       "[",
		Suffix:       "]",
	}
	tl := tools.NewPseudoTranslateTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget("qps"))
}

func TestPseudoTranslateToolCustomPrefixSuffix(t *testing.T) {
	t.Parallel()
	cfg := &tools.PseudoConfig{
		Prefix:       "<<",
		Suffix:       ">>",
		TargetLocale: "qps",
	}
	tl := tools.NewPseudoTranslateTool(cfg)

	block := model.NewBlock("tu1", "Test")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	targetText := resultBlock.TargetText("qps")

	assert.True(t, len(targetText) >= 4)
	assert.Equal(t, "<<", targetText[:2])
	assert.Equal(t, ">>", targetText[len(targetText)-2:])
}

// linkRuns builds the Run sequence for "Click <a>here</a>".
func linkRuns() []model.Run {
	return []model.Run{
		{Text: &model.TextRun{Text: "Click "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "link", Data: "<a>"}},
		{Text: &model.TextRun{Text: "here"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "link", Data: "</a>"}},
	}
}

func TestPseudoTranslateToolPreservesSpans(t *testing.T) {
	t.Parallel()
	cfg := &tools.PseudoConfig{
		Prefix:       "[",
		Suffix:       "]",
		TargetLocale: "qps",
	}
	tl := tools.NewPseudoTranslateTool(cfg)

	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       []*model.Segment{{ID: "s1", Runs: linkRuns()}},
		Targets:      make(map[model.LocaleID][]*model.Segment),
	}
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)

	require.True(t, resultBlock.HasTarget("qps"))
	targetSegs := resultBlock.Targets["qps"]
	require.Len(t, targetSegs, 1)

	runs := targetSegs[0].Runs

	// Inline-code runs should be preserved (PcOpen + PcClose).
	var pcOpens, pcCloses int
	var textParts []string
	for _, r := range runs {
		switch {
		case r.PcOpen != nil:
			pcOpens++
			assert.Equal(t, "link", r.PcOpen.Type)
			assert.Equal(t, "1", r.PcOpen.ID)
		case r.PcClose != nil:
			pcCloses++
			assert.Equal(t, "1", r.PcClose.ID)
		case r.Text != nil:
			textParts = append(textParts, r.Text.Text)
		}
	}
	assert.Equal(t, 1, pcOpens)
	assert.Equal(t, 1, pcCloses)

	// The TextRuns combined should be bracket-wrapped and accented.
	plain := model.RunsPlainText(runs)
	assert.Equal(t, '[', rune(plain[0]))
	assert.Equal(t, ']', rune(plain[len(plain)-1]))
	assert.NotContains(t, plain, "Click")
	assert.NotContains(t, plain, "here")
}

func TestPseudoTranslateToolSpansWithExpansion(t *testing.T) {
	t.Parallel()
	cfg := &tools.PseudoConfig{
		ExpansionPercent: 50,
		Prefix:           "[",
		Suffix:           "]",
		TargetLocale:     "qps",
	}
	tl := tools.NewPseudoTranslateTool(cfg)

	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       []*model.Segment{{ID: "s1", Runs: linkRuns()}},
		Targets:      make(map[model.LocaleID][]*model.Segment),
	}
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	runs := resultBlock.Targets["qps"][0].Runs

	// Expansion padding should appear in the text projection.
	assert.Contains(t, model.RunsPlainText(runs), "~~")

	// Inline-code runs preserved.
	var pcOpens, pcCloses int
	for _, r := range runs {
		switch {
		case r.PcOpen != nil:
			pcOpens++
		case r.PcClose != nil:
			pcCloses++
		}
	}
	assert.Equal(t, 1, pcOpens)
	assert.Equal(t, 1, pcCloses)
}

func TestPseudoConfigValidation(t *testing.T) {
	t.Parallel()
	cfg := &tools.PseudoConfig{ExpansionPercent: -1, TargetLocale: "qps"}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ExpansionPercent")

	cfg.ExpansionPercent = 0
	cfg.TargetLocale = ""
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "TargetLocale")

	cfg.TargetLocale = "qps"
	err = cfg.Validate()
	require.NoError(t, err)
}
