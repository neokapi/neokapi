package bridge

import (
	"bytes"
	"context"
	"io"
	"log"
	"testing"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
	pb "github.com/gokapi/gokapi/core/plugin/proto/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBridgeFormatReaderInterface(t *testing.T) {
	var _ format.DataFormatReader = (*BridgeFormatReader)(nil)
}

func TestBridgeFormatWriterInterface(t *testing.T) {
	var _ format.DataFormatWriter = (*BridgeFormatWriter)(nil)
}

func TestBridgeFormatReaderSignature(t *testing.T) {
	srv := &mockBridgeServer{
		infoResp: &pb.InfoResponse{
			Name:        "html",
			DisplayName: "HTML Filter",
			MimeTypes:   []string{"text/html"},
			Extensions:  []string{".html"},
		},
	}
	b := newTestBridge(t, srv)
	pool := NewBridgePool(1, log.New(io.Discard, "", 0))
	pool.Seed(b)

	reader := NewBridgeFormatReader(pool, b.cfg, "net.sf.okapi.filters.html.HtmlFilter")
	sig := reader.Signature()

	assert.Contains(t, sig.MIMETypes, "text/html")
	assert.Contains(t, sig.Extensions, ".html")
	assert.Equal(t, "html", reader.Name())
	assert.Equal(t, "HTML Filter", reader.DisplayName())
}

func TestBridgeFormatReaderOpenAndRead(t *testing.T) {
	srv := &mockBridgeServer{
		openResp: &pb.OpenResponse{},
		readParts: []*pb.PartMessage{
			{PartType: 0, Layer: &pb.LayerMessage{Id: "doc1", Name: "test.html", Format: "html"}},
			{PartType: 4, Block: &pb.BlockMessage{
				Id: "tu1", Translatable: true,
				Source: []*pb.SegmentMessage{{Id: "s1", Content: &pb.FragmentMessage{CodedText: "Hello"}}},
			}},
			{PartType: 1, Layer: &pb.LayerMessage{Id: "doc1"}},
		},
	}
	b := newTestBridge(t, srv)
	pool := NewBridgePool(1, log.New(io.Discard, "", 0))
	pool.Seed(b)

	reader := NewBridgeFormatReader(pool, b.cfg, "net.sf.okapi.filters.html.HtmlFilter")

	htmlContent := []byte("<html><body>Hello</body></html>")
	doc := &model.RawDocument{
		URI:          "test.html",
		SourceLocale: "en",
		Encoding:     "UTF-8",
		MimeType:     "text/html",
		Reader:       io.NopCloser(bytes.NewReader(htmlContent)),
	}

	ctx := context.Background()
	require.NoError(t, reader.Open(ctx, doc))

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		require.NoError(t, pr.Error)
		parts = append(parts, pr.Part)
	}
	require.NoError(t, reader.Close())

	require.Len(t, parts, 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartBlock, parts[1].Type)
	assert.Equal(t, model.PartLayerEnd, parts[2].Type)

	block := parts[1].Resource.(*model.Block)
	assert.Equal(t, "tu1", block.ID)
	assert.True(t, block.Translatable)
	require.Len(t, block.Source, 1)
	assert.Equal(t, "Hello", block.Source[0].Content.CodedText)
}

func TestBridgeFormatReaderClose(t *testing.T) {
	srv := &mockBridgeServer{
		closeResp: &pb.CloseResponse{},
	}
	b := newTestBridge(t, srv)
	pool := NewBridgePool(1, log.New(io.Discard, "", 0))
	pool.Seed(b)

	reader := NewBridgeFormatReader(pool, b.cfg, "net.sf.okapi.filters.html.HtmlFilter")

	// Simulate that Open was called.
	acquired, err := pool.Acquire(b.cfg)
	require.NoError(t, err)
	reader.bridge = acquired

	require.NoError(t, reader.Close())
}

