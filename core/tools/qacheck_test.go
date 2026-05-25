package tools_test

import (
	"encoding/json"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQACheckTool(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	assert.Equal(t, "qa-check", tl.Name())
	assert.Contains(t, tl.Description(), "quality")
}

func TestQACheckToolPassingBlock(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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
	t.Parallel()
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

func TestQACheckToolNonDeletableSpanMissing(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	// Source has a non-deletable break placeholder.
	sourceRuns := []model.Run{
		{Text: &model.TextRun{Text: "Hello"}},
		{Ph: &model.PlaceholderRun{
			ID: "1", Type: "struct:break", Data: "<br/>",
			Constraints: &model.RunConstraints{Deletable: false},
		}},
		{Text: &model.TextRun{Text: "world"}},
	}
	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       sourceRuns,
		Properties:   make(map[string]string),
		Annotations:  make(map[string]model.Annotation),
	}
	// Target is missing the break placeholder.
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
		if issue.Type == "non-deletable-span-missing" {
			found = true
			assert.Equal(t, tools.QASeverityError, issue.Severity)
			assert.Contains(t, issue.Message, "struct:break")
		}
	}
	assert.True(t, found, "Expected non-deletable-span-missing issue")
}

func TestQACheckToolNonCloneableSpanDuplicated(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	nonCloneable := func() *model.PlaceholderRun {
		return &model.PlaceholderRun{
			ID: "1", Type: "code:variable", Data: "{name}",
			Constraints: &model.RunConstraints{Cloneable: false},
		}
	}

	// Source has one non-cloneable variable placeholder.
	sourceRuns := []model.Run{
		{Text: &model.TextRun{Text: "Hello "}},
		{Ph: nonCloneable()},
		{Text: &model.TextRun{Text: " world"}},
	}
	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       sourceRuns,
		Properties:   make(map[string]string),
		Annotations:  make(map[string]model.Annotation),
	}
	// Target duplicates the variable placeholder.
	targetRuns := []model.Run{
		{Text: &model.TextRun{Text: "Bonjour "}},
		{Ph: nonCloneable()},
		{Text: &model.TextRun{Text: " le "}},
		{Ph: nonCloneable()},
		{Text: &model.TextRun{Text: " monde"}},
	}
	block.SetTargetRuns(model.LocaleFrench, targetRuns)
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "false", resultBlock.Properties[tools.PropQAPassed])

	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropQAIssues]), &issues)
	require.NoError(t, err)
	found := false
	for _, issue := range issues {
		if issue.Type == "non-cloneable-span-duplicated" {
			found = true
			assert.Equal(t, tools.QASeverityError, issue.Severity)
			assert.Contains(t, issue.Message, "code:variable")
		}
	}
	assert.True(t, found, "Expected non-cloneable-span-duplicated issue")
}

func TestQACheckToolDeletableSpanMissingNoConstraintError(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	// Source has a deletable bold pair.
	deletable := &model.RunConstraints{Deletable: true}
	sourceRuns := []model.Run{
		{PcOpen: &model.PcOpenRun{ID: "1", Type: "fmt:bold", Data: "<b>", Constraints: deletable}},
		{Text: &model.TextRun{Text: "Hello"}},
		{PcClose: &model.PcCloseRun{ID: "1", Type: "fmt:bold", Data: "</b>"}},
	}
	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       sourceRuns,
		Properties:   make(map[string]string),
		Annotations:  make(map[string]model.Annotation),
	}
	// Target is missing the bold pair (which are deletable).
	block.SetTargetText(model.LocaleFrench, "Bonjour")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropQAIssues]), &issues)
	require.NoError(t, err)
	for _, issue := range issues {
		assert.NotEqual(t, "non-deletable-span-missing", issue.Type, "Should not flag deletable span as non-deletable")
	}
}

func TestQACheckToolSpanConstraintsDisabled(t *testing.T) {
	t.Parallel()
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	cfg.CheckSpanConstraints = false
	tl := tools.NewQACheckTool(cfg)

	// Source has a non-deletable break placeholder.
	sourceRuns := []model.Run{
		{Text: &model.TextRun{Text: "Hello"}},
		{Ph: &model.PlaceholderRun{
			ID: "1", Type: "struct:break", Data: "<br/>",
			Constraints: &model.RunConstraints{Deletable: false},
		}},
		{Text: &model.TextRun{Text: "world"}},
	}
	block := &model.Block{
		ID:           "tu1",
		Translatable: true,
		Source:       sourceRuns,
		Properties:   make(map[string]string),
		Annotations:  make(map[string]model.Annotation),
	}
	// Target is missing the break placeholder, but check is disabled.
	block.SetTargetText(model.LocaleFrench, "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	var issues []tools.QAIssue
	err := json.Unmarshal([]byte(resultBlock.Properties[tools.PropQAIssues]), &issues)
	require.NoError(t, err)
	for _, issue := range issues {
		assert.NotEqual(t, "non-deletable-span-missing", issue.Type, "Should not check span constraints when disabled")
	}
}

func TestQACheckToolEmptyTargetText(t *testing.T) {
	t.Parallel()
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
