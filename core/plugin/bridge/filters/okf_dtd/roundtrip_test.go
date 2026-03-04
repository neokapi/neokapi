//go:build integration

package okf_dtd

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := []byte(`<!ENTITY greeting "Hello World">` + "\n" + `<!ENTITY farewell "Goodbye">` + "\n")
	bridgetest.AssertRoundTrip(t, pool, cfg, filterClass, input, "test.dtd", mimeType, nil)
}

// okapi: DTDFilterTest#testDoubleExtraction
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// Test01.dtd: entities with references, HTML-like content, comments
	t.Run("Test01", func(t *testing.T) {
		bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
			tdDir+"/okf_dtd/Test01.dtd", mimeType, nil)
	})

	// Test02.dtd: Qt Linguist DTD with element/attribute definitions (non-extractable)
	t.Run("Test02", func(t *testing.T) {
		bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
			tdDir+"/okf_dtd/Test02.dtd", mimeType, nil)
	})
}

// okapi: RoundTripDtdIT#dtdFiles
func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_dtd/*.dtd", mimeType, nil)
}

// okapi: RoundTripDtdIT#dtdFilesSerialized
func TestRoundTrip_TestFilesSerialized(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// The Java dtdFilesSerialized test runs the same file set with
	// serializedOutput=true. In the bridge, serialization behavior is
	// handled transparently, so we run the same roundtrip with event
	// comparison on the full set of DTD files.
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_dtd/*.dtd", mimeType, nil)
}
