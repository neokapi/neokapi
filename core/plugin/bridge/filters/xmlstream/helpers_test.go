//go:build integration

package xmlstream

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

// defaultXMLParams returns the well-formed HTML-like configuration used by
// XmlStreamEventTest and XmlSnippetsTest in the Java test suite. This mirrors
// loading xml_wellformedConfiguration.yml.
func defaultXMLParams(t *testing.T) map[string]any {
	t.Helper()
	tdDir := bridgetest.TestdataDir(t)
	return map[string]any{
		"configFile": filepath.Join(tdDir, "okf_xmlstream", "xml_wellformedConfiguration.yml"),
	}
}

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

// readXMLWithConfig parses an XML snippet with the well-formed configuration.
func readXMLWithConfig(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readXML(t, snippet, defaultXMLParams(t))
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

// findDataPartWithProperty finds the first Data part that has the given property key.
func findDataPartWithProperty(parts []*model.Part, key string) *model.Data {
	for _, p := range parts {
		if p.Type == model.PartData {
			d, ok := p.Resource.(*model.Data)
			if ok && d.Properties != nil {
				if _, exists := d.Properties[key]; exists {
					return d
				}
			}
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
