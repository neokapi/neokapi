//go:build integration

package filters

import (
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

// --- abstractmarkup surefire: SimplifierRulesTest ---
//
// okapi-filter: abstractmarkup
// okapi-unmapped: SimplifierRulesTest#testCodeReduction01 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction02 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction03 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction04 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction05 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction06 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction07 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction08 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction09 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction10 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction11 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction12 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction13 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction14 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction15 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction16 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction17 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction18 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction19 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction20 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction21 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction22 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction23 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction24 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction25 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction26 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction27 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction28 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction29 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction30 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction31 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction32 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction33 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction34 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction35 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction36 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction37 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction38 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction39 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction40 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction41 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction42 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction43 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction44 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction45 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction46 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction47 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction48 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction49 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction50 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction51 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction52 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction53 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction54 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction55 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeReduction56 — Java-internal code simplification test
// okapi-unmapped: SimplifierRulesTest#testCodeWithGandXCodes — Java-internal code simplification test

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
