package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// linkRunsFr is the French analogue of linkRuns: "Cliquez <a>ici</a>".
func linkRunsFr() []model.Run {
	return []model.Run{
		{Text: &model.TextRun{Text: "Cliquez "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "link", Data: "<a>"}},
		{Text: &model.TextRun{Text: "ici"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "link", Data: "</a>"}},
	}
}

func TestInlineCodesRemoveToolTarget(t *testing.T) {
	t.Parallel()
	cfg := &tools.InlineCodesRemoveConfig{
		ApplyTarget:  true,
		TargetLocale: model.LocaleFrench,
	}
	tl := tools.NewInlineCodesRemoveTool(cfg)

	assert.Equal(t, "inline-codes-remove", tl.Name())

	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       []model.Run{{Text: &model.TextRun{Text: "Click here"}}},
		Properties:   make(map[string]string),
	}
	block.SetTargetRuns(model.LocaleFrench, linkRunsFr())
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	runs := resultBlock.TargetRuns(model.LocaleFrench)
	require.NotNil(t, runs)

	assert.Equal(t, "Cliquez ici", model.RunsPlainText(runs))
	assert.False(t, hasAnyInlineCode(runs))
}

func TestInlineCodesRemoveToolSource(t *testing.T) {
	t.Parallel()
	cfg := &tools.InlineCodesRemoveConfig{
		ApplySource: true,
		ApplyTarget: false,
	}
	tl := tools.NewInlineCodesRemoveTool(cfg)

	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       linkRuns(),
		Properties:   make(map[string]string),
	}
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	runs := resultBlock.Source
	assert.Equal(t, "Click here", model.RunsPlainText(runs))
	assert.False(t, hasAnyInlineCode(runs))
}

func TestInlineCodesRemoveToolMixedRunsBecomesPlainText(t *testing.T) {
	t.Parallel()
	cfg := &tools.InlineCodesRemoveConfig{
		ApplySource: true,
		ApplyTarget: false,
	}
	tl := tools.NewInlineCodesRemoveTool(cfg)

	// "Hello <b>world</b> and <img/>"
	runs := []model.Run{
		{Text: &model.TextRun{Text: "Hello "}},
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "b"}},
		{Text: &model.TextRun{Text: "world"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "b"}},
		{Text: &model.TextRun{Text: " and "}},
		{Ph: &model.PlaceholderRun{ID: "2", Type: "img"}},
	}
	require.True(t, hasAnyInlineCode(runs))

	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       runs,
		Properties:   make(map[string]string),
	}
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	out := resultBlock.Source
	assert.Equal(t, "Hello world and ", model.RunsPlainText(out))
	assert.False(t, hasAnyInlineCode(out))
}

func TestInlineCodesRemoveToolSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	cfg := &tools.InlineCodesRemoveConfig{
		ApplySource: true,
		ApplyTarget: false,
	}
	tl := tools.NewInlineCodesRemoveTool(cfg)

	block := &model.Block{
		ID:           "tu1",
		Translatable: false,
		Source:       linkRuns(),
		Properties:   make(map[string]string),
	}
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// Inline codes should still be present since block is non-translatable.
	assert.True(t, hasAnyInlineCode(resultBlock.Source))
}

func TestInlineCodesRemoveConfigValidation(t *testing.T) {
	t.Parallel()
	cfg := &tools.InlineCodesRemoveConfig{
		ApplyTarget:  true,
		TargetLocale: "",
	}
	err := cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "target locale")

	cfg.ApplyTarget = false
	cfg.ApplySource = false
	err = cfg.Validate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ApplySource")

	cfg.ApplySource = true
	err = cfg.Validate()
	require.NoError(t, err)
}

// hasAnyInlineCode reports whether any run in the sequence is something
// other than a TextRun.
func hasAnyInlineCode(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil {
			return true
		}
	}
	return false
}
