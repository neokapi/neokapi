package json_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Test helpers ---

func readJSON(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readJSONWith(t, snippet, nil)
}

func readJSONWith(t *testing.T, snippet string, params map[string]any) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	if params != nil {
		err := reader.Config().ApplyMap(params)
		require.NoError(t, err)
	}
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

func translatableBlocks(parts []*model.Part) []*model.Block {
	var blocks []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok && b.Translatable {
				blocks = append(blocks, b)
			}
		}
	}
	return blocks
}

func blockTexts(blocks []*model.Block) []string {
	return testutil.BlockTexts(blocks)
}

func blockNames(blocks []*model.Block) []string {
	names := make([]string, len(blocks))
	for i, b := range blocks {
		names[i] = b.Name
	}
	return names
}

func findBlockContainingText(blocks []*model.Block, substr string) *model.Block {
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), substr) {
			return b
		}
	}
	return nil
}

func snippetRoundtrip(t *testing.T, snippet string, params map[string]any) string {
	t.Helper()
	ctx := t.Context()
	reader := jsonfmt.NewReader()
	if params != nil {
		require.NoError(t, reader.Config().ApplyMap(params))
	}
	err := reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := jsonfmt.NewWriter()
	if params != nil {
		if v, ok := params["escapeForwardSlashes"]; ok {
			if b, ok := v.(bool); ok {
				writer.Config().EscapeForwardSlashes = b
			}
		}
	}
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	return buf.String()
}

// --- Extraction Tests (from JSONFilterTest.java) ---

// okapi: JSONFilterTest#testValue
func TestSnippets_Value(t *testing.T) {
	t.Parallel()
	parts := readJSON(t, `{"key" : "Text1"}`)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text1")
	assert.Equal(t, "key", blocks[0].Name)
}

// okapi: JSONFilterTest#testObject
func TestSnippets_Object(t *testing.T) {
	t.Parallel()
	parts := readJSON(t, `{"key" : { "key2" : "Text1" } }`)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text1")
}

// okapi: JSONFilterTest#testList
func TestSnippets_List(t *testing.T) {
	t.Parallel()
	// extractIsolatedStrings defaults to false, so array strings are not extracted.
	// But objects within arrays should extract their key-value pairs.
	parts := readJSON(t, `{"items": [{"key": "Text1"}]}`)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text1")
}

// okapi: JSONFilterTest#testAllWithKeyNoException
func TestSnippets_AllWithKeyNoException(t *testing.T) {
	t.Parallel()
	parts := readJSONWith(t, `{"key1": "Text1", "key2": "Text2"}`, map[string]any{
		"extractAllPairs": true,
		"useKeyAsName":    true,
	})

	blocks := translatableBlocks(parts)
	require.Len(t, blocks, 2)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text1")
	assert.Contains(t, texts, "Text2")

	names := blockNames(blocks)
	assert.Contains(t, names, "key1")
	assert.Contains(t, names, "key2")
}

// okapi: JSONFilterTest#testAllWithKeyWithException
func TestSnippets_AllWithKeyWithException(t *testing.T) {
	t.Parallel()
	parts := readJSONWith(t, `{"key1": "Text1", "key2": "Text2", "not_this": "Text3"}`, map[string]any{
		"extractAllPairs": true,
		"exceptions":      "not_this",
	})

	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text1")
	assert.Contains(t, texts, "Text2")
	assert.NotContains(t, texts, "Text3")
}

// okapi: JSONFilterTest#testNoneWithKeywithException
func TestSnippets_NoneWithKeywithException(t *testing.T) {
	t.Parallel()
	parts := readJSONWith(t, `{"key1": "Text1", "key2": "Text2", "extract_me": "Text3"}`, map[string]any{
		"extractAllPairs": false,
		"exceptions":      "extract_me",
	})

	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text3")
	assert.NotContains(t, texts, "Text1")
	assert.NotContains(t, texts, "Text2")
}

// okapi: JSONFilterTest#testPath
func TestSnippets_Path(t *testing.T) {
	t.Parallel()
	parts := readJSONWith(t, `{"parent": {"child": "Text1"}}`, map[string]any{
		"useFullKeyPath":           true,
		"useKeyAsName":             true,
		"useLeadingSlashOnKeyPath": true,
	})

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text1")

	names := blockNames(blocks)
	assert.Contains(t, names, "/parent/child")
}

// okapi: JSONFilterTest#testLeadingSlash
func TestSnippets_LeadingSlash(t *testing.T) {
	t.Parallel()
	parts := readJSONWith(t, `{"parent": {"child": "Text1"}}`, map[string]any{
		"useFullKeyPath":           true,
		"useKeyAsName":             true,
		"useLeadingSlashOnKeyPath": false,
	})

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	names := blockNames(blocks)
	assert.Contains(t, names, "parent/child")
}

// okapi: JSONFilterTest#testStandaloneYes
func TestSnippets_StandaloneYes(t *testing.T) {
	t.Parallel()
	parts := readJSONWith(t, `{"colors": ["Red", "Green", "Blue"]}`, map[string]any{
		"extractIsolatedStrings": true,
	})

	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Red")
	assert.Contains(t, texts, "Green")
	assert.Contains(t, texts, "Blue")
}

// okapi: JSONFilterTest#testStandaloneDefaultWhichIsNo
func TestSnippets_StandaloneDefaultWhichIsNo(t *testing.T) {
	t.Parallel()
	parts := readJSON(t, `{"colors": ["Red", "Green", "Blue"], "label": "Color list"}`)

	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Color list")
	assert.NotContains(t, texts, "Red")
}

// okapi: JSONFilterTest#testEscape
func TestSnippets_Escape(t *testing.T) {
	t.Parallel()
	parts := readJSON(t, `{"key": "\u00E0"}`)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "\u00E0", blocks[0].SourceText()) // à
}

