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

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_xliff2/*.xlf", mimeType, nil)
}
