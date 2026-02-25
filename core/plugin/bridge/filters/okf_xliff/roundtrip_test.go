//go:build integration

package okf_xliff

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// Known failing files:
	// - empty-tgt-lang.xlf: has empty target-language attr, adding one creates duplicate
	// - lqiTest.xlf: references external lqiTestIssues.xml not in testdata
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_xliff/*.xlf", mimeType, nil,
		"empty-tgt-lang.xlf",
		"lqiTest.xlf",
	)
}
