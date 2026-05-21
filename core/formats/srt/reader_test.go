package srt_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/formats/srt"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadSimpleSRT(t *testing.T) {
	ctx := t.Context()
	reader := srt.NewReader()
	input := "1\n00:00:01,000 --> 00:00:04,000\nHello world\n\n2\n00:00:05,000 --> 00:00:08,000\nSecond subtitle\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
	assert.Equal(t, "Second subtitle", blocks[1].SourceText())
	assert.Equal(t, "00:00:01,000 --> 00:00:04,000", blocks[0].Properties["timecode"])
	assert.Equal(t, "1", blocks[0].Properties["sequence"])
	assert.Equal(t, "subtitle.1", blocks[0].Name)
}

func TestReadMultiLineSRT(t *testing.T) {
	ctx := t.Context()
	reader := srt.NewReader()
	input := "1\n00:00:01,000 --> 00:00:04,000\nFirst line\nSecond line\n\n2\n00:00:05,000 --> 00:00:08,000\nAnother subtitle\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "First line\nSecond line", blocks[0].SourceText())
	assert.Equal(t, "Another subtitle", blocks[1].SourceText())
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := srt.NewReader()
	input := "1\n00:00:01,000 --> 00:00:04,000\nHello\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "srt", layer.Format)
}

func TestReaderSignature(t *testing.T) {
	reader := srt.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/x-subrip")
	assert.Contains(t, sig.Extensions, ".srt")
}

func TestReaderMetadata(t *testing.T) {
	reader := srt.NewReader()
	assert.Equal(t, "srt", reader.Name())
	assert.Equal(t, "SRT Subtitles", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := srt.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := srt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks)
}

func TestSequenceNumberAsData(t *testing.T) {
	ctx := t.Context()
	reader := srt.NewReader()
	input := "1\n00:00:01,000 --> 00:00:04,000\nHello\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	hasSeqData := false
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Properties["sequence"] == "1" {
				hasSeqData = true
			}
		}
	}
	assert.True(t, hasSeqData, "sequence number should be emitted as Data")
}

// okapi: RoundTripSrtIT#srtFiles — native extract→write over a real simple.srt fixture, asserting timecodes + subtitle text survive; Okapi's srtFiles does extract→merge→compare over an .srt corpus.
// okapi-skip: RoundTripSrtIT#srtSerializedFiles — Okapi serialized-skeleton variant; native uses its own skeleton store, not Okapi's serialized event/skeleton format.
func TestRoundTrip(t *testing.T) {
	ctx := t.Context()

	original, err := os.ReadFile("testdata/simple.srt")
	require.NoError(t, err)

	f, err := os.Open("testdata/simple.srt")
	require.NoError(t, err)
	reader := srt.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.srt", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := srt.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	// Verify key content is preserved
	assert.Contains(t, output, "Hello, welcome to the show.")
	assert.Contains(t, output, "00:00:01,000 --> 00:00:04,000")
	assert.Contains(t, output, "00:00:05,000 --> 00:00:08,000")
	_ = original
}

// okapi: SrtXliffCompareIT#srtXliffCompareFiles — native extract→write→re-extract verifies extracted subtitle content (text + timecode) is stable; Okapi's srtXliffCompareFiles extracts to XLIFF and compares against a gold XLIFF corpus.
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	ctx := t.Context()

	input := "1\n00:00:01,000 --> 00:00:04,000\nHello world\n\n2\n00:00:05,000 --> 00:00:08,000\nSecond subtitle\n"

	// First extraction.
	reader1 := srt.NewReader()
	err := reader1.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts1 := testutil.CollectParts(t, reader1.Read(ctx))
	reader1.Close()

	// Write.
	var buf bytes.Buffer
	writer := srt.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)
	err = writer.Write(ctx, testutil.PartsToChannel(parts1))
	require.NoError(t, err)
	writer.Close()

	// Second extraction from the written output.
	reader2 := srt.NewReader()
	err = reader2.Open(ctx, testutil.RawDocFromString(buf.String(), model.LocaleEnglish))
	require.NoError(t, err)
	parts2 := testutil.CollectParts(t, reader2.Read(ctx))
	reader2.Close()

	blocks1 := testutil.FilterBlocks(parts1)
	blocks2 := testutil.FilterBlocks(parts2)
	require.NotEmpty(t, blocks1, "first extraction should produce blocks")
	require.Equal(t, len(blocks1), len(blocks2), "double extraction should produce same block count")
	for i := range blocks1 {
		assert.Equal(t, blocks1[i].SourceText(), blocks2[i].SourceText(), "block %d source mismatch", i)
		assert.Equal(t, blocks1[i].Properties["timecode"], blocks2[i].Properties["timecode"], "block %d timecode mismatch", i)
	}
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := t.Context()

	input := "1\n00:00:01,000 --> 00:00:04,000\nHello\n\n2\n00:00:05,000 --> 00:00:08,000\nWorld\n"

	reader := srt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Hello" {
				block.SetTargetText(model.LocaleFrench, "Bonjour")
			} else if block.SourceText() == "World" {
				block.SetTargetText(model.LocaleFrench, "Monde")
			}
		}
	}

	var buf bytes.Buffer
	writer := srt.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Bonjour")
	assert.Contains(t, output, "Monde")
	assert.NotContains(t, output, "Hello")
	assert.NotContains(t, output, "World")
}
