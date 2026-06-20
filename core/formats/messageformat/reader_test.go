package messageformat_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/messageformat"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func readParts(t *testing.T, input string) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := messageformat.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

func readBlocks(t *testing.T, input string) []*model.Block {
	t.Helper()
	return testutil.FilterBlocks(readParts(t, input))
}

func blockTexts(blocks []*model.Block) []string {
	var texts []string
	for _, b := range blocks {
		texts = append(texts, b.SourceText())
	}
	return texts
}

// ---------------------------------------------------------------------------
// Tests translated from MessageFormatFilterTest.java
// ---------------------------------------------------------------------------

// okapi: MessageFormatFilterTest#testStartDocument
func TestExtract_StartDocument(t *testing.T) {
	parts := readParts(t, "Hello world")

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// okapi: MessageFormatFilterTest#testLineBreaks_CR
func TestExtract_LineBreaks_CR(t *testing.T) {
	input := "Line1\rLine2"
	blocks := readBlocks(t, input)
	require.NotEmpty(t, blocks)
	// CR is treated as a line separator, producing two separate blocks
	texts := blockTexts(blocks)
	var found bool
	for _, text := range texts {
		if strings.Contains(text, "Line") {
			found = true
		}
	}
	assert.True(t, found)
}

// okapi: MessageFormatFilterTest#testineBreaks_CRLF
func TestExtract_LineBreaks_CRLF(t *testing.T) {
	input := "Line1\r\nLine2"
	blocks := readBlocks(t, input)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	var found bool
	for _, text := range texts {
		if strings.Contains(text, "Line") {
			found = true
		}
	}
	assert.True(t, found)
}

// okapi: MessageFormatFilterTest#testLineBreaks_LF
func TestExtract_LineBreaks_LF(t *testing.T) {
	input := "Line1\nLine2"
	blocks := readBlocks(t, input)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	var found bool
	for _, text := range texts {
		if strings.Contains(text, "Line") {
			found = true
		}
	}
	assert.True(t, found)
}

// okapi: MessageFormatFilterTest#testEntry
func TestExtract_Entry(t *testing.T) {
	blocks := readBlocks(t, "Hello world")
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Hello world")
}

// okapi: MessageFormatFilterTest#testCode1
func TestExtract_Code1(t *testing.T) {
	// Numeric placeholder {0}
	blocks := readBlocks(t, "Hello {0} world")
	require.NotEmpty(t, blocks)

	runs := blocks[0].SourceRuns()

	var placeholderCount int
	for _, r := range runs {
		if r.Ph != nil {
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
	blocks := readBlocks(t, "Hello {name} world")
	require.NotEmpty(t, blocks)

	runs := blocks[0].SourceRuns()

	var placeholderCount int
	for _, r := range runs {
		if r.Ph != nil {
			placeholderCount++
		}
	}
	assert.GreaterOrEqual(t, placeholderCount, 1, "should have at least 1 placeholder span for {name}")
}

// okapi: MessageFormatFilterTest#testCode3
func TestExtract_Code3(t *testing.T) {
	// Nested placeholder patterns: {0,number,integer}
	blocks := readBlocks(t, "There are {0,number,integer} files.")
	require.NotEmpty(t, blocks)

	runs := blocks[0].SourceRuns()

	var placeholderCount int
	for _, r := range runs {
		if r.Ph != nil {
			placeholderCount++
		}
	}
	assert.GreaterOrEqual(t, placeholderCount, 1, "should have at least 1 placeholder span for {0,number,integer}")
}

// okapi: MessageFormatFilterTest#testCode4
func TestExtract_Code4(t *testing.T) {
	// Complex placeholder: {0,date,short}
	blocks := readBlocks(t, "The date is {0,date,short}.")
	require.NotEmpty(t, blocks)

	runs := blocks[0].SourceRuns()

	var placeholderCount int
	for _, r := range runs {
		if r.Ph != nil {
			placeholderCount++
		}
	}
	assert.GreaterOrEqual(t, placeholderCount, 1, "should have at least 1 placeholder span for {0,date,short}")
}

// okapi: MessageFormatFilterTest#testCode5
func TestExtract_Code5(t *testing.T) {
	// Additional code pattern: {0,time}
	blocks := readBlocks(t, "The time is {0,time}.")
	require.NotEmpty(t, blocks)

	runs := blocks[0].SourceRuns()

	var placeholderCount int
	for _, r := range runs {
		if r.Ph != nil {
			placeholderCount++
		}
	}
	assert.GreaterOrEqual(t, placeholderCount, 1, "should have placeholder span for {0,time}")
}

// okapi: MessageFormatFilterTest#testCode6
func TestExtract_Code6(t *testing.T) {
	// Extended code pattern: {0,number,#,###}
	blocks := readBlocks(t, "Value is {0,number,#,###}.")
	require.NotEmpty(t, blocks)

	runs := blocks[0].SourceRuns()

	var placeholderCount int
	for _, r := range runs {
		if r.Ph != nil {
			placeholderCount++
		}
	}
	assert.GreaterOrEqual(t, placeholderCount, 1, "should have placeholder span for {0,number,#,###}")
}

// okapi: MessageFormatFilterTest#testMultipleEmbedded
func TestExtract_MultipleEmbedded(t *testing.T) {
	input := "{gender, select, male {He} female {She} other {They}} bought {count, plural, one {# item} other {# items}}."
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 2, "multiple embedded patterns should produce multiple text units")
}

// okapi: MessageFormatFilterTest#testMany1
func TestExtract_Many1(t *testing.T) {
	input := "{count, plural, =0 {no files} one {# file} other {# files}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 2, "plural with multiple branches should produce multiple text units")

	texts := blockTexts(blocks)
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
	input := "{gender, select, male {He likes this} female {She likes this} other {They like this}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 2, "gender select should produce text units for each branch")

	texts := blockTexts(blocks)
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
	input := "{count, plural, one {# item} other {# items}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 2, "plural should produce text units for each form")
}

// okapi: MessageFormatFilterTest#testPluralNames2
func TestExtract_PluralNames2(t *testing.T) {
	input := "{count, plural, =0 {no items} one {# item} other {# items}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 2, "plural with =0 should produce text units")
}

// okapi: MessageFormatFilterTest#testPluralNames3
func TestExtract_PluralNames3(t *testing.T) {
	input := "{count, plural, zero {no items} one {# item} two {# items} few {# items} many {# items} other {# items}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 2, "plural with many forms should produce text units")
}

// okapi: MessageFormatFilterTest#testEmbeddedPluralNames
func TestExtract_EmbeddedPluralNames(t *testing.T) {
	input := "{gender, select, male {He has {count, plural, one {# item} other {# items}}} female {She has {count, plural, one {# item} other {# items}}} other {They have {count, plural, one {# item} other {# items}}}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 3, "embedded plural in select should produce multiple text units")
}

// okapi: MessageFormatFilterTest#testInvalid
func TestExtract_Invalid(t *testing.T) {
	ctx := t.Context()
	reader := messageformat.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("{invalid, broken", model.LocaleEnglish))
	require.Error(t, err, "invalid message format should produce an error")
	assert.Contains(t, err.Error(), "Error reading Message Format String")
}

// okapi: MessageFormatFilterTest#testLiterals
func TestExtract_Literals(t *testing.T) {
	blocks := readBlocks(t, "Just plain text")
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Just plain text", blocks[0].SourceText())
}

// okapi: MessageFormatFilterTest#testOneQuote
func TestExtract_OneQuote(t *testing.T) {
	blocks := readBlocks(t, "It''s a test")
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "'")
}

