//go:build integration

package yaml

import (
	"os"
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// readFileBytes reads a file and returns its contents. Fails the test on error.
func readFileBytes(t *testing.T, path string) []byte {
	t.Helper()
	content, err := os.ReadFile(path)
	require.NoError(t, err, "reading file %s", path)
	return content
}

// okapi: YmlFilterTest#testDoubleExtraction
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// The Java testDoubleExtraction loads en.yml and runs extract twice,
	// comparing the events. In the bridge, this is an event-level roundtrip.
	path := tdDir + "/okf_yaml/en.yml"
	content := readFileBytes(t, path)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}

// okapi: YmlFilterTest#testDoubleExtractionWithEscapes
func TestRoundTrip_DoubleExtractionWithEscapes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	path := tdDir + "/okf_yaml/escapes.yml"
	content := readFileBytes(t, path)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}

// okapi: YmlFilterTest#testDoubleExtractionNonStrings
func TestRoundTrip_DoubleExtractionNonStrings(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	path := tdDir + "/okf_yaml/non_strings.yaml"
	content := readFileBytes(t, path)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}

// okapi: YmlFilterTest#testDoubleExtractionLongLine
func TestRoundTrip_DoubleExtractionLongLine(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	path := tdDir + "/okf_yaml/long_line.yml"
	content := readFileBytes(t, path)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}

// okapi: YmlFilterTest#testDoubleExtractionWithMultilines
func TestRoundTrip_DoubleExtractionWithMultilines(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	path := tdDir + "/okf_yaml/folded_literal_examples.yml"
	content := readFileBytes(t, path)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}

// okapi: YmlFilterTest#commentsAfterScalarsRoundTripped
func TestRoundTrip_CommentsAfterScalarsRoundTripped(t *testing.T) {
	// Comments after plain scalars should survive roundtrip.
	snippet := "key: value # important comment\nkey2: other # another comment\n"
	result := snippetRoundtrip(t, snippet, nil)

	assert.Contains(t, result, "# important comment")
	assert.Contains(t, result, "# another comment")
}

// okapi: YmlFilterTest#testRoundTripSubFilterProcessLiteralAsBlock
func TestRoundTrip_SubFilterProcessLiteralAsBlock(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	path := tdDir + "/okf_yaml/literal_html.yml"
	params := map[string]any{
		"subfilter":                      "okf_html",
		"subFilterProcessLiteralAsBlock": true,
	}
	content := readFileBytes(t, path)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, params)
}

// okapi: YmlFilterTest#testRoundtripFailures
func TestRoundTrip_RoundtripFailures(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// The Java test runs roundtrip on several test data files to verify
	// they all pass. We run the same files through event-level roundtrip.
	files := []struct {
		name string
		path string
	}{
		{"Test01", tdDir + "/okf_yaml/Test01.yml"},
		{"Test02", tdDir + "/okf_yaml/Test02.yml"},
		{"Test03", tdDir + "/okf_yaml/Test03.yml"},
	}

	for _, f := range files {
		t.Run(f.name, func(t *testing.T) {
			content := readFileBytes(t, f.path)
			bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, f.path, mimeType, nil)
		})
	}
}

// okapi: RoundTripYamlIT
func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// Known failing:
	// - no-children-1-pretty.yaml: Okapi limitation - YAML parser rejects
	//   !!timestamp and other YAML tags (limited JavaCC grammar).
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_yaml/*.yaml", mimeType, nil,
		"no-children-1-pretty.yaml",
	)
}

// okapi: RoundTripYamlIT (yml extension)
func TestRoundTrip_TestFilesYML(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_yaml/*.yml", mimeType, nil,
	)
}

func TestRoundTrip_SimpleSnippet(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := []byte("greeting: Hello World\nfarewell: Goodbye\n")
	bridgetest.AssertRoundTrip(t, pool, cfg, filterClass, input, "test.yaml", mimeType, nil)
}
