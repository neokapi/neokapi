//go:build integration

package properties

import (
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
)

// okapi: PropertiesFilterTest#testDoubleExtraction
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// Test01: default config
	t.Run("Test01", func(t *testing.T) {
		bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
			tdDir+"/okapi/filters/properties/src/test/resources/Test01.properties", mimeType, nil)
	})

	// Test02: useKeyCondition=true, extractOnlyMatchingKey=true
	t.Run("Test02", func(t *testing.T) {
		params := map[string]any{
			"useKeyCondition":        true,
			"extractOnlyMatchingKey": true,
			"keyCondition":           ".*text.*",
		}
		bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
			tdDir+"/okapi/filters/properties/src/test/resources/Test02.properties", mimeType, params)
	})

	// Test03: useKeyCondition=true, extractOnlyMatchingKey=false
	t.Run("Test03", func(t *testing.T) {
		params := map[string]any{
			"useKeyCondition":        true,
			"extractOnlyMatchingKey": false,
			"keyCondition":           ".*text.*",
		}
		bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
			tdDir+"/okapi/filters/properties/src/test/resources/Test03.properties", mimeType, params)
	})

	// Test04: default config
	t.Run("Test04", func(t *testing.T) {
		bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
			tdDir+"/okapi/filters/properties/src/test/resources/Test04.properties", mimeType, nil)
	})

	// issue_216: subfilter config
	t.Run("issue_216", func(t *testing.T) {
		params := map[string]any{
			"subfilter": "okf_html",
		}
		bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
			tdDir+"/okapi/filters/properties/src/test/resources/issue_216.properties", mimeType, params)
	})
}

// okapi: PropertiesFilterTest#testDoubleExtractionSubFilter
func TestRoundTrip_DoubleExtractionSubFilter(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	params := map[string]any{
		"subfilter": "okf_html",
	}

	// Test01 with subfilter
	t.Run("Test01", func(t *testing.T) {
		bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
			tdDir+"/okapi/filters/properties/src/test/resources/Test01.properties", mimeType, params)
	})

	// Test04 with subfilter
	t.Run("Test04", func(t *testing.T) {
		bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
			tdDir+"/okapi/filters/properties/src/test/resources/Test04.properties", mimeType, params)
	})
}

func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := []byte("greeting=Hello World\nfarewell=Goodbye\n")
	bridgetest.AssertRoundTrip(t, pool, cfg, filterClass, input, "test.properties", mimeType, nil)
}

func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okapi/filters/properties/src/test/resources/*.properties", mimeType, nil)
}
