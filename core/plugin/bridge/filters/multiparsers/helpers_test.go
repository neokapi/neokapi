//go:build integration

package multiparsers

import (
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

// okapi: MultiParsersFilter
const filterClass = "net.sf.okapi.filters.multiparsers.MultiParsersFilter"
const mimeType = "text/csv"

// readCSV parses a CSV snippet with custom filter params and returns the parts.
// okapi: MultiParsersFilter#open
func readCSV(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.csv", mimeType, filterParams)
}

// readCSVFile reads a CSV file from testdata and returns parts.
// okapi: MultiParsersFilter#open
func readCSVFile(t *testing.T, relPath string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	return bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, filterParams)
}

// allBlocks returns all blocks (translatable and non-translatable) from parts.
func allBlocks(parts []*model.Part) []*model.Block {
	return bridgetest.FilterBlocks(parts)
}

// snippetRoundtrip roundtrips a CSV snippet and returns the output string.
func snippetRoundtrip(t *testing.T, snippet string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, []byte(snippet), "test.csv", mimeType, filterParams)
	return string(result.Output)
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
