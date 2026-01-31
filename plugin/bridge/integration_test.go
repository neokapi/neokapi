//go:build integration

package bridge

import (
	"bytes"
	"context"
	"io"
	"log"
	"os"
	"os/exec"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func skipIfNoJava(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("java"); err != nil {
		t.Skip("java not found in PATH, skipping integration test")
	}
}

func skipIfNoJAR(t *testing.T) string {
	t.Helper()
	jarPath := os.Getenv("GOKAPI_BRIDGE_JAR")
	if jarPath == "" {
		t.Skip("GOKAPI_BRIDGE_JAR not set, skipping integration test")
	}
	if _, err := os.Stat(jarPath); os.IsNotExist(err) {
		t.Skipf("JAR not found at %s, skipping integration test", jarPath)
	}
	return jarPath
}

func TestIntegrationBridgeStartStop(t *testing.T) {
	skipIfNoJava(t)
	jarPath := skipIfNoJAR(t)

	bridge := NewJavaBridge(BridgeConfig{
		JARPath: jarPath,
	}, log.Default())

	require.NoError(t, bridge.Start())
	require.NoError(t, bridge.Stop())
}

func TestIntegrationListFilters(t *testing.T) {
	skipIfNoJava(t)
	jarPath := skipIfNoJAR(t)

	bridge := NewJavaBridge(BridgeConfig{
		JARPath: jarPath,
	}, log.Default())

	require.NoError(t, bridge.Start())
	defer bridge.Stop()

	lf, err := bridge.ListFilters()
	require.NoError(t, err)
	require.NotEmpty(t, lf.Filters)

	// Should have the HTML filter registered.
	found := false
	for _, f := range lf.Filters {
		if f.Name == "html" {
			found = true
			assert.Equal(t, "net.sf.okapi.filters.html.HtmlFilter", f.FilterClass)
			break
		}
	}
	assert.True(t, found, "html filter should be in the registry")
}

func TestIntegrationReadHTML(t *testing.T) {
	skipIfNoJava(t)
	jarPath := skipIfNoJAR(t)

	cfg := BridgeConfig{
		JARPath: jarPath,
	}
	b := NewJavaBridge(cfg, log.Default())
	require.NoError(t, b.Start())

	pool := NewBridgePool(1, log.Default())
	pool.Seed(b)
	defer pool.Shutdown()

	htmlContent := []byte("<html><body><p>Hello world</p></body></html>")

	reader := NewBridgeFormatReader(pool, cfg, "net.sf.okapi.filters.html.HtmlFilter")

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

func TestIntegrationReadDOCX(t *testing.T) {
	skipIfNoJava(t)
	jarPath := skipIfNoJAR(t)

	// Check for test DOCX file.
	docxPath := "testdata/sample.docx"
	if _, err := os.Stat(docxPath); os.IsNotExist(err) {
		t.Skip("testdata/sample.docx not found, skipping DOCX integration test")
	}

	docxContent, err := os.ReadFile(docxPath)
	require.NoError(t, err)

	cfg := BridgeConfig{
		JARPath: jarPath,
	}
	b := NewJavaBridge(cfg, log.Default())
	require.NoError(t, b.Start())

	pool := NewBridgePool(1, log.Default())
	pool.Seed(b)
	defer pool.Shutdown()

	reader := NewBridgeFormatReader(pool, cfg, "net.sf.okapi.filters.openxml.OpenXMLFilter")

	doc := &model.RawDocument{
		URI:          "sample.docx",
		SourceLocale: "en",
		Encoding:     "UTF-8",
		MimeType:     "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
		Reader:       io.NopCloser(bytes.NewReader(docxContent)),
	}

	ctx := context.Background()
	require.NoError(t, reader.Open(ctx, doc))

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		require.NoError(t, pr.Error)
		parts = append(parts, pr.Part)
	}

	require.NoError(t, reader.Close())

	// Should have LayerStart and LayerEnd at minimum.
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}