// okapi: JSONFilterTest#testEscapes
func TestSnippets_Escapes(t *testing.T) {
	t.Parallel()
	parts := readJSON(t, `{"key": "a\nb\tc\\d\"e\/f"}`)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "a\nb")
	assert.Contains(t, text, "b\tc")
	assert.Contains(t, text, "c\\d")
	assert.Contains(t, text, "d\"e")
	assert.Contains(t, text, "e/f")
}

// okapi: JSONFilterTest#testEscapedForwardSlashDecoding
func TestSnippets_EscapedForwardSlashDecoding(t *testing.T) {
	t.Parallel()
	parts := readJSON(t, `{"key": "a\/b"}`)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "a/b", blocks[0].SourceText())
}

// okapi: JSONFilterTest#testEscapeForwardSlashes
func TestSnippets_EscapeForwardSlashes(t *testing.T) {
	t.Parallel()
	// Default: escapeForwardSlashes=true, forward slashes are escaped in output.
	output := snippetRoundtrip(t, `{"key": "a/b"}`, nil)
	assert.Contains(t, output, `a\/b`)
}

// okapi: JSONFilterTest#testNoEscapeForwardSlashes
func TestSnippets_NoEscapeForwardSlashes(t *testing.T) {
	t.Parallel()
	output := snippetRoundtrip(t, `{"key": "a/b"}`, map[string]any{
		"escapeForwardSlashes": false,
	})
	assert.Contains(t, output, `a/b`)
	assert.NotContains(t, output, `a\/b`)
}

// okapi: JSONFilterTest#testEmptyValue
func TestSnippets_EmptyValue(t *testing.T) {
	t.Parallel()
	output := snippetRoundtrip(t, `{"key": ""}`, nil)
	assert.Contains(t, output, `""`)
}

// okapi: JSONFilterTest#testDecimalNumber
func TestSnippets_DecimalNumber(t *testing.T) {
	t.Parallel()
	parts := readJSON(t, `{"name": "John", "score": 9.5}`)

	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "John")
	for _, text := range texts {
		assert.NotEqual(t, "9.5", text, "decimal numbers should not be extracted")
	}
}

// okapi: JSONFilterTest#testSimpleEntrySkeleton
func TestSnippets_SimpleEntrySkeleton(t *testing.T) {
	t.Parallel()
	input := "{\n  \"key\" : \"value\"\n}"
	output := snippetRoundtrip(t, input, nil)
	// Should preserve the general structure and whitespace
	assert.Contains(t, output, "key")
	assert.Contains(t, output, "value")
	assert.Contains(t, output, "\n")
}

// okapi: JSONFilterTest#testLineBreaks
func TestSnippets_LineBreaks(t *testing.T) {
	t.Parallel()
	input := "{\r\"key\" : \"value\"\r}"
	output := snippetRoundtrip(t, input, nil)
	assert.Contains(t, output, "value")
}

// okapi: JSONFilterTest#testWhiteSpaceAndComments
func TestSnippets_WhiteSpaceAndComments(t *testing.T) {
	t.Parallel()
	input := `{
  /* block comment */
  // line comment
  # hash comment
  "key": "Text1"
}`
	parts := readJSON(t, input)
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text1")
}

// okapi: JSONFilterTest#testMultilineComment
func TestSnippets_MultilineComment(t *testing.T) {
	t.Parallel()
	input := `{
  /* this is
     a multiline
     comment */
  "key": "Text1"
}`
	parts := readJSON(t, input)
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text1")
}

// okapi: JSONFilterTest#testNestedComments
func TestSnippets_NestedComments(t *testing.T) {
	t.Parallel()
	input := `{
  /* outer /* nested */ comment */
  "key": "Text1"
}`
	parts := readJSON(t, input)
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text1")
}

// okapi: JSONFilterTest#testNoteRules
func TestSnippets_NoteRules(t *testing.T) {
	t.Parallel()
	input := `{"note": "This is a note", "key": "Text1"}`
	parts := readJSONWith(t, input, map[string]any{
		"extractAllPairs": true,
		"noteRules":       "note",
	})

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text1")
	assert.NotContains(t, texts, "This is a note")

	// The note should be attached as an annotation
	b := findBlockContainingText(blocks, "Text1")
	require.NotNil(t, b)
	note, ok := b.Anno("note")
	require.True(t, ok, "should have a note annotation")
	noteAnno, ok := note.(*model.NoteAnnotation)
	require.True(t, ok)
	assert.Equal(t, "This is a note", noteAnno.Text)
}

// okapi: JSONFilterTest#testIdRules
func TestSnippets_IdRules(t *testing.T) {
	t.Parallel()
	input := `{"id": "my-id", "key": "Text1"}`
	parts := readJSONWith(t, input, map[string]any{
		"extractAllPairs": true,
		"idRules":         "id",
	})

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := findBlockContainingText(blocks, "Text1")
	require.NotNil(t, b, "should have block with Text1")
	assert.Equal(t, "my-id", b.Name)
}

// okapi: JSONFilterTest#testNestedIdRules
func TestSnippets_NestedIdRules(t *testing.T) {
	t.Parallel()
	input := `{"items": [{"id": "item1", "text": "Text1"}, {"id": "item2", "text": "Text2"}]}`
	parts := readJSONWith(t, input, map[string]any{
		"extractAllPairs": true,
		"idRules":         "id",
		"useIdStack":      true,
	})

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text1")
	assert.Contains(t, texts, "Text2")
}

// okapi: JSONFilterTest#testGenericMetaRules
func TestSnippets_GenericMetaRules(t *testing.T) {
	t.Parallel()
	input := `{"meta": "metadata-value", "key": "Text1"}`
	parts := readJSONWith(t, input, map[string]any{
		"extractAllPairs":  true,
		"genericMetaRules": "meta",
	})

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text1")
	assert.NotContains(t, texts, "metadata-value")

	// Meta value should be stored as a property
	b := findBlockContainingText(blocks, "Text1")
	require.NotNil(t, b)
	assert.Equal(t, "metadata-value", b.Properties["meta"])
}

