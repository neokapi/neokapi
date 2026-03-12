package tools_test

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatternCheckTool(t *testing.T) {
	cfg := &tools.PatternCheckConfig{
		TargetLocale: model.LocaleFrench,
		Patterns: []tools.PatternRule{
			{Name: "printf-placeholder", Pattern: `%[sdfu]`, MustMatch: true},
		},
	}
	tl := tools.NewPatternCheckTool(cfg)

	assert.Equal(t, "pattern-check", tl.Name())
	assert.Contains(t, tl.Description(), "pattern")
}

func TestPatternCheckPlaceholderPreserved(t *testing.T) {
	cfg := &tools.PatternCheckConfig{
		TargetLocale: model.LocaleFrench,
		Patterns: []tools.PatternRule{
			{Name: "printf-placeholder", Pattern: `%[sdfu]`, MustMatch: true},
		},
	}
	tl := tools.NewPatternCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello %s world")
	block.SetTargetText(model.LocaleFrench, "Bonjour %s le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "true", resultBlock.Properties[tools.PropPatternCheckPassed])
	assert.Equal(t, "[]", resultBlock.Properties[tools.PropPatternCheckIssues])
}

func TestPatternCheckPlaceholderMissing(t *testing.T) {
	cfg := &tools.PatternCheckConfig{
		TargetLocale: model.LocaleFrench,
		Patterns: []tools.PatternRule{
			{Name: "printf-placeholder", Pattern: `%[sdfu]`, MustMatch: true},
		},
	}
	tl := tools.NewPatternCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello %s world %d items")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropPatternCheckPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropPatternCheckIssues]), &issues)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "pattern-mismatch", issues[0].Type)
	assert.Equal(t, tools.QASeverityError, issues[0].Severity)
	assert.Contains(t, issues[0].Message, "printf-placeholder")
	assert.Contains(t, issues[0].Message, "source has 2")
	assert.Contains(t, issues[0].Message, "target has 0")
}

func TestPatternCheckForbiddenPattern(t *testing.T) {
	cfg := &tools.PatternCheckConfig{
		TargetLocale: model.LocaleFrench,
		Patterns: []tools.PatternRule{
			{Name: "html-entity", Pattern: `&\w+;`, MustNotMatch: true},
		},
	}
	tl := tools.NewPatternCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour &amp; le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropPatternCheckPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropPatternCheckIssues]), &issues)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "forbidden-pattern", issues[0].Type)
	assert.Equal(t, tools.QASeverityError, issues[0].Severity)
	assert.Contains(t, issues[0].Message, "html-entity")
}

func TestPatternCheckNoPatternsPass(t *testing.T) {
	cfg := &tools.PatternCheckConfig{
		TargetLocale: model.LocaleFrench,
		Patterns:     nil,
	}
	tl := tools.NewPatternCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "true", resultBlock.Properties[tools.PropPatternCheckPassed])
	assert.Equal(t, "[]", resultBlock.Properties[tools.PropPatternCheckIssues])
}

func TestPatternCheckNoTarget(t *testing.T) {
	cfg := &tools.PatternCheckConfig{
		TargetLocale: model.LocaleFrench,
		Patterns: []tools.PatternRule{
			{Name: "printf-placeholder", Pattern: `%[sdfu]`, MustMatch: true},
		},
	}
	tl := tools.NewPatternCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello %s world")
	// No target set.
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "true", resultBlock.Properties[tools.PropPatternCheckPassed])
}

func TestPatternCheckSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.PatternCheckConfig{
		TargetLocale: model.LocaleFrench,
		Patterns: []tools.PatternRule{
			{Name: "printf-placeholder", Pattern: `%[sdfu]`, MustMatch: true},
		},
	}
	tl := tools.NewPatternCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello %s world")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasPassed := resultBlock.Properties[tools.PropPatternCheckPassed]
	assert.False(t, hasPassed)
}

func TestPatternCheckConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     tools.PatternCheckConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing target locale",
			cfg:     tools.PatternCheckConfig{},
			wantErr: true,
			errMsg:  "TargetLocale",
		},
		{
			name: "empty pattern string",
			cfg: tools.PatternCheckConfig{
				TargetLocale: model.LocaleFrench,
				Patterns:     []tools.PatternRule{{Name: "test", Pattern: ""}},
			},
			wantErr: true,
			errMsg:  "empty",
		},
		{
			name: "invalid regex",
			cfg: tools.PatternCheckConfig{
				TargetLocale: model.LocaleFrench,
				Patterns:     []tools.PatternRule{{Name: "bad", Pattern: "[invalid"}},
			},
			wantErr: true,
			errMsg:  "invalid",
		},
		{
			name: "both MustMatch and MustNotMatch",
			cfg: tools.PatternCheckConfig{
				TargetLocale: model.LocaleFrench,
				Patterns:     []tools.PatternRule{{Name: "conflict", Pattern: `\w+`, MustMatch: true, MustNotMatch: true}},
			},
			wantErr: true,
			errMsg:  "both MustMatch and MustNotMatch",
		},
		{
			name: "valid config",
			cfg: tools.PatternCheckConfig{
				TargetLocale: model.LocaleFrench,
				Patterns:     []tools.PatternRule{{Name: "test", Pattern: `%s`, MustMatch: true}},
			},
		},
		{
			name: "valid config no patterns",
			cfg:  tools.PatternCheckConfig{TargetLocale: model.LocaleFrench},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if tt.wantErr {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestPatternCheckForbiddenPatternNotPresent(t *testing.T) {
	cfg := &tools.PatternCheckConfig{
		TargetLocale: model.LocaleFrench,
		Patterns: []tools.PatternRule{
			{Name: "html-entity", Pattern: `&\w+;`, MustNotMatch: true},
		},
	}
	tl := tools.NewPatternCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "true", resultBlock.Properties[tools.PropPatternCheckPassed])
}
