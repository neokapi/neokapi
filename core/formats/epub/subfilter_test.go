package epub_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/epub"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testResolver returns a mock SubfilterResolver for testing.
type testResolver struct{}

func (t *testResolver) ResolveReader(name string) (format.DataFormatReader, error) {
	return &fakeHTMLReader{}, nil
}

func (t *testResolver) ResolveWriter(name string) (format.DataFormatWriter, error) {
	return &fakeHTMLWriter{}, nil
}

// fakeHTMLReader emits the raw XHTML content as a single block, wrapped in layer markers.
type fakeHTMLReader struct {
	format.BaseFormatReader
	doc *model.RawDocument
}

func (f *fakeHTMLReader) Signature() format.FormatSignature {
	return format.FormatSignature{}
}

func (f *fakeHTMLReader) Open(_ context.Context, doc *model.RawDocument) error {
	f.doc = doc
	return nil
}

func (f *fakeHTMLReader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 8)
	go func() {
		defer close(ch)
		content, _ := io.ReadAll(f.doc.Reader)
		layer := &model.Layer{ID: "html1", Format: "html"}
		ch <- model.PartResult{Part: &model.Part{Type: model.PartLayerStart, Resource: layer}}
		block := model.NewBlock("tu1", string(content))
		ch <- model.PartResult{Part: &model.Part{Type: model.PartBlock, Resource: block}}
		ch <- model.PartResult{Part: &model.Part{Type: model.PartLayerEnd, Resource: layer}}
	}()
	return ch
}

func (f *fakeHTMLReader) Close() error { return nil }

// fakeHTMLWriter reconstructs content by concatenating block texts.
type fakeHTMLWriter struct {
	format.BaseFormatWriter
}

func (f *fakeHTMLWriter) Write(ctx context.Context, parts <-chan *model.Part) error {
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

func TestSubfilter_ReadEPUBWithHTMLReader(t *testing.T) {
	ctx := t.Context()
	data := makeEPUB(t)

	reader := epub.NewReader()
	reader.SetSubfilterResolver(&testResolver{})

	err := reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	// Should have subfiltered child layers for each chapter
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

	// Should have 2 child layers (one per chapter), both with "html" format
	require.Len(t, childLayers, 2)
	for _, l := range childLayers {
		assert.Equal(t, "html", l.Format)
		assert.Equal(t, "epub", l.Properties["subfilter.source"])
	}

	// Each chapter's full XHTML content should be a single block (from fake reader)
	require.Len(t, blocks, 2)
	// The fake reader emits the entire XHTML as one block, so blocks contain full XHTML
	for _, b := range blocks {
		assert.Contains(t, b.SourceText(), "<?xml")
	}
}

func TestSubfilter_NoResolverFallsBack(t *testing.T) {
	ctx := t.Context()
	data := makeEPUB(t)

	reader := epub.NewReader()
	// No resolver — falls back to direct XHTML extraction

	err := reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	texts := testutil.BlockTexts(blocks)

	// Should still extract text from block elements
	assert.Contains(t, texts, "Welcome")
	assert.Contains(t, texts, "This is the first paragraph.")
}

func TestSubfilter_Roundtrip(t *testing.T) {
	ctx := t.Context()
	data := makeEPUB(t)
	resolver := &testResolver{}

	// Read with subfiltering
	reader := epub.NewReader()
	reader.SetSubfilterResolver(resolver)
	err := reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write with subfiltering
	var buf bytes.Buffer
	writer := epub.NewWriter()
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

	// Re-read the output (without subfiltering, to see the content)
	reader2 := epub.NewReader()
	err = reader2.Open(ctx, rawDocFromBytes(buf.Bytes(), model.LocaleEnglish))
	require.NoError(t, err)
	defer reader2.Close()

	blocks := testutil.CollectBlocks(t, reader2.Read(ctx))
	require.NotEmpty(t, blocks, "should have blocks from reconstructed EPUB")
}

func TestSubfilter_SubfilterAwareInterface(t *testing.T) {
	reader := epub.NewReader()
	var _ format.SubfilterAware = reader

	writer := epub.NewWriter()
	var _ format.SubfilterAware = writer
}
