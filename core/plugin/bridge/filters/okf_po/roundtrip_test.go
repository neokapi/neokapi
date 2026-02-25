//go:build integration

package okf_po

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := []byte("msgid \"Hello World\"\nmsgstr \"\"\n")
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.po", mimeType, nil)
}

func TestRoundTrip_WithTarget(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := []byte("msgid \"Hello\"\nmsgstr \"Bonjour\"\n")
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.po", mimeType, nil)
}

func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_po/*.po", mimeType, nil)
}
