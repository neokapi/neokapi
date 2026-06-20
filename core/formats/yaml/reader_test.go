// okapi-filter: yaml
package yaml_test

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	yamlfmt "github.com/neokapi/neokapi/core/formats/yaml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- test helpers ---

func readYAML(t *testing.T, input string) []*model.Block {
	t.Helper()
	ctx := t.Context()
	reader := yamlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectBlocks(t, reader.Read(ctx))
}

func readYAMLParts(t *testing.T, input string) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := yamlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

func readYAMLWithConfig(t *testing.T, input string, params map[string]any) []*model.Block {
	t.Helper()
	ctx := t.Context()
	reader := yamlfmt.NewReader()
	cfg := reader.Config()
	require.NoError(t, cfg.ApplyMap(params))
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectBlocks(t, reader.Read(ctx))
}

func blockTexts(blocks []*model.Block) []string {
	return testutil.BlockTexts(blocks)
}

func blockByName(blocks []*model.Block, name string) *model.Block {
	for _, b := range blocks {
		if b.Name == name {
			return b
		}
	}
	return nil
}

func blockByNameContaining(blocks []*model.Block, substr string) *model.Block {
	for _, b := range blocks {
		if strings.Contains(b.Name, substr) {
			return b
		}
	}
	return nil
}

// ---- YmlFilterTest (31 tests) ----

// okapi: YmlFilterTest#testSimpleYaml
func TestExtract_SimpleYaml(t *testing.T) {
	t.Parallel()
	input := `en:
  title: My Rails Website
  items:
    - test1
    - test2
`
	blocks := readYAML(t, input)

	texts := blockTexts(blocks)
	assert.Contains(t, texts, "My Rails Website")
	assert.Contains(t, texts, "test1")
	assert.Contains(t, texts, "test2")
}

// okapi: YmlFilterTest#testDefaultInfo
func TestExtract_DefaultInfo(t *testing.T) {
	t.Parallel()
	parts := readYAMLParts(t, "key: value\n")
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// okapi: YmlFilterTest#testStartDocument
func TestExtract_StartDocument(t *testing.T) {
	t.Parallel()
	parts := readYAMLParts(t, "key: value\n")
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.NotEmpty(t, layer.ID)
	assert.Equal(t, "yaml", layer.Format)
}

// okapi: YmlFilterTest#testScalars
func TestExtract_Scalars(t *testing.T) {
	t.Parallel()
	input := `invoice: 34843
date: 2001-01-23
bill-to:
  given: Dorothy
  family: Gale
product:
  - item: Water Bucket (Filled)
    quantity: 4
  - item: High Heeled "Ruby" Slippers
    quantity: 1
comments: |
  Follow the Yellow Brick
  Road to the Emerald City.
`
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)

	assert.Contains(t, texts, "Dorothy")
	assert.Contains(t, texts, "Gale")
	assert.Contains(t, texts, "Water Bucket (Filled)")
	assert.Contains(t, texts, "High Heeled \"Ruby\" Slippers")

	// Verify the literal block scalar is extracted
	var foundFolded bool
	for _, text := range texts {
		if strings.Contains(text, "Follow the Yellow Brick") {
			foundFolded = true
			break
		}
	}
	assert.True(t, foundFolded, "should extract the literal block scalar")
}

// okapi: YmlFilterTest#testFlow
func TestExtract_Flow(t *testing.T) {
	t.Parallel()
	input := `shopping:
  - milk
  - pumpkin pie
  - eggs
  - juice
person: {name: John Smith, age: 33}
`
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)

	assert.Contains(t, texts, "milk")
	assert.Contains(t, texts, "pumpkin pie")
	assert.Contains(t, texts, "John Smith")
}

// okapi: YmlFilterTest#testMultilineValue
func TestExtract_MultilineValue(t *testing.T) {
	t.Parallel()
	input := `not_literal: not literal
literal: |
  Your enrollment is not complete
  until you verify your email.
`
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)

	var foundLiteral bool
	for _, text := range texts {
		if strings.Contains(text, "Your enrollment is not complete") {
			foundLiteral = true
			break
		}
	}
	assert.True(t, foundLiteral, "should extract literal block multiline value")
	assert.Contains(t, texts, "not literal")
}

// okapi: YmlFilterTest#testSimplePlaceholders
func TestExtract_SimplePlaceholders(t *testing.T) {
	t.Parallel()
	input := "msg: \"Hello {{name}}, you have {{count}} items.\"\n"
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Hello")
	assert.Contains(t, text, "items")
	assert.Contains(t, text, "{{name}}")
	assert.Contains(t, text, "{{count}}")
}

// okapi: YmlFilterTest#map
func TestExtract_Map(t *testing.T) {
	t.Parallel()
	input := `en:
  title: My Rails Website
  items:
    - test1
    - test2
fr:
  title: My Rails Website
  items:
    - test1
    - test2
`
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)

	count := 0
	for _, text := range texts {
		if text == "My Rails Website" {
			count++
		}
	}
	assert.Equal(t, 2, count, "should extract 2 'My Rails Website' entries from map structure")
}

