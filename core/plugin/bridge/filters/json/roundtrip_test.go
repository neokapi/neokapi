//go:build integration

package json

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Roundtrip / Double Extraction tests — translated from JSONFilterTest.java
// and RoundTripJsonIT.java.
// ---------------------------------------------------------------------------

// okapi: JSONFilterTest#testDoubleExtraction
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	// Java testDoubleExtraction runs double extraction (read→write→re-read→compare)
	// on 14 test files. We replicate this with AssertRoundTripEvents per file.
	files := []string{
		"okapi/filters/json/src/test/resources/1EdwardParallax.json",
		"okapi/filters/json/src/test/resources/array-test.json",
		"okapi/filters/json/src/test/resources/books.json",
		"okapi/filters/json/src/test/resources/geo.json",
		"okapi/filters/json/src/test/resources/test01.json",
		"okapi/filters/json/src/test/resources/test02.json",
		"okapi/filters/json/src/test/resources/test03.json",
		"okapi/filters/json/src/test/resources/test04.json",
		"okapi/filters/json/src/test/resources/test05.json",
		"okapi/filters/json/src/test/resources/test06.json",
		"okapi/filters/json/src/test/resources/test08.json",
		"okapi/filters/json/src/test/resources/test09.json",
		"okapi/filters/json/src/test/resources/twitter.json",
	}

	for _, f := range files {
		name := f
		t.Run(name, func(t *testing.T) {
			path := bridgetest.TestdataFile(t, f)
			content, err := os.ReadFile(path)
			require.NoError(t, err)
			bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
				content, path, mimeType, nil)
		})
	}
}

// okapi: JSONFilterTest#testDoubleExtractionOnPreviousFailure
func TestRoundTrip_DoubleExtractionOnPreviousFailure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okapi/filters/json/src/test/resources/customer_form.json")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, path, mimeType, nil)
}

// okapi: JSONFilterTest#testDoubleExtractionOnInvalid
func TestRoundTrip_DoubleExtractionOnInvalid(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okapi/filters/json/src/test/resources/invalid_by_most_processors.json")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, path, mimeType, nil)
}

// okapi: JSONFilterTest#testSubFilterDoubleExtraction
func TestRoundTrip_SubFilterDoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okapi/filters/json/src/test/resources/test07-subfilter.json")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, path, mimeType, map[string]any{
			"subfilter": "okf_html",
		})
}

// okapi: RoundTripJsonIT
func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// The Java RoundTripJsonIT iterates over all .json files in the
	// json/ test resource directory using EventComparator for
	// semantic comparison.
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okapi/filters/json/src/test/resources/*.json", mimeType, nil)
}
