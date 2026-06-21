package tools_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLengthCheckToolMaxChars(t *testing.T) {
	t.Parallel()
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
	findings := qaFindings(resultBlock)
	require.Len(t, findings, 1)
	assert.Equal(t, "max-chars-exceeded", findings[0].Category)
	assert.Equal(t, check.SeverityMajor, findings[0].Severity)
	assert.Contains(t, findings[0].Message, "23")
	assert.Contains(t, findings[0].Message, "10")
}

func TestLengthCheckToolMaxWords(t *testing.T) {
	t.Parallel()
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
	findings := qaFindings(resultBlock)
	require.Len(t, findings, 1)
	assert.Equal(t, "max-words-exceeded", findings[0].Category)
	assert.Equal(t, check.SeverityMajor, findings[0].Severity)
}

func TestLengthCheckToolMaxPercentage(t *testing.T) {
	t.Parallel()
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
	findings := qaFindings(resultBlock)
	require.Len(t, findings, 1)
	assert.Equal(t, "max-percentage-exceeded", findings[0].Category)
	assert.Equal(t, check.SeverityMinor, findings[0].Severity)
}

func TestLengthCheckToolMinPercentage(t *testing.T) {
	t.Parallel()
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
	findings := qaFindings(resultBlock)
	require.Len(t, findings, 1)
	assert.Equal(t, "min-percentage-exceeded", findings[0].Category)
	assert.Equal(t, check.SeverityMinor, findings[0].Severity)
}

func TestLengthCheckToolPass(t *testing.T) {
	t.Parallel()
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
	assert.Empty(t, qaFindings(resultBlock))
}

func TestLengthCheckToolMultipleViolations(t *testing.T) {
	t.Parallel()
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
	findings := qaFindings(resultBlock)
	assert.Len(t, findings, 3, "Expected 3 findings: max-chars, max-words, max-percentage")
}

func TestLengthCheckToolSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
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
	assert.Empty(t, qaFindings(resultBlock))
}

func TestLengthCheckToolNoTarget(t *testing.T) {
	t.Parallel()
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
	assert.Empty(t, qaFindings(resultBlock))
}

// TestLengthCheckToolCheckSourceMaxChars verifies the source scope flags an
// over-long source block with no target set or target-language configured.
func TestLengthCheckToolCheckSourceMaxChars(t *testing.T) {
	t.Parallel()
	cfg := &tools.LengthCheckConfig{
		CheckSource: true,
		MaxChars:    10,
	}
	tl := tools.NewLengthCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world, this is long") // 25 chars, no target
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	findings := qaFindings(result.Resource.(*model.Block))
	require.Len(t, findings, 1)
	assert.Equal(t, "max-chars-exceeded", findings[0].Category)
	assert.Equal(t, check.SeverityMajor, findings[0].Severity)
	assert.Contains(t, findings[0].Message, "Source has 25")
	assert.Contains(t, findings[0].Message, "10")
}

// TestLengthCheckToolCheckSourceMaxWords verifies the source scope flags a
// source block with too many words.
func TestLengthCheckToolCheckSourceMaxWords(t *testing.T) {
	t.Parallel()
	cfg := &tools.LengthCheckConfig{
		CheckSource: true,
		MaxWords:    2,
	}
	tl := tools.NewLengthCheckTool(cfg)

	block := model.NewBlock("tu1", "one two three") // 3 words, no target
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	findings := qaFindings(result.Resource.(*model.Block))
	require.Len(t, findings, 1)
	assert.Equal(t, "max-words-exceeded", findings[0].Category)
	assert.Equal(t, check.SeverityMajor, findings[0].Severity)
	assert.Contains(t, findings[0].Message, "Source has 3")
}

// TestLengthCheckToolCheckSourcePass verifies a short source passes cleanly and
// that the source scope ignores any target (no ratio checks).
func TestLengthCheckToolCheckSourcePass(t *testing.T) {
	t.Parallel()
	cfg := &tools.LengthCheckConfig{
		CheckSource: true,
		MaxChars:    50,
		MaxWords:    10,
		// Ratio thresholds set but irrelevant in source scope.
		MaxPercentage: 100.0,
	}
	tl := tools.NewLengthCheckTool(cfg)

	block := model.NewBlock("tu1", "Short source")
	// A target is present but must be ignored by the source scope.
	block.SetTargetText(model.LocaleFrench, "Une traduction française bien plus longue que la source")
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	assert.Empty(t, qaFindings(result.Resource.(*model.Block)))
}

// TestLengthCheckToolCheckSourceSkipsNonTranslatable confirms non-translatable
// blocks are skipped in source scope too.
func TestLengthCheckToolCheckSourceSkipsNonTranslatable(t *testing.T) {
	t.Parallel()
	cfg := &tools.LengthCheckConfig{CheckSource: true, MaxChars: 3}
	tl := tools.NewLengthCheckTool(cfg)

	block := model.NewBlock("tu1", "Hello world")
	block.Translatable = false
	part := &model.Part{Type: model.PartBlock, Resource: block}
	result := processPart(t, tl, part)

	assert.Empty(t, qaFindings(result.Resource.(*model.Block)))
}

// TestLengthCheckConfigValidationCheckSource confirms the source scope does not
// require a target locale.
func TestLengthCheckConfigValidationCheckSource(t *testing.T) {
	t.Parallel()
	cfg := tools.LengthCheckConfig{CheckSource: true, MaxChars: 10}
	require.NoError(t, cfg.Validate())
}

func TestLengthCheckConfigValidation(t *testing.T) {
	t.Parallel()
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
