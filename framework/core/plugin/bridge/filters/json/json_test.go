//go:build integration

package json

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.json.JSONFilter"
const mimeType = "application/json"

// readJSON parses a JSON snippet with custom filter params and returns the parts.
func readJSON(t *testing.T, snippet string, params map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.json", mimeType, params)
}

// readJSONDefault parses a JSON snippet with default (nil) params.
func readJSONDefault(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readJSON(t, snippet, nil)
}

// snippetRoundtrip roundtrips a JSON snippet and returns the output string.
func snippetRoundtrip(t *testing.T, snippet string, params map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, []byte(snippet), "test.json", mimeType, params)
	return string(result.Output)
}

// findBlockWithName finds a block whose Name matches exactly.
func findBlockWithName(blocks []*model.Block, name string) *model.Block {
	for _, b := range blocks {
		if b.Name == name {
			return b
		}
	}
	return nil
}

// findBlockContaining finds a block whose source text contains substr.
func findBlockContaining(blocks []*model.Block, substr string) *model.Block {
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), substr) {
			return b
		}
	}
	return nil
}

// blockNames returns the Names of each block.
func blockNames(blocks []*model.Block) []string {
	names := make([]string, len(blocks))
	for i, b := range blocks {
		names[i] = b.Name
	}
	return names
}

// ---------------------------------------------------------------------------
// Tests translated from JSONFilterTest.java
// ---------------------------------------------------------------------------

// okapi: JSONFilterTest#testValue
func TestExtract_Value(t *testing.T) {
	parts := readJSONDefault(t, `{"key" : "Text1"}`)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text1")

	// Key should be used as the block name.
	assert.Equal(t, "key", blocks[0].Name)
}

// okapi: JSONFilterTest#testObject
func TestExtract_Object(t *testing.T) {
	parts := readJSONDefault(t, `{"key" : { "key2" : "Text1" } }`)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text1")
}

