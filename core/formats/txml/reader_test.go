package txml_test

import (
	"bytes"
	"context"
	"testing"

	"github.com/gokapi/gokapi/core/formats/txml"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const simpleTXML = `<?xml version="1.0" encoding="utf-8"?>
<txml locale="en-US" targetlocale="fr-FR" version="1.0" datatype="xml">
<header/>
<body>
<segment segtype="block">
<source>Hello world</source>
<target>Bonjour le monde</target>
</segment>
<segment segtype="block">
<source>Goodbye</source>
<target>Au revoir</target>
</segment>
</body>
</txml>`

const sourceOnlyTXML = `<?xml version="1.0" encoding="utf-8"?>
<txml locale="en-US" targetlocale="de-DE" version="1.0" datatype="xml">
<header/>
<body>
<segment segtype="block">
<source>Source only text</source>
</segment>
</body>
</txml>`

const inlineTagsTXML = `<?xml version="1.0" encoding="utf-8"?>
<txml locale="en-US" targetlocale="fr-FR" version="1.0" datatype="xml">
<header/>
<body>
<segment segtype="block">
<source>Text with <ph>placeholder</ph> inside</source>
</segment>
</body>
</txml>`

func TestReadSimpleTXML(t *testing.T) {
	ctx := context.Background()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(simpleTXML, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello world", blocks[0].SourceText())
	assert.Equal(t, "Goodbye", blocks[1].SourceText())
	assert.True(t, blocks[0].HasTarget("fr-FR"))
	assert.Equal(t, "Bonjour le monde", blocks[0].TargetText("fr-FR"))
	assert.Equal(t, "Au revoir", blocks[1].TargetText("fr-FR"))
}

func TestReadSegType(t *testing.T) {
	ctx := context.Background()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(simpleTXML, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "block", blocks[0].Properties["segtype"])
}

func TestReadSourceOnly(t *testing.T) {
	ctx := context.Background()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sourceOnlyTXML, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Source only text", blocks[0].SourceText())
}

func TestReadInlineTags(t *testing.T) {
	ctx := context.Background()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(inlineTagsTXML, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Text with placeholder inside", blocks[0].SourceText())
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := context.Background()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(simpleTXML, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "txml", layer.Format)
	assert.Equal(t, model.LocaleID("en-US"), layer.Locale)
	assert.Equal(t, "fr-FR", layer.Properties["target-locale"])
}

func TestReaderSignature(t *testing.T) {
	reader := txml.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/x-txml+xml")
	assert.Contains(t, sig.Extensions, ".txml")
	assert.NotNil(t, sig.Sniff)
	assert.True(t, sig.Sniff([]byte(`<txml locale="en-US">`)))
	assert.False(t, sig.Sniff([]byte(`<html>`)))
}

func TestReaderMetadata(t *testing.T) {
	reader := txml.NewReader()
	assert.Equal(t, "txml", reader.Name())
	assert.Equal(t, "Trados XML", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	ctx := context.Background()
	reader := txml.NewReader()
	err := reader.Open(ctx, nil)
	assert.Error(t, err)
}

func TestReadEmpty(t *testing.T) {
	ctx := context.Background()
	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks)
}

func TestRoundTrip(t *testing.T) {
	ctx := context.Background()

	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(simpleTXML, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := txml.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale("fr-FR")

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Hello world")
	assert.Contains(t, output, "Bonjour le monde")
	assert.Contains(t, output, "<txml")
	assert.Contains(t, output, "en-US")
	assert.Contains(t, output, "fr-FR")
}

func TestRoundTripWithNewTarget(t *testing.T) {
	ctx := context.Background()

	reader := txml.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(sourceOnlyTXML, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Add target translations
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			block.SetTargetText("de-DE", "Nur Quelltext")
		}
	}

	var buf bytes.Buffer
	writer := txml.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale("de-DE")

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Source only text")
	assert.Contains(t, output, "Nur Quelltext")
}