// okapi: JSONFilterTest#testGenericMetaRulesWithId
func TestSnippets_GenericMetaRulesWithId(t *testing.T) {
	t.Parallel()
	input := `{"id": "my-id", "meta": "metadata-value", "key": "Text1"}`
	parts := readJSONWith(t, input, map[string]any{
		"extractAllPairs":  true,
		"idRules":          "id",
		"genericMetaRules": "meta",
	})

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := findBlockContainingText(blocks, "Text1")
	require.NotNil(t, b)
	assert.Equal(t, "my-id", b.Name)
}

// okapi: JSONFilterTest#testExtractionRules
func TestSnippets_ExtractionRules(t *testing.T) {
	t.Parallel()
	input := `{"title": "Extract me", "body": "Extract me too", "id": "Skip me"}`
	parts := readJSONWith(t, input, map[string]any{
		"extractAllPairs": false,
		"extractionRules": "title|body",
	})

	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Extract me")
	assert.Contains(t, texts, "Extract me too")
	assert.NotContains(t, texts, "Skip me")
}

// okapi: JSONFilterTest#testMaxwidthRules
func TestSnippets_MaxwidthRules(t *testing.T) {
	t.Parallel()
	input := `{"maxwidth": 100, "title": "Text1"}`
	parts := readJSONWith(t, input, map[string]any{
		"extractAllPairs": false,
		"exceptions":      "title",
		"maxwidthRules":   "maxwidth",
	})

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text1")

	// Max width should be set as a property
	b := findBlockContainingText(blocks, "Text1")
	require.NotNil(t, b)
	assert.Equal(t, "100", b.Properties["maxwidth"])
}

// okapi: JSONFilterTest#testMaxwidthRulesWithSizeChar
func TestSnippets_MaxwidthRulesWithSizeChar(t *testing.T) {
	t.Parallel()
	input := `{"maxwidth": 50, "title": "Text1"}`
	parts := readJSONWith(t, input, map[string]any{
		"extractAllPairs":  false,
		"exceptions":       "title",
		"maxwidthRules":    "maxwidth",
		"maxwidthSizeUnit": "char",
	})

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text1")

	b := findBlockContainingText(blocks, "Text1")
	require.NotNil(t, b)
	assert.Equal(t, "50", b.Properties["maxwidth"])
	assert.Equal(t, "char", b.Properties["maxwidthSizeUnit"])
}

// okapi: JSONFilterTest#testArrayWithinArray
func TestSnippets_ArrayWithinArray(t *testing.T) {
	t.Parallel()
	input := `{"data": [["Text1"]]}`
	parts := readJSONWith(t, input, map[string]any{
		"extractIsolatedStrings":   true,
		"useFullKeyPath":           true,
		"useKeyAsName":             true,
		"useLeadingSlashOnKeyPath": true,
	})

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text1")
}

// okapi: JSONFilterTest#testArrayWithinArrayWithinArray
func TestSnippets_ArrayWithinArrayWithinArray(t *testing.T) {
	t.Parallel()
	input := `{"data": [[["Text1"]]]}`
	parts := readJSONWith(t, input, map[string]any{
		"extractIsolatedStrings":   true,
		"useFullKeyPath":           true,
		"useKeyAsName":             true,
		"useLeadingSlashOnKeyPath": true,
	})

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text1")
}

// okapi: JSONFilterTest#testArrayWithObject
func TestSnippets_ArrayWithObject(t *testing.T) {
	t.Parallel()
	input := `{"items": [{"name": "Text1"}]}`
	parts := readJSONWith(t, input, map[string]any{
		"useFullKeyPath":           true,
		"useKeyAsName":             true,
		"useLeadingSlashOnKeyPath": true,
	})

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text1")
}

// okapi: JSONFilterTest#testNamedArray
func TestSnippets_NamedArray(t *testing.T) {
	t.Parallel()
	input := `{"items": [{"name": "Text1"}, {"name": "Text2"}]}`
	parts := readJSONWith(t, input, map[string]any{
		"useFullKeyPath":           true,
		"useKeyAsName":             true,
		"useLeadingSlashOnKeyPath": true,
	})

	blocks := translatableBlocks(parts)
	require.Len(t, blocks, 2)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text1")
	assert.Contains(t, texts, "Text2")
}

// okapi: JSONFilterTest#testInlineCodeFinderEscaping
func TestSnippets_InlineCodeFinderEscaping(t *testing.T) {
	t.Parallel()
	input := `{"key": "Hello <b>world</b> end"}`
	parts := readJSONWith(t, input, map[string]any{
		"useCodeFinder": true,
		"codeFinderRules": map[string]any{
			"count": 1,
			"rule0": `</?([A-Z0-9a-z]*)\b[^>]*>`,
		},
	})

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Hello")
	assert.Contains(t, text, "world")
}

// okapi: JSONFilterTest#testInlineCodeFinderNewLineCharacter
func TestSnippets_InlineCodeFinderNewLineCharacter(t *testing.T) {
	t.Parallel()
	input := `{"key": "Line1\nLine2"}`
	parts := readJSONWith(t, input, map[string]any{
		"useCodeFinder": true,
		"codeFinderRules": map[string]any{
			"count": 1,
			"rule0": `\n`,
		},
	})

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Line1")
	assert.Contains(t, text, "Line2")
}

// okapi: JSONFilterTest#testSmartQuotes
func TestSnippets_SmartQuotes(t *testing.T) {
	t.Parallel()
	parts := readJSON(t, `{"key": "\u201CHello\u201D"}`)
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Hello")
	assert.Contains(t, text, "\u201C")
	assert.Contains(t, text, "\u201D")
}

// --- Roundtrip Tests ---

