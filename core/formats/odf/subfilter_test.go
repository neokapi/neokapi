package odf_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/odf"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testResolver returns a mock SubfilterResolver for testing.
type testResolver struct{}

func (t *testResolver) ResolveReader(name string) (format.DataFormatReader, error) {
	return &fakeXMLReader{}, nil
}

func (t *testResolver) ResolveWriter(name string) (format.DataFormatWriter, error) {
	return &fakeXMLWriter{}, nil
}

// fakeXMLReader emits the raw content as a single block, wrapped in layer markers.
type fakeXMLReader struct {
	format.BaseFormatReader
	doc *model.RawDocument
}

func (f *fakeXMLReader) Signature() format.FormatSignature {
	return format.FormatSignature{}
}

func (f *fakeXMLReader) Open(_ context.Context, doc *model.RawDocument) error {
	f.doc = doc
	return nil
}

func (f *fakeXMLReader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 8)
	go func() {
		defer close(ch)
		content, _ := io.ReadAll(f.doc.Reader)
		layer := &model.Layer{ID: "xml1", Format: "xml"}
		ch <- model.PartResult{Part: &model.Part{Type: model.PartLayerStart, Resource: layer}}
		block := model.NewBlock("tu1", string(content))
		ch <- model.PartResult{Part: &model.Part{Type: model.PartBlock, Resource: block}}
		ch <- model.PartResult{Part: &model.Part{Type: model.PartLayerEnd, Resource: layer}}
	}()
	return ch
}

func (f *fakeXMLReader) Close() error { return nil }

// fakeXMLWriter reconstructs content by concatenating block texts.
type fakeXMLWriter struct {
	format.BaseFormatWriter
}

func (f *fakeXMLWriter) Write(ctx context.Context, parts <-chan *model.Part) error {
	for part := range parts {
		if part.Type == model.PartBlock {
			if block, ok := part.Resource.(*model.Block); ok {
				text := block.SourceText()
				if !f.Locale.IsEmpty() && block.HasTarget(f.Locale) {
					text = block.TargetText(f.Locale)
				}
				if _, err := f.Output.Write([]byte(text)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func TestSubfilter_ReadODTWithXMLReader(t *testing.T) {
	ctx := t.Context()
	data := makeODFZip(mimeODT, simpleODTContent("Hello, World!", "Second paragraph"))

	reader := odf.NewReader()
	reader.SetSubfilterResolver(&testResolver{})

	doc := testutil.RawDocFromReader(bytes.NewReader(data), "test.odt", model.LocaleEnglish)
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	// Should have subfiltered child layer for content.xml
	var childLayers []*model.Layer
	var blocks []*model.Block
	for _, p := range parts {
		switch p.Type {
		case model.PartLayerStart:
			if layer, ok := p.Resource.(*model.Layer); ok && layer.ParentID != "" {
				childLayers = append(childLayers, layer)
			}
		case model.PartBlock:
			if block, ok := p.Resource.(*model.Block); ok {
				blocks = append(blocks, block)
			}
		}
	}

	// Should have at least one child layer for content.xml (and possibly styles.xml if present)
	require.GreaterOrEqual(t, len(childLayers), 1)
	assert.Equal(t, "xml", childLayers[0].Format)
	assert.Equal(t, "content.xml", childLayers[0].Name)
	assert.Equal(t, "odf", childLayers[0].Properties["subfilter.source"])

	// The fake XML reader emits the entire content.xml as a single block
	require.GreaterOrEqual(t, len(blocks), 1)
	assert.Contains(t, blocks[0].SourceText(), "Hello, World!")
}

func TestSubfilter_NoResolverFallsBack(t *testing.T) {
	ctx := t.Context()
	data := makeODFZip(mimeODT, simpleODTContent("Hello", "World"))

	reader := odf.NewReader()
	// No resolver — falls back to direct ODF parsing

	doc := testutil.RawDocFromReader(bytes.NewReader(data), "test.odt", model.LocaleEnglish)
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Equal(t, "World", blocks[1].SourceText())
}

func TestSubfilter_WithStylesXML(t *testing.T) {
	ctx := t.Context()
	contentXML := simpleODTContent("Content text")
	stylesXML := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-styles
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0">
<office:master-styles>
<text:p>Header text</text:p>
</office:master-styles>
</office:document-styles>`
	data := makeODFZipWithStyles(mimeODT, contentXML, stylesXML)

	reader := odf.NewReader()
	reader.SetSubfilterResolver(&testResolver{})

	doc := testutil.RawDocFromReader(bytes.NewReader(data), "test.odt", model.LocaleEnglish)
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	var childLayers []*model.Layer
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			if l, ok := p.Resource.(*model.Layer); ok && l.ParentID != "" {
				childLayers = append(childLayers, l)
			}
		}
	}

	// Should have two child layers: content.xml and styles.xml
	require.Len(t, childLayers, 2)
	assert.Equal(t, "content.xml", childLayers[0].Name)
	assert.Equal(t, "styles.xml", childLayers[1].Name)
}

func TestSubfilter_Roundtrip(t *testing.T) {
	ctx := t.Context()
	data := makeODFZip(mimeODT, simpleODTContent("Hello", "World"))
	resolver := &testResolver{}

	// Read with subfiltering
	reader := odf.NewReader()
	reader.SetSubfilterResolver(resolver)
	doc := testutil.RawDocFromReader(bytes.NewReader(data), "test.odt", model.LocaleEnglish)
	err := reader.Open(ctx, doc)
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write with subfiltering
	var buf bytes.Buffer
	writer := odf.NewWriter()
	writer.SetSubfilterResolver(resolver)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetOriginalContent(data)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	require.Greater(t, buf.Len(), 0, "output should not be empty")

	// Re-read the output (without subfiltering, to verify structure)
	reader2 := odf.NewReader()
	doc2 := testutil.RawDocFromReader(bytes.NewReader(buf.Bytes()), "test.odt", model.LocaleEnglish)
	err = reader2.Open(ctx, doc2)
	require.NoError(t, err)
	defer reader2.Close()

	blocks := testutil.CollectBlocks(t, reader2.Read(ctx))
	require.NotEmpty(t, blocks, "should have blocks from reconstructed ODF")
}

func TestSubfilter_SubfilterAwareInterface(t *testing.T) {
	reader := odf.NewReader()
	var _ format.SubfilterAware = reader

	writer := odf.NewWriter()
	var _ format.SubfilterAware = writer
}