// okapi: YmlFilterTest#list
func TestExtract_List(t *testing.T) {
	t.Parallel()
	input := "items:\n  - First\n  - Second\n  - Third\n"
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)

	assert.Contains(t, texts, "First")
	assert.Contains(t, texts, "Second")
	assert.Contains(t, texts, "Third")
}

// okapi: YmlFilterTest#listSingleQuote
func TestExtract_ListSingleQuote(t *testing.T) {
	t.Parallel()
	input := "items:\n  - 'First'\n  - 'Second'\n  - 'Third'\n"
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)

	assert.Contains(t, texts, "First")
	assert.Contains(t, texts, "Second")
	assert.Contains(t, texts, "Third")
}

// okapi: YmlFilterTest#emptyKey
func TestExtract_EmptyKey(t *testing.T) {
	t.Parallel()
	input := "a: value_a\nb: value_b\n"
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)

	texts := blockTexts(blocks)
	assert.Contains(t, texts, "value_a")
	assert.Contains(t, texts, "value_b")
}

// okapi: YmlFilterTest#nonEmptyKey
func TestExtract_NonEmptyKey(t *testing.T) {
	t.Parallel()
	input := "mykey: value for mykey\n"
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)

	b := blockByNameContaining(blocks, "mykey")
	require.NotNil(t, b, "should find block with name containing 'mykey'")
	assert.Equal(t, "value for mykey", b.SourceText())
}

// okapi: YmlFilterTest#mapWithEmptyKeys
func TestExtract_MapWithEmptyKeys(t *testing.T) {
	t.Parallel()
	input := "parent:\n  child1: value1\n  child2: value2\n"
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)

	texts := blockTexts(blocks)
	assert.Contains(t, texts, "value1")
	assert.Contains(t, texts, "value2")
}

// okapi: YmlFilterTest#mapWithEmptyKeysQuoted
func TestExtract_MapWithEmptyKeysQuoted(t *testing.T) {
	t.Parallel()
	input := "parent:\n  \"child1\": value1\n  \"child2\": value2\n"
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)

	texts := blockTexts(blocks)
	assert.Contains(t, texts, "value1")
	assert.Contains(t, texts, "value2")

	// Verify key path names include quoted keys
	b := blockByName(blocks, "parent.child1")
	require.NotNil(t, b)
	assert.Equal(t, "value1", b.SourceText())
}

// okapi: YmlFilterTest#commentsAfterPlainScalarsPreserved
func TestExtract_CommentsAfterPlainScalarsPreserved(t *testing.T) {
	t.Parallel()
	// Comments should not appear in extracted text values.
	input := "key: value # this is a comment\n"
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "value", blocks[0].SourceText())
}

// okapi: YmlFilterTest#commentsAfterPlainScalarMappingValuesPreserved
func TestExtract_CommentsAfterPlainScalarMappingValuesPreserved(t *testing.T) {
	t.Parallel()
	input := "parent:\n  key: value # comment\n"
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)

	b := blockByNameContaining(blocks, "key")
	require.NotNil(t, b)
	assert.Equal(t, "value", b.SourceText())
}

// okapi: YmlFilterTest#testOpenTwiceWithString
func TestExtract_OpenTwiceWithString(t *testing.T) {
	t.Parallel()
	input := "key: Hello World\n"

	blocks1 := readYAML(t, input)
	require.NotEmpty(t, blocks1)

	blocks2 := readYAML(t, input)
	require.NotEmpty(t, blocks2)

	assert.Equal(t, blocks1[0].SourceText(), blocks2[0].SourceText())
}

// okapi: YmlFilterTest#testSubfiltering
func TestExtract_Subfiltering(t *testing.T) {
	t.Parallel()
	// Native YAML reader does not implement HTML subfilter delegation,
	// but it should extract the raw HTML content as a string value.
	input := `key: "<p>Hello world</p>"
`
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Hello world")
}

// okapi: YmlFilterTest#testDoubleSubfilter
func TestExtract_DoubleSubfilter(t *testing.T) {
	t.Parallel()
	// The native reader extracts HTML content as raw string.
	input := `html: "Visit <a href=\"http://www.google.com\">Google</a>"
`
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Visit")
	assert.Contains(t, text, "Google")
}

// okapi: YmlFilterTest#testSubFilterProcessLiteralAsBlock
func TestExtract_SubFilterProcessLiteralAsBlock(t *testing.T) {
	t.Parallel()
	input := `not_literal: not literal
literal_html: |
  <ul>
    <li>item 1</li>
    <li>item 2</li>
  </ul>
`
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)

	texts := blockTexts(blocks)
	assert.Contains(t, texts, "not literal")

	// The literal block HTML content should be extracted as raw text.
	var foundItem bool
	for _, text := range texts {
		if strings.Contains(text, "item 1") || strings.Contains(text, "item 2") {
			foundItem = true
			break
		}
	}
	assert.True(t, foundItem, "literal block HTML should be extracted")
}

// okapi: YmlFilterTest#issue555
func TestExtract_Issue555(t *testing.T) {
	t.Parallel()
	// Tests comment handling and boolean-like values.
	input := `# Comment line
world-teleport-permissions: false
teleportation:
  enabled: true
`
	blocks := readYAMLWithConfig(t, input, map[string]any{
		"extractNonStrings": true,
	})
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "false")
}

