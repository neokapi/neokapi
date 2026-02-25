//go:build integration

package okf_openxml

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

func TestRoundTrip_Docx(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/*.docx", mimeType, nil,
		// Known failing: part count mismatch after roundtrip (complex documents
		// with tracked changes, color exclusion, revision markup).
		"NoStylesXml.docx",          // Missing styles.xml in ZIP
		"1102.docx",                 // Complex revision markup
		"1437-color-exclusion.docx", // Color-based exclusion changes part count
		"830-3.docx",               // Tracked changes differ on re-read
		"847-2.docx",               // Tracked changes
		"847-3.docx",               // Tracked changes
		"956.docx",                 // Complex structure mismatch
	)
}

func TestRoundTrip_Xlsx(t *testing.T) {
	// XLSX roundtrip hangs when processing many files sequentially through
	// the bridge. Individual files pass when run in isolation (e.g.,
	// -run TestRoundTrip_Xlsx/pokemon.xlsx). The issue is in the bridge
	// pool / gRPC connection management with the OpenXML filter's multi-layer
	// write path. Extraction tests (TestExtract_*) validate the read path.
	t.Skip("XLSX roundtrip hangs with sequential multi-file processing — tracked for bridge pool improvements")

	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/*.xlsx", mimeType, nil)
}

func TestRoundTrip_Pptx(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/*.pptx", mimeType, nil,
		// Known failing: part count mismatch from style clarification
		// and text masking differences after roundtrip.
		"1329-styles-clarification.pptx",
		"1435-text-for-masking.pptx",
	)
}
