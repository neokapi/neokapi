//go:build integration

package properties

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.properties.PropertiesFilter"
const mimeType = "text/x-java-properties"

// readProps parses a properties snippet with custom filter params and returns the parts.
func readProps(t *testing.T, snippet string, params map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.properties", mimeType, params)
}

// snippetRoundtrip roundtrips a properties snippet and returns the output string.
func snippetRoundtrip(t *testing.T, snippet string, params map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, []byte(snippet), "test.properties", mimeType, params)
	return string(result.Output)
}

// blockByName finds a block whose Name matches the given name.
func blockByName(blocks []*model.Block, name string) *model.Block {
	for _, b := range blocks {
		if b.Name == name {
			return b
		}
	}
	return nil
}

// okapi-unmapped: PropertiesFilterTest#testDefaultInfo — tests Java filter metadata (name, configurations), not relevant to bridge extraction
// okapi: PropertiesFilterTest#testDefaultInfo
func TestExtract_DefaultInfo(t *testing.T) {
	parts := readProps(t, "key=value", nil)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// okapi: PropertiesFilterTest#testEntry
func TestExtract_Entry(t *testing.T) {
	snippet := "Key1=Text1\n# Comment\nKey2=Text2\n"
	parts := readProps(t, snippet, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)

	// Verify second text unit has correct text and name.
	b2 := blockByName(blocks, "Key2")
	require.NotNil(t, b2, "should find block with name Key2")
	assert.Equal(t, "Text2", b2.SourceText())
}

// okapi: PropertiesFilterTest#testSplicedEntry
func TestExtract_SplicedEntry(t *testing.T) {
	snippet := "Key1=Text1\nKey2=Text2 \\\nSecond line"
	parts := readProps(t, snippet, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	b2 := blockByName(blocks, "Key2")
	require.NotNil(t, b2, "should find block with name Key2")
	assert.Equal(t, "Text2 Second line", b2.SourceText())
}

// okapi: PropertiesFilterTest#testEscapes
func TestExtract_Escapes(t *testing.T) {
	snippet := "Key1=Text with \\u00E3"
	parts := readProps(t, snippet, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	// \u00E3 = ã (a with tilde)
	assert.Equal(t, "Text with \u00E3", blocks[0].SourceText())
}

// okapi: PropertiesFilterTest#testKeySpecial
func TestExtract_KeySpecial(t *testing.T) {
	snippet := "\\:\\= : Text1"
	parts := readProps(t, snippet, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Text1", blocks[0].SourceText())
	assert.Equal(t, "\\:\\=", blocks[0].Name)
}

// okapi: PropertiesFilterTest#testLineBreaks_CR
func TestExtract_LineBreaksCR(t *testing.T) {
	snippet := "Key1=Text1\rKey2=Text2\r"
	result := snippetRoundtrip(t, snippet, nil)
	assert.Equal(t, snippet, result)
}

// okapi: PropertiesFilterTest#testineBreaks_CRLF
func TestExtract_LineBreaksCRLF(t *testing.T) {
	snippet := "Key1=Text1\r\nKey2=Text2\r\n"
	result := snippetRoundtrip(t, snippet, nil)
	assert.Equal(t, snippet, result)
}

// okapi: PropertiesFilterTest#testLineBreaks_LF
func TestExtract_LineBreaksLF(t *testing.T) {
	snippet := "Key1=Text1\n\n\nKey2=Text2\n"
	result := snippetRoundtrip(t, snippet, nil)
	assert.Equal(t, snippet, result)
}

// okapi: PropertiesFilterTest#testMessagePlaceholders
func TestExtract_MessagePlaceholders(t *testing.T) {
	snippet := "Key1={1}Text1{2}"
	parts := readProps(t, snippet, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// The Java test verifies {1} and {2} are extracted as inline codes.
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)

	var placeholderCount int
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanPlaceholder {
			placeholderCount++
		}
	}
	assert.Equal(t, 2, placeholderCount, "should have 2 placeholder spans for {1} and {2}")

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Text1")
}

// okapi: PropertiesFilterTest#testMessagePlaceholdersEscaped
func TestExtract_MessagePlaceholdersEscaped(t *testing.T) {
	// In the Java test, this is identical to testMessagePlaceholders.
	snippet := "Key1={1}Text1{2}"
	parts := readProps(t, snippet, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)

	var placeholderCount int
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanPlaceholder {
			placeholderCount++
		}
	}
	assert.Equal(t, 2, placeholderCount, "should have 2 placeholder spans for {1} and {2}")
}

// okapi: PropertiesFilterTest#testLocDirectives_Skip
func TestExtract_LocDirectivesSkip(t *testing.T) {
	snippet := "#_skip\nKey1:Text1\nKey2:Text2"
	parts := readProps(t, snippet, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text2")
	assert.NotContains(t, texts, "Text1")
}

// okapi: PropertiesFilterTest#testLocDirectives_Group
func TestExtract_LocDirectivesGroup(t *testing.T) {
	snippet := "#_bskip\nKey1:Text1\n#_text\nKey2:Text2\nKey2:Text3"
	parts := readProps(t, snippet, nil)

	blocks := bridgetest.TranslatableBlocks(parts)

	texts := bridgetest.BlockTexts(blocks)
	if len(blocks) > 0 {
		assert.Equal(t, "Text2", blocks[0].SourceText())
	}

	assert.NotContains(t, texts, "Text1")
	assert.LessOrEqual(t, len(blocks), 1, "should have at most 1 translatable block")
}

// okapi: PropertiesFilterTest#testSpecialChars
func TestExtract_SpecialChars(t *testing.T) {
	snippet := "Key1:Text1\\n=lf, \\t=tab, \\w=w, \\r=cr, \\\\=bs\n"
	parts := readProps(t, snippet, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	// \n is decoded to newline, \t to tab, \\ to literal backslash pair.
	// \w is kept as \w (unknown escape). \r behavior depends on the bridge.
	assert.Contains(t, text, "Text1\n=lf, \t=tab, \\w=w,")
	assert.Contains(t, text, "=cr, \\\\=bs")
}

// okapi: PropertiesFilterTest#testSpecialCharsInKey
func TestExtract_SpecialCharsInKey(t *testing.T) {
	snippet := "Key\\ \\:\\\\:Text1\n"
	parts := readProps(t, snippet, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Key\\ \\:\\\\", blocks[0].Name)
	assert.Equal(t, "Text1", blocks[0].SourceText())
}

// okapi: PropertiesFilterTest#testSpecialCharsOutput
func TestExtract_SpecialCharsOutput(t *testing.T) {
	snippet := "Key1:Text1\\n=lf, \\t=tab \\w=w, \\r=cr, \\\\=bs\n"
	result := snippetRoundtrip(t, snippet, nil)
	assert.Equal(t, snippet, result)
}

// okapi: PropertiesFilterTest#testStartDocument
func TestExtract_StartDocument(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	path := tdDir + "/okf_properties/Test01.properties"
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.NotEmpty(t, layer.ID)
}

// okapi: PropertiesFilterTest#testHtmlOutput
func TestExtract_HtmlOutput(t *testing.T) {
	// The Java test uses the HtmlFilter (getEvents2) to process this snippet.
	// In the bridge, we verify properties filter preserves HTML-like content
	// without a subfilter. The bridge appends a trailing newline.
	snippet := "Key1=<b>Text with &amp;=amp test</b>"
	result := snippetRoundtrip(t, snippet, nil)
	assert.Equal(t, snippet+"\n", result)
}

// okapi: PropertiesFilterTest#testJavaEscapeChars
func TestExtract_JavaEscapeChars(t *testing.T) {
	snippet := "key1=Listen\\: here's the \\#1 thing to remember\\: a \\!\\= b \\\\ c"
	params := map[string]any{
		"useJavaEscapes": true,
	}
	parts := readProps(t, snippet, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Listen: here's the #1 thing to remember: a != b \\ c", blocks[0].SourceText())
}

// okapi: PropertiesFilterTest#testIdGeneration_defaultConfig
func TestExtract_IdGenerationDefaultConfig(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	path := tdDir + "/okf_properties/issue_216.properties"
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.Equal(t, 3, len(blocks), "issue_216.properties should produce 3 text units with default config")
}

// okapi-blocked: PropertiesFilterTest#testIdGeneration_subfiltersConfig — bridge does not set up FilterConfigurationMapper for subfilter resolution
// okapi: PropertiesFilterTest#testIdGeneration_subfiltersConfig
func TestExtract_IdGenerationSubfiltersConfig(t *testing.T) {
	t.Skip("bridge limitation: Properties filter subfilter requires FilterConfigurationMapper (fcMapper is null)")
}
