package tools_test

import (
	"encoding/json"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/lib/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Segmentation Tool Tests ---

func TestSegmentationTool(t *testing.T) {
	cfg := &tools.SegmentationConfig{}
	tl := tools.NewSegmentationTool(cfg)

	assert.Equal(t, "segmentation", tl.Name())
	assert.Contains(t, tl.Description(), "segment")
}

func TestSegmentationToolSingleSentence(t *testing.T) {
	cfg := &tools.SegmentationConfig{}
	tl := tools.NewSegmentationTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "1", resultBlock.Properties[tools.PropSegmentCount])
	assert.Len(t, resultBlock.Source, 1)
}

func TestSegmentationToolMultipleSentences(t *testing.T) {
	cfg := &tools.SegmentationConfig{}
	tl := tools.NewSegmentationTool(cfg)

	block := model.NewBlock("tu1", "Hello world. This is a test. And another sentence.")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	count := resultBlock.Properties[tools.PropSegmentCount]
	// Should split into multiple segments.
	assert.NotEqual(t, "1", count)
	assert.True(t, len(resultBlock.Source) > 1, "Expected multiple source segments")
}

func TestSegmentationToolExclamationQuestion(t *testing.T) {
	cfg := &tools.SegmentationConfig{}
	tl := tools.NewSegmentationTool(cfg)

	block := model.NewBlock("tu1", "Stop! What are you doing? Please help.")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	count := resultBlock.Properties[tools.PropSegmentCount]
	assert.NotEqual(t, "1", count)
	assert.True(t, len(resultBlock.Source) > 1, "Expected multiple source segments")
}

func TestSegmentationToolEmptyText(t *testing.T) {
	cfg := &tools.SegmentationConfig{}
	tl := tools.NewSegmentationTool(cfg)

	block := model.NewBlock("tu1", "")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "0", resultBlock.Properties[tools.PropSegmentCount])
}

func TestSegmentationToolSkipsNonTranslatable(t *testing.T) {
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

// --- TM Leverage Tool Tests ---

// mockTMProvider implements TMProvider for testing.
type mockTMProvider struct {
	exact map[string]string     // source -> translation
	fuzzy map[string]fuzzyMatch // source -> {translation, score}
}

type fuzzyMatch struct {
	translation string
	score       int
}

func (m *mockTMProvider) LookupExact(source string, _, _ model.LocaleID) (string, bool) {
	if m.exact == nil {
		return "", false
	}
	trans, ok := m.exact[source]
	return trans, ok
}

func (m *mockTMProvider) LookupFuzzy(source string, _, _ model.LocaleID, threshold int) (string, int, bool) {
	if m.fuzzy == nil {
		return "", 0, false
	}
	match, ok := m.fuzzy[source]
	if !ok || match.score < threshold {
		return "", 0, false
	}
	return match.translation, match.score, true
}

func TestTMLeverageTool(t *testing.T) {
	cfg := &tools.TMLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Provider:       &tools.NullTMProvider{},
	}
	tl := tools.NewTMLeverageTool(cfg)

	assert.Equal(t, "tm-leverage", tl.Name())
	assert.Contains(t, tl.Description(), "translation memory")
}

func TestTMLeverageToolExactMatch(t *testing.T) {
	provider := &mockTMProvider{
		exact: map[string]string{
			"Hello world": "Bonjour le monde",
		},
	}
	cfg := &tools.TMLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Provider:       provider,
	}
	tl := tools.NewTMLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
	assert.Equal(t, "100", resultBlock.Properties[tools.PropTMMatchScore])
	assert.Equal(t, "exact", resultBlock.Properties[tools.PropTMMatchType])
}

func TestTMLeverageToolFuzzyMatch(t *testing.T) {
	provider := &mockTMProvider{
		fuzzy: map[string]fuzzyMatch{
			"Hello world": {translation: "Bonjour monde", score: 85},
		},
	}
	cfg := &tools.TMLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Provider:       provider,
	}
	tl := tools.NewTMLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Equal(t, "Bonjour monde", resultBlock.TargetText(model.LocaleFrench))
	assert.Equal(t, "85", resultBlock.Properties[tools.PropTMMatchScore])
	assert.Equal(t, "fuzzy", resultBlock.Properties[tools.PropTMMatchType])
}

func TestTMLeverageToolNoMatch(t *testing.T) {
	provider := &mockTMProvider{}
	cfg := &tools.TMLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Provider:       provider,
	}
	tl := tools.NewTMLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
	_, hasScore := resultBlock.Properties[tools.PropTMMatchScore]
	assert.False(t, hasScore)
}

func TestTMLeverageToolExactOverFuzzy(t *testing.T) {
	provider := &mockTMProvider{
		exact: map[string]string{
			"Hello world": "Bonjour le monde",
		},
		fuzzy: map[string]fuzzyMatch{
			"Hello world": {translation: "Bonjour monde", score: 85},
		},
	}
	cfg := &tools.TMLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Provider:       provider,
	}
	tl := tools.NewTMLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	// Exact match should win.
	assert.Equal(t, "Bonjour le monde", resultBlock.TargetText(model.LocaleFrench))
	assert.Equal(t, "100", resultBlock.Properties[tools.PropTMMatchScore])
	assert.Equal(t, "exact", resultBlock.Properties[tools.PropTMMatchType])
}

func TestTMLeverageToolNullProvider(t *testing.T) {
	cfg := &tools.TMLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Provider:       &tools.NullTMProvider{},
	}
	tl := tools.NewTMLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestTMLeverageToolSkipsNonTranslatable(t *testing.T) {
	provider := &mockTMProvider{
		exact: map[string]string{
			"Hello world": "Bonjour le monde",
		},
	}
	cfg := &tools.TMLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Provider:       provider,
	}
	tl := tools.NewTMLeverageTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestTMLeverageToolEmptySource(t *testing.T) {
	provider := &mockTMProvider{
		exact: map[string]string{
			"": "something",
		},
	}
	cfg := &tools.TMLeverageConfig{
		TargetLocale:   model.LocaleFrench,
		SourceLocale:   model.LocaleEnglish,
		FuzzyThreshold: 70,
		Provider:       provider,
	}
	tl := tools.NewTMLeverageTool(cfg)

	block := model.NewBlock("tu1", "")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.False(t, resultBlock.HasTarget(model.LocaleFrench))
}

func TestTMLeverageConfigValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     tools.TMLeverageConfig
		wantErr bool
		errMsg  string
	}{
		{
			name:    "missing target locale",
			cfg:     tools.TMLeverageConfig{Provider: &tools.NullTMProvider{}},
			wantErr: true,
			errMsg:  "TargetLocale",
		},
		{
			name:    "missing provider",
			cfg:     tools.TMLeverageConfig{TargetLocale: model.LocaleFrench},
			wantErr: true,
			errMsg:  "Provider",
		},
		{
			name:    "threshold out of range",
			cfg:     tools.TMLeverageConfig{TargetLocale: model.LocaleFrench, Provider: &tools.NullTMProvider{}, FuzzyThreshold: 101},
			wantErr: true,
			errMsg:  "FuzzyThreshold",
		},
		{
			name: "valid config",
			cfg: tools.TMLeverageConfig{
				TargetLocale:   model.LocaleFrench,
				Provider:       &tools.NullTMProvider{},
				FuzzyThreshold: 80,
			},
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

// --- QA Check Tool Tests ---

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
