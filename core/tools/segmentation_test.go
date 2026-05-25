package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSegmentationTool(t *testing.T) {
	t.Parallel()
	cfg := &tools.SegmentationConfig{}
	tl := tools.NewSegmentationTool(cfg)

	assert.Equal(t, "segmentation", tl.Name())
	assert.Contains(t, tl.Description(), "segment")
}

func TestSegmentationToolSingleSentence(t *testing.T) {
	t.Parallel()
	cfg := &tools.SegmentationConfig{}
	tl := tools.NewSegmentationTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "1", resultBlock.Properties[tools.PropSegmentCount])
	assert.Equal(t, 1, resultBlock.SourceSegmentCount())
}

func TestSegmentationToolMultipleSentences(t *testing.T) {
	t.Parallel()
	cfg := &tools.SegmentationConfig{}
	tl := tools.NewSegmentationTool(cfg)

	block := model.NewBlock("tu1", "Hello world. This is a test. And another sentence.")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	count := resultBlock.Properties[tools.PropSegmentCount]
	// Should split into multiple segments.
	assert.NotEqual(t, "1", count)
	assert.True(t, resultBlock.SourceSegmentCount() > 1, "Expected multiple source segments")
}

func TestSegmentationToolExclamationQuestion(t *testing.T) {
	t.Parallel()
	cfg := &tools.SegmentationConfig{}
	tl := tools.NewSegmentationTool(cfg)

	block := model.NewBlock("tu1", "Stop! What are you doing? Please help.")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	count := resultBlock.Properties[tools.PropSegmentCount]
	assert.NotEqual(t, "1", count)
	assert.True(t, resultBlock.SourceSegmentCount() > 1, "Expected multiple source segments")
}

func TestSegmentationToolEmptyText(t *testing.T) {
	t.Parallel()
	cfg := &tools.SegmentationConfig{}
	tl := tools.NewSegmentationTool(cfg)

	block := model.NewBlock("tu1", "")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "0", resultBlock.Properties[tools.PropSegmentCount])
}

func TestSegmentationToolSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	cfg := &tools.SegmentationConfig{}
	tl := tools.NewSegmentationTool(cfg)

	block := model.NewBlock("tu1", "First sentence. Second sentence.")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasCount := resultBlock.Properties[tools.PropSegmentCount]
	assert.False(t, hasCount)
}

func TestSegmentationToolCustomRules(t *testing.T) {
	t.Parallel()
	cfg := &tools.SegmentationConfig{
		Rules: []tools.SegmentationRule{
			{BeforeBreak: `;`, AfterBreak: `\s`, IsBreak: true},
		},
	}
	tl := tools.NewSegmentationTool(cfg)

	block := model.NewBlock("tu1", "part one; part two; part three")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "3", resultBlock.Properties[tools.PropSegmentCount])
}

func TestSegmentationConfigValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     tools.SegmentationConfig
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid empty rules",
			cfg:  tools.SegmentationConfig{},
		},
		{
			name: "valid custom rules",
			cfg: tools.SegmentationConfig{
				Rules: []tools.SegmentationRule{
					{BeforeBreak: `\.`, AfterBreak: `\s`, IsBreak: true},
				},
			},
		},
		{
			name: "invalid before regex",
			cfg: tools.SegmentationConfig{
				Rules: []tools.SegmentationRule{
					{BeforeBreak: `[invalid`, IsBreak: true},
				},
			},
			wantErr: true,
			errMsg:  "invalid regex",
		},
		{
			name: "invalid after regex",
			cfg: tools.SegmentationConfig{
				Rules: []tools.SegmentationRule{
					{AfterBreak: `[invalid`, IsBreak: true},
				},
			},
			wantErr: true,
			errMsg:  "invalid regex",
		},
		{
			name: "empty patterns",
			cfg: tools.SegmentationConfig{
				Rules: []tools.SegmentationRule{
					{IsBreak: true},
				},
			},
			wantErr: true,
			errMsg:  "no patterns",
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