// okapi: JSONFilterTest#testDoubleExtraction
// The extract→write→re-extract idempotency asserted below is the same
// contract Okapi's integration-test suite enforces over its JSON file
// corpus and gold XLIFF:
// okapi: RoundTripJsonIT#jsonFiles
// okapi: JsonXliffCompareIT#jsonXliffCompareFiles
// okapi-skip: RoundTripJsonIT#jsonSerializedFiles — Okapi serialized-skeleton roundtrip variant; native uses its own skeleton store (no serialized-skeleton mode)
func TestSnippets_DoubleExtraction(t *testing.T) {
	t.Parallel()
	// Double extraction: read → write → re-read → compare block texts.
	inputs := []struct {
		name  string
		input string
	}{
		{"simple", `{"key": "value"}`},
		{"nested", `{"a": {"b": "value"}}`},
		{"multiple", `{"k1": "v1", "k2": "v2", "k3": "v3"}`},
		{"numbers", `{"name": "Test", "count": 42}`},
		{"mixed", `{"title": "Hello", "items": [{"text": "World"}]}`},
	}

	for _, tt := range inputs {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()

			// First extraction
			reader1 := jsonfmt.NewReader()
			require.NoError(t, reader1.Open(ctx, testutil.RawDocFromString(tt.input, model.LocaleEnglish)))
			parts1 := testutil.CollectParts(t, reader1.Read(ctx))
			reader1.Close()

			blocks1 := testutil.FilterBlocks(parts1)

			// Write back
			var buf bytes.Buffer
			writer := jsonfmt.NewWriter()
			require.NoError(t, writer.SetOutputWriter(&buf))
			ch := testutil.PartsToChannel(parts1)
			require.NoError(t, writer.Write(ctx, ch))
			writer.Close()

			// Second extraction
			reader2 := jsonfmt.NewReader()
			require.NoError(t, reader2.Open(ctx, testutil.RawDocFromString(buf.String(), model.LocaleEnglish)))
			parts2 := testutil.CollectParts(t, reader2.Read(ctx))
			reader2.Close()

			blocks2 := testutil.FilterBlocks(parts2)

			// Compare: same number of blocks, same texts
			require.Len(t, blocks2, len(blocks1), "block count mismatch on double extraction")
			texts1 := testutil.BlockTexts(blocks1)
			texts2 := testutil.BlockTexts(blocks2)
			for _, t1 := range texts1 {
				assert.Contains(t, texts2, t1, "double extraction lost text: %s", t1)
			}
		})
	}
}

// --- Subfilter Tests (require real HTML reader) ---
// These tests use the pattern-based subfilter config.
// For full subfilter tests with the real HTML reader, see subfilter_test.go.

// okapi: JSONFilterTest#testSubfiltersProduceDistinctTextUnitIds
func TestSnippets_SubfiltersProduceDistinctTextUnitIds(t *testing.T) {
	t.Parallel()
	// Without a real subfilter resolver, we test that blocks from different
	// keys get distinct IDs even without subfiltering.
	parts := readJSON(t, `{"key1": "Text1", "key2": "Text2"}`)

	blocks := translatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.False(t, ids[b.ID], "duplicate block ID: %s", b.ID)
		ids[b.ID] = true
	}
}

// --- SubfilterRules Tests ---

// okapi: JSONFilterTest#testSubfilterRules
func TestSnippets_SubfilterRules(t *testing.T) {
	t.Parallel()
	// Without a real HTML resolver, verify that subfilterRules config is accepted
	// and non-matching keys are still extracted as plain blocks.
	input := `{"html_key": "<b>Bold text</b>", "plain_key": "Plain text"}`
	parts := readJSONWith(t, input, map[string]any{
		"subfilterRules": "html_key",
		"subfilter":      "html",
	})

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Without a resolver, both keys should be extracted as plain blocks.
	// The subfilterRules config is just stored — it needs a resolver to apply.
	var foundPlain bool
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "Plain text") {
			foundPlain = true
		}
	}
	assert.True(t, foundPlain, "plain_key should still be extracted")
}

// --- Comment preservation in roundtrip ---

func TestSnippets_CommentPreservationRoundtrip(t *testing.T) {
	t.Parallel()
	input := `{
  // This is a comment
  "key": "value"
}`
	output := snippetRoundtrip(t, input, nil)
	assert.Contains(t, output, "// This is a comment")
	assert.Contains(t, output, "value")
}

func TestSnippets_BlockCommentPreservationRoundtrip(t *testing.T) {
	t.Parallel()
	input := `{
  /* Block comment */
  "key": "value"
}`
	output := snippetRoundtrip(t, input, nil)
	assert.Contains(t, output, "/* Block comment */")
	assert.Contains(t, output, "value")
}

// --- Whitespace preservation ---

func TestSnippets_WhitespacePreservation(t *testing.T) {
	t.Parallel()
	input := "{\n    \"key\" :   \"value\"\n}"
	output := snippetRoundtrip(t, input, nil)
	// Should preserve original whitespace structure
	assert.Contains(t, output, "    \"key\" :   ")
}

// --- Extraction with full key path ---

func TestSnippets_ExtractionRulesWithFullPath(t *testing.T) {
	t.Parallel()
	input := `{"widgets": [{"body": "Extract this", "id": "123"}]}`
	parts := readJSONWith(t, input, map[string]any{
		"extractAllPairs":          false,
		"useFullKeyPath":           true,
		"useLeadingSlashOnKeyPath": true,
		"extractionRules":          `/widgets.*body`,
	})

	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Extract this")
	assert.NotContains(t, texts, "123")
}

// --- Config validation ---

func TestSnippets_ConfigDefaults(t *testing.T) {
	t.Parallel()
	reader := jsonfmt.NewReader()
	cfg := reader.Config().(*jsonfmt.Config)
	assert.True(t, cfg.ExtractAllPairs)
	assert.False(t, cfg.ExtractIsolatedStrings)
	assert.True(t, cfg.UseKeyAsName)
	assert.False(t, cfg.UseFullKeyPath)
	assert.True(t, cfg.UseLeadingSlashOnKeyPath)
	assert.True(t, cfg.EscapeForwardSlashes)
}