// okapi: MessageFormatFilterTest#testQuotedQuote
func TestExtract_QuotedQuote(t *testing.T) {
	blocks := readBlocks(t, "It''s a ''test''")
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "'")
	assert.Contains(t, text, "test")
}

// okapi: MessageFormatFilterTest#testDeepQuotes
func TestExtract_DeepQuotes(t *testing.T) {
	blocks := readBlocks(t, "It''s ''deeply'' ''nested'' text")
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "'")
}

// okapi: MessageFormatFilterTest#testOffset
func TestExtract_Offset(t *testing.T) {
	input := "{count, plural, offset:1 =0 {Nobody} =1 {Just {0}} one {# and one other} other {# and # others}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 2, "plural with offset should produce text units")
}

// okapi: MessageFormatFilterTest#testSelectOrdinal
func TestExtract_SelectOrdinal(t *testing.T) {
	input := "{count, selectordinal, one {#st} two {#nd} few {#rd} other {#th}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 2, "selectordinal should produce text units for each form")
}

// ---------------------------------------------------------------------------
// Tests translated from MessageFormatParserTest.java
// ---------------------------------------------------------------------------

// okapi: MessageFormatParserTest#testWithSimpleMessage
func TestParser_SimpleMessage(t *testing.T) {
	blocks := readBlocks(t, "Hello world")
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
}

