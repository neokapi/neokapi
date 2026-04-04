//go:build integration

package ttx

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
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.ttx.TTXFilter"
const mimeType = "application/x-ttx+xml"

// Target locale as it appears in block targets from the bridge.
// The TTX UserSettings has TargetLanguage="ES-EM" and the bridge preserves
// the original casing from the XML, resulting in "es-EM".
const tgtLocale = "es-EM"

// STARTFILE matches the Java test constant — TTX header with a trailing newline
// after </Raw>\n (the Java STARTFILE ends with "\n" inside the <Raw> element).
const startFile = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>" +
	"<TRADOStag Version=\"2.0\"><FrontMatter>\n" +
	"<ToolSettings CreationDate=\"20070508T094743Z\" CreationTool=\"TRADOS TagEditor\" CreationToolVersion=\"7.0.0.615\"></ToolSettings>\n" +
	"<UserSettings DataType=\"STF\" O-Encoding=\"UTF-8\" SettingsName=\"\" SettingsPath=\"\" SourceLanguage=\"EN-US\" TargetLanguage=\"ES-EM\" SourceDocumentPath=\"abc.rtf\" SettingsRelativePath=\"\" PlugInInfo=\"\"></UserSettings>\n" +
	"</FrontMatter><Body><Raw>\n"

// STARTFILENOLB matches the Java constant — same as startFile but without the
// trailing newline inside <Raw>.
const startFileNoLB = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>" +
	"<TRADOStag Version=\"2.0\"><FrontMatter>\n" +
	"<ToolSettings CreationDate=\"20070508T094743Z\" CreationTool=\"TRADOS TagEditor\" CreationToolVersion=\"7.0.0.615\"></ToolSettings>\n" +
	"<UserSettings DataType=\"STF\" O-Encoding=\"UTF-8\" SettingsName=\"\" SettingsPath=\"\" SourceLanguage=\"EN-US\" TargetLanguage=\"ES-EM\" SourceDocumentPath=\"abc.rtf\" SettingsRelativePath=\"\" PlugInInfo=\"\"></UserSettings>\n" +
	"</FrontMatter><Body><Raw>"

// readTTX parses a TTX snippet using EN-US / ES-EM locales and returns parts.
func readTTX(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return readTTXBytes(t, pool, cfg, []byte(snippet), "test.ttx", filterParams)
}

// readTTXDefault parses a TTX snippet with default (nil) filter params.
func readTTXDefault(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readTTX(t, snippet, nil)
}

// readTTXBytes reads TTX content with the correct EN-US / ES-EM locales.
func readTTXBytes(t *testing.T, pool *bridge.BridgeRegistry, cfg bridge.BridgeConfig, content []byte, uri string, filterParams map[string]any) []*model.Part {
	t.Helper()

	reader := bridge.NewBridgeFormatReader(pool, cfg, filterClass, format.FormatSignature{})
	if filterParams != nil {
		reader.SetFilterParams(filterParams)
	}

	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: "en-us",
		TargetLocale: "es-em",
		Encoding:     "UTF-8",
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

// readTTXFile reads a TTX file from testdata and returns parts.
func readTTXFile(t *testing.T, relPath string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return readTTXBytes(t, pool, cfg, content, path, filterParams)
}

// allBlocks returns all blocks (translatable and non-translatable) from parts.
func allBlocks(parts []*model.Part) []*model.Block {
	return bridgetest.FilterBlocks(parts)
}

// snippetRoundtrip roundtrips a TTX snippet using EN-US / ES-EM locales
// and returns the output string.
func snippetRoundtrip(t *testing.T, snippet string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTripWithLocales(t, pool, cfg, filterClass,
		[]byte(snippet), "test.ttx", mimeType, filterParams,
		"en-us", "es-em")
	return string(result.Output)
}

// fileRoundtrip roundtrips a testdata file using locales appropriate for TTX
// testdata (en-us / fr-fr, which matches the Test01/Test02 files).
func fileRoundtrip(t *testing.T, relPath string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	result := bridgetest.RoundTripWithLocales(t, pool, cfg, filterClass,
		content, path, mimeType, filterParams,
		"en-us", "fr-fr")
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
