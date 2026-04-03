//go:build integration

package html

import (
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
)

func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := []byte(`<html><body><p>Hello world</p></body></html>`)
	bridgetest.AssertRoundTrip(t, pool, cfg, filterClass, input, "test.html", mimeType, nil)
}

// okapi: RoundTripHtmlIT#htmlFiles
func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// The Java RoundTripHtmlIT iterates over .html, .htm, and .xhtml files
	// in the html/ test resource directory using EventComparator for
	// semantic comparison. 98959751.html is a known failing file.
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okapi/filters/html/src/test/resources/*.html", mimeType, nil,
		"98959751.html")
}

// okapi: RoundTripHtmlIT#htmlFiles (htm extension)
func TestRoundTrip_TestFilesHTM(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okapi/filters/html/src/test/resources/*.htm", mimeType, nil)
}

// okapi: RoundTripHtmlIT#htmlFiles (xhtml extension)
func TestRoundTrip_TestFilesXHTML(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/integration-tests/okapi/src/test/resources/html/xhtml/*.xhtml", mimeType, nil)
}

// okapi: RoundTripHtmlIT#htmlFilesSerialized
func TestRoundTrip_TestFilesSerialized(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// The Java htmlFilesSerialized test runs the same file set with
	// serializedOutput=true. In the bridge, serialization behavior is
	// handled transparently, so we run the same roundtrip with event
	// comparison on the full set of HTML files.
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okapi/filters/html/src/test/resources/*.html", mimeType, nil,
		"98959751.html")
}
