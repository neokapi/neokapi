package archive_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/formats/archive"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testResolver returns a mock SubfilterResolver for testing.
type testResolver struct{}

func (t *testResolver) ResolveReader(name string) (format.DataFormatReader, error) {
	return &fakeFormatReader{formatName: name}, nil
}

func (t *testResolver) ResolveWriter(name string) (format.DataFormatWriter, error) {
	return &fakeFormatWriter{formatName: name}, nil
}

// fakeFormatReader emits the raw content as a single block, wrapped in layer markers.
type fakeFormatReader struct {
	format.BaseFormatReader
	formatName string
	doc        *model.RawDocument
}

func (f *fakeFormatReader) Signature() format.FormatSignature {
	return format.FormatSignature{}
}

func (f *fakeFormatReader) Open(_ context.Context, doc *model.RawDocument) error {
	f.doc = doc
	return nil
}

func (f *fakeFormatReader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 8)
	go func() {
		defer close(ch)
		content, _ := io.ReadAll(f.doc.Reader)
		layer := &model.Layer{ID: "sub1", Format: f.formatName}
		ch <- model.PartResult{Part: &model.Part{Type: model.PartLayerStart, Resource: layer}}
		block := model.NewBlock("tu1", string(content))
		ch <- model.PartResult{Part: &model.Part{Type: model.PartBlock, Resource: block}}
		ch <- model.PartResult{Part: &model.Part{Type: model.PartLayerEnd, Resource: layer}}
	}()
	return ch
}

func (f *fakeFormatReader) Close() error { return nil }

// fakeFormatWriter reconstructs content by concatenating block texts.
type fakeFormatWriter struct {
	format.BaseFormatWriter
	formatName string
}

func (f *fakeFormatWriter) Write(ctx context.Context, parts <-chan *model.Part) error {
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

func TestSubfilter_ReadHTMLInArchive(t *testing.T) {
	ctx := context.Background()
	data := makeZip(t, map[string]string{
		"page.html":  "<p>Hello <b>World</b></p>",
		"readme.txt": "Plain text",
		"image.png":  "binary data",
	})

	reader := archive.NewReader()
	reader.SetSubfilterResolver(&testResolver{})

	err := reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	// Should have subfiltered child layer for .html, line-by-line for .txt
	var layers []*model.Layer
	var blocks []*model.Block
	for _, p := range parts {
		switch p.Type {
		case model.PartLayerStart:
			if layer, ok := p.Resource.(*model.Layer); ok {
				layers = append(layers, layer)
			}
		case model.PartBlock:
			if block, ok := p.Resource.(*model.Block); ok {
				blocks = append(blocks, block)
			}
		}
	}

	// Root layer + html child layer + txt child layer = at least 3 layers
	require.GreaterOrEqual(t, len(layers), 3)

	// Find the HTML child layer
	var htmlLayer *model.Layer
	for _, l := range layers {
		if l.Format == "html" {
			htmlLayer = l
			break
		}
	}
	require.NotNil(t, htmlLayer, "should have a child HTML layer")
	assert.Equal(t, "page.html", htmlLayer.Name)
	assert.Equal(t, "archive", htmlLayer.Properties["subfilter.source"])

	// Blocks: one from the HTML subfilter, one line from the TXT
	require.Len(t, blocks, 2)
	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "<p>Hello <b>World</b></p>")
	assert.Contains(t, texts, "Plain text")
}

func TestSubfilter_NoResolverFallsBack(t *testing.T) {
	ctx := context.Background()
	data := makeZip(t, map[string]string{
		"page.html": "Line one\nLine two",
	})

	reader := archive.NewReader()
	// No resolver set — should fall back to line-by-line extraction

	err := reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	require.Len(t, blocks, 2)
	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Line one")
	assert.Contains(t, texts, "Line two")
}

func TestSubfilter_CustomMappings(t *testing.T) {
	ctx := context.Background()
	data := makeZip(t, map[string]string{
		"data.custom": "custom content",
		"readme.txt":  "plain text",
	})

	reader := archive.NewReader()
	reader.SetSubfilterResolver(&testResolver{})

	// Only map *.custom extension, not *.txt
	cfg := reader.Config()
	err := cfg.ApplyMap(map[string]any{
		"filePatterns": []any{"*.txt", "*.custom"},
		"subfilterMappings": []any{
			map[string]any{"pattern": "*.custom", "format": "myformat"},
		},
	})
	require.NoError(t, err)

	err = reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
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

	// Should have one subfiltered layer for *.custom and one plain layer for *.txt
	require.Len(t, childLayers, 2)

	var customLayer, txtLayer *model.Layer
	for _, l := range childLayers {
		if l.Format == "myformat" {
			customLayer = l
		} else {
			txtLayer = l
		}
	}
	require.NotNil(t, customLayer, "should have subfiltered custom layer")
	assert.Equal(t, "data.custom", customLayer.Name)

	require.NotNil(t, txtLayer, "should have plain txt layer")
	assert.Equal(t, "readme.txt", txtLayer.Name)
}

func TestSubfilter_Roundtrip(t *testing.T) {
	ctx := context.Background()
	data := makeZip(t, map[string]string{
		"page.html": "<p>Hello World</p>",
		"image.png": "binary data",
	})
	tmpPath := writeTmpZip(t, data)

	resolver := &testResolver{}

	// Read with subfiltering
	reader := archive.NewReader()
	reader.SetSubfilterResolver(resolver)
	err := reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Write with subfiltering
	var buf bytes.Buffer
	writer := archive.NewWriter()
	writer.SetSubfilterResolver(resolver)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetOriginalZip(tmpPath)
	writer.SetLocale(model.LocaleEnglish)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	// Re-read the output (without subfiltering, to see raw content)
	reader2 := archive.NewReader()
	err = reader2.Open(ctx, rawDocFromBytes(buf.Bytes(), model.LocaleEnglish))
	require.NoError(t, err)
	defer reader2.Close()

	blocks := testutil.CollectBlocks(t, reader2.Read(ctx))
	texts := testutil.BlockTexts(blocks)
	// The HTML content should have been reconstructed through the fake writer
	assert.Contains(t, texts, "<p>Hello World</p>")
}

func TestSubfilter_DefaultMappings(t *testing.T) {
	mappings := archive.DefaultSubfilterMappings()
	require.NotEmpty(t, mappings)

	// Verify key extensions are mapped
	extToFormat := make(map[string]string)
	for _, m := range mappings {
		extToFormat[m.Pattern] = m.Format
	}
	assert.Equal(t, "html", extToFormat["*.html"])
	assert.Equal(t, "html", extToFormat["*.xhtml"])
	assert.Equal(t, "xml", extToFormat["*.xml"])
	assert.Equal(t, "json", extToFormat["*.json"])
	assert.Equal(t, "yaml", extToFormat["*.yaml"])
}

func TestSubfilter_SubfilterAwareInterface(t *testing.T) {
	reader := archive.NewReader()
	var _ format.SubfilterAware = reader

	writer := archive.NewWriter()
	var _ format.SubfilterAware = writer
}
