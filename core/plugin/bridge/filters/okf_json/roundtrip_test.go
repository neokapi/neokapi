//go:build integration

package okf_json

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := []byte(`{"greeting": "Hello World"}`)
	bridgetest.AssertRoundTrip(t, pool, cfg, filterClass, input, "test.json", mimeType, nil)
}

func TestRoundTrip_NestedObjects(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := []byte(`{"page":{"title":"Welcome","body":"Hello World"}}`)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.json", mimeType, nil)
}

func TestRoundTrip_MultipleKeys(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := []byte(`{"a":"First","b":"Second","c":"Third"}`)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.json", mimeType, nil)
}

func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_json/*.json", mimeType, nil)
}
