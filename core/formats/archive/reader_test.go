package archive_test

import (
	"archive/zip"
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/gokapi/gokapi/core/formats/archive"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeZip(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = io.WriteString(w, content)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

func rawDocFromBytes(data []byte, locale model.LocaleID) *model.RawDocument {
	return &model.RawDocument{
		URI:          "test://input.zip",
		SourceLocale: locale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}
}

func TestReadArchiveWithTextEntries(t *testing.T) {
	ctx := context.Background()
	data := makeZip(t, map[string]string{
		"hello.txt":   "Hello world\nSecond line",
		"readme.txt":  "Read me",
		"image.png":   "\x89PNG binary data",
	})

	reader := archive.NewReader()
	err := reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	// hello.txt has 2 lines, readme.txt has 1 line = 3 blocks
	require.Len(t, blocks, 3)

	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello world")
	assert.Contains(t, texts, "Second line")
	assert.Contains(t, texts, "Read me")
}

func TestReadArchiveLayerStructure(t *testing.T) {
	ctx := context.Background()
	data := makeZip(t, map[string]string{
		"doc.txt": "Some text",
	})

	reader := archive.NewReader()
	err := reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	// Root LayerStart, child LayerStart, Block, child LayerEnd, Root LayerEnd
	require.GreaterOrEqual(t, len(parts), 5)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	rootLayer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "archive", rootLayer.Format)
	assert.True(t, rootLayer.IsRoot())

	// Find child layer
	var childLayer *model.Layer
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			l := p.Resource.(*model.Layer)
			if l.ParentID != "" {
				childLayer = l
				break
			}
		}
	}
	require.NotNil(t, childLayer)
	assert.Equal(t, "doc.txt", childLayer.Name)
	assert.Equal(t, rootLayer.ID, childLayer.ParentID)
}

func TestReadArchiveBinaryAsData(t *testing.T) {
	ctx := context.Background()
	data := makeZip(t, map[string]string{
		"image.png": "binary data",
	})

	reader := archive.NewReader()
	err := reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks)

	// Should have a Data part for image.png
	var dataEntry *model.Data
	for _, p := range parts {
		if p.Type == model.PartData {
			dataEntry = p.Resource.(*model.Data)
		}
	}
	require.NotNil(t, dataEntry)
	assert.Equal(t, "image.png", dataEntry.Properties["entry"])
}

func TestReadArchiveWithFilePatterns(t *testing.T) {
	ctx := context.Background()
	data := makeZip(t, map[string]string{
		"readme.txt":  "Read me",
		"config.json": `{"key":"value"}`,
	})

	reader := archive.NewReader()
	err := reader.Config().ApplyMap(map[string]any{
		"filePatterns": []any{"*.json"},
	})
	require.NoError(t, err)

	err = reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	// Only JSON file should be treated as text
	require.Len(t, blocks, 1)
	assert.Equal(t, `{"key":"value"}`, blocks[0].SourceText())
}

func TestReadNilDocument(t *testing.T) {
	ctx := context.Background()
	reader := archive.NewReader()
	err := reader.Open(ctx, nil)
	assert.Error(t, err)
}

func TestReaderSignature(t *testing.T) {
	reader := archive.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/zip")
	assert.Contains(t, sig.Extensions, ".zip")
	assert.Equal(t, []byte{0x50, 0x4B, 0x03, 0x04}, sig.MagicBytes[0])
}

func TestReaderMetadata(t *testing.T) {
	reader := archive.NewReader()
	assert.Equal(t, "archive", reader.Name())
	assert.Equal(t, "ZIP Archive", reader.DisplayName())
}

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()
	entries := map[string]string{
		"hello.txt": "Hello world\nGoodbye",
		"image.png": "binary data",
	}
	data := makeZip(t, entries)

	reader := archive.NewReader()
	err := reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := archive.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetOriginalContent(data)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	// Read back the output archive
	reader2 := archive.NewReader()
	err = reader2.Open(ctx, rawDocFromBytes(buf.Bytes(), model.LocaleEnglish))
	require.NoError(t, err)
	defer reader2.Close()

	blocks := testutil.CollectBlocks(t, reader2.Read(ctx))
	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello world")
	assert.Contains(t, texts, "Goodbye")
}

func TestRoundTripWithTranslation(t *testing.T) {
	ctx := context.Background()
	data := makeZip(t, map[string]string{
		"msg.txt": "Hello",
	})

	reader := archive.NewReader()
	err := reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set translations
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Hello" {
				block.SetTargetText("fr", "Bonjour")
			}
		}
	}

	var buf bytes.Buffer
	writer := archive.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetOriginalContent(data)
	writer.SetLocale("fr")

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	// Verify translated content
	reader2 := archive.NewReader()
	err = reader2.Open(ctx, rawDocFromBytes(buf.Bytes(), model.LocaleEnglish))
	require.NoError(t, err)
	defer reader2.Close()

	blocks := testutil.CollectBlocks(t, reader2.Read(ctx))
	require.Len(t, blocks, 1)
	assert.Equal(t, "Bonjour", blocks[0].SourceText())
}

func TestConfigApplyMap(t *testing.T) {
	cfg := &archive.Config{}

	err := cfg.ApplyMap(map[string]any{
		"filePatterns": []any{"*.txt", "*.json"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"*.txt", "*.json"}, cfg.FilePatterns)
}

func TestConfigApplyMapUnknown(t *testing.T) {
	cfg := &archive.Config{}
	err := cfg.ApplyMap(map[string]any{
		"unknown": "value",
	})
	assert.Error(t, err)
}

func TestConfigReset(t *testing.T) {
	cfg := &archive.Config{FilePatterns: []string{"*.txt"}}
	cfg.Reset()
	assert.Nil(t, cfg.FilePatterns)
}
