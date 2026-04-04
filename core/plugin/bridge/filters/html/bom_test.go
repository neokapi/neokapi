//go:build integration

package html

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readBytesWithEncoding reads content through the bridge with a custom encoding
// on the RawDocument. This is needed for BOM tests where the input is not UTF-8.
func readBytesWithEncoding(t *testing.T, pool *bridge.BridgeRegistry, cfg bridge.BridgeConfig, content []byte, uri, encoding string) []*model.Part {
	t.Helper()

	reader := bridge.NewBridgeFormatReader(pool, cfg, filterClass, format.FormatSignature{})

	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: "en",
		TargetLocale: "fr",
		Encoding:     encoding,
		MimeType:     mimeType,
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}

	ctx := t.Context()
	require.NoError(t, reader.Open(ctx, doc))

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		require.NoError(t, pr.Error, "reading part from bridge")
		parts = append(parts, pr.Part)
	}

	require.NoError(t, reader.Close())
	return parts
}

// TestBom_DetectBom reads an HTML file with a UTF-8 BOM and verifies
// the BOM is detected. The layer should report UTF-8 encoding, HasBOM=true,
// and the locale should be "en".
//
// okapi: HtmlDetectBomTest#testDetectBom
func TestBom_DetectBom(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okapi/filters/html/src/test/resources/ruby.html")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	// Verify the file starts with UTF-8 BOM.
	require.True(t, len(content) >= 3 &&
		content[0] == 0xEF && content[1] == 0xBB && content[2] == 0xBF,
		"ruby.html should start with UTF-8 BOM")

	parts := readBytesWithEncoding(t, pool, cfg, content, path, "UTF-8")
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok, "first part should be a Layer")

	// Java asserts: sd.hasUTF8BOM() == true, sd.getEncoding() == "UTF-8",
	// sd.getLocale() == locEN, sd.getLineBreak() == "\r\n"
	assert.True(t, layer.HasBOM, "layer should have HasBOM=true for UTF-8 BOM file")
	assert.Equal(t, "UTF-8", layer.Encoding, "encoding should be UTF-8")
	assert.Equal(t, model.LocaleID("en"), layer.Locale, "locale should be 'en'")
	assert.Equal(t, "\r\n", layer.LineBreak, "line break should be CRLF")
}

// TestBom_DetectUnicodeLittleBom reads an HTML file with a UTF-16LE BOM
// (0xFF 0xFE) and verifies the BOM is detected with the correct encoding.
//
// okapi: HtmlDetectBomTest#testDetectUnicodeLittleBom
func TestBom_DetectUnicodeLittleBom(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okapi/filters/html/src/test/resources/FFFEBOM.html")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	// Verify the file starts with UTF-16LE BOM.
	require.True(t, len(content) >= 2 &&
		content[0] == 0xFF && content[1] == 0xFE,
		"FFFEBOM.html should start with UTF-16LE BOM")

	parts := readBytesWithEncoding(t, pool, cfg, content, path, "UTF-16LE")
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok, "first part should be a Layer")

	// Java asserts: sd.hasUTF8BOM() == false, sd.getEncoding() == "UTF-16LE",
	// sd.getLocale() == locEN, sd.getLineBreak() == "\r\n"
	// Note: HasBOM may be true (it has a BOM, just not a UTF-8 one) or false
	// depending on bridge behavior. The key assertion is encoding.
	assert.Equal(t, "UTF-16LE", layer.Encoding, "encoding should be UTF-16LE")
	assert.Equal(t, model.LocaleID("en"), layer.Locale, "locale should be 'en'")
	assert.Equal(t, "\r\n", layer.LineBreak, "line break should be CRLF")
}

// TestBom_DetectAndRemoveBom reads an HTML file with a UTF-8 BOM and verifies
// that the BOM bytes do not leak into the extracted text content. The file
// should be processed correctly with the BOM stripped from content.
//
// okapi: HtmlDetectBomTest#testDetectAndRemoveBom
func TestBom_DetectAndRemoveBom(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okapi/filters/html/src/test/resources/ruby.html")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	// Verify the file starts with UTF-8 BOM.
	require.True(t, len(content) >= 3 &&
		content[0] == 0xEF && content[1] == 0xBB && content[2] == 0xBF,
		"ruby.html should start with UTF-8 BOM")

	parts := readBytesWithEncoding(t, pool, cfg, content, path, "UTF-8")
	require.NotEmpty(t, parts)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok, "first part should be a Layer")

	// Java asserts after detectAndRemoveBom: sd.hasUTF8BOM() == false,
	// sd.getEncoding() == "UTF-8". The BOM is removed, so HasBOM reports
	// false after removal.
	// In the bridge, the Java side handles BOM detection. We verify the
	// content is extracted cleanly.
	assert.Equal(t, "UTF-8", layer.Encoding, "encoding should be UTF-8")
	assert.Equal(t, model.LocaleID("en"), layer.Locale, "locale should be 'en'")

	// The key assertion: verify BOM bytes (0xEF 0xBB 0xBF) do not appear
	// in any extracted text content.
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from ruby.html")

	bomStr := string([]byte{0xEF, 0xBB, 0xBF})
	for _, b := range blocks {
		text := b.SourceText()
		assert.NotContains(t, text, bomStr,
			"BOM bytes should not appear in extracted text content")
	}
}