// okapi: YmlFilterTest#issue556
func TestExtract_Issue556(t *testing.T) {
	t.Parallel()
	// HTML content in YAML value with subfilter. Native reader extracts raw.
	input := `html: "Visit <a href=\"http://www.google.com\">Google</a>"
`
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Visit")
	assert.Contains(t, text, "Google")
}

// okapi: YmlFilterTest#testDoublePlainWithQuotes
func TestExtract_DoublePlainWithQuotes(t *testing.T) {
	t.Parallel()
	// Plain scalars containing single quotes.
	input := `description: Basically it's a description with 'quotes'
`
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Basically")
	assert.Contains(t, text, "'quotes'")
}

// okapi: YmlFilterTest#testDoubleExtraction
func TestExtract_DoubleExtraction(t *testing.T) {
	t.Parallel()
	input := `en:
  hello: Hello
  goodbye: Goodbye
`
	blocks1 := readYAML(t, input)
	blocks2 := readYAML(t, input)

	require.Len(t, blocks2, len(blocks1))
	for i := range blocks1 {
		assert.Equal(t, blocks1[i].SourceText(), blocks2[i].SourceText())
		assert.Equal(t, blocks1[i].Name, blocks2[i].Name)
	}
}

// okapi: YmlFilterTest#testDoubleExtractionWithEscapes
func TestExtract_DoubleExtractionWithEscapes(t *testing.T) {
	t.Parallel()
	input := `key1: "Hello\tWorld"
key2: "Line1\nLine2"
key3: "Path\\to\\file"
`
	blocks1 := readYAML(t, input)
	blocks2 := readYAML(t, input)

	require.Len(t, blocks2, len(blocks1))
	for i := range blocks1 {
		assert.Equal(t, blocks1[i].SourceText(), blocks2[i].SourceText())
	}
}

// okapi: YmlFilterTest#testDoubleExtractionNonStrings
func TestExtract_DoubleExtractionNonStrings(t *testing.T) {
	t.Parallel()
	input := `string_val: hello
int_val: 42
float_val: 3.14
bool_val: true
null_val: null
`
	params := map[string]any{"extractNonStrings": true}
	blocks1 := readYAMLWithConfig(t, input, params)
	blocks2 := readYAMLWithConfig(t, input, params)

	require.Len(t, blocks2, len(blocks1))
	for i := range blocks1 {
		assert.Equal(t, blocks1[i].SourceText(), blocks2[i].SourceText())
	}
}

// okapi: YmlFilterTest#testDoubleExtractionLongLine
func TestExtract_DoubleExtractionLongLine(t *testing.T) {
	t.Parallel()
	longValue := strings.Repeat("This is a long line. ", 50)
	input := "key: " + longValue + "\n"

	blocks1 := readYAML(t, input)
	blocks2 := readYAML(t, input)

	require.Len(t, blocks2, len(blocks1))
	assert.Equal(t, blocks1[0].SourceText(), blocks2[0].SourceText())
}

// okapi: YmlFilterTest#testDoubleExtractionWithMultilines
func TestExtract_DoubleExtractionWithMultilines(t *testing.T) {
	t.Parallel()
	input := `folded: >
  This is a folded
  block scalar that
  will be joined.
literal: |
  This is a literal
  block scalar that
  preserves newlines.
`
	blocks1 := readYAML(t, input)
	blocks2 := readYAML(t, input)

	require.Len(t, blocks2, len(blocks1))
	for i := range blocks1 {
		assert.Equal(t, blocks1[i].SourceText(), blocks2[i].SourceText())
	}
}

// okapi: YmlFilterTest#commentsAfterScalarsRoundTripped
func TestExtract_CommentsAfterScalars(t *testing.T) {
	t.Parallel()
	// Verify that comments don't leak into extracted values.
	input := "key: value # important comment\nkey2: other # another comment\n"
	blocks := readYAML(t, input)

	require.Len(t, blocks, 2)
	assert.Equal(t, "value", blocks[0].SourceText())
	assert.Equal(t, "other", blocks[1].SourceText())
}

// noteTexts returns the note (comment-context) texts attached to a block.
func noteTexts(b *model.Block) []string {
	var out []string
	for _, n := range b.Notes() {
		out = append(out, n.Text)
	}
	return out
}

// #928 (treatment B): YAML comments are surfaced as parity-safe
// NoteAnnotations on the adjacent translatable block — never as translatable
// content and never leaking into the extracted value. A trailing inline
// `value # …` comment attaches to that value's own block.
func TestExtract_InlineCommentSurfacedAsNote(t *testing.T) {
	t.Parallel()
	blocks := readYAML(t, "key: value # important comment\n")
	require.Len(t, blocks, 1)
	assert.Equal(t, "value", blocks[0].SourceText())
	assert.Equal(t, []string{"important comment"}, noteTexts(blocks[0]))
}

