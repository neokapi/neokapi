package paraplaintext_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/paraplaintext"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := paraplaintext.NewReader()
	writer := paraplaintext.NewWriter()

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

func TestSkeletonStore_ByteExact_SingleParagraph(t *testing.T) {
	input := "Hello world"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "single paragraph roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_TwoParagraphs(t *testing.T) {
	input := "First paragraph\n\nSecond paragraph"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "two paragraphs should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultiLineParagraphs(t *testing.T) {
	input := "Line 1\nLine 2\n\nLine 3\nLine 4"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multi-line paragraphs should be byte-exact")
}

func TestSkeletonStore_ByteExact_CRLF(t *testing.T) {
	input := "Para one\r\n\r\nPara two"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "CRLF line endings should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_TrailingNewline(t *testing.T) {
	input := "Para one\n\nPara two\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "trailing newline should be preserved")
}

func TestSkeletonStore_ByteExact_EmptyInput(t *testing.T) {
	input := ""
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "empty input should produce empty output")
}

func TestSkeletonStore_ByteExact_TripleBlankLines(t *testing.T) {
	input := "Para one\n\n\nPara two"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "triple blank lines should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_TrailingNewlineCRLF(t *testing.T) {
	input := "Para one\r\n\r\nPara two\r\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "CRLF with trailing newline should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "Hello World\n\nGoodbye"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := paraplaintext.NewReader()
	writer := paraplaintext.NewWriter()

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

	assert.Equal(t, "Bonjour le monde\n\nAu revoir", buf.String())
}

func TestSkeletonStore_WithTranslation_CRLF(t *testing.T) {
	input := "Hello\r\n\r\nWorld"
	ctx := t.Context()
	locale := model.LocaleID("de")

	reader := paraplaintext.NewReader()
	writer := paraplaintext.NewWriter()

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
				b.SetTargetText(locale, "Hallo")
			case "World":
				b.SetTargetText(locale, "Welt")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Equal(t, "Hallo\r\n\r\nWelt", buf.String())
}
