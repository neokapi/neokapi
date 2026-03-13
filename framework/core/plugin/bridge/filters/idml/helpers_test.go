//go:build integration

package idml

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.idml.IDMLFilter"
const mimeType = "application/vnd.adobe.indesign-idml-package"

// readIDML reads an IDML file from testdata/okapi/filters/idml/src/test/resources/ and returns the extracted parts.
func readIDML(t *testing.T, relPath string, params map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okapi/filters/idml/src/test/resources/"+relPath)
	return bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, params)
}

// readIDMLWithConfig reads an IDML file using a config file (.fprm) from testdata.
func readIDMLWithConfig(t *testing.T, idmlRelPath, configRelPath string) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	path := filepath.Join(tdDir, "okapi", "filters", "idml", "src", "test", "resources", idmlRelPath)
	configPath := filepath.Join(tdDir, "okapi", "filters", "idml", "src", "test", "resources", configRelPath)
	params := map[string]any{
		"configFile": configPath,
	}
	return bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, params)
}

// roundtripIDML performs a roundtrip on an IDML file and returns the result.
func roundtripIDML(t *testing.T, relPath string, params map[string]any) bridgetest.RoundTripResult {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okapi/filters/idml/src/test/resources/"+relPath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, params)
}

// roundtripIDMLWithConfig performs a roundtrip on an IDML file with a config file.
func roundtripIDMLWithConfig(t *testing.T, idmlRelPath, configRelPath string) bridgetest.RoundTripResult {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	path := filepath.Join(tdDir, "okapi", "filters", "idml", "src", "test", "resources", idmlRelPath)
	configPath := filepath.Join(tdDir, "okapi", "filters", "idml", "src", "test", "resources", configRelPath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	params := map[string]any{
		"configFile": configPath,
	}
	return bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, params)
}

// assertRoundTripEventsIDML performs a roundtrip event-level comparison on an IDML file.
func assertRoundTripEventsIDML(t *testing.T, relPath string, params map[string]any) bridgetest.RoundTripResult {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okapi/filters/idml/src/test/resources/"+relPath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, params)
}

// assertRoundTripEventsIDMLWithConfig performs a roundtrip event-level comparison
// using a config file.
func assertRoundTripEventsIDMLWithConfig(t *testing.T, idmlRelPath, configRelPath string) bridgetest.RoundTripResult {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	path := filepath.Join(tdDir, "okapi", "filters", "idml", "src", "test", "resources", idmlRelPath)
	configPath := filepath.Join(tdDir, "okapi", "filters", "idml", "src", "test", "resources", configRelPath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	params := map[string]any{
		"configFile": configPath,
	}
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

// blocksWithSpans returns blocks that have at least one span in their first fragment.
func blocksWithSpans(blocks []*model.Block) []*model.Block {
	var result []*model.Block
	for _, b := range blocks {
		frag := b.FirstFragment()
		if frag != nil && len(frag.Spans) > 0 {
			result = append(result, b)
		}
	}
	return result
}