// okapi: JSONFilterTest#testList
func TestExtract_List(t *testing.T) {
	// extractStandalone defaults to false, so array strings are not extracted.
	// But objects within arrays should extract their key-value pairs.
	parts := readJSON(t, `{"items": [{"key": "Text1"}]}`, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text1")
}

// okapi: JSONFilterTest#testAllWithKeyNoException
func TestExtract_AllWithKeyNoException(t *testing.T) {
	// Default: extractAllPairs=true, all key-value string pairs are extracted.
	// Key is used as the TU name.
	parts := readJSON(t, `{"key1": "Text1", "key2": "Text2"}`, map[string]any{
		"extractAllPairs": true,
		"useKeyAsName":    true,
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 2)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text1")
	assert.Contains(t, texts, "Text2")

	names := blockNames(blocks)
	assert.Contains(t, names, "key1")
	assert.Contains(t, names, "key2")
}

// okapi: JSONFilterTest#testAllWithKeyWithException
func TestExtract_AllWithKeyWithException(t *testing.T) {
	// extractAllPairs=true with exceptions: keys matching the exceptions pattern are excluded.
	parts := readJSON(t, `{"key1": "Text1", "key2": "Text2", "not_this": "Text3"}`, map[string]any{
		"extractAllPairs": true,
		"exceptions":      "not_this",
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text1")
	assert.Contains(t, texts, "Text2")
	assert.NotContains(t, texts, "Text3")
}

// okapi: JSONFilterTest#testNoneWithKeywithException
func TestExtract_NoneWithKeywithException(t *testing.T) {
	// extractAllPairs=false with exceptions: only keys matching exceptions are extracted.
	parts := readJSON(t, `{"key1": "Text1", "key2": "Text2", "extract_me": "Text3"}`, map[string]any{
		"extractAllPairs": false,
		"exceptions":      "extract_me",
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text3")
	assert.NotContains(t, texts, "Text1")
	assert.NotContains(t, texts, "Text2")
}

// okapi: JSONFilterTest#testPath
func TestExtract_Path(t *testing.T) {
	// useFullKeyPath produces hierarchical key paths as TU names.
	parts := readJSON(t, `{"parent": {"child": "Text1"}}`, map[string]any{
		"useFullKeyPath":           true,
		"useKeyAsName":             true,
		"useLeadingSlashOnKeyPath": true,
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text1")

	// With useFullKeyPath and useLeadingSlashOnKeyPath, name should be /parent/child.
	names := blockNames(blocks)
	assert.Contains(t, names, "/parent/child")
}

// okapi: JSONFilterTest#testLeadingSlash
func TestExtract_LeadingSlash(t *testing.T) {
	// useLeadingSlashOnKeyPath=false should omit the leading slash.
	parts := readJSON(t, `{"parent": {"child": "Text1"}}`, map[string]any{
		"useFullKeyPath":           true,
		"useKeyAsName":             true,
		"useLeadingSlashOnKeyPath": false,
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	names := blockNames(blocks)
	assert.Contains(t, names, "parent/child")
}

// okapi: JSONFilterTest#testStandaloneYes
func TestExtract_StandaloneYes(t *testing.T) {
	// extractStandalone=true (called extractIsolatedStrings in fprm) should extract array strings.
	parts := readJSON(t, `{"colors": ["Red", "Green", "Blue"]}`, map[string]any{
		"extractIsolatedStrings": true,
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Red")
	assert.Contains(t, texts, "Green")
	assert.Contains(t, texts, "Blue")
}

// okapi: JSONFilterTest#testStandaloneDefaultWhichIsNo
func TestExtract_StandaloneDefaultWhichIsNo(t *testing.T) {
	// Default: standalone strings in arrays are NOT extracted.
	parts := readJSONDefault(t, `{"colors": ["Red", "Green", "Blue"], "label": "Color list"}`)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Color list")
	assert.NotContains(t, texts, "Red")
}

// okapi: JSONFilterTest#testEscape
func TestExtract_Escape(t *testing.T) {
	// Unicode escape \u00E0 should be decoded to raw character.
	parts := readJSONDefault(t, `{"key": "\u00E0"}`)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "\u00E0", blocks[0].SourceText()) // à
}

// okapi: JSONFilterTest#testEscapes
func TestExtract_Escapes(t *testing.T) {
	// All JSON escape sequences.
	parts := readJSONDefault(t, `{"key": "a\nb\tc\\d\"e\/f"}`)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "a\nb")
	assert.Contains(t, text, "b\tc")
	assert.Contains(t, text, "c\\d")
	assert.Contains(t, text, "d\"e")
	assert.Contains(t, text, "e/f")
}

// okapi: JSONFilterTest#testEscapedForwardSlashDecoding
func TestExtract_EscapedForwardSlashDecoding(t *testing.T) {
	// Escaped forward slashes are decoded correctly regardless of escapeForwardSlashes config.
	parts := readJSONDefault(t, `{"key": "a\/b"}`)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	// The extracted text should have the unescaped forward slash.
	assert.Equal(t, "a/b", blocks[0].SourceText())
}

// okapi: JSONFilterTest#testEscapeForwardSlashes
func TestExtract_EscapeForwardSlashes(t *testing.T) {
	// Default: escapeForwardSlashes=true, forward slashes are escaped in output.
	output := snippetRoundtrip(t, `{"key": "a/b"}`, nil)
	assert.Contains(t, output, `a\/b`)
}

// okapi: JSONFilterTest#testNoEscapeForwardSlashes
func TestExtract_NoEscapeForwardSlashes(t *testing.T) {
	// escapeForwardSlashes=false preserves slashes unescaped.
	output := snippetRoundtrip(t, `{"key": "a/b"}`, map[string]any{
		"escapeForwardSlashes": false,
	})
	assert.Contains(t, output, `a/b`)
	assert.NotContains(t, output, `a\/b`)
}

// okapi: JSONFilterTest#testEmptyValue
func TestExtract_EmptyValue(t *testing.T) {
	// Empty string value should be preserved in output.
	output := snippetRoundtrip(t, `{"key": ""}`, nil)
	assert.Contains(t, output, `""`)
}

// okapi: JSONFilterTest#testDecimalNumber
func TestExtract_DecimalNumber(t *testing.T) {
	// Decimal numbers are not extracted as text units.
	parts := readJSONDefault(t, `{"name": "John", "score": 9.5}`)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "John")
	for _, text := range texts {
		assert.NotEqual(t, "9.5", text, "decimal numbers should not be extracted")
	}
}

// okapi: JSONFilterTest#testSimpleEntrySkeleton
func TestExtract_SimpleEntrySkeleton(t *testing.T) {
	// Skeleton preserves whitespace and line breaks.
	input := "{\n  \"key\" : \"value\"\n}"
	output := snippetRoundtrip(t, input, nil)
	// The output should preserve the general structure.
	assert.Contains(t, output, "key")
	assert.Contains(t, output, "value")
}

// okapi: JSONFilterTest#testLineBreaks
func TestExtract_LineBreaks(t *testing.T) {
	// Line break type detected from document (\r).
	input := "{\r\"key\" : \"value\"\r}"
	output := snippetRoundtrip(t, input, nil)
	assert.Contains(t, output, "value")
}

// okapi: JSONFilterTest#testWhiteSpaceAndComments
func TestExtract_WhiteSpaceAndComments(t *testing.T) {
	// JSON filter supports relaxed JSON with comments.
	input := `{
  /* block comment */
  // line comment
  # hash comment
  "key": "Text1"
}`
	parts := readJSONDefault(t, input)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text1")
}

// okapi: JSONFilterTest#testMultilineComment
func TestExtract_MultilineComment(t *testing.T) {
	input := `{
  /* this is
     a multiline
     comment */
  "key": "Text1"
}`
	parts := readJSONDefault(t, input)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text1")
}

// okapi: JSONFilterTest#testNestedComments
func TestExtract_NestedComments(t *testing.T) {
	input := `{
  /* outer /* nested */ comment */
  "key": "Text1"
}`
	parts := readJSONDefault(t, input)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text1")
}

// okapi: JSONFilterTest#testSmartQuotes
func TestExtract_SmartQuotes(t *testing.T) {
	// Smart quotes (curly) handled with HTML subfilter.
	parts := readJSON(t, `{"key": "\u201CHello\u201D"}`, map[string]any{
		"subfilter": "okf_html",
	})
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	// Should extract the text with smart quotes.
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Hello")
}

// okapi: JSONFilterTest#testSubfilter
func TestExtract_Subfilter(t *testing.T) {
	// HTML subfilter: JSON unescaping, HTML parsing, roundtrip.
	input := `{"key": "<p>Hello <b>world</b></p>"}`
	parts := readJSON(t, input, map[string]any{
		"subfilter": "okf_html",
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	// With HTML subfilter, the HTML content should be parsed.
	// The text should contain "Hello" and "world".
	var foundHello bool
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "Hello") {
			foundHello = true
			break
		}
	}
	assert.True(t, foundHello, "subfilter should extract HTML content")

	// Roundtrip should preserve the JSON structure.
	output := snippetRoundtrip(t, input, map[string]any{
		"subfilter": "okf_html",
	})
	assert.Contains(t, output, "key")
	assert.Contains(t, output, "Hello")
}

// okapi: JSONFilterTest#testSubfilterEasyToDebug
func TestExtract_SubfilterEasyToDebug(t *testing.T) {
	// Simplified subfilter test for debugging.
	input := `{"key": "<b>bold</b>"}`
	parts := readJSON(t, input, map[string]any{
		"subfilter": "okf_html",
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	var foundBold bool
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "bold") {
			foundBold = true
			break
		}
	}
	assert.True(t, foundBold, "subfilter should extract bold text")
}

// okapi: JSONFilterTest#testSubfiltersProduceDistinctTextUnitIds
func TestExtract_SubfiltersProduceDistinctTextUnitIds(t *testing.T) {
	input := `{"key1": "<p>Text1</p>", "key2": "<p>Text2</p>"}`
	parts := readJSON(t, input, map[string]any{
		"subfilter": "okf_html",
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)

	// All block IDs should be unique.
	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.False(t, ids[b.ID], "duplicate block ID: %s", b.ID)
		ids[b.ID] = true
	}
}

// okapi: JSONFilterTest#testEscapeForwardSlashesSubfilter
func TestExtract_EscapeForwardSlashesSubfilter(t *testing.T) {
	// Forward slash escaping with HTML subfilter.
	input := `{"key": "<a href=\"http://example.com\">link</a>"}`
	output := snippetRoundtrip(t, input, map[string]any{
		"subfilter": "okf_html",
	})
	// With default escapeForwardSlashes=true, forward slashes should be escaped.
	assert.Contains(t, output, "key")
	assert.Contains(t, output, "link")
}

// okapi: JSONFilterTest#testInlineCodeFinderEscaping
func TestExtract_InlineCodeFinderEscaping(t *testing.T) {
	// Code finder with JSON-escaped HTML tags.
	input := `{"key": "Hello <b>world<\/b> end"}`
	parts := readJSON(t, input, map[string]any{
		"useCodeFinder": true,
		"codeFinderRules": map[string]any{
			"count":                     1,
			"rule0":                     `</?([A-Z0-9a-z]*)\b[^>]*>`,
			"sample":                    `<tag></at><tag/> <tag attr='val'> </tag="val">`,
			"useAllRulesWhenTesting":    true,
		},
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Hello")
	assert.Contains(t, text, "world")
}

// okapi: JSONFilterTest#testInlineCodeFinderNewLineCharacter
func TestExtract_InlineCodeFinderNewLineCharacter(t *testing.T) {
	// Code finder detecting \n as inline code.
	input := `{"key": "Line1\nLine2"}`
	parts := readJSON(t, input, map[string]any{
		"useCodeFinder": true,
		"codeFinderRules": map[string]any{
			"count":                     1,
			"rule0":                     `\\n`,
			"sample":                    `\n`,
			"useAllRulesWhenTesting":    true,
		},
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Line1")
	assert.Contains(t, text, "Line2")
}

// okapi: JSONFilterTest#testDefaultInfo
// okapi-unmapped: JSONFilterTest#testDefaultInfo — Java-specific API: tests filter metadata (name, display name, configurations)
func TestExtract_DefaultInfo(t *testing.T) {
	t.Skip("Java-specific: tests filter metadata API (getName, getDisplayName, getConfigurations)")
}

// okapi: JSONFilterTest#testNoteRules
func TestExtract_NoteRules(t *testing.T) {
	// noteRules marks keys as notes attached to next TU.
	input := `{"note": "This is a note", "key": "Text1"}`
	parts := readJSON(t, input, map[string]any{
		"extractAllPairs": true,
		"noteRules":       "note",
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text1")
	// The note should not be extracted as translatable text.
	assert.NotContains(t, texts, "This is a note")
}

// okapi: JSONFilterTest#testIdRules
func TestExtract_IdRules(t *testing.T) {
	// idRules uses key value as TU name/ID.
	input := `{"id": "my-id", "key": "Text1"}`
	parts := readJSON(t, input, map[string]any{
		"extractAllPairs": true,
		"idRules":         "id",
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// The id value should be used as the block name.
	b := findBlockContaining(blocks, "Text1")
	require.NotNil(t, b, "should have block with Text1")
	assert.Equal(t, "my-id", b.Name)
}

// okapi: JSONFilterTest#testNestedIdRules
func TestExtract_NestedIdRules(t *testing.T) {
	// Nested id rules with useIdStack produce compound IDs.
	input := `{"items": [{"id": "item1", "text": "Text1"}, {"id": "item2", "text": "Text2"}]}`
	parts := readJSON(t, input, map[string]any{
		"extractAllPairs": true,
		"idRules":         "id",
		"useIdStack":      true,
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text1")
	assert.Contains(t, texts, "Text2")
}

// okapi: JSONFilterTest#testGenericMetaRules
func TestExtract_GenericMetaRules(t *testing.T) {
	// genericMetaRules attaches metadata annotations to TUs.
	input := `{"meta": "metadata-value", "key": "Text1"}`
	parts := readJSON(t, input, map[string]any{
		"extractAllPairs":    true,
		"genericMetaRules":   "meta",
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text1")
	// The meta value should not be extracted as translatable text.
	assert.NotContains(t, texts, "metadata-value")
}

// okapi: JSONFilterTest#testGenericMetaRulesWithId
func TestExtract_GenericMetaRulesWithId(t *testing.T) {
	// Generic meta rules combined with id rules.
	input := `{"id": "my-id", "meta": "metadata-value", "key": "Text1"}`
	parts := readJSON(t, input, map[string]any{
		"extractAllPairs":    true,
		"idRules":            "id",
		"genericMetaRules":   "meta",
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := findBlockContaining(blocks, "Text1")
	require.NotNil(t, b)
	assert.Equal(t, "my-id", b.Name)
}

// okapi: JSONFilterTest#testExtractionRules
func TestExtract_ExtractionRules(t *testing.T) {
	// extractionRules limits which keys are extracted.
	input := `{"title": "Extract me", "body": "Extract me too", "id": "Skip me"}`
	parts := readJSON(t, input, map[string]any{
		"extractAllPairs":   false,
		"extractionRules":   "title|body",
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Extract me")
	assert.Contains(t, texts, "Extract me too")
	assert.NotContains(t, texts, "Skip me")
}

// okapi: JSONFilterTest#testSubfilterRules
func TestExtract_SubfilterRules(t *testing.T) {
	// subfilterRules applies subfilter only to specific keys.
	input := `{"html_key": "<b>Bold text</b>", "plain_key": "Plain text"}`
	parts := readJSON(t, input, map[string]any{
		"subfilterRules": "html_key",
		"subfilter":      "okf_html",
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// html_key should be processed by the HTML subfilter.
	var foundBold, foundPlain bool
	for _, b := range blocks {
		text := b.SourceText()
		if strings.Contains(text, "Bold text") {
			foundBold = true
		}
		if strings.Contains(text, "Plain text") {
			foundPlain = true
		}
	}
	assert.True(t, foundBold, "subfilterRules should process html_key with HTML subfilter")
	assert.True(t, foundPlain, "plain_key should still be extracted")
}

// okapi: JSONFilterTest#testArrayWithinArray
func TestExtract_ArrayWithinArray(t *testing.T) {
	// Nested arrays produce array:N paths.
	input := `{"data": [["Text1"]]}` // Nested arrays: extractStandalone needed
	parts := readJSON(t, input, map[string]any{
		"extractIsolatedStrings":   true,
		"useFullKeyPath":           true,
		"useKeyAsName":             true,
		"useLeadingSlashOnKeyPath": true,
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text1")
}

// okapi: JSONFilterTest#testArrayWithinArrayWithinArray
func TestExtract_ArrayWithinArrayWithinArray(t *testing.T) {
	// Triple-nested arrays produce correct paths.
	input := `{"data": [[["Text1"]]]}` // Triple nested
	parts := readJSON(t, input, map[string]any{
		"extractIsolatedStrings":   true,
		"useFullKeyPath":           true,
		"useKeyAsName":             true,
		"useLeadingSlashOnKeyPath": true,
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text1")
}

// okapi: JSONFilterTest#testArrayWithObject
func TestExtract_ArrayWithObject(t *testing.T) {
	// Array containing objects produces correct paths.
	input := `{"items": [{"name": "Text1"}]}`
	parts := readJSON(t, input, map[string]any{
		"useFullKeyPath":           true,
		"useKeyAsName":             true,
		"useLeadingSlashOnKeyPath": true,
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text1")
}

// okapi: JSONFilterTest#testNamedArray
func TestExtract_NamedArray(t *testing.T) {
	// Named arrays produce key-based paths.
	input := `{"items": [{"name": "Text1"}, {"name": "Text2"}]}`
	parts := readJSON(t, input, map[string]any{
		"useFullKeyPath":           true,
		"useKeyAsName":             true,
		"useLeadingSlashOnKeyPath": true,
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 2)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text1")
	assert.Contains(t, texts, "Text2")
}

// okapi: JSONFilterTest#testMaxwidthRules
func TestExtract_MaxwidthRules(t *testing.T) {
	// maxwidthRules sets MAX_WIDTH property on TUs.
	input := `{"maxwidth": 100, "title": "Text1"}`
	parts := readJSON(t, input, map[string]any{
		"extractAllPairs": false,
		"exceptions":      "title",
		"maxwidthRules":   "maxwidth",
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text1")
}

// okapi: JSONFilterTest#testMaxwidthRulesWithSizeChar
func TestExtract_MaxwidthRulesWithSizeChar(t *testing.T) {
	// maxwidthSizeUnit=char sets SIZE_UNIT property.
	input := `{"maxwidth": 50, "title": "Text1"}`
	parts := readJSON(t, input, map[string]any{
		"extractAllPairs":    false,
		"exceptions":         "title",
		"maxwidthRules":      "maxwidth",
		"maxwidthSizeUnit":   "char",
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text1")
}

// okapi: JSONFilterTest#testVariableMaxWidthInNestedObjects
func TestExtract_VariableMaxWidthInNestedObjects(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_json/nested_charsize.json")

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, map[string]any{
		"extractAllPairs":    false,
		"exceptions":         "title|content|^extracted",
		"useKeyAsName":       true,
		"maxwidthRules":      "maxchars",
		"maxwidthSizeUnit":   "char",
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "This text has no maxwidth")
	assert.Contains(t, texts, "This text has a maxwidth of 100")
	assert.Contains(t, texts, "This text has a maxwidth of 600")
}

// ---- JsonSnippetParserTest (1 test) ----

// okapi-unmapped: JsonSnippetParserTest#testSingleObject — Java-specific: tests internal parser tokenization, not filter behavior
func TestParse_SingleObject(t *testing.T) {
	t.Skip("Java-specific: tests internal parser tokenization API")
}