// okapi: JSONFilterTest#testDefaultInfo
// Java-specific: tests filter metadata (name, display name, configurations)
func TestSnippets_DefaultInfo(t *testing.T) {
	t.Parallel()
	reader := jsonfmt.NewReader()
	assert.Equal(t, "json", reader.Name())
	assert.Equal(t, "JSON", reader.DisplayName())
}

// --- Variable max width from file ---

// okapi: JSONFilterTest#testVariableMaxWidthInNestedObjects
func TestSnippets_VariableMaxWidthInNestedObjects(t *testing.T) {
	t.Parallel()
	input := `{
  "sections": [
    {"maxchars": 100, "title": "This text has a maxwidth of 100"},
    {"title": "This text has no maxwidth"},
    {"maxchars": 600, "title": "This text has a maxwidth of 600"}
  ]
}`
	parts := readJSONWith(t, input, map[string]any{
		"extractAllPairs":  false,
		"exceptions":       "title",
		"useKeyAsName":     true,
		"maxwidthRules":    "maxchars",
		"maxwidthSizeUnit": "char",
	})

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "This text has no maxwidth")
	assert.Contains(t, texts, "This text has a maxwidth of 100")
	assert.Contains(t, texts, "This text has a maxwidth of 600")

	// Check maxwidth properties
	b100 := findBlockContainingText(blocks, "maxwidth of 100")
	require.NotNil(t, b100)
	assert.Equal(t, "100", b100.Properties["maxwidth"])
	assert.Equal(t, "char", b100.Properties["maxwidthSizeUnit"])

	bNone := findBlockContainingText(blocks, "no maxwidth")
	require.NotNil(t, bNone)
	_, hasMaxwidth := bNone.Properties["maxwidth"]
	assert.False(t, hasMaxwidth, "block without maxchars should not have maxwidth property")

	b600 := findBlockContainingText(blocks, "maxwidth of 600")
	require.NotNil(t, b600)
	assert.Equal(t, "600", b600.Properties["maxwidth"])
}

// --- Metadata with nested notes ---

// okapi: JSONFilterTest#metaDataAndExtractionRulesNestedNotes
func TestSnippets_MetaDataAndExtractionRulesNestedNotes(t *testing.T) {
	t.Parallel()
	input := `{
  "widgets": [
    {
      "id": "115013866768",
      "name": {"prefix": "The Year of the ", "animal": "Tiger"},
      "image": "tiger.jpg",
      "body": "This is the blurb about tigers"
    }
  ]
}`
	parts := readJSONWith(t, input, map[string]any{
		"extractAllPairs":          false,
		"useFullKeyPath":           true,
		"useLeadingSlashOnKeyPath": true,
		"extractionRules":          `/widgets.*body`,
		"noteRules":                `/widgets.*name`,
		"idRules":                  `/widgets.*id`,
		"genericMetaRules":         `/widgets.*image`,
	})

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Body value should be extracted
	var foundBody bool
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "blurb") {
			foundBody = true
			break
		}
	}
	assert.True(t, foundBody, "body value should be extracted")

	// Name values should be notes, not translatable text
	texts := blockTexts(blocks)
	for _, text := range texts {
		assert.NotEqual(t, "Tiger", text, "nested name values should be notes")
	}
}

// --- Metadata with extraction rules and subfilter ---

// okapi: JSONFilterTest#metaDataAndExtractionRulesWithSubfilter
func TestSnippets_MetaDataAndExtractionRulesWithSubfilter(t *testing.T) {
	t.Parallel()
	input := `{
  "widgets": [
    {
      "id": "115013866768",
      "name": "The Year of the Tiger",
      "image": "tiger.jpg",
      "body": "This is the blurb about tigers"
    }
  ]
}`
	parts := readJSONWith(t, input, map[string]any{
		"extractAllPairs":          false,
		"useFullKeyPath":           true,
		"useLeadingSlashOnKeyPath": true,
		"extractionRules":          `/widgets.*body`,
		"noteRules":                `/widgets.*name`,
		"idRules":                  `/widgets.*id`,
		"genericMetaRules":         `/widgets.*image`,
	})

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Body values should be extracted
	var foundBody bool
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "blurb") {
			foundBody = true
			break
		}
	}
	assert.True(t, foundBody, "body values should be extracted via extractionRules")

	// Name keys should NOT be extracted as translatable
	texts := blockTexts(blocks)
	for _, text := range texts {
		assert.NotEqual(t, "The Year of the Tiger", text, "name values should be notes, not translatable text")
	}

	// ID keys should not be translatable text
	for _, text := range texts {
		assert.NotEqual(t, "115013866768", text, "id values should be used as TU names, not translatable text")
	}
}

// okapi: JSONFilterTest#testSubfilterEasyToDebug
func TestSnippets_SubfilterEasyToDebug(t *testing.T) {
	t.Parallel()
	// Simplified subfilter test: verify that the reader handles a simple
	// HTML-like value without a subfilter resolver (treated as plain text).
	input := `{"key": "<b>bold</b>"}`
	parts := readJSON(t, input)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "bold")
}

