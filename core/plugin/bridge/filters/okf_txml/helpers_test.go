//go:build integration

package okf_txml

import (
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

const filterClass = "net.sf.okapi.filters.txml.TXMLFilter"
const mimeType = "text/xml"

// STARTFILE mirrors the Java TXMLFilterTest.STARTFILE constant.
const STARTFILE = "<?xml version=\"1.0\" encoding=\"UTF-8\"?>\r" +
	"<txml locale=\"en\" version=\"1.0\" segtype=\"sentence\" createdby=\"WF2.3.0\" datatype=\"regexp\" " +
	"targetlocale=\"fr\" file_extension=\"html\" editedby=\"WF2.3.0\">\r" +
	"<skeleton>&lt;html&gt;\r" +
	"&lt;p&gt;</skeleton>"

// readTXML parses a TXML snippet with custom filter params and returns the parts.
func readTXML(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.txml", mimeType, filterParams)
}

// readTXMLDefault parses a TXML snippet with default (nil) params.
func readTXMLDefault(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readTXML(t, snippet, nil)
}

// readTXMLFile reads a TXML file from testdata and returns parts.
func readTXMLFile(t *testing.T, relPath string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	return bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, filterParams)
}

// allBlocks returns all blocks (translatable and non-translatable) from parts.
func allBlocks(parts []*model.Part) []*model.Block {
	return bridgetest.FilterBlocks(parts)
}

// snippetRoundtrip roundtrips a TXML snippet and returns the output string.
func snippetRoundtrip(t *testing.T, snippet string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, []byte(snippet), "test.txml", mimeType, filterParams)
	return string(result.Output)
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
