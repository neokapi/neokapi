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

// streamingRoundtrip drives a concurrent streaming round-trip via a
// NewStreamingSkeletonStore, forwarding the reader's Parts into the writer while
// the reader is still producing. Output must match the buffered skeleton path.
func streamingRoundtrip(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := paraplaintext.NewReader()
	writer := paraplaintext.NewWriter()
	store := format.NewStreamingSkeletonStore()
	reader.SetSkeletonStore(store)
	writer.SetSkeletonStore(store)

	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish)))

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	partsCh := make(chan *model.Part, 64)
	readCh := reader.Read(ctx)
	go func() {
		defer close(partsCh)
		for res := range readCh {
			if res.Error == nil && res.Part != nil {
				partsCh <- res.Part
			}
		}
		store.CloseWrite()
		reader.Close()
	}()

	require.NoError(t, writer.Write(ctx, partsCh))
	writer.Close()
	return buf.String()
}

// TestStreamingMatchesBuffered asserts the streaming skeleton path is
// byte-identical to the buffered path across the paragraph-grouping edge cases.
func TestStreamingMatchesBuffered(t *testing.T) {
	inputs := []string{
		"Hello world",
		"First paragraph\n\nSecond paragraph",
		"Line 1\nLine 2\n\nLine 3\nLine 4",
		"Para one\r\n\r\nPara two",
		"Para one\n\nPara two\n",
		"Para one\n\n\nPara two",
		"Para one\r\n\r\nPara two\r\n",
		"\n\nLeading blanks\n\nthen text",
	}
	for _, in := range inputs {
		buffered := snippetRoundtripWithSkeleton(t, in)
		streaming := streamingRoundtrip(t, in)
		assert.Equal(t, buffered, streaming, "streaming must match buffered for %q", in)
		assert.Equal(t, in, streaming, "streaming must be byte-exact for %q", in)
	}
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
