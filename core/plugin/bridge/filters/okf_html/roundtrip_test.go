//go:build integration

package okf_html

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := []byte(`<html><body><p>Hello world</p></body></html>`)
	bridgetest.AssertRoundTrip(t, pool, cfg, filterClass, input, "test.html", mimeType, nil)
}

// okapi: RoundTripHtmlIT
func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_html/*.html", mimeType, nil)
}