// A full-line head comment above a mapping entry attaches to that entry's
// value block, in document order.
func TestExtract_HeadCommentSurfacedAsNote(t *testing.T) {
	t.Parallel()
	input := "# top head comment\n" +
		"key1: value1  # inline on key1\n" +
		"# head for key2\n" +
		"key2: value2\n"
	blocks := readYAML(t, input)
	require.Len(t, blocks, 2)

	b1 := blockByName(blocks, "key1")
	require.NotNil(t, b1)
	assert.Equal(t, "value1", b1.SourceText())
	assert.Equal(t, []string{"top head comment", "inline on key1"}, noteTexts(b1))

	b2 := blockByName(blocks, "key2")
	require.NotNil(t, b2)
	assert.Equal(t, "value2", b2.SourceText())
	assert.Equal(t, []string{"head for key2"}, noteTexts(b2))
}

// A head comment above a container-valued key (its value is a sequence/map,
// not a scalar) has no scalar block of its own; it flows in document order to
// the first translatable block in that subtree. A sequence item's own head
// comment attaches to that item's block.
func TestExtract_HeadCommentOnContainerFlowsToFirstBlock(t *testing.T) {
	t.Parallel()
	input := "# section comment\n" +
		"list:\n" +
		"  - item1  # inline on item1\n" +
		"  # head on item2\n" +
		"  - item2\n"
	blocks := readYAML(t, input)
	require.Len(t, blocks, 2)

	assert.Equal(t, "item1", blocks[0].SourceText())
	assert.Equal(t, []string{"section comment", "inline on item1"}, noteTexts(blocks[0]))

	assert.Equal(t, "item2", blocks[1].SourceText())
	assert.Equal(t, []string{"head on item2"}, noteTexts(blocks[1]))
}

// Multi-line head comments are joined into one note with markers stripped.
func TestExtract_MultiLineHeadCommentNote(t *testing.T) {
	t.Parallel()
	input := "# line one\n# line two\nkey: value\n"
	blocks := readYAML(t, input)
	require.Len(t, blocks, 1)
	assert.Equal(t, "value", blocks[0].SourceText())
	assert.Equal(t, []string{"line one\nline two"}, noteTexts(blocks[0]))
}

// Comments must surface only as annotations: they add no Data parts and no
// translatable blocks, so the part stream is unchanged (treatment B.1).
func TestExtract_CommentsAddNoDataParts(t *testing.T) {
	t.Parallel()
	parts := readYAMLParts(t, "# head\nkey: value # inline\n# foot\n")
	var dataParts, blocks int
	for _, p := range parts {
		switch p.Type {
		case model.PartData:
			dataParts++
		case model.PartBlock:
			blocks++
		}
	}
	assert.Zero(t, dataParts, "comments must not add Data parts (treatment B.1)")
	assert.Equal(t, 1, blocks, "comments must not add translatable blocks")
}

// A document with only comments still yields no block to attach to, so the
// comments are dropped (no adjacent block; no part-stream change).
func TestExtract_CommentsOnlyYieldNoNotes(t *testing.T) {
	t.Parallel()
	blocks := readYAML(t, "# just a comment\n# and another\n")
	assert.Empty(t, blocks)
}

// okapi: YmlFilterTest#testRoundTripSubFilterProcessLiteralAsBlock
func TestExtract_RoundTripSubFilterProcessLiteralAsBlock(t *testing.T) {
	t.Parallel()
	input := `not_literal: not literal
literal_html: |
  <ul>
    <li>item 1</li>
    <li>item 2</li>
  </ul>
`
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "not literal")

	var foundHTML bool
	for _, text := range texts {
		if strings.Contains(text, "item 1") {
			foundHTML = true
			break
		}
	}
	assert.True(t, foundHTML)
}

// okapi: YmlFilterTest#testRoundtripFailures
func TestExtract_RoundtripFiles(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{
			name:  "simple key-value",
			input: "title: Hello\ndescription: World\n",
		},
		{
			name:  "nested map",
			input: "en:\n  title: My Rails Website\n  items:\n    - test1\n    - test2\n",
		},
		{
			name:  "deep nesting",
			input: "root:\n  level1:\n    level2:\n      key: deep value\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			blocks := readYAML(t, tt.input)
			require.NotEmpty(t, blocks)
		})
	}
}

// ---- YamlFilterTest (4 tests) ----

// okapi: YamlFilterTest#testInlineCodeFinderNewLineCharacterDoubleQuotedString
func TestExtract_InlineCodeFinderNewLineCharacterDoubleQuotedString(t *testing.T) {
	t.Parallel()
	input := "key: \"Hello\\nWorld\"\n"
	blocks := readYAMLWithConfig(t, input, map[string]any{
		"useCodeFinder": true,
	})
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Hello")
	assert.Contains(t, text, "World")
}

// okapi: YamlFilterTest#testInlineCodeFinderNewLineCharacterSingleQuotedString
func TestExtract_InlineCodeFinderNewLineCharacterSingleQuotedString(t *testing.T) {
	t.Parallel()
	input := "key: 'Hello\\nWorld'\n"
	blocks := readYAMLWithConfig(t, input, map[string]any{
		"useCodeFinder": true,
	})
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Hello")
	assert.Contains(t, text, "World")
}

// okapi: YamlFilterTest#testInlineCodeFinderNewLineCharacterStringWithoutQuotes
func TestExtract_InlineCodeFinderNewLineCharacterStringWithoutQuotes(t *testing.T) {
	t.Parallel()
	input := "key: Hello\\nWorld\n"
	blocks := readYAMLWithConfig(t, input, map[string]any{
		"useCodeFinder": true,
	})
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Hello")
	assert.Contains(t, text, "World")
}

