package tools_test

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCharsCheckToolForbiddenChars(t *testing.T) {
	t.Parallel()
	cfg := &tools.CharsCheckConfig{
		TargetLocale:   model.LocaleFrench,
		ForbiddenChars: "{}[]",
		CheckCorrupted: false,
	}
	tl := tools.NewCharsCheckTool(cfg)

	assert.Equal(t, "chars-check", tl.Name())
	assert.Contains(t, tl.Description(), "character")

	block := model.NewBlock("tu1", "Hello world")
	block.SetTargetText(model.LocaleFrench, "Bonjour {le} monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropCharsCheckPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropCharsCheckIssues]), &issues)
	require.NoError(t, err)
	// Should find both { and }
	assert.Len(t, issues, 2)
	for _, issue := range issues {
		assert.Equal(t, "forbidden-char", issue.Type)
		assert.Equal(t, tools.QASeverityError, issue.Severity)
	}
}

func TestCharsCheckToolRequiredCharsMissing(t *testing.T) {
	t.Parallel()
	cfg := &tools.CharsCheckConfig{
		TargetLocale:   model.LocaleFrench,
		RequiredChars:  ".!",
		CheckCorrupted: false,
	}
	tl := tools.NewCharsCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world!")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde") // Missing !
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropCharsCheckPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropCharsCheckIssues]), &issues)
	require.NoError(t, err)
	require.Len(t, issues, 1)
	assert.Equal(t, "required-char-missing", issues[0].Type)
	assert.Equal(t, tools.QASeverityWarning, issues[0].Severity)
	assert.Contains(t, issues[0].Message, "!")
}

func TestCharsCheckToolMojibakeDetection(t *testing.T) {
	t.Parallel()
	cfg := tools.NewCharsCheckConfig(model.LocaleFrench)
	tl := tools.NewCharsCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	// Ã¤ is a common mojibake pattern (ä decoded as Latin-1)
	block.SetTargetText(model.LocaleFrench, "Bonjour l\u00c3\u00a4 monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropCharsCheckPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropCharsCheckIssues]), &issues)
	require.NoError(t, err)
	found := false
	for _, issue := range issues {
		if issue.Type == "mojibake" {
			found = true
			assert.Equal(t, tools.QASeverityError, issue.Severity)
			assert.Contains(t, issue.Message, "mojibake")
		}
	}
	assert.True(t, found, "Expected mojibake issue")
}

func TestCharsCheckToolReplacementChar(t *testing.T) {
	t.Parallel()
	cfg := tools.NewCharsCheckConfig(model.LocaleFrench)
	tl := tools.NewCharsCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText(model.LocaleFrench, "Bonjour \uFFFD monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropCharsCheckPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropCharsCheckIssues]), &issues)
	require.NoError(t, err)
	found := false
	for _, issue := range issues {
		if issue.Type == "replacement-char" {
			found = true
			assert.Equal(t, tools.QASeverityError, issue.Severity)
			assert.Contains(t, issue.Message, "U+FFFD")
		}
	}
	assert.True(t, found, "Expected replacement-char issue")
}

func TestCharsCheckToolControlChars(t *testing.T) {
	t.Parallel()
	cfg := tools.NewCharsCheckConfig(model.LocaleFrench)
	tl := tools.NewCharsCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.SetTargetText(model.LocaleFrench, "Bonjour\x01monde") // SOH control char
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropCharsCheckPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropCharsCheckIssues]), &issues)
	require.NoError(t, err)
	found := false
	for _, issue := range issues {
		if issue.Type == "control-char" {
			found = true
			assert.Equal(t, tools.QASeverityError, issue.Severity)
			assert.Contains(t, issue.Message, "U+0001")
		}
	}
	assert.True(t, found, "Expected control-char issue")
}

func TestCharsCheckToolAllowedControlChars(t *testing.T) {
	t.Parallel()
	cfg := tools.NewCharsCheckConfig(model.LocaleFrench)
	tl := tools.NewCharsCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	// Tab, newline, and carriage return should be allowed.
	block.SetTargetText(model.LocaleFrench, "Bonjour\tle\nmonde\r")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "true", resultBlock.Properties[tools.PropCharsCheckPassed])
	assert.Equal(t, "[]", resultBlock.Properties[tools.PropCharsCheckIssues])
}

func TestCharsCheckToolCleanTextPasses(t *testing.T) {
	t.Parallel()
	cfg := &tools.CharsCheckConfig{
		TargetLocale:   model.LocaleFrench,
		ForbiddenChars: "{}[]",
		RequiredChars:  ".",
		CheckCorrupted: true,
	}
	tl := tools.NewCharsCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world.")
	block.SetTargetText(model.LocaleFrench, "Bonjour le monde.")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "true", resultBlock.Properties[tools.PropCharsCheckPassed])
	assert.Equal(t, "[]", resultBlock.Properties[tools.PropCharsCheckIssues])
}

func TestCharsCheckToolSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	cfg := tools.NewCharsCheckConfig(model.LocaleFrench)
	tl := tools.NewCharsCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	block.Translatable = false
	block.SetTargetText(model.LocaleFrench, "Bonjour\x01monde")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasPassed := resultBlock.Properties[tools.PropCharsCheckPassed]
	assert.False(t, hasPassed)
}

func TestCharsCheckToolNoTarget(t *testing.T) {
	t.Parallel()
	cfg := tools.NewCharsCheckConfig(model.LocaleFrench)
	tl := tools.NewCharsCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	_, hasPassed := resultBlock.Properties[tools.PropCharsCheckPassed]
	assert.False(t, hasPassed)
}

func TestCharsCheckConfigValidation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		cfg     tools.CharsCheckConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing target locale",
			cfg:     tools.CharsCheckConfig{},
			wantErr: true,
			errMsg:  "TargetLocale",
		},
		{
			name: "valid config",
			cfg:  tools.CharsCheckConfig{TargetLocale: model.LocaleFrench, CheckCorrupted: true},
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