func TestBridgeFormatWriterWrite(t *testing.T) {
	translatedHTML := "<html><body>Bonjour</body></html>"
	srv := &mockBridgeServer{
		writeOutput: []byte(translatedHTML),
	}
	b := newTestBridge(t, srv)
	pool := NewBridgePool(1, log.New(io.Discard, "", 0))
	pool.Seed(b)

	writer := NewBridgeFormatWriter(pool, b.cfg, "net.sf.okapi.filters.html.HtmlFilter")

	originalContent := []byte("<html><body>Hello</body></html>")
	writer.SetOriginalContent(originalContent)
	writer.SetLocale("fr")
	writer.SetEncoding("UTF-8")

	var outputBuf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&outputBuf))

	partsCh := make(chan *model.Part, 3)
	partsCh <- &model.Part{
		Type:     model.PartLayerStart,
		Resource: &model.Layer{ID: "doc1"},
	}
	partsCh <- &model.Part{
		Type: model.PartBlock,
		Resource: &model.Block{
			ID:           "tu1",
			Translatable: true,
			Source:       []*model.Segment{{ID: "s1", Content: model.NewFragment("Hello")}},
			Targets: map[model.LocaleID][]*model.Segment{
				"fr": {{ID: "s1", Content: model.NewFragment("Bonjour")}},
			},
		},
	}
	partsCh <- &model.Part{
		Type:     model.PartLayerEnd,
		Resource: &model.Layer{ID: "doc1"},
	}
	close(partsCh)

	require.NoError(t, writer.Write(context.Background(), partsCh))
	assert.Equal(t, translatedHTML, outputBuf.String())
}

func TestBridgeFormatWriterSetOutput(t *testing.T) {
	srv := &mockBridgeServer{}
	b := newTestBridge(t, srv)
	pool := NewBridgePool(1, log.New(io.Discard, "", 0))
	pool.Seed(b)

	writer := NewBridgeFormatWriter(pool, b.cfg, "net.sf.okapi.filters.html.HtmlFilter")

	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))
	assert.Equal(t, "net.sf.okapi.filters.html.HtmlFilter", writer.filterClass)
}

func TestBridgeFormatReaderSetFilterParams(t *testing.T) {
	srv := &mockBridgeServer{}
	b := newTestBridge(t, srv)
	pool := NewBridgePool(1, log.New(io.Discard, "", 0))
	pool.Seed(b)

	reader := NewBridgeFormatReader(pool, b.cfg, "net.sf.okapi.filters.json.JSONFilter")

	params := map[string]any{
		"extractStandalone": true,
		"extractAllPairs":   false,
	}
	reader.SetFilterParams(params)

	assert.NotNil(t, reader.filterParams)
	assert.Equal(t, true, reader.filterParams["extractStandalone"])
	assert.Equal(t, false, reader.filterParams["extractAllPairs"])
}

func TestBridgeFormatWriterSetFilterParams(t *testing.T) {
	srv := &mockBridgeServer{}
	b := newTestBridge(t, srv)
	pool := NewBridgePool(1, log.New(io.Discard, "", 0))
	pool.Seed(b)

	writer := NewBridgeFormatWriter(pool, b.cfg, "net.sf.okapi.filters.json.JSONFilter")

	params := map[string]any{
		"extractStandalone": true,
		"maxDepth":          10,
	}
	writer.SetFilterParams(params)

	assert.NotNil(t, writer.filterParams)
	assert.Equal(t, true, writer.filterParams["extractStandalone"])
	assert.Equal(t, 10, writer.filterParams["maxDepth"])
}

func TestBridgeFormatReaderReadContextCancel(t *testing.T) {
	manyParts := make([]*pb.PartMessage, 100)
	for i := range manyParts {
		manyParts[i] = &pb.PartMessage{
			PartType: int32(model.PartData),
			Data:     &pb.DataMessage{Id: "d"},
		}
	}
	srv := &mockBridgeServer{
		readParts: manyParts,
	}
	b := newTestBridge(t, srv)

	// Don't use pool — assign bridge directly.
	reader := &BridgeFormatReader{
		bridge: b,
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	var results []model.PartResult
	ch := reader.Read(ctx)
	for pr := range ch {
		results = append(results, pr)
	}

	// Should have received a context canceled error at some point.
	hasError := false
	for _, r := range results {
		if r.Error != nil {
			hasError = true
			break
		}
	}
	assert.True(t, hasError || len(results) < 100, "context cancellation should stop part emission")
}
