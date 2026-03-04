//go:build integration

package okf_ttml

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

// okapi: RoundTripTtmlIT#ttmlFiles
// okapi: RoundTripTtmlIT#ttmlSerializedFiles
// Runs roundtrip tests for all .ttml files in the okapi-testdata directory.
// The Java RoundTripTtmlIT runs both non-serialized and serialized modes with
// EventComparator; in the bridge, serialization is transparent, so we use
// AssertRoundTripEvents which re-reads the output and compares events.
func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_ttml/*.ttml", mimeType, nil)
}

// okapi-skip: TtmlXliffCompareIT — XLIFF compare tests require XLIFF serialization
// which is outside the scope of bridge filter roundtrip tests.
