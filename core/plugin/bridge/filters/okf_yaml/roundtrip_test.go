//go:build integration

package okf_yaml

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := []byte("greeting: Hello World\nfarewell: Goodbye\n")
	bridgetest.AssertRoundTrip(t, pool, cfg, filterClass, input, "test.yaml", mimeType, nil)
}

func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// Known failing:
	// - no-children-1-pretty.yaml: Okapi limitation — YAML parser rejects
	//   !!timestamp and other YAML tags (limited JavaCC grammar).
	// Note: unknown-tags-example, ios_emoji_surrogate, example2_17,
	// example2_17_control are in subdirectories and don't match *.yaml glob.
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_yaml/*.yaml", mimeType, nil,
		"no-children-1-pretty.yaml",
	)
}
