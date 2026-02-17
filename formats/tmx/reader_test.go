package tmx_test

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/gokapi/gokapi/model"
	"github.com/gokapi/gokapi/formats/tmx"
	"github.com/gokapi/gokapi/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadSimpleTMX(t *testing.T) {
	ctx := context.Background()
	reader := tmx.NewReader()

	f, err := os.Open("testdata/simple.tmx")
	require.NoError(t, err)
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.tmx", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
	assert.Equal(t, "Goodbye", blocks[1].SourceText())

	// Check French translations
	assert.True(t, blocks[0].HasTarget("fr"))
	assert.Equal(t, "Bonjour le monde", blocks[0].TargetText("fr"))
	assert.True(t, blocks[1].HasTarget("fr"))
	assert.Equal(t, "Au revoir", blocks[1].TargetText("fr"))
}

func TestReadMultipleLanguages(t *testing.T) {
	ctx := context.Background()
	reader := tmx.NewReader()

	f, err := os.Open("testdata/simple.tmx")
	require.NoError(t, err)
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.tmx", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)

	// Second TU has German too
	assert.True(t, blocks[1].HasTarget("de"))
	assert.Equal(t, "Auf Wiedersehen", blocks[1].TargetText("de"))
}

func TestReadTMXFromString(t *testing.T) {
	ctx := context.Background()
	reader := tmx.NewReader()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header srclang="en" datatype="plaintext"/>
  <body>
    <tu tuid="greeting">
      <tuv xml:lang="en"><seg>Hello</seg></tuv>
      <tuv xml:lang="es"><seg>Hola</seg></tuv>
    </tu>
  </body>
</tmx>`

	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Equal(t, "greeting", blocks[0].Name)
	assert.True(t, blocks[0].HasTarget("es"))
	assert.Equal(t, "Hola", blocks[0].TargetText("es"))
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := context.Background()
	reader := tmx.NewReader()
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header srclang="en"/>
  <body>
    <tu tuid="tu1"><tuv xml:lang="en"><seg>Hello</seg></tuv></tu>
  </body>
</tmx>`

	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "tmx", layer.Format)
	assert.True(t, layer.IsMultilingual)
}

func TestTMXHeaderAsData(t *testing.T) {
	ctx := context.Background()
	reader := tmx.NewReader()
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header srclang="en" datatype="plaintext" creationtool="gokapi"/>
  <body>
    <tu tuid="tu1"><tuv xml:lang="en"><seg>Hello</seg></tuv></tu>
  </body>
</tmx>`

	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	hasHeader := false
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Name == "tmx-header" {
				hasHeader = true
				assert.Equal(t, "en", data.Properties["srclang"])
				assert.Equal(t, "plaintext", data.Properties["datatype"])
			}
		}
	}
	assert.True(t, hasHeader, "TMX header should be emitted as Data")
}

func TestReaderSignature(t *testing.T) {
	reader := tmx.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/x-tmx+xml")
	assert.Contains(t, sig.Extensions, ".tmx")
}

func TestReaderMetadata(t *testing.T) {
	reader := tmx.NewReader()
	assert.Equal(t, "tmx", reader.Name())
	assert.Equal(t, "TMX", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := context.Background()
	reader := tmx.NewReader()
	err := reader.Open(ctx, nil)
	assert.Error(t, err)
}

func TestReadEmpty(t *testing.T) {
	ctx := context.Background()
	reader := tmx.NewReader()
	input := `<?xml version="1.0"?>
<tmx version="1.4">
  <header srclang="en"/>
  <body></body>
</tmx>`

	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks)
}

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()

	f, err := os.Open("testdata/simple.tmx")
	require.NoError(t, err)
	reader := tmx.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.tmx", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := tmx.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "<tmx")
	assert.Contains(t, output, "Hello World")
	assert.Contains(t, output, "Bonjour le monde")
	assert.Contains(t, output, "Goodbye")
	assert.Contains(t, output, "Au revoir")
	assert.Contains(t, output, "Auf Wiedersehen")
}

func TestRoundTripReread(t *testing.T) {
	ctx := context.Background()

	input := `<?xml version="1.0" encoding="UTF-8"?>
<tmx version="1.4">
  <header srclang="en" datatype="plaintext"/>
  <body>
    <tu tuid="tu1">
      <tuv xml:lang="en"><seg>Hello</seg></tuv>
      <tuv xml:lang="fr"><seg>Bonjour</seg></tuv>
    </tu>
  </body>
</tmx>`

	// Read
	reader := tmx.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write
	var buf bytes.Buffer
	writer := tmx.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)
	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	// Re-read the written output
	reader2 := tmx.NewReader()
	err = reader2.Open(ctx, testutil.RawDocFromString(buf.String(), model.LocaleEnglish))
	require.NoError(t, err)
	blocks := testutil.CollectBlocks(t, reader2.Read(ctx))
	reader2.Close()

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.True(t, blocks[0].HasTarget("fr"))
	assert.Equal(t, "Bonjour", blocks[0].TargetText("fr"))
}
