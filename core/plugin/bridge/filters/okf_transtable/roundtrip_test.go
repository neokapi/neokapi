//go:build integration

package okf_transtable

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

// okapi: RoundTripTranstableIT#transtableFiles
// okapi: RoundTripTranstableIT#transtableSerializedFiles
func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// test01.xml.txt is the only roundtrip file and is known failing in Java
	// integration tests (RoundTripTranstableIT adds it to knownFailingFiles).
	// The TransTable writer omits the header row, so re-reading the output
	// fails with "Unexpected header."
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_transtable/*.txt", mimeType, nil,
		"test01.xml.txt", // Known failing in Java integration tests
	)
}

// okapi-skip: TranstableXliffCompareIT — XLIFF compare tests are Java-only (compare extracted XLIFF output)
