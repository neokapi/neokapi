package srt_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/srt"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func srtSkeletonRoundtrip(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := srt.NewReader()
	writer := srt.NewWriter()

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

func TestSkeletonStore_ByteExact_BasicSRT(t *testing.T) {
	input := "1\n00:00:01,000 --> 00:00:04,000\nHello world\n\n2\n00:00:05,000 --> 00:00:08,000\nSecond subtitle\n"
	output := srtSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "basic SRT roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_CRLF(t *testing.T) {
	input := "1\r\n00:00:01,000 --> 00:00:04,000\r\nHello world\r\n\r\n2\r\n00:00:05,000 --> 00:00:08,000\r\nSecond subtitle\r\n"
	output := srtSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "CRLF line endings should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_MultiLineSubtitles(t *testing.T) {
	input := "1\n00:00:01,000 --> 00:00:04,000\nFirst line\nSecond line\n\n2\n00:00:05,000 --> 00:00:08,000\nAnother subtitle\n"
	output := srtSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "multi-line subtitles should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultiLineSubtitlesCRLF(t *testing.T) {
	input := "1\r\n00:00:01,000 --> 00:00:04,000\r\nFirst line\r\nSecond line\r\n\r\n2\r\n00:00:05,000 --> 00:00:08,000\r\nAnother subtitle\r\n"
	output := srtSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "multi-line CRLF subtitles should be byte-exact")
}

func TestSkeletonStore_ByteExact_NoTrailingNewline(t *testing.T) {
	input := "1\n00:00:01,000 --> 00:00:04,000\nHello world"
	output := srtSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "no trailing newline should be preserved")
}

func TestSkeletonStore_ByteExact_ThreeEntries(t *testing.T) {
	input := "1\n00:00:01,000 --> 00:00:04,000\nFirst\n\n2\n00:00:05,000 --> 00:00:08,000\nSecond\n\n3\n00:00:09,000 --> 00:00:12,000\nThird\n"
	output := srtSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "three entries should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "1\n00:00:01,000 --> 00:00:04,000\nHello\n\n2\n00:00:05,000 --> 00:00:08,000\nGoodbye\n"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := srt.NewReader()
	writer := srt.NewWriter()

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

	expected := "1\n00:00:01,000 --> 00:00:04,000\nBonjour\n\n2\n00:00:05,000 --> 00:00:08,000\nAu revoir\n"
	assert.Equal(t, expected, buf.String())
}

func TestSkeletonStore_WithTranslation_CRLF(t *testing.T) {
	input := "1\r\n00:00:01,000 --> 00:00:04,000\r\nHello\r\n\r\n2\r\n00:00:05,000 --> 00:00:08,000\r\nWorld\r\n"
	ctx := t.Context()
	locale := model.LocaleID("de")

	reader := srt.NewReader()
	writer := srt.NewWriter()

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

	// Structural line endings (CRLF) are preserved from skeleton;
	// only the translatable text is replaced.
	expected := "1\r\n00:00:01,000 --> 00:00:04,000\r\nHallo\r\n\r\n2\r\n00:00:05,000 --> 00:00:08,000\r\nWelt\r\n"
	assert.Equal(t, expected, buf.String())
}

func TestSkeletonStore_ByteExact_FormattingTags(t *testing.T) {
	input := "1\n00:00:01,000 --> 00:00:04,000\n<i>Italic text</i>\n\n2\n00:00:05,000 --> 00:00:08,000\n<b>Bold</b> and <u>underline</u>\n"
	output := srtSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "formatting tags should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_TestdataFile(t *testing.T) {
	input := "1\n00:00:01,000 --> 00:00:04,000\nHello, welcome to the show.\n\n2\n00:00:05,000 --> 00:00:08,000\nThis is the second subtitle.\n\n3\n00:00:09,000 --> 00:00:12,000\nAnd this is the third\nwith multiple lines.\n"
	output := srtSkeletonRoundtrip(t, input)
	assert.Equal(t, input, output, "testdata-style content should be byte-exact")
}
