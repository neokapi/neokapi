//go:build integration

package mif

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
)

// okapi: RoundTripTest#consequentialEmptyParaLinesMerged
func TestRoundTrip_ConsequentialEmptyParaLinesMerged(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okapi/filters/mif/src/test/resources/1187_crlf.mif")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading test file: %v", err)
	}

	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}

// okapi: RoundTripTest#tabsEncodedOnExtractionAndHardReturnsEncodedOnMerge
func TestRoundTrip_TabsEncodedOnExtractionAndHardReturnsEncodedOnMerge(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okapi/filters/mif/src/test/resources/1188_crlf.mif")
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading test file: %v", err)
	}

	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}

// okapi: RoundTripTest#hardReturnsAsNonTextualRoundTripped
func TestRoundTrip_HardReturnsAsNonTextualRoundTripped(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	params := configParams(t, "okf_mif@non-textual-hard-returns.fprm")

	// Parameterized test: 7 MIF files that test hard returns as non-textual.
	files := []string{
		"987.mif",
		"990-marker.mif",
		"990-pgf-num-format-1.mif",
		"990-pgf-num-format-2.mif",
		"990-ref-format-1.mif",
		"990-ref-format-2.mif",
		"990-text-line.mif",
	}

	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			path := bridgetest.TestdataFile(t, "okapi/filters/mif/src/test/resources/"+f)
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading test file %s: %v", f, err)
			}

			bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, params)
		})
	}
}

// okapi: RoundTripTest#roundTripsWithDifferentParameters
func TestRoundTrip_WithDifferentParameters(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	// Parameterized test: 29 MIF files that must roundtrip successfully.
	files := []string{
		"893.mif",
		"895.mif",
		"896.mif",
		"896-changed.mif",
		"896-autonumber-building-blocks.mif",
		"902-1.mif",
		"902-2.mif",
		"902-3.mif",
		"904.mif",
		"909-1.mif",
		"909-2.mif",
		"909-3.mif",
		"938-2.mif",
		"942-1.mif",
		"942-2.mif",
		"945.mif",
		"987.mif",
		"ImportedText.mif",
		"JATest.mif",
		"Test01.mif",
		"Test01-v8.mif",
		"Test02-v9.mif",
		"Test03.mif",
		"Test04.mif",
		"TestEncoding-v9.mif",
		"TestEncoding-v10.mif",
		"TestFootnote.mif",
		"TestMarkers.mif",
		"TestParaLines.mif",
	}

	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			path := bridgetest.TestdataFile(t, "okapi/filters/mif/src/test/resources/"+f)
			content, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading test file %s: %v", f, err)
			}

			bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
		})
	}
}
