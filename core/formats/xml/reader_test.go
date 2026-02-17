package xml_test

import (
	"context"
	"testing"

	xmlfmt "github.com/gokapi/gokapi/core/formats/xml"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadSimpleXML(t *testing.T) {
	ctx := context.Background()
	reader := xmlfmt.NewReader()
	input := `<?xml version="1.0"?><root><message>Hello World</message></root>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
	assert.Equal(t, "root.message", blocks[0].Name)
}

func TestReadMultipleElements(t *testing.T) {
	ctx := context.Background()
	reader := xmlfmt.NewReader()
	input := `<?xml version="1.0"?><resources><string>Title</string><string>Description</string></resources>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Title", blocks[0].SourceText())
	assert.Equal(t, "Description", blocks[1].SourceText())
}

func TestReadNestedXML(t *testing.T) {
	ctx := context.Background()
	reader := xmlfmt.NewReader()
	input := `<?xml version="1.0"?><root><section><title>Section Title</title></section></root>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Section Title", blocks[0].SourceText())
	assert.Equal(t, "root.section.title", blocks[0].Name)
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := context.Background()
	reader := xmlfmt.NewReader()
	input := `<?xml version="1.0"?><root><msg>Test</msg></root>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 2)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "xml", layer.Format)
}

func TestReadWithTranslatableConfig(t *testing.T) {
	ctx := context.Background()
	reader := xmlfmt.NewReader()

	cfg := &xmlfmt.Config{
		TranslatableElements: []string{"title", "description"},
	}
	err := reader.SetConfig(cfg)
	require.NoError(t, err)

	input := `<?xml version="1.0"?><root><title>Hello</title><id>123</id><description>World</description></root>`
	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Len(t, blocks, 2)
	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello")
	assert.Contains(t, texts, "World")
}

func TestReaderSignature(t *testing.T) {
	reader := xmlfmt.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/xml")
	assert.Contains(t, sig.Extensions, ".xml")
}

func TestReaderMetadata(t *testing.T) {
	reader := xmlfmt.NewReader()
	assert.Equal(t, "xml", reader.Name())
	assert.Equal(t, "XML", reader.DisplayName())
}

func TestReadEmpty(t *testing.T) {
	ctx := context.Background()
	reader := xmlfmt.NewReader()
	input := `<?xml version="1.0"?><root></root>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	assert.Empty(t, blocks)
}

func TestReadNilDocument(t *testing.T) {
	ctx := context.Background()
	reader := xmlfmt.NewReader()
	err := reader.Open(ctx, nil)
	assert.Error(t, err)
}
