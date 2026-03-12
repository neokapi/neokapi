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

func TestQACheckToolNonDeletableSpanMissing(t *testing.T) {
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	// Source has a non-deletable break span.
	sourceFrag := model.NewFragment("Hello\uE003world")
	sourceFrag.Spans = []*model.Span{
		{SpanType: model.SpanPlaceholder, Type: "struct:break", ID: "1", Data: "<br/>", Deletable: false},
	}
	block := &model.Block{
		ID:          "tu1",
		Translatable: true,
		Source:      []*model.Segment{{ID: "s1", Content: sourceFrag}},
		Targets:     make(map[model.LocaleID][]*model.Segment),
		Properties:  make(map[string]string),
		Annotations: make(map[string]model.Annotation),
	}
	// Target is missing the break span.
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
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	// Source has one non-cloneable variable span.
	sourceFrag := model.NewFragment("Hello \uE003 world")
	sourceFrag.Spans = []*model.Span{
		{SpanType: model.SpanPlaceholder, Type: "code:variable", ID: "1", Data: "{name}", Cloneable: false},
	}
	block := &model.Block{
		ID:          "tu1",
		Translatable: true,
		Source:      []*model.Segment{{ID: "s1", Content: sourceFrag}},
		Targets:     make(map[model.LocaleID][]*model.Segment),
		Properties:  make(map[string]string),
		Annotations: make(map[string]model.Annotation),
	}
	// Target duplicates the variable span.
	targetFrag := model.NewFragment("Bonjour \uE003 le \uE003 monde")
	targetFrag.Spans = []*model.Span{
		{SpanType: model.SpanPlaceholder, Type: "code:variable", ID: "1", Data: "{name}", Cloneable: false},
		{SpanType: model.SpanPlaceholder, Type: "code:variable", ID: "1", Data: "{name}", Cloneable: false},
	}
	block.SetTargetFragment(model.LocaleFrench, targetFrag)
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
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	tl := tools.NewQACheckTool(cfg)

	// Source has a deletable bold span.
	sourceFrag := model.NewFragment("\uE001Hello\uE002")
	sourceFrag.Spans = []*model.Span{
		{SpanType: model.SpanOpening, Type: "fmt:bold", ID: "1", Data: "<b>", Deletable: true},
		{SpanType: model.SpanClosing, Type: "fmt:bold", ID: "1", Data: "</b>", Deletable: true},
	}
	block := &model.Block{
		ID:          "tu1",
		Translatable: true,
		Source:      []*model.Segment{{ID: "s1", Content: sourceFrag}},
		Targets:     make(map[model.LocaleID][]*model.Segment),
		Properties:  make(map[string]string),
		Annotations: make(map[string]model.Annotation),
	}
	// Target is missing the bold spans (which are deletable).
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
	cfg := tools.NewQACheckConfig(model.LocaleFrench)
	cfg.CheckSpanConstraints = false
	tl := tools.NewQACheckTool(cfg)

	// Source has a non-deletable break span.
	sourceFrag := model.NewFragment("Hello\uE003world")
	sourceFrag.Spans = []*model.Span{
		{SpanType: model.SpanPlaceholder, Type: "struct:break", ID: "1", Data: "<br/>", Deletable: false},
	}
	block := &model.Block{
		ID:          "tu1",
		Translatable: true,
		Source:      []*model.Segment{{ID: "s1", Content: sourceFrag}},
		Targets:     make(map[model.LocaleID][]*model.Segment),
		Properties:  make(map[string]string),
		Annotations: make(map[string]model.Annotation),
	}
	// Target is missing the break span, but check is disabled.
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
