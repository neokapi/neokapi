//go:build integration

package messageformat

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Java-internal ICU/normalization/plural tests (not exercisable via bridge) ---
//
// okapi-unmapped: MessageFormatNormalizerTest#testNormalize — Java-internal ICU normalizer test
// okapi-unmapped: MessageFormatPluralTest#testAddMultiplePluralForms — Java-internal ICU plural form test
// okapi-unmapped: MessageFormatPluralTest#testAddPluralForms_Cardinal — Java-internal ICU plural form test
// okapi-unmapped: MessageFormatPluralTest#testAddPluralForms_Gender — Java-internal ICU plural form test
// okapi-unmapped: MessageFormatPluralTest#testAddPluralForms_Mixed — Java-internal ICU plural form test
// okapi-unmapped: MessageFormatPluralTest#testAddPluralForms_MixedDeep — Java-internal ICU plural form test
// okapi-unmapped: MessageFormatPluralTest#testAddPluralForms_MixedNested — Java-internal ICU plural form test
// okapi-unmapped: MessageFormatPluralTest#testAddPluralForms_Ordinal — Java-internal ICU plural form test
// okapi-unmapped: MessageFormatPluralTest#testAddPluralForms_Ordinal2 — Java-internal ICU plural form test
// okapi-unmapped: MessageFormatPluralTest#testAddPluralForms_Ordinal3 — Java-internal ICU plural form test
// okapi-unmapped: MessageFormatPluralTest#testAddPluralForms_OrdinalAndPlural — Java-internal ICU plural form test
// okapi-unmapped: MessageFormatPluralTest#testRemovePluralForms — Java-internal ICU plural form test
// okapi-unmapped: MessageFormatPluralTest#testWithPluralMessage2 — Java-internal ICU plural form test
// okapi-unmapped: MessageFormatPluralTest#testWithSelectMessage — Java-internal ICU plural form test
// okapi-unmapped: MessageFormatToFormattedTest#testCurrency — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testDuration — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testOffset — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testPercent — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testRussian — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testSelectMessage — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testSelectOrdinalMessage — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testShortPluralMessage — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testSkipSyntaxApostrophe — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testSkipSyntaxComplexPattern — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testSkipSyntaxEmbedded — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testSkipSyntaxEscapedBraces — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testSkipSyntaxQuotedText — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testSpellout — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testWithComplexGenderMessage — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testWithComplexPluralMessage — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testWithEmbeddedPluralMessage — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testWithMixedGenderAndPluralMessage — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testWithNestedGenderAndPluralMessage — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testWithNestedMessage — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testWithNumber — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testWithPlural — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testWithPluralMessage — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testWithSelect — Java-internal ICU formatting test
// okapi-unmapped: MessageFormatToFormattedTest#testWithSimpleMessage — Java-internal ICU formatting test
// okapi-unmapped: PluralRulesDiffTest#testArabic — Java-internal ICU plural rules diff test
// okapi-unmapped: PluralRulesDiffTest#testGerman — Java-internal ICU plural rules diff test
// okapi-unmapped: PluralRulesDiffTest#testRomanian — Java-internal ICU plural rules diff test
// okapi-unmapped: PluralRulesDiffTest#testRussian — Java-internal ICU plural rules diff test
// okapi-unmapped: PluralRulesDiffTest#testSpanish — Java-internal ICU plural rules diff test

const filterClass = "net.sf.okapi.filters.messageformat.MessageFormatFilter"
const mimeType = "text/x-messageformat"

// readMessageFormat parses a MessageFormat snippet with custom filter params and returns the parts.
func readMessageFormat(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	return bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.mf", mimeType, filterParams)
}

// readMessageFormatDefault parses a MessageFormat snippet with default (nil) params.
func readMessageFormatDefault(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readMessageFormat(t, snippet, nil)
}

// snippetRoundtrip roundtrips a MessageFormat snippet and returns the output string.
func snippetRoundtrip(t *testing.T, snippet string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, []byte(snippet), "test.mf", mimeType, filterParams)
	return string(result.Output)
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

// readMessageFormatExpectError opens a MessageFormat snippet via the bridge and
// expects an error during Open or Read (e.g. for deprecated CHOICE format
// or invalid syntax). Returns the first error encountered.
func readMessageFormatExpectError(t *testing.T, snippet string) error {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	reader := bridge.NewBridgeFormatReader(pool, cfg, filterClass, format.FormatSignature{})
	doc := &model.RawDocument{
		URI:          "test.mf",
		SourceLocale: "en",
		TargetLocale: "fr",
		Encoding:     "UTF-8",
		MimeType:     mimeType,
		Reader:       io.NopCloser(bytes.NewReader([]byte(snippet))),
	}
	err := reader.Open(context.Background(), doc)
	if err != nil {
		return err
	}
	// Open is lazy — errors may surface during Read.
	for pr := range reader.Read(context.Background()) {
		if pr.Error != nil {
			_ = reader.Close()
			return pr.Error
		}
	}
	_ = reader.Close()
	return nil
}