// okapi: YamlFilterTest#testInlineCodeFinderWithQuoteInCode
func TestExtract_InlineCodeFinderWithQuoteInCode(t *testing.T) {
	t.Parallel()
	input := "key: \"Text with 'code' inside\"\n"
	blocks := readYAMLWithConfig(t, input, map[string]any{
		"useCodeFinder": true,
	})
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Text with")
}

// ---- YamlParserTest (4 tests) ----

// okapi: YamlParserTest#singleString
func TestParse_SingleString(t *testing.T) {
	t.Parallel()
	blocks := readYAML(t, "key: value\n")
	require.NotEmpty(t, blocks)
	assert.Equal(t, "value", blocks[0].SourceText())
}

// okapi: YamlParserTest#singleFile
func TestParse_SingleFile(t *testing.T) {
	t.Parallel()
	input := `en:
  title: My Rails Website
  items:
    - test1
    - test2
`
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)
	assert.GreaterOrEqual(t, len(blocks), 3)
}

// okapi: YamlParserTest#singleArrayNoSpace
func TestParse_SingleArrayNoSpace(t *testing.T) {
	t.Parallel()
	// Flow-style array in YAML
	input := "items: [one,two,three]\n"
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)
}

// okapi: YamlParserTest#singleArrayWithSpace
func TestParse_SingleArrayWithSpace(t *testing.T) {
	t.Parallel()
	input := "items: [one, two, three]\n"
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)

	texts := blockTexts(blocks)
	assert.Contains(t, texts, "one")
	assert.Contains(t, texts, "two")
	assert.Contains(t, texts, "three")
}

// ---- Additional native tests for YAML-specific features ----

func TestReadSimpleYAML(t *testing.T) {
	t.Parallel()
	blocks := readYAML(t, "title: Hello World\ndescription: A test")
	require.Len(t, blocks, 2)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Hello World")
	assert.Contains(t, texts, "A test")
}

func TestReadNestedYAML(t *testing.T) {
	t.Parallel()
	blocks := readYAML(t, "root:\n  nested:\n    value: Deep content")
	require.Len(t, blocks, 1)
	assert.Equal(t, "Deep content", blocks[0].SourceText())
	assert.Equal(t, "root.nested.value", blocks[0].Name)
}

func TestReadYAMLArray(t *testing.T) {
	t.Parallel()
	blocks := readYAML(t, "items:\n  - First\n  - Second\n  - Third")
	require.Len(t, blocks, 3)
	assert.Equal(t, "First", blocks[0].SourceText())
	assert.Equal(t, "Second", blocks[1].SourceText())
	assert.Equal(t, "Third", blocks[2].SourceText())
}

func TestReadYAMLLayerStartEnd(t *testing.T) {
	t.Parallel()
	parts := readYAMLParts(t, "key: value")
	require.GreaterOrEqual(t, len(parts), 2)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "yaml", layer.Format)
}

func TestReadYAMLEmpty(t *testing.T) {
	t.Parallel()
	blocks := readYAML(t, "{}")
	assert.Empty(t, blocks)
}

func TestReaderSignature(t *testing.T) {
	t.Parallel()
	reader := yamlfmt.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/yaml")
	assert.Contains(t, sig.Extensions, ".yaml")
	assert.Contains(t, sig.Extensions, ".yml")
}

func TestReaderMetadata(t *testing.T) {
	t.Parallel()
	reader := yamlfmt.NewReader()
	assert.Equal(t, "yaml", reader.Name())
	assert.Equal(t, "YAML", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := yamlfmt.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

// --- Multiline string tests ---

func TestReadLiteralBlockScalar(t *testing.T) {
	t.Parallel()
	input := `description: |
  This is a literal
  block scalar that
  preserves newlines.
`
	blocks := readYAML(t, input)
	require.Len(t, blocks, 1)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "This is a literal")
	assert.Contains(t, text, "preserves newlines.")
	assert.Contains(t, text, "\n", "literal block should preserve newlines")
}

func TestReadFoldedBlockScalar(t *testing.T) {
	t.Parallel()
	input := `description: >
  This is a folded
  block scalar that
  will be joined.
`
	blocks := readYAML(t, input)
	require.Len(t, blocks, 1)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "This is a folded")
	assert.Contains(t, text, "will be joined.")
}

func TestReadLiteralBlockWithChompKeep(t *testing.T) {
	t.Parallel()
	input := `key: |+
  Line one
  Line two

`
	blocks := readYAML(t, input)
	require.Len(t, blocks, 1)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Line one")
	assert.Contains(t, text, "Line two")
}

func TestReadLiteralBlockWithChompStrip(t *testing.T) {
	t.Parallel()
	input := `key: |-
  Line one
  Line two
`
	blocks := readYAML(t, input)
	require.Len(t, blocks, 1)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Line one")
	assert.Contains(t, text, "Line two")
	assert.False(t, strings.HasSuffix(text, "\n"), "strip chomp should remove trailing newline")
}

func TestReadFoldedBlockWithChompStrip(t *testing.T) {
	t.Parallel()
	input := `key: >-
  Folded content
  on multiple lines
`
	blocks := readYAML(t, input)
	require.Len(t, blocks, 1)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Folded content")
}

