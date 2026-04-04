package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCharsListingTool(t *testing.T) {
	cfg := &tools.CharsListingConfig{
		IncludeSource: true,
		IncludeTarget: false,
	}
	result := tools.NewCharsListingTool(cfg)
	tl := result.Tool()

	assert.Equal(t, "chars-listing", tl.Name())
	assert.Contains(t, tl.Description(), "character")
}

func TestCharsListingUniqueCount(t *testing.T) {
	cfg := &tools.CharsListingConfig{
		IncludeSource: true,
		IncludeTarget: false,
	}
	result := tools.NewCharsListingTool(cfg)
	tl := result.Tool()

	// "Hello" has 4 unique chars: H, e, l, o
	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	out := processPart(t, tl, part)

	resultBlock := out.Resource.(*model.Block)
	assert.Equal(t, "4", resultBlock.Properties[tools.PropCharsListingCount])

	// Verify accumulated char counts.
	counts := result.CharCounts()
	assert.Equal(t, 1, counts['H'])
	assert.Equal(t, 1, counts['e'])
	assert.Equal(t, 2, counts['l'])
	assert.Equal(t, 1, counts['o'])
}

func TestCharsListingSourceAndTarget(t *testing.T) {
	cfg := &tools.CharsListingConfig{
		IncludeSource: true,
		IncludeTarget: true,
		TargetLocale:  model.LocaleFrench,
	}
	result := tools.NewCharsListingTool(cfg)
	tl := result.Tool()

	// "Hi" source + "Oi" target = unique chars: H, i, O (3 unique)
	block := model.NewBlock("tu1", "Hi")
	block.SetTargetText(model.LocaleFrench, "Oi")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	out := processPart(t, tl, part)

	resultBlock := out.Resource.(*model.Block)
	assert.Equal(t, "3", resultBlock.Properties[tools.PropCharsListingCount])

	counts := result.CharCounts()
	assert.Equal(t, 1, counts['H'])
	assert.Equal(t, 2, counts['i']) // appears in both source and target
	assert.Equal(t, 1, counts['O'])
}

func TestCharsListingAccumulatesAcrossBlocks(t *testing.T) {
	cfg := &tools.CharsListingConfig{
		IncludeSource: true,
		IncludeTarget: false,
	}
	result := tools.NewCharsListingTool(cfg)
	tl := result.Tool()

	block1 := model.NewBlock("tu1", "ab")
	block2 := model.NewBlock("tu2", "bc")
	parts := []*model.Part{
		{Type: model.PartBlock, Resource: block1},
		{Type: model.PartBlock, Resource: block2},
	}
	results := processMultipleParts(t, tl, parts)
	require.Len(t, results, 2)

	// Block 1: "ab" = 2 unique chars
	rb1 := results[0].Resource.(*model.Block)
	assert.Equal(t, "2", rb1.Properties[tools.PropCharsListingCount])

	// Block 2: "bc" = 2 unique chars
	rb2 := results[1].Resource.(*model.Block)
	assert.Equal(t, "2", rb2.Properties[tools.PropCharsListingCount])

	// Accumulated: a=1, b=2, c=1 (3 unique chars total)
	counts := result.CharCounts()
	assert.Equal(t, 1, counts['a'])
	assert.Equal(t, 2, counts['b'])
	assert.Equal(t, 1, counts['c'])
	assert.Len(t, counts, 3)
}

func TestCharsListingSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.CharsListingConfig{
		IncludeSource: true,
		IncludeTarget: false,
	}
	result := tools.NewCharsListingTool(cfg)
	tl := result.Tool()

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	out := processPart(t, tl, part)

	resultBlock := out.Resource.(*model.Block)
	_, hasCount := resultBlock.Properties[tools.PropCharsListingCount]
	assert.False(t, hasCount)

	counts := result.CharCounts()
	assert.Empty(t, counts)
}

func TestCharsListingConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     tools.CharsListingConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "neither source nor target",
			cfg:     tools.CharsListingConfig{},
			wantErr: true,
			errMsg:  "at least one",
		},
		{
			name:    "target without locale",
			cfg:     tools.CharsListingConfig{IncludeTarget: true},
			wantErr: true,
			errMsg:  "TargetLocale",
		},
		{
			name: "source only valid",
			cfg:  tools.CharsListingConfig{IncludeSource: true},
		},
		{
			name: "target with locale valid",
			cfg:  tools.CharsListingConfig{IncludeTarget: true, TargetLocale: model.LocaleFrench},
		},
		{
			name: "both valid",
			cfg:  tools.CharsListingConfig{IncludeSource: true, IncludeTarget: true, TargetLocale: model.LocaleFrench},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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

func TestCharsListingConfigReset(t *testing.T) {
	cfg := &tools.CharsListingConfig{
		IncludeSource: false,
		IncludeTarget: false,
		TargetLocale:  model.LocaleFrench,
	}
	cfg.Reset()
	assert.True(t, cfg.IncludeSource)
	assert.True(t, cfg.IncludeTarget)
	assert.True(t, cfg.TargetLocale.IsEmpty())
}
