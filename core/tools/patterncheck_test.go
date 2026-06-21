package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatternCheckTool(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
	assert.Empty(t, qaFindings(resultBlock))
}

func TestPatternCheckPlaceholderMissing(t *testing.T) {
	t.Parallel()
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
	findings := qaFindings(resultBlock)
	require.Len(t, findings, 1)
	assert.Equal(t, "pattern-mismatch", findings[0].Category)
	assert.Equal(t, check.SeverityMajor, findings[0].Severity)
	assert.Contains(t, findings[0].Message, "printf-placeholder")
	assert.Contains(t, findings[0].Message, "source has 2")
	assert.Contains(t, findings[0].Message, "target has 0")
}

func TestPatternCheckForbiddenPattern(t *testing.T) {
	t.Parallel()
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
	findings := qaFindings(resultBlock)
	require.Len(t, findings, 1)
	assert.Equal(t, "forbidden-pattern", findings[0].Category)
	assert.Equal(t, check.SeverityMajor, findings[0].Severity)
	assert.Contains(t, findings[0].Message, "html-entity")
	assert.Equal(t, "&amp;", findings[0].OriginalText)
}

func TestPatternCheckNoPatternsPass(t *testing.T) {
	t.Parallel()
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
	assert.Empty(t, qaFindings(resultBlock))
}

func TestPatternCheckNoTarget(t *testing.T) {
	t.Parallel()
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
	assert.Empty(t, qaFindings(resultBlock))
}

func TestPatternCheckSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
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
	assert.Empty(t, qaFindings(resultBlock))
}

// TestPatternCheckCheckSourceForbidden verifies the source scope flags a
// forbidden (MustNotMatch) pattern present in the source, with no target.
func TestPatternCheckCheckSourceForbidden(t *testing.T) {
	t.Parallel()
	cfg := &tools.PatternCheckConfig{
		CheckSource: true,
		Patterns: []tools.PatternRule{
			{Name: "todo-marker", Pattern: `(?i)todo`, MustNotMatch: true},
		},
	}
	tl := tools.NewPatternCheckTool(cfg)

	block := model.NewBlock("tu1", "Draft copy — TODO: rewrite this") // no target
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	findings := qaFindings(result.Resource.(*model.Block))
	require.Len(t, findings, 1)
	assert.Equal(t, "forbidden-pattern", findings[0].Category)
	assert.Equal(t, check.SeverityMajor, findings[0].Severity)
	assert.Contains(t, findings[0].Message, "todo-marker")
	assert.Contains(t, findings[0].Message, "found in source")
	assert.Equal(t, "TODO", findings[0].OriginalText)
}

// TestPatternCheckCheckSourceRequiredMissing verifies the source scope flags a
// required (MustMatch) pattern that is absent from the source.
func TestPatternCheckCheckSourceRequiredMissing(t *testing.T) {
	t.Parallel()
	cfg := &tools.PatternCheckConfig{
		CheckSource: true,
		Patterns: []tools.PatternRule{
			{Name: "copyright", Pattern: `(?i)copyright`, MustMatch: true},
		},
	}
	tl := tools.NewPatternCheckTool(cfg)

	block := model.NewBlock("tu1", "Welcome to Acme")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	findings := qaFindings(result.Resource.(*model.Block))
	require.Len(t, findings, 1)
	assert.Equal(t, "pattern-missing", findings[0].Category)
	assert.Equal(t, check.SeverityMajor, findings[0].Severity)
	assert.Contains(t, findings[0].Message, "not found in source")
}

// TestPatternCheckCheckSourcePass verifies a clean source produces no findings
// and that the source scope ignores any target.
func TestPatternCheckCheckSourcePass(t *testing.T) {
	t.Parallel()
	cfg := &tools.PatternCheckConfig{
		CheckSource: true,
		Patterns: []tools.PatternRule{
			{Name: "todo-marker", Pattern: `(?i)todo`, MustNotMatch: true},
		},
	}
	tl := tools.NewPatternCheckTool(cfg)

	block := model.NewBlock("tu1", "Polished source copy")
	// A target containing the forbidden pattern must be ignored in source scope.
	block.SetTargetText(model.LocaleFrench, "TODO traduire")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	assert.Empty(t, qaFindings(result.Resource.(*model.Block)))
}

// TestPatternCheckConfigValidationCheckSource confirms the source scope does
// not require a target locale.
func TestPatternCheckConfigValidationCheckSource(t *testing.T) {
	t.Parallel()
	cfg := tools.PatternCheckConfig{
		CheckSource: true,
		Patterns:    []tools.PatternRule{{Name: "t", Pattern: `todo`, MustNotMatch: true}},
	}
	require.NoError(t, cfg.Validate())
}

func TestPatternCheckConfigValidation(t *testing.T) {
	t.Parallel()
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

func TestPatternCheckForbiddenPatternNotPresent(t *testing.T) {
	t.Parallel()
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
	assert.Empty(t, qaFindings(resultBlock))
}
