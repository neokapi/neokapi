package xliff2_test

import (
	"bytes"
	"testing"

	"github.com/neokapi/neokapi/core/formats/xliff2"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const sampleXLIFF2 = `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="2.0" xmlns="urn:oasis:names:tc:xliff:document:2.0"
       srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="u1" name="greeting">
      <segment id="s1">
        <source>Hello World</source>
        <target>Bonjour le monde</target>
      </segment>
    </unit>
    <unit id="u2">
      <segment id="s1">
        <source>Goodbye</source>
        <target>Au revoir</target>
      </segment>
    </unit>
    <unit id="u3">
      <notes>
        <note>This needs review</note>
      </notes>
      <segment id="s1">
        <source>Welcome</source>
      </segment>
    </unit>
  </file>
</xliff>`

// okapi: XLIFF2FilterTest#testSimple
func TestReadXLIFF2(t *testing.T) {
	ctx := t.Context()
	reader := xliff2.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sampleXLIFF2, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 3)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
	assert.Equal(t, "Goodbye", blocks[1].SourceText())
	assert.Equal(t, "Welcome", blocks[2].SourceText())
}

func TestReadXLIFF2Targets(t *testing.T) {
	ctx := t.Context()
	reader := xliff2.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sampleXLIFF2, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	assert.True(t, blocks[0].HasTarget(model.LocaleFrench))
	assert.Equal(t, "Bonjour le monde", blocks[0].TargetText(model.LocaleFrench))
	assert.True(t, blocks[1].HasTarget(model.LocaleFrench))
	assert.False(t, blocks[2].HasTarget(model.LocaleFrench))
}

func TestReadXLIFF2UnitIDs(t *testing.T) {
	ctx := t.Context()
	reader := xliff2.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sampleXLIFF2, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	assert.Equal(t, "u1", blocks[0].ID)
	assert.Equal(t, "greeting", blocks[0].Name)
	assert.Equal(t, "u2", blocks[1].ID)
	assert.Equal(t, "u3", blocks[2].ID)
}

// okapi: XLIFF2FilterTest#testSimpleMeta
func TestReadXLIFF2Notes(t *testing.T) {
	ctx := t.Context()
	reader := xliff2.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sampleXLIFF2, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	notes := blocks[2].Notes()
	require.Len(t, notes, 1)
	assert.Equal(t, "This needs review", notes[0].Text)
}

func TestReadXLIFF2LayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := xliff2.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sampleXLIFF2, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

func TestWriteXLIFF2(t *testing.T) {
	ctx := t.Context()
	reader := xliff2.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sampleXLIFF2, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := xliff2.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)

	output := buf.String()
	assert.Contains(t, output, "<xliff")
	assert.Contains(t, output, "version=\"2.0\"")
	assert.Contains(t, output, "Hello World")
	assert.Contains(t, output, "Bonjour le monde")
}

func TestReaderSignature(t *testing.T) {
	reader := xliff2.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.Extensions, ".xlf")
	assert.NotNil(t, sig.Sniff)
}

func TestReaderMetadata(t *testing.T) {
	reader := xliff2.NewReader()
	assert.Equal(t, "xliff2", reader.Name())
	assert.Equal(t, "XLIFF 2.x", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := xliff2.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}
