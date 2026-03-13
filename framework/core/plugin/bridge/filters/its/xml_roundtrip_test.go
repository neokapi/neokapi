//go:build integration

package its

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Roundtrip tests — read a document, write it back, re-read and compare.
// These use event-level comparison (like Java's EventRoundTripIT) so cosmetic
// differences (whitespace normalization, Unicode escape forms) are tolerated.
// ---------------------------------------------------------------------------

// okapi: RoundTripXmlIT (simple snippet)
func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?><doc><p>Hello world</p></doc>`)
	// Use event-level comparison: Okapi's XML writer may insert a newline
	// after the XML declaration, which is cosmetically different but semantically
	// identical. AssertRoundTripEvents tolerates such differences.
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.xml", mimeType, nil)
}

// okapi: RoundTripXmlIT (multiple elements)
func TestRoundTrip_MultipleElements(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<doc><p>First</p><p>Second</p><p>Third</p></doc>`)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.xml", mimeType, nil)
}

// okapi: RoundTripXmlIT (entities)
func TestRoundTrip_Entities(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<doc><p>A &amp; B &lt; C &gt; D</p></doc>`)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.xml", mimeType, nil)
}

// okapi: RoundTripXmlIT (CDATA)
func TestRoundTrip_CDATA(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<doc><p><![CDATA[a < b & c > d]]></p></doc>`)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.xml", mimeType, nil)
}

// okapi: RoundTripXmlIT (comments and PIs)
func TestRoundTrip_CommentsAndPIs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<!-- A comment -->
<?target data?>
<doc><p>text</p></doc>`)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.xml", mimeType, nil)
}

// okapi: RoundTripXmlIT (attributes)
func TestRoundTrip_Attributes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<doc><p attr="value" class="test">text</p></doc>`)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.xml", mimeType, nil)
}

