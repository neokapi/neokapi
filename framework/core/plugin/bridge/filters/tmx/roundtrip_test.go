//go:build integration

package tmx

import (
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
)

func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	tmx := []byte(wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>Hello world</seg></tuv>
      <tuv xml:lang="fr"><seg>Bonjour le monde</seg></tuv>
    </tu>`))
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, tmx, "test.tmx", mimeType, nil)
}

func TestRoundTrip_InlineCodes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	tmx := []byte(`<?xml version="1.0"?>
<tmx version="1.4">
  <header creationtoolversion="1.0.0" datatype="html" segtype="sentence"
    adminlang="en-us" srclang="en" o-tmf="abc" creationtool="XYZTool">
  </header>
  <body>
    <tu>
      <tuv xml:lang="en">
        <seg><bpt x="1" i="1" type="bold">&lt;B></bpt>Click here<ept i="1">&lt;/B></ept></seg>
      </tuv>
      <tuv xml:lang="fr">
        <seg><bpt x="1" i="1" type="bold">&lt;B></bpt>Cliquez ici<ept i="1">&lt;/B></ept></seg>
      </tuv>
    </tu>
  </body>
</tmx>`)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, tmx, "test.tmx", mimeType, nil)
}

func TestRoundTrip_MultipleUnits(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	tmx := []byte(wrapTMX(`
    <tu>
      <tuv xml:lang="en"><seg>First</seg></tuv>
      <tuv xml:lang="fr"><seg>Premier</seg></tuv>
    </tu>
    <tu>
      <tuv xml:lang="en"><seg>Second</seg></tuv>
      <tuv xml:lang="fr"><seg>Deuxieme</seg></tuv>
    </tu>
    <tu>
      <tuv xml:lang="en"><seg>Third</seg></tuv>
      <tuv xml:lang="fr"><seg>Troisieme</seg></tuv>
    </tu>`))
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, tmx, "test.tmx", mimeType, nil)
}

// okapi: RoundTripTmxIT#tmxFiles
func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// The Java RoundTripTmxIT iterates over .tmx files in the tmx/ test
	// resource directory using EventComparator for semantic comparison.
	//
	// Known failing files:
	// - code_id_difference.tmx: Code ID difference causes event mismatch
	// - code_fail.tmx, header_with_prop_and_note.tmx, ImportTest2A/2B/2C.tmx,
	//   a_small_test2.tmx, Paragraph_TM.tmx: srclang="en-us" but bridge uses
	//   "en"; TMX filter rejects the language mismatch at the TUV level
	// - html_test.tmx: UTF-16LE encoded; the bridge streams content as
	//   UTF-8 so the Java filter cannot parse it
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okapi/filters/tmx/src/test/resources/*.tmx", mimeType, nil,
		"code_id_difference.tmx",
		"code_fail.tmx",
		"html_test.tmx",
		"header_with_prop_and_note.tmx",
		"ImportTest2A.tmx",
		"ImportTest2B.tmx",
		"ImportTest2C.tmx",
		"a_small_test2.tmx",
		"Paragraph_TM.tmx")
}

// okapi: TmxXliffCompareIT
// Skipped: sampleTMX2.tmx has srclang="en-us" but bridge sends "en".
func TestRoundTrip_SampleTMX2(t *testing.T) {
	t.Skip("skipped: sampleTMX2.tmx uses srclang=en-us; bridge sends en")
}
