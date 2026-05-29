package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/check"
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
	findings := qaFindings(resultBlock)
	// Should find both { and }
	require.Len(t, findings, 2)
	for _, f := range findings {
		assert.Equal(t, "forbidden-char", f.Category)
		assert.Equal(t, check.SeverityMajor, f.Severity)
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
	findings := qaFindings(resultBlock)
	require.Len(t, findings, 1)
	assert.Equal(t, "required-char-missing", findings[0].Category)
	assert.Equal(t, check.SeverityMinor, findings[0].Severity)
	assert.Contains(t, findings[0].Message, "!")
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
	f, found := findFinding(qaFindings(resultBlock), "mojibake")
	require.True(t, found, "Expected mojibake finding")
	assert.Equal(t, check.SeverityMajor, f.Severity)
	assert.Contains(t, f.Message, "mojibake")
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
	f, found := findFinding(qaFindings(resultBlock), "replacement-char")
	require.True(t, found, "Expected replacement-char finding")
	assert.Equal(t, check.SeverityMajor, f.Severity)
	assert.Contains(t, f.Message, "U+FFFD")
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
	f, found := findFinding(qaFindings(resultBlock), "control-char")
	require.True(t, found, "Expected control-char finding")
	assert.Equal(t, check.SeverityMajor, f.Severity)
	assert.Contains(t, f.Message, "U+0001")
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
	assert.Empty(t, qaFindings(resultBlock))
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
	assert.Empty(t, qaFindings(resultBlock))
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
	assert.Empty(t, qaFindings(resultBlock))
}

func TestCharsCheckToolNoTarget(t *testing.T) {
	t.Parallel()
	cfg := tools.NewCharsCheckConfig(model.LocaleFrench)
	tl := tools.NewCharsCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	resultBlock := result.Resource.(*model.Block)
	assert.Empty(t, qaFindings(resultBlock))
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
