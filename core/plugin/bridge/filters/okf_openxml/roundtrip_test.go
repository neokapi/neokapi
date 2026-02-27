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

	// Known failing: Okapi OpenXML filter limitations (not bridge bugs).
	// - 1102, 847-2, 847-3, 956: Structural Data parts dropped during roundtrip
	//   in documents with tracked changes and complex fields.
	// - 830-3: Structural Data part added during roundtrip (Okapi normalizes
	//   skeleton between consecutive blocks).
	// - 1437-color-exclusion: Span Type CSS property order changes after roundtrip
	//   (HashMap iteration order instability in Okapi).
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/*.docx", mimeType, nil,
		"1102.docx",
		"1437-color-exclusion.docx",
		"830-3.docx",
		"847-2.docx",
		"847-3.docx",
		"956.docx",
	)
}

func TestRoundTrip_Xlsx(t *testing.T) {
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

	// Known failing: Okapi OpenXML filter loses PPTX style metadata during
	// roundtrip (style inheritance collapse, font stack truncation).
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/*.pptx", mimeType, nil,
		"1329-styles-clarification.pptx",
		"1435-text-for-masking.pptx",
	)
}
