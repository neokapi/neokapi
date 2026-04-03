package json

import (
	"bytes"
	"context"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testResolver returns a mock SubfilterResolver backed by a format registry
// with only the HTML reader/writer registered.
type testResolver struct{}

func (t *testResolver) ResolveReader(name string) (format.DataFormatReader, error) {
	// Return a minimal HTML-like reader that extracts text from simple tags
	return &fakeHTMLReader{}, nil
}

func (t *testResolver) ResolveWriter(name string) (format.DataFormatWriter, error) {
	return &fakeHTMLWriter{}, nil
}

// fakeHTMLReader is a minimal reader that emits blocks from simple HTML-like content.
// It doesn't actually parse HTML — just emits the raw text as a single block,
// wrapped in document-level layer markers.
type fakeHTMLReader struct {
	format.BaseFormatReader
	doc *model.RawDocument
}

func (f *fakeHTMLReader) Name() string                              { return "html" }
func (f *fakeHTMLReader) DisplayName() string                       { return "HTML" }
func (f *fakeHTMLReader) Config() format.DataFormatConfig           { return nil }
func (f *fakeHTMLReader) SetConfig(_ format.DataFormatConfig) error { return nil }
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

		// Emit root layer (will be skipped by JSON reader's subfilter logic)
		layer := &model.Layer{ID: "html1", Format: "html"}
		ch <- model.PartResult{Part: &model.Part{Type: model.PartLayerStart, Resource: layer}}

		// Emit the content as a single block
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
				if _, err := f.Output.Write([]byte(text)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// okapi: JSONFilterTest#testSubfilter
func TestSubfilter_ReadHTMLInJSON(t *testing.T) {
	t.Parallel()
	input := `{"title": "Hello", "body": "<p>Rich <b>content</b></p>"}`

	reader := NewReader()
	reader.cfg.Subfilters = []format.SubfilterMapping{
		{Pattern: "body", Format: "html"},
	}
	reader.SetSubfilterResolver(&testResolver{})

	doc := &model.RawDocument{
		URI:          "test.json",
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

	// Should have: LayerStart(json), Block(title), LayerStart(html child), Block(html content), LayerEnd(html child), LayerEnd(json)
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

	// Two layers: root JSON + child HTML
	require.Len(t, layers, 2, "should have root JSON layer and child HTML layer")
	assert.Equal(t, "json", layers[0].Format)
	assert.Equal(t, "html", layers[1].Format)
	assert.Equal(t, layers[0].ID, layers[1].ParentID, "child layer should reference parent")

	// Two blocks: "Hello" (title) and HTML content (body) — in JSON key order
	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Contains(t, blocks[1].SourceText(), "<p>Rich <b>content</b></p>")
}

func TestSubfilter_NoMatchPassesThrough(t *testing.T) {
	t.Parallel()
	input := `{"title": "Hello", "body": "<p>HTML</p>"}`

	reader := NewReader()
	reader.cfg.Subfilters = []format.SubfilterMapping{
		{Pattern: "description", Format: "html"}, // doesn't match "body"
	}
	reader.SetSubfilterResolver(&testResolver{})

	doc := &model.RawDocument{
		URI:          "test.json",
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

	// Both should be plain text blocks, no subfiltering — in JSON key order
	require.Len(t, blocks, 2)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Equal(t, "<p>HTML</p>", blocks[1].SourceText())
}

func TestSubfilter_WildcardPattern(t *testing.T) {
	t.Parallel()
	input := `{"en": {"body": "<p>Hello</p>"}, "fr": {"body": "<p>Bonjour</p>"}}`

	reader := NewReader()
	reader.cfg.Subfilters = []format.SubfilterMapping{
		{Pattern: "*.body", Format: "html"},
	}
	reader.SetSubfilterResolver(&testResolver{})

	doc := &model.RawDocument{
		URI:          "test.json",
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

	// Both "en.body" and "fr.body" should produce child HTML layers
	require.Len(t, childLayers, 2, "should have two child HTML layers")
	assert.Equal(t, "html", childLayers[0].Format)
	assert.Equal(t, "html", childLayers[1].Format)
}

// okapi: JSONFilterTest#testSubFilterDoubleExtraction
func TestSubfilter_Roundtrip(t *testing.T) {
	t.Parallel()
	input := `{"title": "Hello", "body": "<p>Rich content</p>"}`

	// Read with subfiltering
	reader := NewReader()
	reader.cfg.Subfilters = []format.SubfilterMapping{
		{Pattern: "body", Format: "html"},
	}
	resolver := &testResolver{}
	reader.SetSubfilterResolver(resolver)

	doc := &model.RawDocument{
		URI:          "test.json",
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

	// Verify roundtrip produced valid JSON with both keys
	output := buf.String()
	assert.Contains(t, output, `"title"`)
	assert.Contains(t, output, `"Hello"`)
	assert.Contains(t, output, `"body"`)
	// Forward slashes are escaped by default in JSON output
	assert.Contains(t, output, `Rich content`)
}

func TestMatchGlob(t *testing.T) {
	t.Parallel()
	tests := []struct {
		pattern string
		path    string
		want    bool
	}{
		{"body", "body", true},
		{"body", "title", false},
		{"*.body", "en.body", true},
		{"*.body", "body", false},
		{"*.body", "en.fr.body", false}, // single * matches one segment
	}
	for _, tt := range tests {
		t.Run(tt.pattern+"_"+tt.path, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, matchGlob(tt.pattern, tt.path))
		})
	}
}
