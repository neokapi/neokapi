package xliff_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/formats/xliff"
	"github.com/gokapi/gokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleXLIFF = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file original="test.txt" source-language="en" target-language="fr" datatype="plaintext">
    <body>
      <trans-unit id="1">
        <source>Hello World</source>
        <target>Bonjour le monde</target>
      </trans-unit>
      <trans-unit id="2">
        <source>Goodbye</source>
        <target>Au revoir</target>
      </trans-unit>
      <trans-unit id="3">
        <source>Untranslated</source>
      </trans-unit>
    </body>
  </file>
</xliff>`

func TestReadXLIFF(t *testing.T) {
	ctx := context.Background()
	reader := xliff.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sampleXLIFF, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 3)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
	assert.Equal(t, "Goodbye", blocks[1].SourceText())
	assert.Equal(t, "Untranslated", blocks[2].SourceText())
}

func TestReadXLIFFTargets(t *testing.T) {
	ctx := context.Background()
	reader := xliff.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sampleXLIFF, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	assert.True(t, blocks[0].HasTarget(model.LocaleFrench))
	assert.Equal(t, "Bonjour le monde", blocks[0].TargetText(model.LocaleFrench))

	assert.True(t, blocks[1].HasTarget(model.LocaleFrench))
	assert.Equal(t, "Au revoir", blocks[1].TargetText(model.LocaleFrench))

	assert.False(t, blocks[2].HasTarget(model.LocaleFrench))
}

func TestReadXLIFFLayerStart(t *testing.T) {
	ctx := context.Background()
	reader := xliff.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sampleXLIFF, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 2)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

func TestReadXLIFFBlockIDs(t *testing.T) {
	ctx := context.Background()
	reader := xliff.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sampleXLIFF, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	assert.Equal(t, "1", blocks[0].ID)
	assert.Equal(t, "2", blocks[1].ID)
	assert.Equal(t, "3", blocks[2].ID)
}

func TestWriteXLIFF(t *testing.T) {
	ctx := context.Background()
	reader := xliff.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sampleXLIFF, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := xliff.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "<xliff")
	assert.Contains(t, output, "Hello World")
	assert.Contains(t, output, "Bonjour le monde")
	assert.Contains(t, output, "version=\"1.2\"")
}

func TestReaderSignature(t *testing.T) {
	reader := xliff.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.Extensions, ".xlf")
	assert.Contains(t, sig.Extensions, ".xliff")
}

func TestReaderMetadata(t *testing.T) {
	reader := xliff.NewReader()
	assert.Equal(t, "xliff", reader.Name())
	assert.Equal(t, "XLIFF 1.2", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := context.Background()
	reader := xliff.NewReader()
	err := reader.Open(ctx, nil)
	assert.Error(t, err)
}