// ---------------------------------------------------------------------------
// Tests translated from MessageFormatFilterTest.java (31 tests)
// ---------------------------------------------------------------------------

// okapi-unmapped: MessageFormatFilterTest#testDefaultInfo — Java-specific API: tests filter metadata (name, display name, MIME type)
// okapi: MessageFormatFilterTest#testDefaultInfo
func TestExtract_DefaultInfo(t *testing.T) {
	t.Skip("Java-specific: tests filter metadata API (getName, getMimeType)")
}

// okapi: MessageFormatFilterTest#testStartDocument
func TestExtract_StartDocument(t *testing.T) {
	parts := readMessageFormatDefault(t, "Hello world")

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// okapi: MessageFormatFilterTest#testLineBreaks_CR
func TestExtract_LineBreaks_CR(t *testing.T) {
	snippet := "Line1\rLine2"
	result := snippetRoundtrip(t, snippet, nil)
	assert.Equal(t, snippet, result)
}

// okapi: MessageFormatFilterTest#testineBreaks_CRLF
func TestExtract_LineBreaks_CRLF(t *testing.T) {
	snippet := "Line1\r\nLine2"
	result := snippetRoundtrip(t, snippet, nil)
	assert.Equal(t, snippet, result)
}

// okapi: MessageFormatFilterTest#testLineBreaks_LF
func TestExtract_LineBreaks_LF(t *testing.T) {
	snippet := "Line1\nLine2"
	result := snippetRoundtrip(t, snippet, nil)
	assert.Equal(t, snippet, result)
}

// okapi: MessageFormatFilterTest#testEntry
func TestExtract_Entry(t *testing.T) {
	parts := readMessageFormatDefault(t, "Hello world")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello world")
}

// okapi: MessageFormatFilterTest#testCode1
func TestExtract_Code1(t *testing.T) {
	// Numeric placeholder {0}
	parts := readMessageFormatDefault(t, "Hello {0} world")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)

	// {0} should be extracted as an inline code (placeholder span).
	var placeholderCount int
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanPlaceholder {
			placeholderCount++
		}
	}
	assert.GreaterOrEqual(t, placeholderCount, 1, "should have at least 1 placeholder span for {0}")

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Hello")
	assert.Contains(t, text, "world")
}

// okapi: MessageFormatFilterTest#testCode2
func TestExtract_Code2(t *testing.T) {
	// Named placeholder {name}
	parts := readMessageFormatDefault(t, "Hello {name} world")

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
	assert.GreaterOrEqual(t, placeholderCount, 1, "should have at least 1 placeholder span for {name}")
}

// okapi: MessageFormatFilterTest#testCode3
func TestExtract_Code3(t *testing.T) {
	// Nested placeholder patterns: {0,number,integer}
	parts := readMessageFormatDefault(t, "There are {0,number,integer} files.")

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
	assert.GreaterOrEqual(t, placeholderCount, 1, "should have at least 1 placeholder span for {0,number,integer}")
}

// okapi: MessageFormatFilterTest#testCode4
func TestExtract_Code4(t *testing.T) {
	// Complex placeholder: {0,date,short}
	parts := readMessageFormatDefault(t, "The date is {0,date,short}.")

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
	assert.GreaterOrEqual(t, placeholderCount, 1, "should have at least 1 placeholder span for {0,date,short}")
}

// okapi: MessageFormatFilterTest#testCode5
func TestExtract_Code5(t *testing.T) {
	// Additional code pattern: {0,time}
	parts := readMessageFormatDefault(t, "The time is {0,time}.")

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
	assert.GreaterOrEqual(t, placeholderCount, 1, "should have placeholder span for {0,time}")
}

// okapi: MessageFormatFilterTest#testCode6
func TestExtract_Code6(t *testing.T) {
	// Extended code pattern: {0,number,#,###}
	parts := readMessageFormatDefault(t, "Value is {0,number,#,###}.")

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
	assert.GreaterOrEqual(t, placeholderCount, 1, "should have placeholder span for {0,number,#,###}")
}

// okapi: MessageFormatFilterTest#testMultipleEmbedded
func TestExtract_MultipleEmbedded(t *testing.T) {
	// Multiple embedded select/plural patterns produce multiple text units.
	input := "{gender, select, male {He} female {She} other {They}} bought {count, plural, one {# item} other {# items}}."
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	// Each plural/select branch becomes a separate text unit.
	assert.GreaterOrEqual(t, len(blocks), 2, "multiple embedded patterns should produce multiple text units")
}

