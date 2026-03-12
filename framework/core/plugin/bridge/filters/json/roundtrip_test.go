//go:build integration

package json

import (
	"os"
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
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
		"okf_json/1EdwardParallax.json",
		"okf_json/array-test.json",
		"okf_json/books.json",
		"okf_json/geo.json",
		"okf_json/test01.json",
		"okf_json/test02.json",
		"okf_json/test03.json",
		"okf_json/test04.json",
		"okf_json/test05.json",
		"okf_json/test06.json",
		"okf_json/test08.json",
		"okf_json/test09.json",
		"okf_json/twitter.json",
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

	path := bridgetest.TestdataFile(t, "okf_json/customer_form.json")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, path, mimeType, nil)
}

// okapi: JSONFilterTest#testDoubleExtractionOnInvalid
func TestRoundTrip_DoubleExtractionOnInvalid(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okf_json/invalid_by_most_processors.json")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
		content, path, mimeType, nil)
}

// okapi: JSONFilterTest#testSubFilterDoubleExtraction
func TestRoundTrip_SubFilterDoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okf_json/test07-subfilter.json")
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
		tdDir+"/okf_json/*.json", mimeType, nil)
}