// okapi: JSONFilterTest#testDoubleExtractionOnPreviousFailure
func TestSnippets_DoubleExtractionOnPreviousFailure(t *testing.T) {
	t.Parallel()
	// Tests double extraction on a customer-form-like JSON structure
	// that caused failures in earlier Okapi versions.
	input := `{
  "form": {
    "fields": [
      {"label": "First Name", "type": "text", "required": true},
      {"label": "Last Name", "type": "text", "required": true},
      {"label": "Email", "type": "email", "required": false}
    ],
    "submit": "Submit Form"
  }
}`
	ctx := t.Context()

	// First extraction
	reader1 := jsonfmt.NewReader()
	require.NoError(t, reader1.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts1 := testutil.CollectParts(t, reader1.Read(ctx))
	reader1.Close()

	blocks1 := testutil.FilterBlocks(parts1)

	// Write back
	var buf bytes.Buffer
	writer := jsonfmt.NewWriter()
	require.NoError(t, writer.SetOutputWriter(&buf))
	ch := testutil.PartsToChannel(parts1)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	// Second extraction
	reader2 := jsonfmt.NewReader()
	require.NoError(t, reader2.Open(ctx, testutil.RawDocFromString(buf.String(), model.LocaleEnglish)))
	parts2 := testutil.CollectParts(t, reader2.Read(ctx))
	reader2.Close()

	blocks2 := testutil.FilterBlocks(parts2)

	// Compare: same number of blocks, same texts
	require.Len(t, blocks2, len(blocks1), "block count mismatch on double extraction")
	texts1 := testutil.BlockTexts(blocks1)
	texts2 := testutil.BlockTexts(blocks2)
	for _, t1 := range texts1 {
		assert.Contains(t, texts2, t1, "double extraction lost text: %s", t1)
	}
}

// okapi: JSONFilterTest#testDoubleExtractionOnInvalid
func TestSnippets_DoubleExtractionOnInvalid(t *testing.T) {
	t.Parallel()
	// Tests double extraction on relaxed JSON with comments.
	// The native reader supports comments as relaxed JSON.
	input := `{
  // This is a comment
  "key": "value",
  /* Block comment */
  "key2": "value2"
}`
	ctx := t.Context()

	// First extraction
	reader1 := jsonfmt.NewReader()
	require.NoError(t, reader1.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts1 := testutil.CollectParts(t, reader1.Read(ctx))
	reader1.Close()

	blocks1 := testutil.FilterBlocks(parts1)

	// Write back
	var buf bytes.Buffer
	writer := jsonfmt.NewWriter()
	require.NoError(t, writer.SetOutputWriter(&buf))
	ch := testutil.PartsToChannel(parts1)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	// Second extraction
	reader2 := jsonfmt.NewReader()
	require.NoError(t, reader2.Open(ctx, testutil.RawDocFromString(buf.String(), model.LocaleEnglish)))
	parts2 := testutil.CollectParts(t, reader2.Read(ctx))
	reader2.Close()

	blocks2 := testutil.FilterBlocks(parts2)

	// Compare
	require.Len(t, blocks2, len(blocks1), "block count mismatch on double extraction")
	texts1 := testutil.BlockTexts(blocks1)
	texts2 := testutil.BlockTexts(blocks2)
	for _, t1 := range texts1 {
		assert.Contains(t, texts2, t1, "double extraction lost text: %s", t1)
	}
}

// okapi: JSONFilterTest#testEscapeForwardSlashesSubfilter
func TestSnippets_EscapeForwardSlashesSubfilter(t *testing.T) {
	t.Parallel()
	// Forward slash escaping behavior applies to subfiltered content too.
	// Without a real HTML subfilter, we verify the escaping works on
	// content that contains forward slashes and HTML-like tags.
	input := `{"key": "<a href=\"http://example.com\">link</a>"}`
	output := snippetRoundtrip(t, input, nil)
	// With default escapeForwardSlashes=true, forward slashes should be escaped
	assert.Contains(t, output, `http:\/\/example.com`)
	assert.Contains(t, output, "link")
}

// okapi-unmapped: JsonSnippetParserTest#testSingleObject — Java-specific: tests internal parser tokenization, not filter behavior

// --- Exact skeleton roundtrip tests ---

// TestSnippets_ExactRoundtrip_Simple verifies byte-exact roundtrip for simple JSON.
func TestSnippets_ExactRoundtrip_Simple(t *testing.T) {
	t.Parallel()
	input := `{"key": "value"}`
	output := snippetRoundtrip(t, input, nil)
	assert.Equal(t, input, output)
}

// TestSnippets_ExactRoundtrip_Whitespace verifies whitespace is preserved exactly.
func TestSnippets_ExactRoundtrip_Whitespace(t *testing.T) {
	t.Parallel()
	input := "{\n    \"key\" :   \"value\"\n}"
	output := snippetRoundtrip(t, input, nil)
	assert.Equal(t, input, output)
}

// TestSnippets_ExactRoundtrip_Comments verifies comments survive roundtrip.
func TestSnippets_ExactRoundtrip_Comments(t *testing.T) {
	t.Parallel()
	input := "{\n  // line comment\n  \"key\": \"value\"\n}"
	output := snippetRoundtrip(t, input, nil)
	assert.Equal(t, input, output)
}

// TestSnippets_ExactRoundtrip_BlockComment verifies block comments survive roundtrip.
func TestSnippets_ExactRoundtrip_BlockComment(t *testing.T) {
	t.Parallel()
	input := "{\n  /* block */\n  \"key\": \"value\"\n}"
	output := snippetRoundtrip(t, input, nil)
	assert.Equal(t, input, output)
}

// TestSnippets_ExactRoundtrip_UnicodeEscapes verifies unicode escapes in
// non-translatable values are preserved exactly.
func TestSnippets_ExactRoundtrip_UnicodeEscapes(t *testing.T) {
	t.Parallel()
	input := `{"id": "\u0041\u0042", "text": "Hello"}`
	output := snippetRoundtrip(t, input, map[string]any{
		"extractAllPairs": true,
		"idRules":         "id",
	})
	// The id value should be preserved exactly (raw bytes), not re-encoded
	assert.Contains(t, output, `"\u0041\u0042"`)
	assert.Contains(t, output, `"Hello"`)
}

// TestSnippets_ExactRoundtrip_EscapedSlash verifies escaped forward slashes
// in non-translatable values are preserved.
func TestSnippets_ExactRoundtrip_EscapedSlash(t *testing.T) {
	t.Parallel()
	input := `{"note": "see a\/b", "text": "Hello"}`
	output := snippetRoundtrip(t, input, map[string]any{
		"extractAllPairs": true,
		"noteRules":       "note",
	})
	// Note value should be preserved exactly
	assert.Contains(t, output, `"see a\/b"`)
}

// TestSnippets_ExactRoundtrip_Nested verifies nested structures roundtrip exactly.
func TestSnippets_ExactRoundtrip_Nested(t *testing.T) {
	t.Parallel()
	input := `{"parent": {"child": "value"}, "other": "text"}`
	output := snippetRoundtrip(t, input, nil)
	assert.Equal(t, input, output)
}

// TestSnippets_ExactRoundtrip_Array verifies arrays roundtrip exactly.
func TestSnippets_ExactRoundtrip_Array(t *testing.T) {
	t.Parallel()
	input := `{"items": ["a", "b", "c"]}`
	output := snippetRoundtrip(t, input, map[string]any{
		"extractIsolatedStrings": true,
	})
	assert.Equal(t, input, output)
}

// TestSnippets_ExactRoundtrip_NonStringValues verifies numbers, bools, nulls
// are preserved exactly.
func TestSnippets_ExactRoundtrip_NonStringValues(t *testing.T) {
	t.Parallel()
	input := `{"name": "Test", "count": 42, "rate": 3.14, "active": true, "deleted": false, "meta": null}`
	output := snippetRoundtrip(t, input, nil)
	assert.Equal(t, input, output)
}

// TestSnippets_ExactRoundtrip_MultilineFormatted verifies pretty-printed JSON roundtrips.
func TestSnippets_ExactRoundtrip_MultilineFormatted(t *testing.T) {
	t.Parallel()
	input := `{
  "title": "Hello",
  "nested": {
    "key": "World"
  },
  "count": 5
}`
	output := snippetRoundtrip(t, input, nil)
	assert.Equal(t, input, output)
}

// TestSnippets_ExactRoundtrip_TrailingNewline verifies trailing whitespace is preserved.
func TestSnippets_ExactRoundtrip_TrailingNewline(t *testing.T) {
	t.Parallel()
	input := "{\"key\": \"value\"}\n"
	output := snippetRoundtrip(t, input, nil)
	assert.Equal(t, input, output)
}

// TestSnippets_ExactRoundtrip_EmptyObject verifies empty objects roundtrip.
func TestSnippets_ExactRoundtrip_EmptyObject(t *testing.T) {
	t.Parallel()
	input := `{"data": {}, "text": "Hello"}`
	output := snippetRoundtrip(t, input, nil)
	assert.Equal(t, input, output)
}

// TestSnippets_ExactRoundtrip_MixedComments verifies all comment styles survive.
func TestSnippets_ExactRoundtrip_MixedComments(t *testing.T) {
	t.Parallel()
	input := `{
  // line comment
  /* block comment */
  # hash comment
  "key": "value"
}`
	output := snippetRoundtrip(t, input, nil)
	assert.Equal(t, input, output)
}

// --- Skeleton roundtrip with translations ---

// TestSnippets_SkeletonTranslation_UseFullKeyPath verifies translations work
// when UseFullKeyPath changes the block name.
func TestSnippets_SkeletonTranslation_UseFullKeyPath(t *testing.T) {
	t.Parallel()
	input := `{"parent": {"child": "Hello"}}`
	ctx := t.Context()

	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"useFullKeyPath":           true,
		"useKeyAsName":             true,
		"useLeadingSlashOnKeyPath": true,
	}))
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set translation
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Hello" {
				assert.Equal(t, "/parent/child", block.Name)
				block.SetTargetText(model.LocaleFrench, "Bonjour")
			}
		}
	}

	var buf bytes.Buffer
	writer := jsonfmt.NewWriter()
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale(model.LocaleFrench)
	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Contains(t, buf.String(), `"Bonjour"`)
	assert.NotContains(t, buf.String(), `"Hello"`)
}

