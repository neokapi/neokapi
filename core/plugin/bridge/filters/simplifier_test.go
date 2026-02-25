//go:build integration

package filters

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

// simplifierTestEntry defines a filter input to test with the simplifier.
type simplifierTestEntry struct {
	name        string
	filterClass string
	content     string
	uri         string
	mimeType    string
}

// simplifierTestFilters lists filters to include in roundtrip-with-simplifier testing.
// Entries are added as each filter's tests are implemented.
var simplifierTestFilters = []simplifierTestEntry{
	{
		name:        "html",
		filterClass: "net.sf.okapi.filters.html.HtmlFilter",
		content:     "<html><body><p>Hello <b>world</b></p></body></html>",
		uri:         "test.html",
		mimeType:    "text/html",
	},
}

// TestRoundTripSimplifier verifies that extraction → write roundtrips work
// correctly for each filter when inline codes are present. The simplifier
// rules in Okapi reduce complex inline markup to simpler coded text
// representations. This test ensures the bridge preserves that correctly.
func TestRoundTripSimplifier(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	for _, entry := range simplifierTestFilters {
		t.Run(entry.name, func(t *testing.T) {
			result := bridgetest.RoundTrip(t, pool, cfg,
				entry.filterClass, []byte(entry.content),
				entry.uri, entry.mimeType, nil)

			require.NotEmpty(t, result.Parts, "should produce parts")
			require.NotEmpty(t, result.Output, "should produce output")
		})
	}
}
