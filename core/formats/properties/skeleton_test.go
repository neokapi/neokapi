package properties_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/properties"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skelRoundtrip(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := properties.NewReader()
	writer := properties.NewWriter()

	store, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer store.Close()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
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

func TestSkeletonStore_ByteExact_SimpleKeyValue(t *testing.T) {
	input := "app.title=Hello World"
	output := skelRoundtrip(t, input)
	assert.Equal(t, input, output, "simple key=value roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_ColonSeparator(t *testing.T) {
	input := "key:value"
	output := skelRoundtrip(t, input)
	assert.Equal(t, input, output, "key:value roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_SpaceSeparator(t *testing.T) {
	input := "key value"
	output := skelRoundtrip(t, input)
	assert.Equal(t, input, output, "key<space>value roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleEntries(t *testing.T) {
	input := "key1=value1\nkey2=value2\nkey3=value3"
	output := skelRoundtrip(t, input)
	assert.Equal(t, input, output, "multiple entries roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MixedSeparators(t *testing.T) {
	input := "key1=value1\nkey2:value2\nkey3 value3"
	output := skelRoundtrip(t, input)
	assert.Equal(t, input, output, "mixed separators roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_Comments(t *testing.T) {
	input := "# This is a comment\nkey=value"
	output := skelRoundtrip(t, input)
	assert.Equal(t, input, output, "comments should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_ExclamationComment(t *testing.T) {
	input := "! Another comment style\nkey=value"
	output := skelRoundtrip(t, input)
	assert.Equal(t, input, output, "exclamation comments should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_BlankLines(t *testing.T) {
	input := "key1=value1\n\nkey2=value2"
	output := skelRoundtrip(t, input)
	assert.Equal(t, input, output, "blank lines should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleBlankLines(t *testing.T) {
	input := "key1=value1\n\n\nkey2=value2"
	output := skelRoundtrip(t, input)
	assert.Equal(t, input, output, "multiple blank lines should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_TrailingNewline(t *testing.T) {
	input := "key1=value1\nkey2=value2\n"
	output := skelRoundtrip(t, input)
	assert.Equal(t, input, output, "trailing newline should be preserved")
}

func TestSkeletonStore_ByteExact_UnicodeEscapes(t *testing.T) {
	input := "greeting=\\u3053\\u3093\\u306B\\u3061\\u306F"
	output := skelRoundtrip(t, input)
	assert.Equal(t, input, output, "unicode escapes should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_ContinuationLines(t *testing.T) {
	input := "key=hello \\\n    world"
	output := skelRoundtrip(t, input)
	assert.Equal(t, input, output, "continuation lines should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleContinuationLines(t *testing.T) {
	input := "key=line1 \\\n    line2 \\\n    line3"
	output := skelRoundtrip(t, input)
	assert.Equal(t, input, output, "multiple continuation lines should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_EmptyValue(t *testing.T) {
	input := "key="
	output := skelRoundtrip(t, input)
	assert.Equal(t, input, output, "empty value should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_CRLF(t *testing.T) {
	input := "key1=value1\r\nkey2=value2"
	output := skelRoundtrip(t, input)
	assert.Equal(t, input, output, "CRLF line endings should be preserved byte-exact")
}

// Mirrors Okapi PropertiesFilterTest#testLineBreaks_CR: a bare-CR-separated
// pair extracts as two entries and round-trips byte-for-byte.
func TestSkeletonStore_ByteExact_CR(t *testing.T) {
	input := "key1=value1\rkey2=value2\r"
	output := skelRoundtrip(t, input)
	assert.Equal(t, input, output, "bare CR line endings should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_EmptyInput(t *testing.T) {
	input := ""
	output := skelRoundtrip(t, input)
	assert.Equal(t, input, output, "empty input should produce empty output")
}

func TestSkeletonStore_ByteExact_CommentsAndBlanksAndEntries(t *testing.T) {
	input := "# Header comment\n\napp.title=Hello World\napp.desc=A description\n\n# Section 2\nother.key=Other value"
	output := skelRoundtrip(t, input)
	assert.Equal(t, input, output, "complex file with comments, blanks, and entries should roundtrip byte-exact")
}

func TestSkeletonStore_ByteExact_SpacesAroundEquals(t *testing.T) {
	input := "key = value with spaces"
	output := skelRoundtrip(t, input)
	assert.Equal(t, input, output, "spaces around equals should be preserved byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "greeting=Hello\nfarewell=Goodbye"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := properties.NewReader()
	writer := properties.NewWriter()

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
			case "Hello":
				b.SetTargetText(locale, "Bonjour")
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

	assert.Equal(t, "greeting=Bonjour\nfarewell=Au revoir", buf.String())
}

func TestSkeletonStore_WithTranslation_UnicodeOutput(t *testing.T) {
	input := "key=Hello"
	ctx := t.Context()
	locale := model.LocaleID("ja")

	reader := properties.NewReader()
	writer := properties.NewWriter()

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
			b.SetTargetText(locale, "\u3053\u3093\u306B\u3061\u306F")
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	// Non-ASCII characters should be encoded as \uXXXX in output
	// (lowercase hex matches okapi/tikal convention).
	assert.Equal(t, "key=\\u3053\\u3093\\u306b\\u3061\\u306f", buf.String())
}

func TestSkeletonStore_WithTranslation_PreservesStructure(t *testing.T) {
	input := "# Comment\n\ngreeting=Hello\n"
	ctx := t.Context()
	locale := model.LocaleID("de")

	reader := properties.NewReader()
	writer := properties.NewWriter()

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
			b.SetTargetText(locale, "Hallo")
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	// Comments, blank lines, and trailing newline should be preserved
	assert.Equal(t, "# Comment\n\ngreeting=Hallo\n", buf.String())
}
