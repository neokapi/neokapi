//go:build integration

package okf_xliff2

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// Known failing:
	// - translated.xlf: Okapi XLIFF2 reader throws CTag→MTag ClassCastException
	//   when re-reading the roundtripped output (Okapi bug, not bridge bug).
	// - invalid-target.xlf: Invalid XLIFF 2.0 file — references <data> elements
	//   without declaring <originalData>. Okapi rejects it on read.
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_xliff2/*.xlf", mimeType, nil,
		"translated.xlf",
		"invalid-target.xlf",
	)
}