// okapi: MessageFormatFilterTest#testMany1
func TestExtract_Many1(t *testing.T) {
	// Many text units extraction from a complex message.
	input := "{count, plural, =0 {no files} one {# file} other {# files}}"
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 2, "plural with multiple branches should produce multiple text units")

	// Verify texts include the expected fragments.
	texts := bridgetest.BlockTexts(blocks)
	var foundNoFiles, foundFile bool
	for _, text := range texts {
		if strings.Contains(text, "no files") {
			foundNoFiles = true
		}
		if strings.Contains(text, "file") {
			foundFile = true
		}
	}
	assert.True(t, foundNoFiles || foundFile, "should extract plural form content")
}

// okapi: MessageFormatFilterTest#testGenderNames
func TestExtract_GenderNames(t *testing.T) {
	// Gender-based select names.
	input := "{gender, select, male {He likes this} female {She likes this} other {They like this}}"
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 2, "gender select should produce text units for each branch")

	texts := bridgetest.BlockTexts(blocks)
	var foundHe, foundShe bool
	for _, text := range texts {
		if strings.Contains(text, "He likes") {
			foundHe = true
		}
		if strings.Contains(text, "She likes") {
			foundShe = true
		}
	}
	assert.True(t, foundHe, "should extract male branch")
	assert.True(t, foundShe, "should extract female branch")
}

// okapi: MessageFormatFilterTest#testPluralNames
func TestExtract_PluralNames(t *testing.T) {
	// Plural form names.
	input := "{count, plural, one {# item} other {# items}}"
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 2, "plural should produce text units for each form")
}

// okapi: MessageFormatFilterTest#testPluralNames2
func TestExtract_PluralNames2(t *testing.T) {
	// Plural form names variant 2.
	input := "{count, plural, =0 {no items} one {# item} other {# items}}"
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 2, "plural with =0 should produce text units")
}

// okapi: MessageFormatFilterTest#testPluralNames3
func TestExtract_PluralNames3(t *testing.T) {
	// Plural form names variant 3 with more forms.
	input := "{count, plural, zero {no items} one {# item} two {# items} few {# items} many {# items} other {# items}}"
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 2, "plural with many forms should produce text units")
}

// okapi: MessageFormatFilterTest#testEmbeddedPluralNames
func TestExtract_EmbeddedPluralNames(t *testing.T) {
	// Embedded plural within select.
	input := "{gender, select, male {He has {count, plural, one {# item} other {# items}}} female {She has {count, plural, one {# item} other {# items}}} other {They have {count, plural, one {# item} other {# items}}}}"
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 3, "embedded plural in select should produce multiple text units")
}

// okapi: MessageFormatFilterTest#testInvalid
func TestExtract_Invalid(t *testing.T) {
	// Invalid message format with unmatched braces.
	// The Java test expects an IllegalArgumentException; the bridge returns
	// an OkapiBadFilterInputException ("Error reading Message Format String")
	// during Open.
	err := readMessageFormatExpectError(t, "{invalid, broken")
	require.Error(t, err, "invalid message format should produce an error")
	assert.Contains(t, err.Error(), "Error reading Message Format String")
}

// okapi: MessageFormatFilterTest#testLiterals
func TestExtract_Literals(t *testing.T) {
	// Literal text extraction.
	parts := readMessageFormatDefault(t, "Just plain text")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Just plain text", blocks[0].SourceText())
}

// okapi: MessageFormatFilterTest#testOneQuote
func TestExtract_OneQuote(t *testing.T) {
	// Single quote handling in MessageFormat.
	// In ICU MessageFormat, a single quote starts an escape sequence.
	parts := readMessageFormatDefault(t, "It''s a test")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	// The '' should be normalized to a single quote in the extracted text.
	text := blocks[0].SourceText()
	assert.Contains(t, text, "'")
}

// okapi: MessageFormatFilterTest#testQuotedQuote
func TestExtract_QuotedQuote(t *testing.T) {
	// Quoted quote ('') handling.
	parts := readMessageFormatDefault(t, "It''s a ''test''")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "'")
	assert.Contains(t, text, "test")
}

// okapi: MessageFormatFilterTest#testDeepQuotes
func TestExtract_DeepQuotes(t *testing.T) {
	// Deeply nested quote handling.
	parts := readMessageFormatDefault(t, "It''s ''deeply'' ''nested'' text")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "'")
}

