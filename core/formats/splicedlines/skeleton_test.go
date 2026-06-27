package splicedlines_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/splicedlines"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := splicedlines.NewReader()
	writer := splicedlines.NewWriter()

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

// snippetRoundtripStreaming drives a concurrent streaming round-trip: a
// NewStreamingSkeletonStore wired into both reader and writer, the reader's
// Parts forwarded into the writer while the reader is still producing. Output
// must be byte-identical to the buffered skeleton path.
func snippetRoundtripStreaming(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := splicedlines.NewReader()
	writer := splicedlines.NewWriter()

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

// TestStreamingMatchesBuffered asserts the streaming skeleton path produces the
// same bytes as the buffered path across the byte-exact fixtures.
func TestStreamingMatchesBuffered(t *testing.T) {
	inputs := []string{
		"Hello world",
		"Line 1\nLine 2\nLine 3",
		"Line 1\r\nLine 2\r\nLine 3",
		"Line 1\\\nContinued\nLine 2",
		"Line 1\\\r\nContinued\r\nLine 2",
		"Line 1\n\nLine 2",
		"Line 1\nLine 2\n",
		"Line 1\nLine 2",
		"A\\\nB\\\nC\nD",
	}
	for _, in := range inputs {
		buffered := snippetRoundtripWithSkeleton(t, in)
		streaming := snippetRoundtripStreaming(t, in)
		assert.Equal(t, buffered, streaming, "streaming output must match buffered for %q", in)
		assert.Equal(t, in, streaming, "streaming output must be byte-exact for %q", in)
	}
}

func TestSkeletonStore_ByteExact_SimpleLine(t *testing.T) {
	input := "Hello world"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "single line roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleLines(t *testing.T) {
	input := "Line 1\nLine 2\nLine 3"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multiple lines with LF should be byte-exact")
}

func TestSkeletonStore_ByteExact_CRLF(t *testing.T) {
	input := "Line 1\r\nLine 2\r\nLine 3"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "CRLF line endings should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_Continuation(t *testing.T) {
	input := "Line 1\\\nContinued\nLine 2"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "continuation lines should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_ContinuationCRLF(t *testing.T) {
	input := "Line 1\\\r\nContinued\r\nLine 2"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "CRLF continuation lines should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_EmptyLines(t *testing.T) {
	input := "Line 1\n\nLine 2"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "empty lines should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_TrailingNewline(t *testing.T) {
	input := "Line 1\nLine 2\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "trailing newline should be preserved")
}

func TestSkeletonStore_ByteExact_NoTrailingNewline(t *testing.T) {
	input := "Line 1\nLine 2"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "no trailing newline should be preserved")
}

func TestSkeletonStore_ByteExact_MultipleContinuation(t *testing.T) {
	input := "A\\\nB\\\nC\nD"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multiple continuation lines should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "Hello World\nGoodbye"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := splicedlines.NewReader()
	writer := splicedlines.NewWriter()

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

	assert.Equal(t, "Bonjour le monde\nAu revoir", buf.String())
}

func TestSkeletonStore_WithTranslation_CRLF(t *testing.T) {
	input := "Hello\r\nWorld"
	ctx := t.Context()
	locale := model.LocaleID("de")

	reader := splicedlines.NewReader()
	writer := splicedlines.NewWriter()

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

	assert.Equal(t, "Hallo\r\nWelt", buf.String())
}
