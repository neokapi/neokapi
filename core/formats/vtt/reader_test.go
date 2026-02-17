package vtt_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/formats/vtt"
	"github.com/gokapi/gokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadSimpleVTT(t *testing.T) {
	ctx := context.Background()
	reader := vtt.NewReader()
	input := "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nHello world\n\n00:00:05.000 --> 00:00:08.000\nSecond subtitle\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
	assert.Equal(t, "Second subtitle", blocks[1].SourceText())
	assert.Equal(t, "00:00:01.000 --> 00:00:04.000", blocks[0].Properties["timecode"])
	assert.Equal(t, "subtitle.1", blocks[0].Name)
}

func TestReadVTTWithCueIDs(t *testing.T) {
	ctx := context.Background()
	reader := vtt.NewReader()
	input := "WEBVTT\n\nintro\n00:00:01.000 --> 00:00:04.000\nHello world\n\nmain\n00:00:05.000 --> 00:00:08.000\nSecond subtitle\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
	assert.Equal(t, "intro", blocks[0].Properties["cue-id"])
	assert.Equal(t, "Second subtitle", blocks[1].SourceText())
	assert.Equal(t, "main", blocks[1].Properties["cue-id"])
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := context.Background()
	reader := vtt.NewReader()
	input := "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nHello\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "vtt", layer.Format)
}

func TestReaderSignature(t *testing.T) {
	reader := vtt.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/vtt")
	assert.Contains(t, sig.Extensions, ".vtt")
}

func TestReaderMetadata(t *testing.T) {
	reader := vtt.NewReader()
	assert.Equal(t, "vtt", reader.Name())
	assert.Equal(t, "WebVTT", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := context.Background()
	reader := vtt.NewReader()
	err := reader.Open(ctx, nil)
	assert.Error(t, err)
}

func TestReadEmpty(t *testing.T) {
	ctx := context.Background()
	reader := vtt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("WEBVTT\n", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks)
}

func TestVTTHeaderAsData(t *testing.T) {
	ctx := context.Background()
	reader := vtt.NewReader()
	input := "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nHello\n"
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	hasHeader := false
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Name == "vtt-header" {
				hasHeader = true
				assert.Equal(t, "WEBVTT", data.Properties["content"])
			}
		}
	}
	assert.True(t, hasHeader, "WEBVTT header should be emitted as Data")
}

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()

	input := "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nHello world\n\n00:00:05.000 --> 00:00:08.000\nSecond subtitle\n"

	reader := vtt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := vtt.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "WEBVTT")
	assert.Contains(t, output, "00:00:01.000 --> 00:00:04.000")
	assert.Contains(t, output, "Hello world")
	assert.Contains(t, output, "00:00:05.000 --> 00:00:08.000")
	assert.Contains(t, output, "Second subtitle")
}

func TestRoundTripWithCueIDs(t *testing.T) {
	ctx := context.Background()

	input := "WEBVTT\n\nintro\n00:00:01.000 --> 00:00:04.000\nHello world\n"

	reader := vtt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := vtt.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "WEBVTT")
	assert.Contains(t, output, "intro")
	assert.Contains(t, output, "Hello world")
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := context.Background()

	input := "WEBVTT\n\n00:00:01.000 --> 00:00:04.000\nHello\n\n00:00:05.000 --> 00:00:08.000\nWorld\n"

	reader := vtt.NewReader()
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
	writer := vtt.NewWriter()
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
