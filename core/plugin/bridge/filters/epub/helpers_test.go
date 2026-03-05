//go:build integration

package epub

import (
	"os"
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.epub.EpubFilter"
const mimeType = "application/epub+zip"

// readEPUB reads an EPUB file from testdata/okf_epub/ and returns the extracted parts.
func readEPUB(t *testing.T, relPath string, params map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_epub/"+relPath)
	return bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, params)
}

// roundtripEPUB performs a roundtrip on an EPUB file and returns the result.
func roundtripEPUB(t *testing.T, relPath string, params map[string]any) bridgetest.RoundTripResult {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_epub/"+relPath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, params)
}

// assertRoundTripEventsEPUB performs a roundtrip event-level comparison on an EPUB file.
func assertRoundTripEventsEPUB(t *testing.T, relPath string, params map[string]any) bridgetest.RoundTripResult {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_epub/"+relPath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, params)
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
