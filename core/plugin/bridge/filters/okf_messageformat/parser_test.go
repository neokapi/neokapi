//go:build integration

package okf_messageformat

import (
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Tests translated from MessageFormatParserTest.java (22 tests)
//
// The Java parser tests verify internal parsing behavior. We map them to
// bridge extraction tests that exercise the same message patterns through
// the filter, validating that the parser correctly segments and extracts
// text units from each pattern type.
// ---------------------------------------------------------------------------

// okapi: MessageFormatParserTest#testWithSimpleMessage
func TestParser_SimpleMessage(t *testing.T) {
	parts := readMessageFormatDefault(t, "Hello world")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
}

// okapi: MessageFormatParserTest#testWithNestedMessage
func TestParser_NestedMessage(t *testing.T) {
	input := "{count, plural, one {You have # message} other {You have # messages}}"
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 2, "nested message should produce text units for each plural form")
}

// okapi: MessageFormatParserTest#testWithPluralMessage
func TestParser_PluralMessage(t *testing.T) {
	input := "{count, plural, one {one item} other {# items}}"
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 2, "plural message should produce text units")

	texts := bridgetest.BlockTexts(blocks)
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
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 2, "plural should produce text units for each form")
}

// okapi: MessageFormatParserTest#testWithComplexPluralMessage
func TestParser_ComplexPluralMessage(t *testing.T) {
	input := "{count, plural, =0 {no files} =1 {one file} one {# file} other {# files}}"
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 2, "complex plural should produce multiple text units")
}

// okapi: MessageFormatParserTest#testSelectMessage
func TestParser_SelectMessage(t *testing.T) {
	input := "{gender, select, male {He} female {She} other {They}}"
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 2, "select should produce text units for each branch")
}

// okapi: MessageFormatParserTest#testSelectOrdinalMessage
func TestParser_SelectOrdinalMessage(t *testing.T) {
	input := "{count, selectordinal, one {#st place} two {#nd place} few {#rd place} other {#th place}}"
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 2, "selectordinal should produce text units for each form")
}

// okapi: MessageFormatParserTest#testWithComplexGenderMessage
func TestParser_ComplexGenderMessage(t *testing.T) {
	input := "{gender, select, male {He bought a new car} female {She bought a new car} other {They bought a new car}}"
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 2, "complex gender select should produce multiple text units")

	texts := bridgetest.BlockTexts(blocks)
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
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 2, "embedded plural should produce text units")
}

// okapi: MessageFormatParserTest#testWithNestedGenderAndPluralMessage
func TestParser_NestedGenderAndPluralMessage(t *testing.T) {
	input := "{gender, select, male {He ate {count, plural, one {# apple} other {# apples}}} female {She ate {count, plural, one {# apple} other {# apples}}} other {They ate {count, plural, one {# apple} other {# apples}}}}"
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 3, "nested gender+plural should produce many text units")
}

// okapi: MessageFormatParserTest#testWithMixedGenderAndPluralMessage
func TestParser_MixedGenderAndPluralMessage(t *testing.T) {
	input := "{gender, select, male {He has {count, plural, one {# cat} other {# cats}}} female {She has {count, plural, one {# cat} other {# cats}}} other {They have {count, plural, one {# cat} other {# cats}}}}"
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 3, "mixed gender+plural should produce many text units")
}

// okapi: MessageFormatParserTest#testRussian
func TestParser_Russian(t *testing.T) {
	// Russian locale with more plural forms.
	input := "{count, plural, one {# \u0444\u0430\u0439\u043b} few {# \u0444\u0430\u0439\u043b\u0430} many {# \u0444\u0430\u0439\u043b\u043e\u0432} other {# \u0444\u0430\u0439\u043b\u043e\u0432}}"
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 2, "Russian plural forms should produce text units")
}

