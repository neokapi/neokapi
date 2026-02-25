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

	// Known failing files from Java's RoundTripYamlIT:
	// - unknown-tags-example.yaml, no-children-1-pretty.yaml: !!timestamp tag rejected
	// - ios_emoji_surrogate.yaml, emoji1.yaml: emoji surrogate pair handling
	// - example2_17.yaml, example2_17_control.yaml: control character encoding
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_yaml/*.yaml", mimeType, nil,
		"unknown-tags-example.yaml",
		"no-children-1-pretty.yaml",
		"ios_emoji_surrogate.yaml",
		"emoji1.yaml",
		"example2_17.yaml",
		"example2_17_control.yaml",
	)
}
