package rtf_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/formats/rtf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigDefaults(t *testing.T) {
	cfg := &rtf.Config{}
	cfg.Reset()
	assert.Equal(t, "rtf", cfg.FormatName())
	assert.False(t, cfg.ExtractHeadersFooters)
	assert.False(t, cfg.ExtractAnnotations)
	assert.False(t, cfg.ExtractBookmarks)
	assert.False(t, cfg.UseCodeFinder)
	assert.Empty(t, cfg.CodeFinderRules)
	require.NoError(t, cfg.Validate())
}

func TestConfigApplyMapAll(t *testing.T) {
	cfg := &rtf.Config{}
	cfg.Reset()

	err := cfg.ApplyMap(map[string]any{
		"extractHeadersFooters": true,
		"extractAnnotations":    true,
		"extractBookmarks":      true,
		"useCodeFinder":         true,
		"codeFinderRules":       []any{"pattern1"},
	})
	require.NoError(t, err)

	assert.True(t, cfg.ExtractHeadersFooters)
	assert.True(t, cfg.ExtractAnnotations)
	assert.True(t, cfg.ExtractBookmarks)
	assert.True(t, cfg.UseCodeFinder)
	assert.Equal(t, []string{"pattern1"}, cfg.CodeFinderRules)
}

func TestConfigApplyMapUnknown(t *testing.T) {
	cfg := &rtf.Config{}
	cfg.Reset()
	err := cfg.ApplyMap(map[string]any{"unknown": "value"})
	require.Error(t, err)
}

func TestConfigApplyMapTypeMismatch(t *testing.T) {
	cfg := &rtf.Config{}
	cfg.Reset()
	err := cfg.ApplyMap(map[string]any{"extractHeadersFooters": "notabool"})
	require.Error(t, err)
}

func TestConfigApplyMapCodeFinderRulesBridgeStyle(t *testing.T) {
	cfg := &rtf.Config{}
	cfg.Reset()
	err := cfg.ApplyMap(map[string]any{
		"codeFinderRules": map[string]any{
			"count": float64(2),
			"rule0": "pat1",
			"rule1": "pat2",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"pat1", "pat2"}, cfg.CodeFinderRules)
}

func TestConfigKind(t *testing.T) {
	cfg := &rtf.Config{}
	kind := cfg.ConfigKind()
	assert.Contains(t, string(kind), "Rtf")
}

func TestExtractHeadersFootersEnabled(t *testing.T) {
	ctx := context.Background()
	reader := rtf.NewReader()

	cfg := reader.Config().(*rtf.Config)
	cfg.ExtractHeadersFooters = true

	// RTF with header and body text
	input := `{\rtf1\ansi\deff0
{\header This is header text}
\pard Body paragraph\par
}`

	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	// With headers enabled, should extract header text too
	var foundBody, foundHeader bool
	for _, b := range blocks {
		if b.SourceText() == "Body paragraph" {
			foundBody = true
		}
		if b.SourceText() == "This is header text" {
			foundHeader = true
		}
	}
	assert.True(t, foundBody, "should extract body text")
	assert.True(t, foundHeader, "should extract header text when enabled")
}

func TestExtractHeadersFootersDisabled(t *testing.T) {
	ctx := context.Background()
	reader := rtf.NewReader()

	// Default config: headers/footers disabled
	input := `{\rtf1\ansi\deff0
{\header This is header text}
\pard Body paragraph\par
}`

	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	// With headers disabled, should only extract body text
	var foundHeader bool
	for _, b := range blocks {
		if b.SourceText() == "This is header text" {
			foundHeader = true
		}
	}
	assert.False(t, foundHeader, "should NOT extract header text when disabled")
	// Should have at least the body paragraph
	require.GreaterOrEqual(t, len(blocks), 1)
}