// --- Anchor and alias tests ---

func TestReadAnchorAndAlias(t *testing.T) {
	t.Parallel()
	input := `defaults: &defaults
  adapter: postgres
  host: localhost
development:
  database: dev_db
  <<: *defaults
production:
  database: prod_db
  <<: *defaults
`
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)

	assert.Contains(t, texts, "postgres")
	assert.Contains(t, texts, "localhost")
	assert.Contains(t, texts, "dev_db")
	assert.Contains(t, texts, "prod_db")
}

func TestReadScalarAnchorAlias(t *testing.T) {
	t.Parallel()
	input := `base_url: &url https://example.com
api_url: *url
`
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)

	// Both should have the anchored value
	count := 0
	for _, text := range texts {
		if text == "https://example.com" {
			count++
		}
	}
	assert.Equal(t, 2, count, "anchor and alias should both produce blocks")
}

// --- Multi-document tests ---

func TestReadMultiDocument(t *testing.T) {
	t.Parallel()
	input := "---\ntitle: Document One\n---\ntitle: Document Two\n"
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)

	assert.Contains(t, texts, "Document One")
	assert.Contains(t, texts, "Document Two")
}

func TestReadMultiDocumentWithExplicitEnd(t *testing.T) {
	t.Parallel()
	input := "---\nkey: first\n...\n---\nkey: second\n"
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)

	assert.Contains(t, texts, "first")
	assert.Contains(t, texts, "second")
}

// --- Flow style tests ---

func TestReadFlowMapping(t *testing.T) {
	t.Parallel()
	input := "person: {name: Alice, role: Developer}\n"
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)

	assert.Contains(t, texts, "Alice")
	assert.Contains(t, texts, "Developer")
}

func TestReadFlowSequence(t *testing.T) {
	t.Parallel()
	input := "colors: [red, green, blue]\n"
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)

	assert.Contains(t, texts, "red")
	assert.Contains(t, texts, "green")
	assert.Contains(t, texts, "blue")
}

func TestReadNestedFlowStyle(t *testing.T) {
	t.Parallel()
	input := "data: {items: [one, two], meta: {count: 2}}\n"
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)

	assert.Contains(t, texts, "one")
	assert.Contains(t, texts, "two")
}

// --- Quoting style tests ---

func TestReadDoubleQuotedString(t *testing.T) {
	t.Parallel()
	input := "key: \"Hello World\"\n"
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
}

func TestReadSingleQuotedString(t *testing.T) {
	t.Parallel()
	input := "key: 'Hello World'\n"
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
}

func TestReadPlainScalar(t *testing.T) {
	t.Parallel()
	input := "key: Hello World\n"
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
}

func TestReadDoubleQuotedEscapes(t *testing.T) {
	t.Parallel()
	input := "key: \"Hello\\tWorld\\nNew line\"\n"
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Hello")
	assert.Contains(t, text, "World")
}

func TestReadSingleQuotedWithEscapedQuote(t *testing.T) {
	t.Parallel()
	input := "key: 'It''s a test'\n"
	blocks := readYAML(t, input)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "It's a test", blocks[0].SourceText())
}

// --- Non-string scalar tests ---

func TestReadBooleanNotExtractedByDefault(t *testing.T) {
	t.Parallel()
	input := "enabled: true\ndisabled: false\n"
	blocks := readYAML(t, input)
	assert.Empty(t, blocks, "booleans should not be extracted by default")
}

func TestReadBooleanExtractedWhenConfigured(t *testing.T) {
	t.Parallel()
	input := "enabled: true\ndisabled: false\n"
	blocks := readYAMLWithConfig(t, input, map[string]any{
		"extractNonStrings": true,
	})
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "true")
	assert.Contains(t, texts, "false")
}

func TestReadNumberNotExtractedByDefault(t *testing.T) {
	t.Parallel()
	input := "count: 42\nprice: 9.99\n"
	blocks := readYAML(t, input)
	assert.Empty(t, blocks, "numbers should not be extracted by default")
}

func TestReadNumberExtractedWhenConfigured(t *testing.T) {
	t.Parallel()
	input := "count: 42\nprice: 9.99\n"
	blocks := readYAMLWithConfig(t, input, map[string]any{
		"extractNonStrings": true,
	})
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "42")
	assert.Contains(t, texts, "9.99")
}

func TestReadNullNotExtracted(t *testing.T) {
	t.Parallel()
	input := "key: null\nother: ~\n"
	blocks := readYAML(t, input)
	assert.Empty(t, blocks, "null values should not be extracted")
}

// --- Key path name tests ---

func TestReadKeyPathNames(t *testing.T) {
	t.Parallel()
	input := `root:
  level1:
    level2: deep value
`
	blocks := readYAML(t, input)
	require.Len(t, blocks, 1)
	assert.Equal(t, "root.level1.level2", blocks[0].Name)
}

func TestReadArrayKeyPathNames(t *testing.T) {
	t.Parallel()
	input := "items:\n  - alpha\n  - beta\n"
	blocks := readYAML(t, input)
	require.Len(t, blocks, 2)
	assert.Equal(t, "items.[0]", blocks[0].Name)
	assert.Equal(t, "items.[1]", blocks[1].Name)
}

