//go:build integration

package okf_xliff

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>Hello world</source>
      </trans-unit>
    </body>
  </file>
</xliff>`)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, xliff, "test.xlf", mimeType, nil)
}

func TestRoundTrip_WithTarget(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>Hello</source>
        <target>Bonjour</target>
      </trans-unit>
    </body>
  </file>
</xliff>`)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, xliff, "test.xlf", mimeType, nil)
}

func TestRoundTrip_InlineCodes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="htmlbody" original="test">
    <body>
      <trans-unit id="1">
        <source>Click <bpt id="1">&lt;b&gt;</bpt>here<ept id="1">&lt;/b&gt;</ept></source>
      </trans-unit>
    </body>
  </file>
</xliff>`)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, xliff, "test.xlf", mimeType, nil)
}

func TestRoundTrip_MultipleUnits(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	xliff := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<xliff version="1.2" xmlns="urn:oasis:names:tc:xliff:document:1.2">
  <file source-language="en" target-language="fr" datatype="plaintext" original="test">
    <body>
      <trans-unit id="1">
        <source>First</source>
      </trans-unit>
      <trans-unit id="2">
        <source>Second</source>
      </trans-unit>
      <trans-unit id="3">
        <source>Third</source>
      </trans-unit>
    </body>
  </file>
</xliff>`)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, xliff, "test.xlf", mimeType, nil)
}

func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// Known failing:
	// - empty-tgt-lang.xlf: Bridge bug — the write phase adds target-language="fr"
	//   without removing the existing empty target-language="" attribute, producing
	//   a duplicate XML attribute that fails on re-read.
	// - TS09-12-Test01.xlf: Okapi assigns non-deterministic integer IDs to inline
	//   codes (bpt/ept) across reads, causing span ID mismatch in event roundtrip.
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_xliff/*.xlf", mimeType, nil,
		"empty-tgt-lang.xlf",
		"TS09-12-Test01.xlf",
	)
}
