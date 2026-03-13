//go:build integration

package table

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

// Filter class constants for each sub-filter.
const (
	csvFilterClass   = "net.sf.okapi.filters.table.csv.CommaSeparatedValuesFilter"
	tsvFilterClass   = "net.sf.okapi.filters.table.tsv.TabSeparatedValuesFilter"
	fwcFilterClass   = "net.sf.okapi.filters.table.fwc.FixedWidthColumnsFilter"
	tableFilterClass = "net.sf.okapi.filters.table.base.BaseTableFilter"
)

// MIME type constants for each sub-filter.
const (
	csvMimeType   = "text/csv"
	tsvMimeType   = "text/tab-separated-values"
	fwcMimeType   = "text/plain"
	tableMimeType = "text/plain"
)

// testdata directory prefix for all okf_table test files.
const tableTestdataPrefix = "okapi/filters/table/src/test/resources/"

// --- CSV helpers ---

// readCSV parses a CSV snippet with custom filter params and returns the parts.
func readCSV(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, csvFilterClass, snippet, "test.csv", csvMimeType, filterParams)
}

// readCSVDefault parses a CSV snippet with default (nil) params.
func readCSVDefault(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readCSV(t, snippet, nil)
}

// readCSVFile reads a CSV test file from testdata and returns parts.
func readCSVFile(t *testing.T, relPath string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	return bridgetest.ReadFile(t, pool, cfg, csvFilterClass, path, csvMimeType, filterParams)
}

// csvRoundtrip roundtrips a CSV snippet and returns the output string.
func csvRoundtrip(t *testing.T, snippet string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTrip(t, pool, cfg, csvFilterClass, []byte(snippet), "test.csv", csvMimeType, filterParams)
	return string(result.Output)
}

// csvFileRoundtrip roundtrips a testdata file with the CSV filter and returns the output string.
func csvFileRoundtrip(t *testing.T, relPath string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	result := bridgetest.RoundTrip(t, pool, cfg, csvFilterClass, content, path, csvMimeType, filterParams)
	return string(result.Output)
}

// csvFileRoundtripResult roundtrips a testdata file and returns the full result.
func csvFileRoundtripResult(t *testing.T, relPath string, filterParams map[string]any) bridgetest.RoundTripResult {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	return bridgetest.RoundTrip(t, pool, cfg, csvFilterClass, content, path, csvMimeType, filterParams)
}

// --- TSV helpers ---

// readTSV parses a TSV snippet with custom filter params and returns the parts.
func readTSV(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, tsvFilterClass, snippet, "test.tsv", tsvMimeType, filterParams)
}

// readTSVFile reads a TSV test file from testdata and returns parts.
func readTSVFile(t *testing.T, relPath string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	return bridgetest.ReadFile(t, pool, cfg, tsvFilterClass, path, tsvMimeType, filterParams)
}

// tsvFileRoundtrip roundtrips a testdata file with the TSV filter and returns the output string.
func tsvFileRoundtrip(t *testing.T, relPath string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	result := bridgetest.RoundTrip(t, pool, cfg, tsvFilterClass, content, path, tsvMimeType, filterParams)
	return string(result.Output)
}

// --- FWC helpers ---

// readFWC parses a FWC snippet with custom filter params and returns the parts.
func readFWC(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, fwcFilterClass, snippet, "test.txt", fwcMimeType, filterParams)
}

// readFWCFile reads a FWC test file from testdata and returns parts.
func readFWCFile(t *testing.T, relPath string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	return bridgetest.ReadFile(t, pool, cfg, fwcFilterClass, path, fwcMimeType, filterParams)
}

// fwcFileRoundtrip roundtrips a testdata file with the FWC filter and returns the output string.
func fwcFileRoundtrip(t *testing.T, relPath string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	result := bridgetest.RoundTrip(t, pool, cfg, fwcFilterClass, content, path, fwcMimeType, filterParams)
	return string(result.Output)
}

// --- Table (base) helpers ---

// readTable parses content with the base table filter and returns the parts.
func readTable(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, tableFilterClass, snippet, "test.txt", tableMimeType, filterParams)
}

// readTableFile reads a test file from testdata with the base table filter.
func readTableFile(t *testing.T, relPath string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	return bridgetest.ReadFile(t, pool, cfg, tableFilterClass, path, tableMimeType, filterParams)
}

// tableFileRoundtrip roundtrips a testdata file with the base table filter.
func tableFileRoundtrip(t *testing.T, relPath string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	result := bridgetest.RoundTrip(t, pool, cfg, tableFilterClass, content, path, tableMimeType, filterParams)
	return string(result.Output)
}

// --- Shared utility functions ---

// tdDir returns the okf_table testdata base directory.
func tdDir(t *testing.T) string {
	t.Helper()
	return bridgetest.TestdataDir(t) + "/okapi/filters/table/src/test/resources"
}

// configParams returns filterParams with configFile set to the given .fprm path.
func configParams(configPath string) map[string]any {
	return map[string]any{"configFile": configPath}
}

// allBlocks returns all blocks (translatable and non-translatable) from parts.
func allBlocks(parts []*model.Part) []*model.Block {
	return bridgetest.FilterBlocks(parts)
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

// sharedPool returns the shared bridge pool and config.
func sharedPool(t *testing.T) (*bridge.BridgePool, bridge.BridgeConfig) {
	t.Helper()
	return bridgetest.SharedBridge(t)
}

// readFileContent reads a file from disk.
func readFileContent(path string) ([]byte, error) {
	return os.ReadFile(path)
}
