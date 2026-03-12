//go:build integration

package icml

import (
	"os"
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.icml.ICMLFilter"
const mimeType = "application/x-icml+xml"

// readICML parses an ICML file from testdata with custom filter params and returns the parts.
func readICMLFile(t *testing.T, relPath string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	return bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, filterParams)
}

// readICMLFileWithParams reads an ICML file from testdata with specific parameters.
func readICMLFileWithParams(t *testing.T, relPath string, filterParams map[string]any) []*model.Part {
	t.Helper()
	return readICMLFile(t, relPath, filterParams)
}

// allBlocks returns all blocks (translatable and non-translatable) from parts.
func allBlocks(parts []*model.Part) []*model.Block {
	return bridgetest.FilterBlocks(parts)
}

// fileRoundtrip roundtrips a testdata file and returns the output string.
func fileRoundtrip(t *testing.T, relPath string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, filterParams)
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
