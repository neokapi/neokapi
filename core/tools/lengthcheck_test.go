package tools_test

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLengthCheckToolMaxChars(t *testing.T) {
	cfg := &tools.LengthCheckConfig{
		TargetLocale: model.LocaleFrench,
		MaxChars:     10,
	}
	tl := tools.NewLengthCheckTool(cfg)

	assert.Equal(t, "length-check", tl.Name())
	assert.Contains(t, tl.Description(), "length")

	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde entier") // 23 chars
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropLengthCheckPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropLengthCheckIssues]), &issues)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "max-chars-exceeded", issues[0].Type)
	assert.Equal(t, tools.QASeverityError, issues[0].Severity)
	assert.Contains(t, issues[0].Message, "23")
	assert.Contains(t, issues[0].Message, "10")
}

func TestLengthCheckToolMaxWords(t *testing.T) {
	cfg := &tools.LengthCheckConfig{
		TargetLocale: model.LocaleFrench,
		MaxWords:     2,
	}
	tl := tools.NewLengthCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde") // 3 words
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropLengthCheckPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropLengthCheckIssues]), &issues)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "max-words-exceeded", issues[0].Type)
	assert.Equal(t, tools.QASeverityError, issues[0].Severity)
}

func TestLengthCheckToolMaxPercentage(t *testing.T) {
	cfg := &tools.LengthCheckConfig{
		TargetLocale:  model.LocaleFrench,
		MaxPercentage: 150.0,
	}
	tl := tools.NewLengthCheckTool(cfg)

	block := model.NewBlock("tu1", "Hi")                         // 2 chars
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde!") // 17 chars = 850%
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropLengthCheckPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropLengthCheckIssues]), &issues)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "max-percentage-exceeded", issues[0].Type)
	assert.Equal(t, tools.QASeverityWarning, issues[0].Severity)
}

func TestLengthCheckToolMinPercentage(t *testing.T) {
	cfg := &tools.LengthCheckConfig{
		TargetLocale:  model.LocaleFrench,
		MinPercentage: 50.0,
	}
	tl := tools.NewLengthCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world how are you") // 23 chars
	block.SetTargetText(model.LocaleFrench, "Bon")            // 3 chars = ~13%
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropLengthCheckPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropLengthCheckIssues]), &issues)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "min-percentage-exceeded", issues[0].Type)
	assert.Equal(t, tools.QASeverityWarning, issues[0].Severity)
}

func TestLengthCheckToolPass(t *testing.T) {
	cfg := &tools.LengthCheckConfig{
		TargetLocale:  model.LocaleFrench,
		MaxChars:      50,
		MaxWords:      10,
		MaxPercentage: 200.0,
		MinPercentage: 50.0,
	}
	tl := tools.NewLengthCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "true", resultBlock.Properties[tools.PropLengthCheckPassed])
	assert.Equal(t, "[]", resultBlock.Properties[tools.PropLengthCheckIssues])
}

func TestLengthCheckToolMultipleViolations(t *testing.T) {
	cfg := &tools.LengthCheckConfig{
		TargetLocale:  model.LocaleFrench,
		MaxChars:      5,
		MaxWords:      1,
		MaxPercentage: 100.0,
	}
	tl := tools.NewLengthCheckTool(cfg)

	block := model.NewBlock("tu1", "Hi")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde") // 16 chars, 3 words, 800%
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropLengthCheckPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropLengthCheckIssues]), &issues)
	require.NoError(t, err)
	assert.Len(t, issues, 3, "Expected 3 issues: max-chars, max-words, max-percentage")
}

func TestLengthCheckToolSkipsNonTranslatable(t *testing.T) {
	cfg := &tools.LengthCheckConfig{
		TargetLocale: model.LocaleFrench,
		MaxChars:     5,
	}
	tl := tools.NewLengthCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	block.SetTargetText(model.LocaleFrench, "Very long translation text")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasPassed := resultBlock.Properties[tools.PropLengthCheckPassed]
	assert.False(t, hasPassed)
}

func TestLengthCheckToolNoTarget(t *testing.T) {
	cfg := &tools.LengthCheckConfig{
		TargetLocale: model.LocaleFrench,
		MaxChars:     5,
	}
	tl := tools.NewLengthCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	// No target set.
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasPassed := resultBlock.Properties[tools.PropLengthCheckPassed]
	assert.False(t, hasPassed)
}

func TestLengthCheckConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     tools.LengthCheckConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing target locale",
			cfg:     tools.LengthCheckConfig{},
			wantErr: true,
			errMsg:  "TargetLocale",
		},
		{
			name:    "negative max chars",
			cfg:     tools.LengthCheckConfig{TargetLocale: model.LocaleFrench, MaxChars: -1},
			wantErr: true,
			errMsg:  "MaxChars",
		},
		{
			name:    "negative max words",
			cfg:     tools.LengthCheckConfig{TargetLocale: model.LocaleFrench, MaxWords: -1},
			wantErr: true,
			errMsg:  "MaxWords",
		},
		{
			name:    "negative max percentage",
			cfg:     tools.LengthCheckConfig{TargetLocale: model.LocaleFrench, MaxPercentage: -1},
			wantErr: true,
			errMsg:  "MaxPercentage",
		},
		{
			name:    "negative min percentage",
			cfg:     tools.LengthCheckConfig{TargetLocale: model.LocaleFrench, MinPercentage: -1},
			wantErr: true,
			errMsg:  "MinPercentage",
		},
		{
			name: "valid config",
			cfg:  tools.LengthCheckConfig{TargetLocale: model.LocaleFrench, MaxChars: 100},
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
