package tools_test

import (
	"encoding/json"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQACheckTool(t *testing.T) {
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	assert.Equal(t, "qa-check", tl.Name())
	assert.Contains(t, tl.Description(), "quality")
}

func TestQACheckToolPassingBlock(t *testing.T) {
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "true", resultBlock.Properties[tools.PropQAPassed])
	assert.Equal(t, "[]", resultBlock.Properties[tools.PropQAIssues])
}

func TestQACheckToolEmptyTarget(t *testing.T) {
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	// No target set.
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropQAPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropQAIssues]), &issues)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "empty-target", issues[0].Type)
	assert.Equal(t, tools.QASeverityError, issues[0].Severity)
}

func TestQACheckToolLeadingWhitespace(t *testing.T) {
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "  Bonjour le monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropQAPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropQAIssues]), &issues)
	require.NoError(t, err)
	found := false
	for _, issue := range issues {
		if issue.Type == "leading-whitespace" {
			found = true
			assert.Equal(t, tools.QASeverityWarning, issue.Severity)
		}
	}
	assert.True(t, found, "Expected leading-whitespace issue")
}

func TestQACheckToolTrailingWhitespace(t *testing.T) {
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde  ")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropQAPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropQAIssues]), &issues)
	require.NoError(t, err)
	found := false
	for _, issue := range issues {
		if issue.Type == "trailing-whitespace" {
			found = true
			assert.Equal(t, tools.QASeverityWarning, issue.Severity)
		}
	}
	assert.True(t, found, "Expected trailing-whitespace issue")
}

func TestQACheckToolDoubleSpaces(t *testing.T) {
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour  le  monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropQAPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropQAIssues]), &issues)
	require.NoError(t, err)
	found := false
	for _, issue := range issues {
		if issue.Type == "double-spaces" {
			found = true
			assert.Equal(t, tools.QASeverityWarning, issue.Severity)
		}
	}
	assert.True(t, found, "Expected double-spaces issue")
}

func TestQACheckToolTargetSameAsSource(t *testing.T) {
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropQAPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropQAIssues]), &issues)
	require.NoError(t, err)
	found := false
	for _, issue := range issues {
		if issue.Type == "target-same-as-source" {
			found = true
			assert.Equal(t, tools.QASeverityWarning, issue.Severity)
		}
	}
	assert.True(t, found, "Expected target-same-as-source issue")
}

func TestQACheckToolMultipleIssues(t *testing.T) {
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "  Hello  world ")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropQAPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropQAIssues]), &issues)
	require.NoError(t, err)
	// Should have at least leading whitespace, double spaces, and trailing whitespace issues.
	assert.True(t, len(issues) >= 2, "Expected multiple issues, got %d", len(issues))
}

func TestQACheckToolSkipsNonTranslatable(t *testing.T) {
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasPassed := resultBlock.Properties[tools.PropQAPassed]
	assert.False(t, hasPassed)
}

func TestQACheckToolDisabledChecks(t *testing.T) {
	cfg := &tools.QACheckConfig{
		TargetLocale:            model.LocaleFrench,
		CheckLeadingWhitespace:  false,
		CheckTrailingWhitespace: false,
		CheckDoubleSpaces:       false,
		CheckEmptyTarget:        false,
		CheckTargetSameAsSource: false,
	}
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "  Hello  world ")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "true", resultBlock.Properties[tools.PropQAPassed])
	assert.Equal(t, "[]", resultBlock.Properties[tools.PropQAIssues])
}

func TestQACheckConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     tools.QACheckConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing target locale",
			cfg:     tools.QACheckConfig{},
			wantErr: true,
			errMsg:  "TargetLocale",
		},
		{
			name: "valid config",
			cfg:  tools.QACheckConfig{TargetLocale: model.LocaleFrench},
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

func TestQACheckToolEmptyTargetText(t *testing.T) {
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropQAPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropQAIssues]), &issues)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "empty-target", issues[0].Type)
}