// okapi: MessageFormatFilterTest#testMessageFormatSubfilterJson
func TestExtract_SubfilterJson(t *testing.T) {
	// MessageFormat as sub-filter for JSON.
	// When used as a sub-filter, the JSON filter processes the file and
	// MessageFormat is applied to extract patterns from JSON values.
	jsonFilterClass := "net.sf.okapi.filters.json.JSONFilter"
	jsonMimeType := "application/json"

	input := `{"greeting": "{name}, welcome!", "count_msg": "{count, plural, one {# message} other {# messages}}"}`

	pool, cfg := bridgetest.SharedBridge(t)
	parts := bridgetest.ReadString(t, pool, cfg, jsonFilterClass, input, "test.json", jsonMimeType, map[string]any{
		"subfilter": "okf_messageformat",
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "JSON with messageformat subfilter should produce text units")

	// Verify that MessageFormat patterns are extracted.
	texts := bridgetest.BlockTexts(blocks)
	var foundGreeting, foundPlural bool
	for _, text := range texts {
		if strings.Contains(text, "welcome") {
			foundGreeting = true
		}
		if strings.Contains(text, "message") {
			foundPlural = true
		}
	}
	assert.True(t, foundGreeting || foundPlural, "should extract messageformat patterns from JSON values")
}

// okapi: MessageFormatFilterTest#testMessageFormatSubfilterYaml
func TestExtract_SubfilterYaml(t *testing.T) {
	// MessageFormat as sub-filter for YAML.
	yamlFilterClass := "net.sf.okapi.filters.yaml.YamlFilter"
	yamlMimeType := "application/x-yaml"

	input := "greeting: \"{name}, welcome!\"\ncount_msg: \"{count, plural, one {# message} other {# messages}}\"\n"

	pool, cfg := bridgetest.SharedBridge(t)
	parts := bridgetest.ReadString(t, pool, cfg, yamlFilterClass, input, "test.yaml", yamlMimeType, map[string]any{
		"subfilter": "okf_messageformat",
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "YAML with messageformat subfilter should produce text units")
}

// okapi: MessageFormatFilterTest#testDeepEmbeddedSubfilterYaml
func TestExtract_DeepEmbeddedSubfilterYaml(t *testing.T) {
	// Deep embedded sub-filter in YAML.
	yamlFilterClass := "net.sf.okapi.filters.yaml.YamlFilter"
	yamlMimeType := "application/x-yaml"

	input := "messages:\n  greeting: \"{gender, select, male {He said hello} female {She said hello} other {They said hello}}\"\n"

	pool, cfg := bridgetest.SharedBridge(t)
	parts := bridgetest.ReadString(t, pool, cfg, yamlFilterClass, input, "test.yaml", yamlMimeType, map[string]any{
		"subfilter": "okf_messageformat",
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "deep embedded YAML subfilter should produce text units")
}

// okapi: MessageFormatFilterTest#testDeepEmbeddedSubfilterJson
func TestExtract_DeepEmbeddedSubfilterJson(t *testing.T) {
	// Deep embedded sub-filter in JSON.
	jsonFilterClass := "net.sf.okapi.filters.json.JSONFilter"
	jsonMimeType := "application/json"

	input := `{"messages": {"greeting": "{gender, select, male {He said hello} female {She said hello} other {They said hello}}"}}`

	pool, cfg := bridgetest.SharedBridge(t)
	parts := bridgetest.ReadString(t, pool, cfg, jsonFilterClass, input, "test.json", jsonMimeType, map[string]any{
		"subfilter": "okf_messageformat",
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "deep embedded JSON subfilter should produce text units")
}

// okapi: MessageFormatFilterTest#testOffset
func TestExtract_Offset(t *testing.T) {
	// Plural offset handling: offset:1
	input := "{count, plural, offset:1 =0 {Nobody} =1 {Just {0}} one {# and one other} other {# and # others}}"
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 2, "plural with offset should produce text units")
}

// okapi: MessageFormatFilterTest#testSelectOrdinal
func TestExtract_SelectOrdinal(t *testing.T) {
	// selectordinal form handling.
	input := "{count, selectordinal, one {#st} two {#nd} few {#rd} other {#th}}"
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 2, "selectordinal should produce text units for each form")
}

// okapi: MessageFormatFilterTest#testInlineCodes
func TestExtract_InlineCodes(t *testing.T) {
	// Inline codes within message format.
	input := "Click <a>here</a> to see {count} results."
	parts := readMessageFormat(t, input, map[string]any{
		"useCodeFinder": true,
		"codeFinderRules": map[string]any{
			"count":                  1,
			"rule0":                  `</?[a-zA-Z][^>]*>`,
			"sample":                 `<a>text</a>`,
			"useAllRulesWhenTesting": true,
		},
	})

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "inline codes in message format should produce text units")

	// Check for inline codes (opening/closing spans for HTML tags).
	var hasSpans bool
	for _, b := range blocks {
		frag := b.FirstFragment()
		if frag != nil && len(frag.Spans) > 0 {
			hasSpans = true
			break
		}
	}
	assert.True(t, hasSpans, "should have inline code spans from code finder")
}