// TestSnippets_SkeletonTranslation_IdRules verifies translations work
// when idRules changes the block name to a custom ID.
func TestSnippets_SkeletonTranslation_IdRules(t *testing.T) {
	t.Parallel()
	input := `{"id": "msg-1", "text": "Hello"}`
	ctx := t.Context()

	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"extractAllPairs": true,
		"idRules":         "id",
	}))
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set translation
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Hello" {
				assert.Equal(t, "msg-1", block.Name)
				block.SetTargetText(model.LocaleGerman, "Hallo")
			}
		}
	}

	var buf bytes.Buffer
	writer := jsonfmt.NewWriter()
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale(model.LocaleGerman)
	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Contains(t, buf.String(), `"Hallo"`)
	assert.NotContains(t, buf.String(), `"Hello"`)
	// ID value should be preserved
	assert.Contains(t, buf.String(), `"msg-1"`)
}

// TestSnippets_SkeletonTranslation_Exceptions verifies translations work
// when exceptions exclude some keys.
func TestSnippets_SkeletonTranslation_Exceptions(t *testing.T) {
	t.Parallel()
	input := `{"title": "Hello", "internal_id": "abc123"}`
	ctx := t.Context()

	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"extractAllPairs": true,
		"exceptions":      "internal_id",
	}))
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set translation for the extracted block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			block.SetTargetText(model.LocaleFrench, "Bonjour")
		}
	}

	var buf bytes.Buffer
	writer := jsonfmt.NewWriter()
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale(model.LocaleFrench)
	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Contains(t, buf.String(), `"Bonjour"`)
	// Non-extracted value should be preserved exactly
	assert.Contains(t, buf.String(), `"abc123"`)
}

