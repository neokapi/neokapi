package versifiedtext_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/versifiedtext"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func versifiedSkeletonRoundtrip(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := versifiedtext.NewReader()
	writer := versifiedtext.NewWriter()

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

// versifiedStreamingRoundtrip drives a concurrent streaming round-trip via a
// NewStreamingSkeletonStore, forwarding the reader's Parts into the writer while
// the reader is still producing. Output must match the buffered skeleton path.
func versifiedStreamingRoundtrip(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := versifiedtext.NewReader()
	writer := versifiedtext.NewWriter()
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

// TestVersifiedStreamingMatchesBuffered asserts the streaming skeleton path is
// byte-identical to the buffered path across a range of inputs.
func TestVersifiedStreamingMatchesBuffered(t *testing.T) {
	inputs := []string{
		"\\v1 In the beginning",
		"\\v1 First verse\n\\v2 Second verse",
		"\\v1 First\r\n\\v2 Second",
		"\\v1 First\n\n\\v2 After a stanza break",
		"A plain line\n\\v1 A verse\nAnother plain line",
		"\\v1 With trailing newline\n",
	}
	for _, in := range inputs {
		buffered := versifiedSkeletonRoundtrip(t, in)
		streaming := versifiedStreamingRoundtrip(t, in)
		assert.Equal(t, buffered, streaming, "streaming must match buffered for %q", in)
	}
}

// ---------------------------------------------------------------------------
// Byte-exact roundtrip tests
// ---------------------------------------------------------------------------

func TestSkeletonStore_ByteExact_SingleVerse(t *testing.T) {
	input := "\\v1 In the beginning was the Word."
	output := versifiedSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "single verse roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleVerses(t *testing.T) {
	input := "\\v1 First verse.\n\\v2 Second verse.\n\\v3 Third verse."
	output := versifiedSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "multiple verses roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_StanzaBreak(t *testing.T) {
	input := "\\v1 First verse.\n\\v2 Second verse.\n\n\\v3 Third verse."
	output := versifiedSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "stanza break roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_CRLF(t *testing.T) {
	input := "\\v1 First verse.\r\n\\v2 Second verse.\r\n\\v3 Third verse."
	output := versifiedSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "CRLF line endings should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_TrailingNewline(t *testing.T) {
	input := "\\v1 First verse.\n\\v2 Second verse.\n"
	output := versifiedSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "trailing newline should be preserved")
}

func TestSkeletonStore_ByteExact_NonVerseLines(t *testing.T) {
	input := "A plain line\nAnother plain line"
	output := versifiedSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "non-verse lines roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MixedContent(t *testing.T) {
	input := "Title of the poem\n\\v1 First verse.\n\\v2 Second verse.\n\nPlain line in stanza two."
	output := versifiedSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "mixed content roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_EmptyInput(t *testing.T) {
	input := ""
	output := versifiedSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "empty input should produce empty output")
}

func TestSkeletonStore_ByteExact_SpacedVerseMarker(t *testing.T) {
	input := "\\v 1 Text after spaced marker."
	output := versifiedSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "spaced verse marker roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_NumericDotMarker(t *testing.T) {
	input := "1. In the beginning."
	output := versifiedSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "numeric dot marker roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultiDigitVerse(t *testing.T) {
	input := "\\v12 Verse twelve."
	output := versifiedSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "multi-digit verse roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_SimpleVerFile(t *testing.T) {
	input := "\\v1 In the beginning was the Word.\n\\v2 And the Word was with God.\n\\v3 And the Word was God.\n\n\\v4 In him was life.\n\\v5 And the life was the light of men."
	output := versifiedSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "simple.ver content roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_CRLFStanzaBreak(t *testing.T) {
	input := "\\v1 First.\r\n\r\n\\v2 Second."
	output := versifiedSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "CRLF stanza break should be preserved")
}

// ---------------------------------------------------------------------------
// Translation test
// ---------------------------------------------------------------------------

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "\\v1 Hello\n\\v2 World"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := versifiedtext.NewReader()
	writer := versifiedtext.NewWriter()

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
			case "World":
				b.SetTargetText(locale, "Monde")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	// Verse markers preserved, only text content translated
	assert.Equal(t, "\\v1 Bonjour\n\\v2 Monde", buf.String())
}

func TestSkeletonStore_WithTranslation_CRLF(t *testing.T) {
	input := "\\v1 Hello\r\n\\v2 World"
	ctx := t.Context()
	locale := model.LocaleID("de")

	reader := versifiedtext.NewReader()
	writer := versifiedtext.NewWriter()

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

	// Line endings should be preserved even with translation
	assert.Equal(t, "\\v1 Hallo\r\n\\v2 Welt", buf.String())
}

func TestSkeletonStore_WithTranslation_NonVerse(t *testing.T) {
	input := "Title line\n\\v1 Hello"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := versifiedtext.NewReader()
	writer := versifiedtext.NewWriter()

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
			case "Title line":
				b.SetTargetText(locale, "Ligne de titre")
			case "Hello":
				b.SetTargetText(locale, "Bonjour")
			}
		}
	}

	var buf bytes.Buffer
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(ctx, ch))
	writer.Close()

	assert.Equal(t, "Ligne de titre\n\\v1 Bonjour", buf.String())
}