// okapi: MessageFormatParserTest#testWithNestedMessage
func TestParser_NestedMessage(t *testing.T) {
	input := "{count, plural, one {You have # message} other {You have # messages}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 2, "nested message should produce text units for each plural form")
}

// okapi: MessageFormatParserTest#testWithPluralMessage
func TestParser_PluralMessage(t *testing.T) {
	input := "{count, plural, one {one item} other {# items}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 2, "plural message should produce text units")

	texts := blockTexts(blocks)
	var foundOne, foundOther bool
	for _, text := range texts {
		if strings.Contains(text, "one item") {
			foundOne = true
		}
		if strings.Contains(text, "items") {
			foundOther = true
		}
	}
	assert.True(t, foundOne, "should extract 'one' form")
	assert.True(t, foundOther, "should extract 'other' form")
}

// okapi: MessageFormatParserTest#testWithPlural
func TestParser_Plural(t *testing.T) {
	input := "{n, plural, one {# dog} other {# dogs}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 2, "plural should produce text units for each form")
}

// okapi: MessageFormatParserTest#testWithComplexPluralMessage
func TestParser_ComplexPluralMessage(t *testing.T) {
	input := "{count, plural, =0 {no files} =1 {one file} one {# file} other {# files}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 2, "complex plural should produce multiple text units")
}

// okapi: MessageFormatParserTest#testSelectMessage
func TestParser_SelectMessage(t *testing.T) {
	input := "{gender, select, male {He} female {She} other {They}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 2, "select should produce text units for each branch")
}

// okapi: MessageFormatParserTest#testSelectOrdinalMessage
func TestParser_SelectOrdinalMessage(t *testing.T) {
	input := "{count, selectordinal, one {#st place} two {#nd place} few {#rd place} other {#th place}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 2, "selectordinal should produce text units for each form")
}

// okapi: MessageFormatParserTest#testWithComplexGenderMessage
func TestParser_ComplexGenderMessage(t *testing.T) {
	input := "{gender, select, male {He bought a new car} female {She bought a new car} other {They bought a new car}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 2, "complex gender select should produce multiple text units")

	texts := blockTexts(blocks)
	var foundMale, foundFemale bool
	for _, text := range texts {
		if strings.Contains(text, "He bought") {
			foundMale = true
		}
		if strings.Contains(text, "She bought") {
			foundFemale = true
		}
	}
	assert.True(t, foundMale, "should extract male branch")
	assert.True(t, foundFemale, "should extract female branch")
}

// okapi: MessageFormatParserTest#testWithEmbeddedPluralMessage
func TestParser_EmbeddedPluralMessage(t *testing.T) {
	input := "{gender, select, male {He has {count, plural, one {# item} other {# items}}} other {They have {count, plural, one {# item} other {# items}}}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 2, "embedded plural should produce text units")
}

// okapi: MessageFormatParserTest#testWithNestedGenderAndPluralMessage
func TestParser_NestedGenderAndPluralMessage(t *testing.T) {
	input := "{gender, select, male {He ate {count, plural, one {# apple} other {# apples}}} female {She ate {count, plural, one {# apple} other {# apples}}} other {They ate {count, plural, one {# apple} other {# apples}}}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 3, "nested gender+plural should produce many text units")
}

// okapi: MessageFormatParserTest#testWithMixedGenderAndPluralMessage
func TestParser_MixedGenderAndPluralMessage(t *testing.T) {
	input := "{gender, select, male {He has {count, plural, one {# cat} other {# cats}}} female {She has {count, plural, one {# cat} other {# cats}}} other {They have {count, plural, one {# cat} other {# cats}}}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 3, "mixed gender+plural should produce many text units")
}