// okapi: MessageFormatParserTest#testOffset
func TestParser_Offset(t *testing.T) {
	input := "{count, plural, offset:1 =1 {Just you} one {You and # other} other {You and # others}}"
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 2, "plural with offset should produce text units")
}

// okapi: MessageFormatParserTest#testFormattedMessage
func TestParser_FormattedMessage(t *testing.T) {
	input := "On {0,date} at {0,time}, there was {1} on planet {2,number}."
	parts := readMessageFormatDefault(t, input)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Should have placeholders for the formatted arguments.
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)

	var placeholderCount int
	for _, s := range frag.Spans {
		if s.SpanType == model.SpanPlaceholder {
			placeholderCount++
		}
	}
	assert.GreaterOrEqual(t, placeholderCount, 3, "should have placeholders for {0,date}, {0,time}, {1}, {2,number}")
}

// okapi: MessageFormatParserTest#testChoiceMessage
func TestParser_ChoiceMessage(t *testing.T) {
	// Choice format (legacy Java MessageFormat) is deprecated in ICU.
	// The Okapi filter rejects it with OkapiBadFilterInputException:
	// "CHOICE format is deprecated and is not supported".
	input := "{0,choice,0#no files|1#one file|1<{0,number,integer} files}"
	err := readMessageFormatExpectError(t, input)
	require.Error(t, err, "CHOICE format should be rejected by the filter")
	assert.Contains(t, err.Error(), "Error reading Message Format String")
}

// okapi: MessageFormatParserTest#testCountChoice
func TestParser_CountChoice(t *testing.T) {
	// Choice format is deprecated in ICU and rejected by the Okapi filter.
	input := "{0,choice,0#zero|1#one|2#two}"
	err := readMessageFormatExpectError(t, input)
	require.Error(t, err, "CHOICE format should be rejected by the filter")
	assert.Contains(t, err.Error(), "Error reading Message Format String")
}

// okapi: MessageFormatParserTest#testInvalidMessageFormat
func TestParser_InvalidMessageFormat(t *testing.T) {
	// Invalid format with unmatched braces. The Java parser throws
	// IllegalArgumentException; the bridge wraps it as
	// OkapiBadFilterInputException during Open.
	err := readMessageFormatExpectError(t, "{broken, invalid}")
	require.Error(t, err, "invalid message format should produce an error")
	assert.Contains(t, err.Error(), "Error reading Message Format String")
}

// okapi: MessageFormatParserTest#testSkipSyntaxApostrophe
func TestParser_SkipSyntaxApostrophe(t *testing.T) {
	// Single apostrophe as escape character.
	parts := readMessageFormatDefault(t, "It''s fine")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "'")
}

// okapi: MessageFormatParserTest#testSkipSyntaxQuotedText
func TestParser_SkipSyntaxQuotedText(t *testing.T) {
	// Quoted text: 'literal' passes through as literal.
	parts := readMessageFormatDefault(t, "Text with 'quoted' content")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: MessageFormatParserTest#testSkipSyntaxEscapedBraces
func TestParser_SkipSyntaxEscapedBraces(t *testing.T) {
	// Escaped braces: '{' and '}' pass through as literals.
	parts := readMessageFormatDefault(t, "Use '{' and '}' for placeholders")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "{")
	assert.Contains(t, text, "}")
}

// okapi: MessageFormatParserTest#testSkipSyntaxComplexPattern
func TestParser_SkipSyntaxComplexPattern(t *testing.T) {
	// Complex pattern with escaped quotes and braces.
	parts := readMessageFormatDefault(t, "Text with ''quotes'' and '{braces}'")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "'")
}

// okapi: MessageFormatParserTest#testSkipSyntaxEmbedded
func TestParser_SkipSyntaxEmbedded(t *testing.T) {
	// Embedded pattern with skip syntax.
	parts := readMessageFormatDefault(t, "{count, plural, one {It''s # item} other {It''s # items}}")

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.GreaterOrEqual(t, len(blocks), 2, "embedded pattern with skip syntax should produce text units")
}