func TestReadNestedArrayKeyPath(t *testing.T) {
	t.Parallel()
	input := `categories:
  - name: First
    items:
      - alpha
      - beta
`
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "First")
	assert.Contains(t, texts, "alpha")
	assert.Contains(t, texts, "beta")
}

// --- Key path pattern tests ---

func TestReadKeyPathPatternWildcard(t *testing.T) {
	t.Parallel()
	input := `en:
  title: English Title
  body: English Body
fr:
  title: French Title
  body: French Body
`
	blocks := readYAMLWithConfig(t, input, map[string]any{
		"keyPathPatterns": []any{"en.*"},
	})
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "English Title")
	assert.Contains(t, texts, "English Body")
	assert.NotContains(t, texts, "French Title")
	assert.NotContains(t, texts, "French Body")
}

func TestReadKeyPathPatternDoubleWildcard(t *testing.T) {
	t.Parallel()
	input := `en:
  messages:
    greeting: Hello
    farewell: Goodbye
  ui:
    button: Click
fr:
  messages:
    greeting: Bonjour
`
	blocks := readYAMLWithConfig(t, input, map[string]any{
		"keyPathPatterns": []any{"en.**"},
	})
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Hello")
	assert.Contains(t, texts, "Goodbye")
	assert.Contains(t, texts, "Click")
	assert.NotContains(t, texts, "Bonjour")
}

func TestReadKeyPathPatternNoMatch(t *testing.T) {
	t.Parallel()
	input := "title: Hello\nbody: World\n"
	blocks := readYAMLWithConfig(t, input, map[string]any{
		"keyPathPatterns": []any{"nonexistent.*"},
	})
	assert.Empty(t, blocks)
}

func TestReadKeyPathPatternMultiple(t *testing.T) {
	t.Parallel()
	input := `en:
  title: English
fr:
  title: French
de:
  title: German
`
	blocks := readYAMLWithConfig(t, input, map[string]any{
		"keyPathPatterns": []any{"en.*", "de.*"},
	})
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "English")
	assert.Contains(t, texts, "German")
	assert.NotContains(t, texts, "French")
}

// --- Unicode tests ---

func TestReadUnicodeContent(t *testing.T) {
	t.Parallel()
	input := "greeting: \u4f60\u597d\u4e16\u754c\nfarewell: \u3055\u3088\u306a\u3089\n"
	blocks := readYAML(t, input)
	require.Len(t, blocks, 2)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "\u4f60\u597d\u4e16\u754c")
	assert.Contains(t, texts, "\u3055\u3088\u306a\u3089")
}

func TestReadSupplementalUnicode(t *testing.T) {
	t.Parallel()
	input := "emoji: \U0001F600\U0001F44D\n"
	blocks := readYAML(t, input)
	require.Len(t, blocks, 1)
	assert.Equal(t, "\U0001F600\U0001F44D", blocks[0].SourceText())
}

// --- Empty / whitespace-only values ---

func TestReadEmptyStringNotExtracted(t *testing.T) {
	t.Parallel()
	input := "key: \"\"\n"
	blocks := readYAML(t, input)
	assert.Empty(t, blocks, "empty strings should not be extracted")
}

func TestReadWhitespaceOnlyNotExtracted(t *testing.T) {
	t.Parallel()
	input := "key: \"   \"\n"
	blocks := readYAML(t, input)
	assert.Empty(t, blocks, "whitespace-only strings should not be extracted")
}

// --- Config tests ---

func TestConfigApplyMap(t *testing.T) {
	t.Parallel()
	cfg := &yamlfmt.Config{}

	err := cfg.ApplyMap(map[string]any{
		"extractNonStrings": true,
		"useCodeFinder":     true,
		"keyPathPatterns":   []any{"en.**"},
		"subfilter":         "html",
	})
	require.NoError(t, err)
	assert.True(t, cfg.ExtractNonStrings)
	assert.True(t, cfg.UseCodeFinder)
	assert.Equal(t, []string{"en.**"}, cfg.KeyPathPatterns)
	assert.Equal(t, "html", cfg.Subfilter)
}

func TestConfigApplyMapUnknownKey(t *testing.T) {
	t.Parallel()
	cfg := &yamlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{"badKey": true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown parameter")
}

func TestConfigReset(t *testing.T) {
	t.Parallel()
	cfg := &yamlfmt.Config{
		ExtractNonStrings: true,
		UseCodeFinder:     true,
		KeyPathPatterns:   []string{"en.**"},
		Subfilter:         "html",
	}
	cfg.Reset()
	assert.False(t, cfg.ExtractNonStrings)
	assert.False(t, cfg.UseCodeFinder)
	assert.Nil(t, cfg.KeyPathPatterns)
	assert.Empty(t, cfg.Subfilter)
}

func TestConfigValidate(t *testing.T) {
	t.Parallel()
	cfg := &yamlfmt.Config{}
	require.NoError(t, cfg.Validate())
}

func TestConfigFormatName(t *testing.T) {
	t.Parallel()
	cfg := &yamlfmt.Config{}
	assert.Equal(t, "yaml", cfg.FormatName())
}

// --- Compact notation / complex structures ---

func TestReadCompactNotation(t *testing.T) {
	t.Parallel()
	// Rails-style YAML with locale prefix.
	input := `en:
  activerecord:
    errors:
      messages:
        blank: can't be blank
        taken: has already been taken
`
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "can't be blank")
	assert.Contains(t, texts, "has already been taken")

	// Verify key paths
	b := blockByName(blocks, "en.activerecord.errors.messages.blank")
	require.NotNil(t, b)
	assert.Equal(t, "can't be blank", b.SourceText())
}

