package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiffLeverageTool(t *testing.T) {
	t.Parallel()
	cfg := &tools.DiffLeverageConfig{
		TargetLocale:  model.LocaleFrench,
		PreviousTexts: map[string]tools.PreviousBlock{},
		CaseSensitive: true,
	}
	tl := tools.NewDiffLeverageTool(cfg)

	assert.Equal(t, "diff-leverage", tl.Name())
	assert.Contains(t, tl.Description(), "previous version")
}

func TestDiffLeverageUnchangedSource(t *testing.T) {
	t.Parallel()
	cfg := &tools.DiffLeverageConfig{
		TargetLocale: model.LocaleFrench,
		PreviousTexts: map[string]tools.PreviousBlock{
			"tu1": {SourceText: "Hello world", TargetText: "Bonjour le monde"},
		},
		CaseSensitive: true,
	}
	tl := tools.NewDiffLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "unchanged", resultBlock.Properties[tools.PropDiffLeverageStatus])
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))

	// An identical source keeps its prior translation as-is: no draft downgrade.
	// (Coverage falls to the presence baseline of `translated`.)
	if tgt := resultBlock.Target(model.LocaleFrench); assert.NotNil(t, tgt) {
		assert.Empty(t, tgt.Status, "unchanged leverage must not downgrade to draft")
	}
}

func TestDiffLeverageModifiedSource(t *testing.T) {
	t.Parallel()
	cfg := &tools.DiffLeverageConfig{
		TargetLocale: model.LocaleFrench,
		PreviousTexts: map[string]tools.PreviousBlock{
			"tu1": {SourceText: "Hello world", TargetText: "Bonjour le monde"},
		},
		CaseSensitive: true,
	}
	tl := tools.NewDiffLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello new world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "modified", resultBlock.Properties[tools.PropDiffLeverageStatus])
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestDiffLeverageNewBlock(t *testing.T) {
	t.Parallel()
	cfg := &tools.DiffLeverageConfig{
		TargetLocale: model.LocaleFrench,
		PreviousTexts: map[string]tools.PreviousBlock{
			"tu1": {SourceText: "Hello world", TargetText: "Bonjour le monde"},
		},
		CaseSensitive: true,
	}
	tl := tools.NewDiffLeverageTool(cfg)

	block := model.NewBlock("tu2", "Goodbye world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "new", resultBlock.Properties[tools.PropDiffLeverageStatus])
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestDiffLeverageFuzzyMatch(t *testing.T) {
	t.Parallel()
	cfg := &tools.DiffLeverageConfig{
		TargetLocale: model.LocaleFrench,
		PreviousTexts: map[string]tools.PreviousBlock{
			"tu1": {SourceText: "Hello world", TargetText: "Bonjour le monde"},
		},
		CaseSensitive: true,
		FuzzyMatch:    true,
	}
	tl := tools.NewDiffLeverageTool(cfg)

	// Small change — should be above 70% similarity.
	block := model.NewBlock("tu1", "Hello World")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "leveraged", resultBlock.Properties[tools.PropDiffLeverageStatus])
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))

	score := resultBlock.Properties[tools.PropDiffLeverageScore]
	require.NotEmpty(t, score, "Expected a similarity score")

	// The leveraged target rode over onto a changed source, so it needs review:
	// it must be stamped `draft`, not counted as fully `translated`.
	tgt := resultBlock.Target(model.LocaleFrench)
	if assert.NotNil(t, tgt) {
		assert.Equal(t, model.TargetStatusDraft, tgt.Status)
	}
}

func TestDiffLeverageFuzzyMatchBelowThreshold(t *testing.T) {
	t.Parallel()
	cfg := &tools.DiffLeverageConfig{
		TargetLocale: model.LocaleFrench,
		PreviousTexts: map[string]tools.PreviousBlock{
			"tu1": {SourceText: "Hello world", TargetText: "Bonjour le monde"},
		},
		CaseSensitive: true,
		FuzzyMatch:    true,
	}
	tl := tools.NewDiffLeverageTool(cfg)

	// Completely different text — below 70%.
	block := model.NewBlock("tu1", "Something entirely different here now")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "modified", resultBlock.Properties[tools.PropDiffLeverageStatus])
	_, hasScore := resultBlock.Properties[tools.PropDiffLeverageScore]
	assert.False(t, hasScore, "Should not have a score for non-leveraged blocks")
}

func TestDiffLeverageSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	cfg := &tools.DiffLeverageConfig{
		TargetLocale: model.LocaleFrench,
		PreviousTexts: map[string]tools.PreviousBlock{
			"tu1": {SourceText: "Hello world", TargetText: "Bonjour le monde"},
		},
		CaseSensitive: true,
	}
	tl := tools.NewDiffLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasStatus := resultBlock.Properties[tools.PropDiffLeverageStatus]
	assert.False(t, hasStatus)
}

func TestDiffLeverageCaseInsensitive(t *testing.T) {
	t.Parallel()
	cfg := &tools.DiffLeverageConfig{
		TargetLocale: model.LocaleFrench,
		PreviousTexts: map[string]tools.PreviousBlock{
			"tu1": {SourceText: "Hello World", TargetText: "Bonjour le monde"},
		},
		CaseSensitive: false,
	}
	tl := tools.NewDiffLeverageTool(cfg)

	block := model.NewBlock("tu1", "hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "unchanged", resultBlock.Properties[tools.PropDiffLeverageStatus])
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
}

func TestDiffLeverageConfigValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     tools.DiffLeverageConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing target locale",
			cfg:     tools.DiffLeverageConfig{PreviousTexts: map[string]tools.PreviousBlock{}},
			wantErr: true,
			errMsg:  "TargetLocale",
		},
		{
			name:    "missing previous texts",
			cfg:     tools.DiffLeverageConfig{TargetLocale: model.LocaleFrench},
			wantErr: true,
			errMsg:  "PreviousTexts",
		},
		{
			name: "valid config",
			cfg: tools.DiffLeverageConfig{
				TargetLocale:  model.LocaleFrench,
				PreviousTexts: map[string]tools.PreviousBlock{},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestDiffLeverageConfigReset(t *testing.T) {
	t.Parallel()
	cfg := &tools.DiffLeverageConfig{
		TargetLocale:  model.LocaleFrench,
		PreviousTexts: map[string]tools.PreviousBlock{"tu1": {}},
		CaseSensitive: false,
		FuzzyMatch:    true,
	}
	cfg.Reset()

	assert.True(t, cfg.TargetLocale.IsEmpty())
	assert.Nil(t, cfg.PreviousTexts)
	assert.True(t, cfg.CaseSensitive)
	assert.False(t, cfg.FuzzyMatch)
}
