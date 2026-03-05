//go:build integration

package its

import (
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

const filterClass = "net.sf.okapi.filters.xml.XMLFilter"
const mimeType = "text/xml"

// readXML parses an XML snippet with custom filter params and returns the parts.
func readXML(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.xml", mimeType, filterParams)
}

// readXMLDefault parses an XML snippet with default (nil) params.
func readXMLDefault(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readXML(t, snippet, nil)
}

// readXMLWithConfig parses an XML snippet using a named .fprm config from testdata.
func readXMLWithConfig(t *testing.T, snippet string, configName string) []*model.Part {
	t.Helper()
	tdDir := bridgetest.TestdataDir(t)
	params := map[string]any{
		"configFile": tdDir + "/okf_xml/" + configName,
	}
	return readXML(t, snippet, params)
}

// readXMLFile reads a testdata file with optional config.
func readXMLFile(t *testing.T, relPath string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	return bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, filterParams)
}

// allBlocks returns all blocks (translatable and non-translatable) from parts.
func allBlocks(parts []*model.Part) []*model.Block {
	return bridgetest.FilterBlocks(parts)
}

// findBlockContaining finds a block whose source text contains the given substring.
func findBlockContaining(blocks []*model.Block, substr string) *model.Block {
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), substr) {
			return b
		}
	}
	return nil
}

// snippetRoundtrip roundtrips an XML snippet and returns the output string.
func snippetRoundtrip(t *testing.T, snippet string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, []byte(snippet), "test.xml", mimeType, filterParams)
	return string(result.Output)
}

// snippetRoundtripWithURI roundtrips an XML snippet with a custom URI.
func snippetRoundtripWithURI(t *testing.T, snippet string, uri string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, []byte(snippet), uri, mimeType, filterParams)
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
