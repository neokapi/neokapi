package vtt_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/vtt"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func snippetRoundtripWithSkeleton(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := vtt.NewReader()
	writer := vtt.NewWriter()

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

func TestSkeletonStore_ByteExact_SimpleCue(t *testing.T) {
	input := "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nHello world\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "simple cue roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultipleCues(t *testing.T) {
	input := "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nHello world\n\n00:00:05.000 --> 00:00:08.000\nSecond subtitle\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multiple cues roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_CRLF(t *testing.T) {
	input := "WEBVTT\r\n\r\n00:00:01.000 --> 00:00:04.000\r\nHello world\r\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "CRLF line endings should be preserved byte-exact")
}

func TestSkeletonStore_ByteExact_CueWithID(t *testing.T) {
	input := "WEBVTT\n\nintro\n00:00:01.000 --> 00:00:04.000\nHello world\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "cue with ID roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_CueSettings(t *testing.T) {
	input := "WEBVTT\n\n00:00:02.680 --> 00:00:04.720 align:middle line:84%\nThis is an orange.\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "cue settings roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_MultilineCueText(t *testing.T) {
	input := "WEBVTT\n\n00:00:00.000 --> 00:00:03.000\nLine one\nLine two\nLine three.\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multiline cue text roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_NoTrailingNewline(t *testing.T) {
	input := "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nHello world"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "no trailing newline should be preserved")
}

func TestSkeletonStore_ByteExact_MultipleCuesWithIDs(t *testing.T) {
	input := "WEBVTT\n\n1\n00:00:02.680 --> 00:00:04.720\nThanks everyone\nfor joining us today,\n\n2\n00:00:04.800 --> 00:00:06.960\nI am so excited\nto be with you.\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "multiple cues with IDs roundtrip should be byte-exact")
}

func TestSkeletonStore_ByteExact_EmptyHeaderOnly(t *testing.T) {
	input := "WEBVTT\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "header-only file should be byte-exact")
}

func TestSkeletonStore_ByteExact_ThreeCues(t *testing.T) {
	input := "WEBVTT\n\n00:00:02.680 --> 00:00:04.720\nThis is a hippopotamus,\n\n00:00:04.800 --> 00:00:06.960\na very large animal,\n\n00:00:06.960 --> 00:00:09.100\nthat lives in Africa.\n"
	output := snippetRoundtripWithSkeleton(t, input)
	assert.Equal(t, input, output, "three cues roundtrip should be byte-exact")
}

func TestSkeletonStore_WithTranslation(t *testing.T) {
	input := "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nHello\n\n00:00:05.000 --> 00:00:08.000\nWorld\n"
	ctx := t.Context()
	locale := model.LocaleID("fr")

	reader := vtt.NewReader()
	writer := vtt.NewWriter()

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

	expected := "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nBonjour\n\n00:00:05.000 --> 00:00:08.000\nMonde\n"
	assert.Equal(t, expected, buf.String())
}

func TestSkeletonStore_WithTranslation_CRLF(t *testing.T) {
	input := "WEBVTT\r\n\r\n00:00:01.000 --> 00:00:04.000\r\nHello\r\n\r\n00:00:05.000 --> 00:00:08.000\r\nWorld\r\n"
	ctx := t.Context()
	locale := model.LocaleID("de")

	reader := vtt.NewReader()
	writer := vtt.NewWriter()

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
	expected := "WEBVTT\r\n\r\n00:00:01.000 --> 00:00:04.000\r\nHallo\r\n\r\n00:00:05.000 --> 00:00:08.000\r\nWelt\r\n"
	assert.Equal(t, expected, buf.String())
}