// TestSnippets_SkeletonTranslation_NoteRules verifies note values are
// preserved while translatable values get translated.
func TestSnippets_SkeletonTranslation_NoteRules(t *testing.T) {
	t.Parallel()
	input := `{"note": "translator hint", "text": "Hello"}`
	ctx := t.Context()

	reader := jsonfmt.NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"extractAllPairs": true,
		"noteRules":       "note",
	}))
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			block.SetTargetText(model.LocaleSpanish, "Hola")
		}
	}

	var buf bytes.Buffer
	writer := jsonfmt.NewWriter()
	require.NoError(t, writer.SetOutputWriter(&buf))
	writer.SetLocale(model.LocaleSpanish)
	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Contains(t, buf.String(), `"Hola"`)
	assert.Contains(t, buf.String(), `"translator hint"`)
}

// --- Skeleton Store Roundtrip Tests ---

func snippetRoundtripWithSkeleton(t *testing.T, snippet string, params map[string]any) string {
	t.Helper()
	ctx := t.Context()

	reader := jsonfmt.NewReader()
	writer := jsonfmt.NewWriter()

	if params != nil {
		require.NoError(t, reader.Config().ApplyMap(params))
		if v, ok := params["escapeForwardSlashes"]; ok {
			if b, ok := v.(bool); ok {
				writer.Config().EscapeForwardSlashes = b
			}
		}
	}

	// Wire skeleton store.
	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(snippet, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	return buf.String()
}

func TestSkeletonStore_ByteExact(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name   string
		input  string
		params map[string]any
	}{
		{"simple", `{"key": "value"}`, nil},
		{"whitespace", "{\n    \"key\" :   \"value\"\n}", nil},
		{"line_comment", "{\n  // line comment\n  \"key\": \"value\"\n}", nil},
		{"block_comment", "{\n  /* block */\n  \"key\": \"value\"\n}", nil},
		{"hash_comment", "{\n  # hash comment\n  \"key\": \"value\"\n}", nil},
		{"mixed_comments", "{\n  // line\n  /* block */\n  # hash\n  \"key\": \"value\"\n}", nil},
		{"nested", `{"parent": {"child": "value"}, "other": "text"}`, nil},
		{"array_isolated", `{"items": ["a", "b", "c"]}`, map[string]any{"extractIsolatedStrings": true}},
		{"array_non_isolated", `{"items": ["a", "b", "c"]}`, nil},
		{"non_string_values", `{"name": "Test", "count": 42, "rate": 3.14, "active": true, "deleted": false, "meta": null}`, nil},
		{"multiline", "{\n  \"title\": \"Hello\",\n  \"nested\": {\n    \"key\": \"World\"\n  },\n  \"count\": 5\n}", nil},
		{"trailing_newline", "{\"key\": \"value\"}\n", nil},
		{"empty_object", `{"data": {}, "text": "Hello"}`, nil},
		{"unicode_escapes_non_translatable", `{"id": "\u0041\u0042", "text": "Hello"}`, map[string]any{"idRules": "id"}},
		{"escaped_slash_non_translatable", `{"note": "see a\/b", "text": "Hello"}`, map[string]any{"noteRules": "note"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			output := snippetRoundtripWithSkeleton(t, tc.input, tc.params)
			assert.Equal(t, tc.input, output, "skeleton store roundtrip should be byte-exact")
		})
	}
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	t.Parallel()
	input := `{"greeting": "Hello World", "farewell": "Goodbye"}`
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := jsonfmt.NewReader()
	writer := jsonfmt.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			switch b.SourceText() {
			case "Hello World":
				b.SetTargetText(locale, "Bonjour le monde")
			case "Goodbye":
				b.SetTargetText(locale, "Au revoir")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	output := buf.String()
	assert.Equal(t, `{"greeting": "Bonjour le monde", "farewell": "Au revoir"}`, output)
}

func TestSkeletonStore_WithTranslation_UseFullKeyPath(t *testing.T) {
	t.Parallel()
	input := `{"parent": {"child": "Hello"}}`
	ctx := t.Context()
	locale := model.LocaleID("de")

	reader := jsonfmt.NewReader()
	writer := jsonfmt.NewWriter()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{
		"useKeyAsName":   true,
		"useFullKeyPath": true,
	}))

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.SourceText() == "Hello" {
				b.SetTargetText(locale, "Hallo")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Equal(t, `{"parent": {"child": "Hallo"}}`, buf.String())
}

func TestSkeletonStore_WithTranslation_IdRules(t *testing.T) {
	t.Parallel()
	input := `{"id": "my-id", "text": "Hello"}`
	ctx := t.Context()
	locale := model.LocaleID("es")

	reader := jsonfmt.NewReader()
	writer := jsonfmt.NewWriter()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{"idRules": "id"}))

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.SourceText() == "Hello" {
				b.SetTargetText(locale, "Hola")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Equal(t, `{"id": "my-id", "text": "Hola"}`, buf.String())
}

func TestSkeletonStore_PreservesFormatting(t *testing.T) {
	t.Parallel()
	input := `{
  "title" : "Hello World",
  "nested" : {
    "description" : "A description",
    "count" : 42
  },
  "tags" : [ "a", "b" ]
}
`
	output := snippetRoundtripWithSkeleton(t, input, nil)
	assert.Equal(t, input, output, "skeleton store should preserve all formatting including extra spaces")
}

func TestSkeletonStore_Exceptions(t *testing.T) {
	t.Parallel()
	input := `{"title": "Hello", "id": "skip-me", "body": "World"}`
	output := snippetRoundtripWithSkeleton(t, input, map[string]any{
		"extractAllPairs": true,
		"exceptions":      "^id$",
	})
	assert.Equal(t, input, output, "skeleton store should preserve non-extracted values byte-exact")
}

func TestSkeletonStore_EscapeForwardSlashes(t *testing.T) {
	t.Parallel()
	// With escapeForwardSlashes=true (default), translated text should have \/
	input := `{"url": "http://example.com"}`
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := jsonfmt.NewReader()
	writer := jsonfmt.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			b := p.Resource.(*model.Block)
			if b.SourceText() == "http://example.com" {
				b.SetTargetText(locale, "http://exemple.fr")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Contains(t, buf.String(), `http:\/\/exemple.fr`)
}
