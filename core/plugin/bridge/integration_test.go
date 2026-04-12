//go:build integration

package bridge

import (
	"bytes"
	"io"
	"log"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skipIfNoJava(t *testing.T) {
	t.Helper()
	// The Start() method handles java discovery.
}

func skipIfNoJAR(t *testing.T) string {
	t.Helper()
	jarPath := os.Getenv("NEOKAPI_BRIDGE_JAR")
	if jarPath == "" {
		t.Skip("NEOKAPI_BRIDGE_JAR not set, skipping integration test")
	}
	if _, err := os.Stat(jarPath); os.IsNotExist(err) {
		t.Skipf("JAR not found at %s, skipping integration test", jarPath)
	}
	return jarPath
}

func TestIntegrationBridgeStartStop(t *testing.T) {
	jarPath := skipIfNoJAR(t)

	bridge := NewJavaBridge(BridgeConfig{
		Command: "java",
		Args:    []string{"-jar", jarPath},
	}, log.Default())

	require.NoError(t, bridge.Start())
	require.NoError(t, bridge.Stop())
}

func TestIntegrationReadHTML(t *testing.T) {
	jarPath := skipIfNoJAR(t)

	cfg := BridgeConfig{
		Command: "java",
		Args:    []string{"-jar", jarPath},
	}

	registry := NewBridgeRegistry(1, 1, log.Default())
	require.NoError(t, registry.Warmup(cfg))
	defer registry.Shutdown()

	htmlContent := []byte("<html><body><p>Hello world</p></body></html>")

	reader := NewBridgeFormatReader(registry, cfg, "net.sf.okapi.filters.html.HtmlFilter", format.FormatSignature{})

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

	// Should have LayerStart, some parts, and LayerEnd.
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")

	// Should have at least one Block with "Hello world".
	hasBlock := false
	for _, p := range parts {
		if p.Type == model.PartBlock {
			hasBlock = true
			block := p.Resource.(*model.Block)
			assert.True(t, block.Translatable)
			if len(block.Source) > 0 {
				text := block.Source[0].Content.Text()
				assert.Contains(t, text, "Hello world")
			}
		}
	}
	assert.True(t, hasBlock, "should have at least one Block part")
}