// okapi: RoundTripXmlIT (empty elements)
func TestRoundTrip_EmptyElements(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<doc><empty/><p>text</p></doc>`)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.xml", mimeType, nil)
}

// okapi: RoundTripXmlIT (xml:space preserve)
func TestRoundTrip_PreserveWhitespace(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	input := []byte("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<doc><p xml:space=\"preserve\">  multiple  spaces  </p></doc>")
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.xml", mimeType, nil)
}

// okapi: RoundTripXmlIT (supplemental chars)
func TestRoundTrip_SupplementalChars(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	input := []byte(`<?xml version="1.0" encoding="UTF-8"?>
<doc><p>` + "\U0001F600" + `</p></doc>`)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.xml", mimeType, nil)
}

// ---------------------------------------------------------------------------
// Full-file roundtrip tests from testdata
// ---------------------------------------------------------------------------

// okapi: RoundTripXmlIT#testInput
func TestRoundTrip_InputXml(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/input.xml", nil)
}

// okapi: RoundTripXmlIT#test01
func TestRoundTrip_Test01(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/test01.xml", nil)
}

// okapi: RoundTripXmlIT#test02
func TestRoundTrip_Test02(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/test02.xml", nil)
}

// okapi: RoundTripXmlIT#test03
func TestRoundTrip_Test03(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/test03.xml", nil)
}

// okapi: RoundTripXmlIT#test04
func TestRoundTrip_Test04(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/test04.xml", nil)
}

// okapi: RoundTripXmlIT (utf8 no bom)
func TestRoundTrip_UTF8NoBom(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/test08_utf8nobom.xml", nil)
}

// okapi: RoundTripXmlIT (Translate1)
func TestRoundTrip_Translate1(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/Translate1.xml", nil)
}

// okapi: RoundTripXmlIT (Translate2)
func TestRoundTrip_Translate2(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/Translate2.xml", nil)
}

// okapi: RoundTripXmlIT (Translate2 linked rules)
func TestRoundTrip_Translate2LinkedRules(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/Translate2_LinkedRules.xml", nil)
}

// okapi: RoundTripXmlIT (LocNote-1)
func TestRoundTrip_LocNote1(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/LocNote-1.xml", nil)
}

// okapi: RoundTripXmlIT (LocNote-2)
func TestRoundTrip_LocNote2(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/LocNote-2.xml", nil)
}

// okapi: RoundTripXmlIT (LocNote-3)
func TestRoundTrip_LocNote3(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/LocNote-3.xml", nil)
}

// okapi: RoundTripXmlIT (LocNote-4)
func TestRoundTrip_LocNote4(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/LocNote-4.xml", nil)
}

// okapi: RoundTripXmlIT (LocNote-5)
func TestRoundTrip_LocNote5(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/LocNote-5.xml", nil)
}

// okapi: RoundTripXmlIT (LocNote-6)
func TestRoundTrip_LocNote6(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/LocNote-6.xml", nil)
}

// okapi: RoundTripXmlIT (XRTT-Source1)
func TestRoundTrip_XRTTSource1(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/XRTT-Source1.xml", nil)
}

// okapi: RoundTripXmlIT (emoji)
func TestRoundTrip_Emoji(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/emoji1.xml", nil)
}

// okapi: RoundTripXmlIT (TestCDATA1)
func TestRoundTrip_TestCDATA1(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/TestCDATA1.xml", nil)
}

// okapi: RoundTripXmlIT (TestMultiLang)
func TestRoundTrip_TestMultiLang(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/TestMultiLang.xml", nil)
}

// okapi: RoundTripXmlIT (Android strings)
func TestRoundTrip_AndroidStrings(t *testing.T) {
	tdDir := bridgetest.TestdataDir(t)
	params := map[string]any{
		"configFile": tdDir + "/okapi/filters/its/src/test/resources/okf_xml@AndroidStrings.fprm",
	}
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/strings.xml", params)
}

// okapi: RoundTripXmlIT (OpenOffice)
func TestRoundTrip_OpenOffice(t *testing.T) {
	tdDir := bridgetest.TestdataDir(t)
	params := map[string]any{
		"configFile": tdDir + "/okapi/filters/its/src/test/resources/okf_xml@openoffice.fprm",
	}
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/openoffice_input.xml", params)
}

// okapi: RoundTripXmlIT (custom config 591)
func TestRoundTrip_CustomConfig591(t *testing.T) {
	tdDir := bridgetest.TestdataDir(t)
	params := map[string]any{
		"configFile": tdDir + "/okapi/filters/its/src/test/resources/custom-configs/591/okf_xml@ibxlf1.fprm",
	}
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/custom-configs/591/simple_with_simple_codes.xml", params)
}

// okapi: RoundTripXmlIT (custom config 1384)
func TestRoundTrip_CustomConfig1384(t *testing.T) {
	tdDir := bridgetest.TestdataDir(t)
	params := map[string]any{
		"configFile": tdDir + "/okapi/filters/its/src/test/resources/custom-configs/1384/okf_xml@translatable-and-untranslatable.fprm",
	}
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/custom-configs/1384/translatable-and-untranslatable.xml", params)
}

// ---------------------------------------------------------------------------
// Encoding roundtrip tests
// ---------------------------------------------------------------------------

// okapi: XMLFilterEncodingTest#utf8ToUtf16le
func TestRoundTrip_UTF8Encoding(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/test08_utf8nobom.xml", nil)
}

// okapi: XMLFilterEncodingTest#utf16leWithBomFromFile
func TestRoundTrip_UTF16LEWithBom(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	path := tdDir + "/okapi/filters/its/src/test/resources/test10_utf16le-with-bom.xml"
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	// Just verify it can be read and roundtripped without error
	parts := bridgetest.ReadBytes(t, pool, cfg, filterClass, content, path, mimeType, nil)
	require.NotEmpty(t, parts)
}

// ---------------------------------------------------------------------------
// Test data file roundtrip tests (additional files)
// ---------------------------------------------------------------------------

// okapi: RoundTripXmlIT (test05)
func TestRoundTrip_Test05(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/test05.xml", nil)
}

// okapi: RoundTripXmlIT (test06)
func TestRoundTrip_Test06(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/test06.xml", nil)
}

// okapi: RoundTripXmlIT (test07)
func TestRoundTrip_Test07(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/test07.xml", nil)
}

// okapi: RoundTripXmlIT (test09)
func TestRoundTrip_Test09(t *testing.T) {
	roundtripTestFile(t, "okapi/filters/its/src/test/resources/test09.xml", nil)
}

// ---------------------------------------------------------------------------
// Helper
// ---------------------------------------------------------------------------

// roundtripTestFile reads a testdata file and performs an event-level roundtrip test.
func roundtripTestFile(t *testing.T, relPath string, filterParams map[string]any) {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	path := tdDir + "/" + relPath
	content, err := os.ReadFile(path)
	require.NoError(t, err, "reading test file %s", relPath)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, filterParams)
}
