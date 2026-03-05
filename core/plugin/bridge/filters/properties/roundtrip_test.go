//go:build integration

package properties

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

// okapi: PropertiesFilterTest#testDoubleExtraction
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// Test01: default config
	t.Run("Test01", func(t *testing.T) {
		bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
			tdDir+"/okf_properties/Test01.properties", mimeType, nil)
	})

	// Test02: useKeyCondition=true, extractOnlyMatchingKey=true
	t.Run("Test02", func(t *testing.T) {
		params := map[string]any{
			"useKeyCondition":        true,
			"extractOnlyMatchingKey": true,
			"keyCondition":           ".*text.*",
		}
		bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
			tdDir+"/okf_properties/Test02.properties", mimeType, params)
	})

	// Test03: useKeyCondition=true, extractOnlyMatchingKey=false
	t.Run("Test03", func(t *testing.T) {
		params := map[string]any{
			"useKeyCondition":        true,
			"extractOnlyMatchingKey": false,
			"keyCondition":           ".*text.*",
		}
		bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
			tdDir+"/okf_properties/Test03.properties", mimeType, params)
	})

	// Test04: default config
	t.Run("Test04", func(t *testing.T) {
		bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
			tdDir+"/okf_properties/Test04.properties", mimeType, nil)
	})

	// issue_216: subfilter config — blocked by bridge limitation
	// okapi-blocked: subfilter requires FilterConfigurationMapper
	t.Run("issue_216", func(t *testing.T) {
		t.Skip("bridge limitation: Properties filter subfilter requires FilterConfigurationMapper (fcMapper is null)")
	})
}

// okapi-blocked: PropertiesFilterTest#testDoubleExtractionSubFilter — bridge does not set up FilterConfigurationMapper for subfilter resolution
// okapi: PropertiesFilterTest#testDoubleExtractionSubFilter
func TestRoundTrip_DoubleExtractionSubFilter(t *testing.T) {
	t.Skip("bridge limitation: Properties filter subfilter requires FilterConfigurationMapper (fcMapper is null)")
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
		tdDir+"/okf_properties/*.properties", mimeType, nil)
}
