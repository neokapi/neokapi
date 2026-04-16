package bridge

import (
	"bytes"
	"io"
	"log"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	pb "github.com/neokapi/neokapi/core/plugin/proto/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBridgeFormatReaderInterface(t *testing.T) {
	var _ format.DataFormatReader = (*BridgeFormatReader)(nil)
}

func TestBridgeFormatReaderSignature(t *testing.T) {
	sig := format.FormatSignature{
		MIMETypes:  []string{"text/html"},
		Extensions: []string{".html"},
	}
	registry := NewBridgeRegistry(1, 1, log.New(io.Discard, "", 0))
	defer registry.Shutdown()

	reader := NewBridgeFormatReader(registry, BridgeConfig{}, "net.sf.okapi.filters.html.HtmlFilter", sig)
	got := reader.Signature()

	assert.Contains(t, got.MIMETypes, "text/html")
	assert.Contains(t, got.Extensions, ".html")
}

func TestBridgeFormatReaderOpenAndRead(t *testing.T) {
	srv := &mockBridgeServer{
		processReadParts: []*pb.PartMessage{
			{PartType: int32(model.PartLayerStart), Layer: &pb.LayerMessage{Id: "doc1", Name: "test.html", Format: "html"}},
			{PartType: int32(model.PartBlock), Block: &pb.BlockMessage{
				Id: "tu1", Translatable: true,
				Source: []*pb.SegmentMessage{{Id: "s1", Runs: []*pb.RunMessage{{Kind: &pb.RunMessage_Text{Text: &pb.TextRunMessage{Text: "Hello"}}}}}},
			}},
			{PartType: int32(model.PartLayerEnd), Layer: &pb.LayerMessage{Id: "doc1"}},
		},
	}
	b := newTestBridge(t, srv)

	registry := NewBridgeRegistry(1, 1, log.New(io.Discard, "", 0))
	defer registry.Shutdown()
	registry.mu.Lock()
	key := b.cfg.PoolKey()
	sem := make(chan struct{}, 1)
	sem <- struct{}{}
	ready := make(chan struct{})
	close(ready)
	registry.bridges[key] = &managedBridge{bridge: b, sem: sem, cfg: b.cfg, ready: ready}
	registry.mu.Unlock()

	reader := NewBridgeFormatReader(registry, b.cfg, "net.sf.okapi.filters.html.HtmlFilter", format.FormatSignature{})

	htmlContent := []byte("<html><body>Hello</body></html>")
	doc := &model.RawDocument{
		URI:          "test.html",
		SourceLocale: "en",
		Encoding:     "UTF-8",
		MimeType:     "text/html",
		Reader:       io.NopCloser(bytes.NewReader(htmlContent)),
	}

	ctx := t.Context()
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
	assert.Equal(t, "Hello", model.RunsPlainText(block.Source[0].Runs))
}

func TestBridgeProcessorExecute(t *testing.T) {
	srv := &mockBridgeServer{
		processReadParts: []*pb.PartMessage{
			{PartType: int32(model.PartLayerStart), Layer: &pb.LayerMessage{Id: "doc1"}},
			{PartType: int32(model.PartBlock), Block: &pb.BlockMessage{
				Id: "tu1", Translatable: true,
				Source: []*pb.SegmentMessage{{Id: "s1", Runs: []*pb.RunMessage{{Kind: &pb.RunMessage_Text{Text: &pb.TextRunMessage{Text: "Hello"}}}}}},
			}},
			{PartType: int32(model.PartLayerEnd), Layer: &pb.LayerMessage{Id: "doc1"}},
		},
		processOutput: []byte("output-bytes"),
	}
	b := newTestBridge(t, srv)

	registry := NewBridgeRegistry(1, 1, log.New(io.Discard, "", 0))
	defer registry.Shutdown()
	registry.mu.Lock()
	key := b.cfg.PoolKey()
	sem := make(chan struct{}, 1)
	sem <- struct{}{}
	ready := make(chan struct{})
	close(ready)
	registry.bridges[key] = &managedBridge{bridge: b, sem: sem, cfg: b.cfg, ready: ready}
	registry.mu.Unlock()

	processor := NewBridgeProcessor(registry, b.cfg, "net.sf.okapi.filters.html.HtmlFilter")

	var captured []*model.Part
	result, err := processor.Execute(t.Context(), ProcessExecuteParams{
		Content:      []byte("<html>Hello</html>"),
		SourceLocale: "en",
		TargetLocale: "fr",
		OutputLocale: "fr",
	}, func(parts <-chan *model.Part) <-chan *model.Part {
		out := make(chan *model.Part, 64)
		go func() {
			defer close(out)
			for p := range parts {
				captured = append(captured, p)
				out <- p
			}
		}()
		return out
	})

	require.NoError(t, err)
	assert.Len(t, captured, 3)
	assert.Equal(t, []byte("output-bytes"), result.Output)
}

func TestBridgeFormatReaderSetFilterParams(t *testing.T) {
	registry := NewBridgeRegistry(1, 1, log.New(io.Discard, "", 0))
	defer registry.Shutdown()

	reader := NewBridgeFormatReader(registry, BridgeConfig{}, "net.sf.okapi.filters.json.JSONFilter", format.FormatSignature{})

	params := map[string]any{
		"extractStandalone": true,
		"extractAllPairs":   false,
	}
	reader.SetFilterParams(params)

	assert.NotNil(t, reader.filterParams)
	assert.Equal(t, true, reader.filterParams["extractStandalone"])
	assert.Equal(t, false, reader.filterParams["extractAllPairs"])
}

func TestBridgeProcessorSetFilterParams(t *testing.T) {
	registry := NewBridgeRegistry(1, 1, log.New(io.Discard, "", 0))
	defer registry.Shutdown()

	processor := NewBridgeProcessor(registry, BridgeConfig{}, "net.sf.okapi.filters.json.JSONFilter")

	params := map[string]any{
		"extractStandalone": true,
		"maxDepth":          10,
	}
	processor.SetFilterParams(params)

	assert.NotNil(t, processor.filterParams)
	assert.Equal(t, true, processor.filterParams["extractStandalone"])
	assert.Equal(t, 10, processor.filterParams["maxDepth"])
}