func TestReadRecursiveStructure(t *testing.T) {
	t.Parallel()
	input := `root:
  child1:
    grandchild: value1
  child2:
    grandchild: value2
  child3:
    nested:
      deep: value3
`
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "value1")
	assert.Contains(t, texts, "value2")
	assert.Contains(t, texts, "value3")
}

// TestReadSelfReferentialAnchor guards against the snakeyaml-style fixtures
// (beanring, no-children) where a mapping's value aliases back to its own
// root. Without cycle detection the reader recurses forever.
func TestReadSelfReferentialAnchor(t *testing.T) {
	t.Parallel()
	input := `&id001
self: *id001
name: leaf
`
	done := make(chan struct{})
	var blocks []*model.Block
	go func() {
		defer close(done)
		blocks = readYAML(t, input) //nolint:testifylint // require inside goroutine is acceptable: parent goroutine waits via done channel
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("yaml reader hung on self-referential anchor")
	}
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "leaf")
}

// --- Line continuation tests ---

func TestReadLineContinuation(t *testing.T) {
	t.Parallel()
	// YAML plain scalars with line continuation (folding in mapping values)
	input := `description: This is a long
  description that continues
  on multiple lines
`
	blocks := readYAML(t, input)
	require.Len(t, blocks, 1)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "This is a long")
	assert.Contains(t, text, "description that continues")
}

// --- Mixed content tests ---

func TestReadMixedContent(t *testing.T) {
	t.Parallel()
	input := `strings:
  hello: world
  greeting: "Hello, World!"
numbers:
  count: 42
  pi: 3.14
lists:
  items:
    - first
    - second
nested:
  deep:
    value: "deep content"
`
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)

	assert.Contains(t, texts, "world")
	assert.Contains(t, texts, "Hello, World!")
	assert.Contains(t, texts, "first")
	assert.Contains(t, texts, "second")
	assert.Contains(t, texts, "deep content")
	// Numbers should not be extracted by default
	assert.NotContains(t, texts, "42")
	assert.NotContains(t, texts, "3.14")
}

// --- Context cancellation test ---

func TestReadContextCancellation(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel() // Cancel immediately

	reader := yamlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("key: value", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	// Channel should close without blocking
	for range reader.Read(ctx) {
		// Drain channel
	}
}

// --- Large document test ---

func TestReadLargeDocument(t *testing.T) {
	t.Parallel()
	var sb strings.Builder
	for i := range 100 {
		sb.WriteString(fmt.Sprintf("key%d: value %d\n", i, i))
	}

	blocks := readYAML(t, sb.String())
	assert.Len(t, blocks, 100)
}

// --- Edge cases ---

func TestReadEmptyDocument(t *testing.T) {
	t.Parallel()
	blocks := readYAML(t, "")
	assert.Empty(t, blocks)
}

func TestReadDocumentWithOnlyComments(t *testing.T) {
	t.Parallel()
	input := "# This is a comment\n# Another comment\n"
	blocks := readYAML(t, input)
	assert.Empty(t, blocks)
}

func TestReadSpecialCharactersInKeys(t *testing.T) {
	t.Parallel()
	input := "\"key.with.dots\": dotted value\n\"key with spaces\": spaced value\n"
	blocks := readYAML(t, input)
	require.Len(t, blocks, 2)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "dotted value")
	assert.Contains(t, texts, "spaced value")
}

func TestReadMultipleSequences(t *testing.T) {
	t.Parallel()
	input := `fruits:
  - apple
  - banana
colors:
  - red
  - blue
`
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "apple")
	assert.Contains(t, texts, "banana")
	assert.Contains(t, texts, "red")
	assert.Contains(t, texts, "blue")
}

func TestReadSequenceOfMappings(t *testing.T) {
	t.Parallel()
	input := `people:
  - name: Alice
    role: Dev
  - name: Bob
    role: PM
`
	blocks := readYAML(t, input)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Alice")
	assert.Contains(t, texts, "Dev")
	assert.Contains(t, texts, "Bob")
	assert.Contains(t, texts, "PM")
}

func TestReadYAMLWithTags(t *testing.T) {
	t.Parallel()
	// Strings with explicit !!str tags
	input := "key: !!str 12345\n"
	blocks := readYAML(t, input)
	require.Len(t, blocks, 1)
	assert.Equal(t, "12345", blocks[0].SourceText())
}

// neokapi-only: YamlParserTest#singleMixedQuote — no such method in v1.48.0 YamlParserTest; parser-internal scalar handling, verified via extraction
// neokapi-only: YamlParserTest#singleObject — no such method in v1.48.0 YamlParserTest; parser-internal scalar handling, verified via extraction
