//go:build integration

package okf_xmlstream

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := []byte(`<?xml version="1.0" encoding="UTF-8"?><root><text>Hello world</text></root>`)
	bridgetest.AssertRoundTrip(t, pool, cfg, filterClass, input, "test.xml", mimeType, nil)
}

func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_xmlstream/*.xml", mimeType, nil)
}
