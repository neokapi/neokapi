package mosestext_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/mosestext"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := mosestext.NewReader()
	writer := mosestext.NewWriter()

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

	reader := mosestext.NewReader()
	writer := mosestext.NewWriter()
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
// byte-identical to the buffered path, including a multi-line <mrk> segment.
func TestStreamingMatchesBuffered(t *testing.T) {
	inputs := []string{
		"Hello world",
		"Line 1\nLine 2\nLine 3",
		"Line 1\r\nLine 2\r\nLine 3",
		"Line 1\rLine 2\rLine 3",
		"Plain line\n\nAfter blank",
		"<mrk mtype=\"seg\">First\nSecond\nThird</mrk>\nPlain after",
		"<mrk mtype=\"seg\">One-liner</mrk>\n",
	}
	for _, in := range inputs {
		buffered := snippetRoundtripWithSkeleton(t, in)
		streaming := streamingRoundtrip(t, in)
		assert.Equal(t, buffered, streaming, "streaming must match buffered for %q", in)
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

func TestSkeletonStore_ByteExact_EmptyLines(t *testing.T) {
	input := "Line 1\n\nLine 2\n\n\nLine 3"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "empty lines between content should be preserved")
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

func TestSkeletonStore_ByteExact_EmptyInput(t *testing.T) {
	input := ""
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "empty input should produce empty output")
}

func TestSkeletonStore_ByteExact_SingleNewline(t *testing.T) {
	input := "\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "single newline should be preserved")
}

func TestSkeletonStore_ByteExact_CRLF_TrailingNewline(t *testing.T) {
	input := "Line 1\r\nLine 2\r\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "CRLF with trailing newline should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleEmptyLinesCRLF(t *testing.T) {
	input := "Line 1\r\n\r\nLine 2"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "empty CRLF lines should be preserved")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "Hello World\nGoodbye"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := mosestext.NewReader()
	writer := mosestext.NewWriter()

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

	reader := mosestext.NewReader()
	writer := mosestext.NewWriter()

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