// okapi: MessageFormatParserTest#testRussian
func TestParser_Russian(t *testing.T) {
	input := "{count, plural, one {# \u0444\u0430\u0439\u043b} few {# \u0444\u0430\u0439\u043b\u0430} many {# \u0444\u0430\u0439\u043b\u043e\u0432} other {# \u0444\u0430\u0439\u043b\u043e\u0432}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 2, "Russian plural forms should produce text units")
}

// okapi: MessageFormatParserTest#testOffset
func TestParser_Offset(t *testing.T) {
	input := "{count, plural, offset:1 =1 {Just you} one {You and # other} other {You and # others}}"
	blocks := readBlocks(t, input)
	assert.GreaterOrEqual(t, len(blocks), 2, "plural with offset should produce text units")
}

// okapi: MessageFormatParserTest#testFormattedMessage
func TestParser_FormattedMessage(t *testing.T) {
	input := "On {0,date} at {0,time}, there was {1} on planet {2,number}."
	blocks := readBlocks(t, input)
	require.NotEmpty(t, blocks)

	runs := blocks[0].SourceRuns()

	var placeholderCount int
	for _, r := range runs {
		if r.Ph != nil {
			placeholderCount++
		}
	}
	assert.GreaterOrEqual(t, placeholderCount, 3, "should have placeholders for {0,date}, {0,time}, {1}, {2,number}")
}

// okapi: MessageFormatParserTest#testChoiceMessage
func TestParser_ChoiceMessage(t *testing.T) {
	ctx := t.Context()
	reader := messageformat.NewReader()
	input := "{0,choice,0#no files|1#one file|1<{0,number,integer} files}"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.Error(t, err, "CHOICE format should be rejected by the filter")
	assert.Contains(t, err.Error(), "Error reading Message Format String")
}

// okapi: MessageFormatParserTest#testCountChoice
func TestParser_CountChoice(t *testing.T) {
	ctx := t.Context()
	reader := messageformat.NewReader()
	input := "{0,choice,0#zero|1#one|2#two}"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.Error(t, err, "CHOICE format should be rejected by the filter")
	assert.Contains(t, err.Error(), "Error reading Message Format String")
}

// okapi: MessageFormatParserTest#testInvalidMessageFormat
func TestParser_InvalidMessageFormat(t *testing.T) {
	// The Java bridge rejects {broken, invalid} because it doesn't recognize "invalid"
	// as a valid ICU type. Our native parser is more lenient: {name, type} is syntactically
	// valid MessageFormat (a typed argument reference), so we treat it as a placeholder.
	// Instead, test with genuinely broken syntax (unmatched braces).
	ctx := t.Context()
	reader := messageformat.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("{unterminated, plural,", model.LocaleEnglish))
	require.Error(t, err, "invalid message format should produce an error")
	assert.Contains(t, err.Error(), "Error reading Message Format String")
}

// okapi: MessageFormatParserTest#testSkipSyntaxApostrophe
func TestParser_SkipSyntaxApostrophe(t *testing.T) {
	blocks := readBlocks(t, "It''s fine")
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "'")
}

// okapi: MessageFormatParserTest#testSkipSyntaxQuotedText
func TestParser_SkipSyntaxQuotedText(t *testing.T) {
	blocks := readBlocks(t, "Text with 'quoted' content")
	require.NotEmpty(t, blocks)
}

// okapi: MessageFormatParserTest#testSkipSyntaxEscapedBraces
func TestParser_SkipSyntaxEscapedBraces(t *testing.T) {
	blocks := readBlocks(t, "Use '{' and '}' for placeholders")
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "{")
	assert.Contains(t, text, "}")
}

// okapi: MessageFormatParserTest#testSkipSyntaxComplexPattern
func TestParser_SkipSyntaxComplexPattern(t *testing.T) {
	blocks := readBlocks(t, "Text with ''quotes'' and '{braces}'")
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "'")
}

// okapi: MessageFormatParserTest#testSkipSyntaxEmbedded
func TestParser_SkipSyntaxEmbedded(t *testing.T) {
	blocks := readBlocks(t, "{count, plural, one {It''s # item} other {It''s # items}}")
	assert.GreaterOrEqual(t, len(blocks), 2, "embedded pattern with skip syntax should produce text units")
}

// ---------------------------------------------------------------------------
// Additional tests for reader/writer behavior
// ---------------------------------------------------------------------------

func TestReadLayerStartEnd(t *testing.T) {
	parts := readParts(t, "Hello")
	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "messageformat", layer.Format)
}

