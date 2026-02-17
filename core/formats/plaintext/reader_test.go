package plaintext_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/gokapi/gokapi/core/formats/plaintext"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadSimple(t *testing.T) {
	ctx := context.Background()
	reader := plaintext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Hello world\nSecond line\nThird line", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Len(t, blocks, 3)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
	assert.Equal(t, "Second line", blocks[1].SourceText())
	assert.Equal(t, "Third line", blocks[2].SourceText())
}

func TestReadWithEmptyLines(t *testing.T) {
	ctx := context.Background()
	reader := plaintext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Line one\n\nLine three", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Len(t, blocks, 2)
	assert.Equal(t, "Line one", blocks[0].SourceText())
	assert.Equal(t, "Line three", blocks[1].SourceText())
}

func TestReadUnicode(t *testing.T) {
	ctx := context.Background()
	reader := plaintext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Hello 世界\nBonjour le monde\nこんにちは世界", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	assert.Len(t, blocks, 3)
	assert.Equal(t, "Hello 世界", blocks[0].SourceText())
	assert.Equal(t, "Bonjour le monde", blocks[1].SourceText())
	assert.Equal(t, "こんにちは世界", blocks[2].SourceText())
}

func TestReadEmpty(t *testing.T) {
	ctx := context.Background()
	reader := plaintext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks)
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := context.Background()
	reader := plaintext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Hello", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "plaintext", layer.Format)
}

func TestReaderSignature(t *testing.T) {
	reader := plaintext.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/plain")
	assert.Contains(t, sig.Extensions, ".txt")
}

func TestReaderMetadata(t *testing.T) {
	reader := plaintext.NewReader()
	assert.Equal(t, "plaintext", reader.Name())
	assert.Equal(t, "Plain Text", reader.DisplayName())
}

func TestRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{"simple", "testdata/simple.txt"},
		{"unicode", "testdata/unicode.txt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original, err := os.ReadFile(tt.file)
			require.NoError(t, err)

			ctx := context.Background()

			// Read
			f, err := os.Open(tt.file)
			require.NoError(t, err)
			reader := plaintext.NewReader()
			err = reader.Open(ctx, testutil.RawDocFromReader(f, tt.file, model.LocaleEnglish))
			require.NoError(t, err)

			parts := testutil.CollectParts(t, reader.Read(ctx))
			reader.Close()

			// Write
			var buf bytes.Buffer
			writer := plaintext.NewWriter()
			err = writer.SetOutputWriter(&buf)
			require.NoError(t, err)
			writer.SetLocale(model.LocaleEnglish)

			ch := testutil.PartsToChannel(parts)
			err = writer.Write(ctx, ch)
			require.NoError(t, err)
			writer.Close()

			assert.Equal(t, string(original), buf.String())
		})
	}
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	ctx := context.Background()

	reader := plaintext.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("Hello\nWorld", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set French targets
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

	// Write with French locale
	var buf bytes.Buffer
	writer := plaintext.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	assert.Equal(t, "Bonjour\nMonde", buf.String())
}

func TestReadNilDocument(t *testing.T) {
	ctx := context.Background()
	reader := plaintext.NewReader()
	err := reader.Open(ctx, nil)
	assert.Error(t, err)
}
