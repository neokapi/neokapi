//go:build integration

package xliff2

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

// okapi: RoundTripXliff2IT
func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// Known failing:
	// - translated.xlf: Okapi XLIFF2 reader throws CTag->MTag ClassCastException
	//   when re-reading the roundtripped output (Okapi bug, not bridge bug).
	// - invalid-target.xlf: Invalid XLIFF 2.0 file -- references <data> elements
	//   without declaring <originalData>. Okapi rejects it on read.
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_xliff2/*.xlf", mimeType, nil,
		"translated.xlf",
		"invalid-target.xlf",
	)
}

func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>Hello world</source>
      </segment>
    </unit>
  </file>
</xliff>`)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.xlf", mimeType, nil)
}

// okapi: Xliff2FilterWriterTest#testWriteHTMLAsXliff2
func TestWrite_WriteHTMLAsXliff2(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// The Java Xliff2FilterWriterTest#testWriteHTMLAsXliff2 roundtrips
	// the gold XLIFF 2 output file to verify the writer produces valid output.
	path := bridgetest.TestdataFile(t, "okf_xliff2/test02.xlf")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}