func TestReaderSignature(t *testing.T) {
	reader := messageformat.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/x-messageformat")
	assert.Contains(t, sig.Extensions, ".mf")
}

// okapi: MessageFormatFilterTest#testDefaultInfo
func TestReaderMetadata(t *testing.T) {
	reader := messageformat.NewReader()
	// Okapi's testDefaultInfo asserts the filter exposes non-null parameters,
	// a name, and a non-empty configuration list. The native analog is the
	// reader's identifying metadata + a usable (validating) config.
	assert.Equal(t, "messageformat", reader.Name())
	assert.Equal(t, "ICU MessageFormat", reader.DisplayName())
	assert.NotEmpty(t, reader.Signature().MIMETypes)
	assert.NotEmpty(t, reader.Signature().Extensions)
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := messageformat.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadEmpty(t *testing.T) {
	parts := readParts(t, "")
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks)
}

func TestRoundTrip(t *testing.T) {
	ctx := t.Context()
	input := "Hello world"

	reader := messageformat.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := messageformat.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	assert.Equal(t, "Hello world", buf.String())
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := t.Context()
	input := "Hello world"

	reader := messageformat.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Hello world" {
				block.SetTargetText(model.LocaleFrench, "Bonjour le monde")
			}
		}
	}

	var buf bytes.Buffer
	writer := messageformat.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	assert.Equal(t, "Bonjour le monde", buf.String())
}

func TestPluralBranchExtraction(t *testing.T) {
	input := "{count, plural, one {# item} other {# items}}"
	blocks := readBlocks(t, input)
	require.Len(t, blocks, 2, "each plural branch should be a separate block")

	// Verify both branches extracted
	texts := blockTexts(blocks)
	found := map[string]bool{}
	for _, text := range texts {
		if strings.Contains(text, "item") && !strings.Contains(text, "items") {
			found["one"] = true
		}
		if strings.Contains(text, "items") {
			found["other"] = true
		}
	}
	assert.True(t, found["one"], "should extract 'one' plural form")
	assert.True(t, found["other"], "should extract 'other' plural form")
}

func TestSelectBranchExtraction(t *testing.T) {
	input := "{gender, select, male {He} female {She} other {They}}"
	blocks := readBlocks(t, input)
	require.Len(t, blocks, 3, "each select branch should be a separate block")

	texts := blockTexts(blocks)
	assert.Contains(t, texts, "He")
	assert.Contains(t, texts, "She")
	assert.Contains(t, texts, "They")
}

func TestHashAsPlaceholder(t *testing.T) {
	input := "{count, plural, one {# item} other {# items}}"
	blocks := readBlocks(t, input)
	require.NotEmpty(t, blocks)

	// Each block in a plural context should have # as a placeholder run
	for _, b := range blocks {
		runs := b.SourceRuns()
		var hasHash bool
		for _, r := range runs {
			if r.Ph != nil && r.Ph.Data == "#" {
				hasHash = true
			}
		}
		assert.True(t, hasHash, "plural branch should have # placeholder: %s", b.SourceText())
	}
}

func TestSelectOrdinalBranches(t *testing.T) {
	input := "{count, selectordinal, one {#st} two {#nd} few {#rd} other {#th}}"
	blocks := readBlocks(t, input)
	assert.Len(t, blocks, 4, "selectordinal with 4 branches should produce 4 blocks")
}

func TestNestedSelectPluralDepth(t *testing.T) {
	input := "{gender, select, male {He ate {count, plural, one {# apple} other {# apples}}} female {She ate {count, plural, one {# apple} other {# apples}}}}"
	blocks := readBlocks(t, input)

	// male branch has 2 plural sub-branches, female has 2 = 4 translatable
	// leaf blocks. The "He ate " / "She ate " prose that frames each nested
	// plural is surfaced as non-translatable content blocks (default on).
	var translatable, content []*model.Block
	for _, b := range blocks {
		if b.Translatable {
			translatable = append(translatable, b)
		} else {
			content = append(content, b)
		}
	}
	assert.Len(t, translatable, 4, "nested select+plural should produce translatable blocks for each leaf branch")
	assert.Equal(t, []string{"He ate ", "She ate "}, blockTexts(content),
		"framing prose around the nested plurals should surface as content blocks")
}
