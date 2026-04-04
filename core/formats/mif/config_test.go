package mif_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/formats/mif"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigDefaults(t *testing.T) {
	cfg := &mif.Config{}
	cfg.Reset()
	assert.Equal(t, "mif", cfg.FormatName())
	assert.True(t, cfg.ExtractBodyPages)
	assert.True(t, cfg.ExtractMasterPages)
	assert.True(t, cfg.ExtractReferencePages)
	assert.True(t, cfg.ExtractHiddenPages)
	assert.True(t, cfg.ExtractVariables)
	assert.True(t, cfg.ExtractIndexMarkers)
	assert.False(t, cfg.ExtractLinks)
	assert.False(t, cfg.ExtractReferenceFormats)
	assert.False(t, cfg.ExtractPgfNumFormatsInline)
	assert.True(t, cfg.ExtractHardReturnsAsText)
	assert.True(t, cfg.UseCodeFinder)
	assert.NotEmpty(t, cfg.CodeFinderRules)
	require.NoError(t, cfg.Validate())
}

func TestConfigApplyMapAllOptions(t *testing.T) {
	cfg := &mif.Config{}
	cfg.Reset()

	err := cfg.ApplyMap(map[string]any{
		"extractBodyPages":           false,
		"extractMasterPages":         false,
		"extractReferencePages":      false,
		"extractHiddenPages":         false,
		"extractVariables":           false,
		"extractIndexMarkers":        false,
		"extractLinks":               true,
		"extractReferenceFormats":    true,
		"extractPgfNumFormatsInline": true,
		"extractHardReturnsAsText":   false,
		"useCodeFinder":              false,
		"codeFinderRules":            []any{"rule1", "rule2"},
	})
	require.NoError(t, err)

	assert.False(t, cfg.ExtractBodyPages)
	assert.False(t, cfg.ExtractMasterPages)
	assert.False(t, cfg.ExtractReferencePages)
	assert.False(t, cfg.ExtractHiddenPages)
	assert.False(t, cfg.ExtractVariables)
	assert.False(t, cfg.ExtractIndexMarkers)
	assert.True(t, cfg.ExtractLinks)
	assert.True(t, cfg.ExtractReferenceFormats)
	assert.True(t, cfg.ExtractPgfNumFormatsInline)
	assert.False(t, cfg.ExtractHardReturnsAsText)
	assert.False(t, cfg.UseCodeFinder)
	assert.Equal(t, []string{"rule1", "rule2"}, cfg.CodeFinderRules)
}

func TestConfigApplyMapUnknown(t *testing.T) {
	cfg := &mif.Config{}
	cfg.Reset()
	err := cfg.ApplyMap(map[string]any{"unknown": "value"})
	require.Error(t, err)
}

func TestConfigApplyMapTypeMismatch(t *testing.T) {
	cfg := &mif.Config{}
	cfg.Reset()
	err := cfg.ApplyMap(map[string]any{"extractBodyPages": "notabool"})
	require.Error(t, err)
}

func TestConfigApplyMapCodeFinderRulesBridgeStyle(t *testing.T) {
	cfg := &mif.Config{}
	cfg.Reset()
	err := cfg.ApplyMap(map[string]any{
		"codeFinderRules": map[string]any{
			"count": float64(2),
			"rule0": "pattern1",
			"rule1": "pattern2",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"pattern1", "pattern2"}, cfg.CodeFinderRules)
}

func TestConfigKind(t *testing.T) {
	cfg := &mif.Config{}
	kind := cfg.ConfigKind()
	assert.Contains(t, string(kind), "Mif")
}

func TestExtractMasterPagesDisabled(t *testing.T) {
	ctx := t.Context()
	reader := mif.NewReader()

	// Apply config to disable master pages (they become skipped)
	cfg := reader.Config().(*mif.Config)
	cfg.ExtractMasterPages = false

	input := `<MIFFile 2015>
<MasterPage
 <Para
  <PgfTag ` + "`Body'>" + `
  <ParaLine
   <String ` + "`Master page text.'>" + `
  >
 >
>
<TextFlow
 <Para
  <PgfTag ` + "`Body'>" + `
  <ParaLine
   <String ` + "`Body text.'>" + `
  >
 >
>
`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	// Only body text should be extracted; master page content should be skipped
	require.Len(t, blocks, 1)
	assert.Equal(t, "Body text.", blocks[0].SourceText())
}

func TestExtractHardReturnsAsTextDisabled(t *testing.T) {
	ctx := t.Context()
	reader := mif.NewReader()

	cfg := reader.Config().(*mif.Config)
	cfg.ExtractHardReturnsAsText = false

	input := `<MIFFile 2015>
<TextFlow
 <Para
  <PgfTag ` + "`Body'>" + `
  <ParaLine
   <String ` + "`Before return'>" + `
   <Char HardReturn>
   <String ` + "`After return.'>" + `
  >
 >
>
`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	// Hard return should NOT be included when disabled
	assert.Equal(t, "Before returnAfter return.", blocks[0].SourceText())
	assert.NotContains(t, blocks[0].SourceText(), "\n")
}
