//go:build integration

package xliff

import (
	"os"
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

// readXLIFF reads an XLIFF snippet with the given filter params and returns parts.
func readXLIFF(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, filterClass,
		snippet, "test.xlf", mimeType, filterParams)
}

// readXLIFFDefault reads an XLIFF snippet with default filter params.
func readXLIFFDefault(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readXLIFF(t, snippet, nil)
}

// readXLIFFFile reads an XLIFF file from testdata with the given filter params.
func readXLIFFFile(t *testing.T, relPath string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	return bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, filterParams)
}

// snippetRoundtrip writes XLIFF through the bridge and returns the output string.
func snippetRoundtrip(t *testing.T, snippet string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass,
		[]byte(snippet), "test.xlf", mimeType, filterParams)
	return string(result.Output)
}

// fileRoundtrip reads an XLIFF file, round-trips it, and returns the output string.
func fileRoundtrip(t *testing.T, relPath string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass,
		data, relPath, mimeType, filterParams)
	return string(result.Output)
}

// fileRoundtripEvents reads an XLIFF file and asserts event-level roundtrip equality.
func fileRoundtripEvents(t *testing.T, relPath string, filterParams map[string]any) {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		data, relPath, mimeType, filterParams)
}

// findBlockContaining returns the first block whose source text contains substr.
func findBlockContaining(blocks []*model.Block, substr string) *model.Block {
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), substr) {
			return b
		}
	}
	return nil
}

// wrapXLIFF wraps body content in a standard XLIFF 1.2 envelope.
func wrapXLIFF(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
` + body + `
    </body>
  </file>
</xliff>`
}

// wrapXLIFFNoNS wraps body in XLIFF without namespace (for testing namespace handling).
func wrapXLIFFNoNS(body string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
` + body + `
    </body>
  </file>
</xliff>`
}

// wrapXLIFFDatatype wraps body with a specific datatype attribute.
func wrapXLIFFDatatype(body, datatype string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="` + datatype + `" original="test">
    <body>
` + body + `
    </body>
  </file>
</xliff>`
}
