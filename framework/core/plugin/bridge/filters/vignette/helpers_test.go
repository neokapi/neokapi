//go:build integration

package vignette

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.vignette.VignetteFilter"
const mimeType = "text/xml"

// readVignette parses a Vignette XML snippet with custom filter params and returns the parts.
// Uses en-us / es-es locales to match the Java test setUp.
func readVignette(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return readVignetteBytes(t, pool, cfg, []byte(snippet), "test.xml", filterParams)
}

// readVignetteDefault parses a Vignette XML snippet with default (nil) params.
func readVignetteDefault(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readVignette(t, snippet, nil)
}

// readVignetteBytes reads Vignette content with en-us / es-es locales.
func readVignetteBytes(t *testing.T, pool *bridge.BridgePool, cfg bridge.BridgeConfig, content []byte, uri string, filterParams map[string]any) []*model.Part {
	t.Helper()

	reader := bridge.NewBridgeFormatReader(pool, cfg, filterClass)
	if filterParams != nil {
		reader.SetFilterParams(filterParams)
	}

	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: "en-us",
		TargetLocale: "es-es",
		Encoding:     "UTF-8",
		MimeType:     mimeType,
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}

	ctx := context.Background()
	require.NoError(t, reader.Open(ctx, doc))

	var parts []*model.Part
	for pr := range reader.Read(ctx) {
		require.NoError(t, pr.Error, "reading part from bridge")
		parts = append(parts, pr.Part)
	}
	require.NoError(t, reader.Close())
	return parts
}

// readVignetteFile reads a Vignette file from testdata and returns parts.
func readVignetteFile(t *testing.T, relPath string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return readVignetteBytes(t, pool, cfg, content, path, filterParams)
}

// allBlocks returns all blocks (translatable and non-translatable) from parts.
func allBlocks(parts []*model.Part) []*model.Block {
	return bridgetest.FilterBlocks(parts)
}

// snippetRoundtrip roundtrips a Vignette XML snippet using en-us / es-es locales
// and returns the output string.
func snippetRoundtrip(t *testing.T, snippet string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTripWithLocales(t, pool, cfg, filterClass,
		[]byte(snippet), "test.xml", mimeType, filterParams,
		"en-us", "es-es")
	return string(result.Output)
}

// fileRoundtrip roundtrips a testdata file and returns the output string.
func fileRoundtrip(t *testing.T, relPath string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	result := bridgetest.RoundTripWithLocales(t, pool, cfg, filterClass,
		content, path, mimeType, filterParams,
		"en-us", "es-es")
	return string(result.Output)
}

// countPartsByType counts parts of a given type.
func countPartsByType(parts []*model.Part, pt model.PartType) int {
	n := 0
	for _, p := range parts {
		if p.Type == pt {
			n++
		}
	}
	return n
}

// createSimpleDoc creates the simple Vignette document from the Java test.
// okapi: VignetteFilterTest#createSimpleDoc
func createSimpleDoc() string {
	return "<importProject>" +
		"<importContentInstance><contentInstance>" +
		"<attribute name=\"SMCCONTENT-CONTENT-ID\"><valueString>id1ES</valueString></attribute>" +
		"<attribute name=\"SMCCONTENT-BODY\"><valueCLOB>&lt;p&gt;ES&lt;/p&gt;</valueCLOB></attribute>" +
		"<attribute name=\"SOURCE_ID\"><valueString>id1</valueString></attribute>" +
		"<attribute name=\"LOCALE_ID\"><valueString>es_ES</valueString></attribute>" +
		"</contentInstance></importContentInstance>" +
		"<stuff/>" +
		"<importContentInstance><contentInstance>" +
		"<attribute name=\"SMCCONTENT-CONTENT-ID\"><valueString>id1</valueString></attribute>" +
		"<attribute name=\"SMCCONTENT-BODY\"><valueCLOB>&lt;p&gt;ENtext&lt;/p&gt;</valueCLOB></attribute>" +
		"<attribute name=\"SOURCE_ID\"><valueString>id1</valueString></attribute>" +
		"<attribute name=\"LOCALE_ID\"><valueString>en_US</valueString></attribute>" +
		"</contentInstance></importContentInstance>" +
		"<importProject>"
}

// createComplexDoc creates the complex Vignette document from the Java test.
// okapi: VignetteFilterTest#createComplexDoc
func createComplexDoc() string {
	return "<importProject>" +
		// ES id1
		"<importContentInstance><contentInstance>" +
		"<attribute name=\"SMCCONTENT-CONTENT-ID\"><valueString>id1ES</valueString></attribute>" +
		"<attribute name=\"SMCCONTENT-BODY\"><valueCLOB>ES-id1</valueCLOB></attribute>" +
		"<attribute name=\"SOURCE_ID\"><valueString>id1</valueString></attribute>" +
		"<attribute name=\"LOCALE_ID\"><valueString>es_ES</valueString></attribute>" +
		"</contentInstance></importContentInstance>" +
		// EN id2
		"<importContentInstance><contentInstance>" +
		"<attribute name=\"SMCCONTENT-CONTENT-ID\"><valueString>id2</valueString></attribute>" +
		"<attribute name=\"SMCCONTENT-BODY\"><valueCLOB>EN-id2</valueCLOB></attribute>" +
		"<attribute name=\"SOURCE_ID\"><valueString>id2</valueString></attribute>" +
		"<attribute name=\"LOCALE_ID\"><valueString>en_US</valueString></attribute>" +
		"</contentInstance></importContentInstance>" +
		"<importProject>" +
		// ES id2
		"<importContentInstance><contentInstance>" +
		"<attribute name=\"SMCCONTENT-CONTENT-ID\"><valueString>id2ES</valueString></attribute>" +
		"<attribute name=\"SMCCONTENT-BODY\"><valueCLOB>ES-id2</valueCLOB></attribute>" +
		"<attribute name=\"SOURCE_ID\"><valueString>id2</valueString></attribute>" +
		"<attribute name=\"LOCALE_ID\"><valueString>es_ES</valueString></attribute>" +
		"</contentInstance></importContentInstance>" +
		// EN id1
		"<importContentInstance><contentInstance>" +
		"<attribute name=\"SMCCONTENT-CONTENT-ID\"><valueString>id1</valueString></attribute>" +
		"<attribute name=\"SMCCONTENT-BODY\"><valueCLOB>EN-id1</valueCLOB></attribute>" +
		"<attribute name=\"SOURCE_ID\"><valueString>id1</valueString></attribute>" +
		"<attribute name=\"LOCALE_ID\"><valueString>en_US</valueString></attribute>" +
		"</contentInstance></importContentInstance>" +
		"<importProject>"
}
