package xml

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testResolver returns mock sub-format readers/writers for testing.
type testResolver struct{}

func (t *testResolver) ResolveReader(name string) (format.DataFormatReader, error) {
	return &fakeHTMLReader{}, nil
}

func (t *testResolver) ResolveWriter(name string) (format.DataFormatWriter, error) {
	return &fakeHTMLWriter{}, nil
}

// fakeHTMLReader emits the raw text as a single block.
type fakeHTMLReader struct {
	format.BaseFormatReader
	doc *model.RawDocument
}

func (f *fakeHTMLReader) Name() string                              { return "html" }
func (f *fakeHTMLReader) DisplayName() string                       { return "HTML" }
func (f *fakeHTMLReader) Config() format.DataFormatConfig            { return nil }
func (f *fakeHTMLReader) SetConfig(_ format.DataFormatConfig) error  { return nil }
func (f *fakeHTMLReader) Signature() format.FormatSignature {
	return format.FormatSignature{MIMETypes: []string{"text/html"}}
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

// fakeHTMLWriter concatenates block texts.
type fakeHTMLWriter struct {
	format.BaseFormatWriter
}

func (f *fakeHTMLWriter) Write(ctx context.Context, parts <-chan *model.Part) error {
	for part := range parts {
		if part.Type == model.PartBlock {
			if block, ok := part.Resource.(*model.Block); ok {
				if _, err := f.Output.Write([]byte(block.SourceText())); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func TestXMLSubfilter_ReadHTMLInElement(t *testing.T) {
	input := `<root><title>Hello</title><body><![CDATA[<p>Rich <b>content</b></p>]]></body></root>`

	reader := NewReader()
	reader.cfg.Subfilters = []format.SubfilterMapping{
		{Pattern: "root.body", Format: "html"},
	}
	reader.SetSubfilterResolver(&testResolver{})

	doc := &model.RawDocument{
		URI:          "test.xml",
		SourceLocale: "en",
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader([]byte(input))),
	}

	ctx := context.Background()
	require.NoError(t, reader.Open(ctx, doc))

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		require.NoError(t, pr.Error)
		parts = append(parts, pr.Part)
	}
	require.NoError(t, reader.Close())

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

	// Two layers: root XML + child HTML
	require.Len(t, layers, 2, "should have root XML layer and child HTML layer")
	assert.Equal(t, "xml", layers[0].Format)
	assert.Equal(t, "html", layers[1].Format)
	assert.Equal(t, layers[0].ID, layers[1].ParentID, "child layer should reference parent")

	// Two blocks: "Hello" (title) and HTML content (body)
	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Contains(t, blocks[1].SourceText(), "<p>Rich <b>content</b></p>")
}

func TestXMLSubfilter_NoMatchPassesThrough(t *testing.T) {
	input := `<root><title>Hello</title><body>Plain text</body></root>`

	reader := NewReader()
	reader.cfg.Subfilters = []format.SubfilterMapping{
		{Pattern: "root.description", Format: "html"}, // doesn't match "body"
	}
	reader.SetSubfilterResolver(&testResolver{})

	doc := &model.RawDocument{
		URI:          "test.xml",
		SourceLocale: "en",
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader([]byte(input))),
	}

	ctx := context.Background()
	require.NoError(t, reader.Open(ctx, doc))

	var blocks []*model.Block
	for pr := range reader.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part.Type == model.PartBlock {
			if block, ok := pr.Part.Resource.(*model.Block); ok {
				blocks = append(blocks, block)
			}
		}
	}
	reader.Close()

	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Equal(t, "Plain text", blocks[1].SourceText())
}

func TestXMLSubfilter_WildcardPattern(t *testing.T) {
	input := `<root><en><body>Hello</body></en><fr><body>Bonjour</body></fr></root>`

	reader := NewReader()
	reader.cfg.Subfilters = []format.SubfilterMapping{
		{Pattern: "root.*.body", Format: "html"},
	}
	reader.SetSubfilterResolver(&testResolver{})

	doc := &model.RawDocument{
		URI:          "test.xml",
		SourceLocale: "en",
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader([]byte(input))),
	}

	ctx := context.Background()
	require.NoError(t, reader.Open(ctx, doc))

	var childLayers []*model.Layer
	for pr := range reader.Read(ctx) {
		require.NoError(t, pr.Error)
		if pr.Part.Type == model.PartLayerStart {
			if layer, ok := pr.Part.Resource.(*model.Layer); ok && layer.IsEmbedded() {
				childLayers = append(childLayers, layer)
			}
		}
	}
	reader.Close()

	require.Len(t, childLayers, 2, "should have two child HTML layers")
	assert.Equal(t, "html", childLayers[0].Format)
	assert.Equal(t, "html", childLayers[1].Format)
}

func TestXMLSubfilter_Roundtrip(t *testing.T) {
	input := `<root><title>Hello</title><body><![CDATA[<p>Rich content</p>]]></body></root>`

	// Read with subfiltering
	reader := NewReader()
	reader.cfg.Subfilters = []format.SubfilterMapping{
		{Pattern: "root.body", Format: "html"},
	}
	resolver := &testResolver{}
	reader.SetSubfilterResolver(resolver)

	doc := &model.RawDocument{
		URI:          "test.xml",
		SourceLocale: "en",
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader([]byte(input))),
	}

	ctx := context.Background()
	require.NoError(t, reader.Open(ctx, doc))

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		require.NoError(t, pr.Error)
		parts = append(parts, pr.Part)
	}
	reader.Close()

	// Write with subfiltering
	writer := NewWriter()
	writer.SetSubfilterResolver(resolver)
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	partCh := make(chan *model.Part, len(parts))
	for _, p := range parts {
		partCh <- p
	}
	close(partCh)

	require.NoError(t, writer.Write(ctx, partCh))
	require.NoError(t, writer.Close())

	output := buf.String()
	assert.Contains(t, output, "Hello")
	assert.Contains(t, output, "<p>Rich content</p>")
}

func TestXMLMatchGlob(t *testing.T) {
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"root.body", "root.body", true},
		{"root.body", "root.title", false},
		{"root.*.body", "root.en.body", true},
		{"root.*.body", "root.body", false},
		{"root.*.body", "root.en.fr.body", false},
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			assert.Equal(t, tt.want, matchGlob(tt.pattern, tt.path))
		})
	}
}
