//go:build integration

package xmlstream

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := []byte(`<?xml version="1.0" encoding="UTF-8"?><root><text>Hello world</text></root>`)
	bridgetest.AssertRoundTrip(t, pool, cfg, filterClass, input, "test.xml", mimeType, nil)
}

// okapi: RoundTripXmlStreamIT
func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_xmlstream/*.xml", mimeType, nil)
}

// ---------------------------------------------------------------------------
// Tests translated from DitaExtractionComparisionTest.java (5 tests)
// ---------------------------------------------------------------------------

// okapi: DitaExtractionComparisionTest#testStartDocument
func TestDita_StartDocument(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "bookmap-readme.dita")

	content, err := os.ReadFile(xmlPath)
	require.NoError(t, err)
	parts := bridgetest.ReadBytes(t, pool, cfg, filterClass, content, xmlPath, mimeType, nil)
	require.NotEmpty(t, parts)
}

// okapi: DitaExtractionComparisionTest#testDoubleExtraction
func TestDita_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "bookmap-readme.dita")

	content, err := os.ReadFile(xmlPath)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, xmlPath, mimeType, nil)
}

// okapi: DitaExtractionComparisionTest#testDoubleExtractionSingle
func TestDita_DoubleExtractionSingle(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "changingtheoil.dita")

	content, err := os.ReadFile(xmlPath)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, xmlPath, mimeType, nil)
}

// okapi: DitaExtractionComparisionTest#testReconstructFile
func TestDita_ReconstructFile(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "bookmap-readme.dita")

	content, err := os.ReadFile(xmlPath)
	require.NoError(t, err)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, content, xmlPath, mimeType, nil)
	require.NotEmpty(t, result.Output)
}

// okapi: DitaExtractionComparisionTest#testOpenTwice
func TestDita_OpenTwice(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "bookmap-readme.dita")

	content, err := os.ReadFile(xmlPath)
	require.NoError(t, err)

	// First read.
	parts1 := bridgetest.ReadBytes(t, pool, cfg, filterClass, content, xmlPath, mimeType, nil)
	require.NotEmpty(t, parts1)

	// Second read — should work identically.
	parts2 := bridgetest.ReadBytes(t, pool, cfg, filterClass, content, xmlPath, mimeType, nil)
	require.NotEmpty(t, parts2)
	require.Equal(t, len(parts1), len(parts2))
}

// ---------------------------------------------------------------------------
// Tests translated from DocTypeExtractionTest.java (2 tests)
// ---------------------------------------------------------------------------

// okapi: DocTypeExtractionTest#testDoubleExtraction
func TestDocType_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "doctype.xml")

	content, err := os.ReadFile(xmlPath)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, xmlPath, mimeType, nil)
}

// okapi: DocTypeExtractionTest#testEvents
func TestDocType_Events(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "doctype.xml")

	content, err := os.ReadFile(xmlPath)
	require.NoError(t, err)
	parts := bridgetest.ReadBytes(t, pool, cfg, filterClass, content, xmlPath, mimeType, nil)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "doctype.xml should have translatable content")
}

// ---------------------------------------------------------------------------
// Tests translated from PIExtractionTest.java (2 tests)
// ---------------------------------------------------------------------------

// okapi: PIExtractionTest#testDoubleExtraction
func TestPI_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "PI-Problem.xml")

	content, err := os.ReadFile(xmlPath)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, xmlPath, mimeType, nil)
}

// okapi: PIExtractionTest#testDoubleExtraction2
func TestPI_DoubleExtraction2(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "PI-Problem2.dita")

	content, err := os.ReadFile(xmlPath)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, xmlPath, mimeType, nil)
}

// ---------------------------------------------------------------------------
// Tests translated from PropertyXmlExtractionComparisionTest.java (7 tests)
// ---------------------------------------------------------------------------

// okapi: PropertyXmlExtractionComparisionTest#testStartDocument
func TestPropertyXml_StartDocument(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "java_properties", "about.xml")
	configPath := filepath.Join(tdDir, "okf_xmlstream", "java_properties", "okf_xmlstream@javaproperties.fprm")

	params := map[string]any{
		"configFile": configPath,
	}
	content, err := os.ReadFile(xmlPath)
	require.NoError(t, err)
	parts := bridgetest.ReadBytes(t, pool, cfg, filterClass, content, xmlPath, mimeType, params)
	require.NotEmpty(t, parts)
}

// okapi: PropertyXmlExtractionComparisionTest#testDoubleExtraction
func TestPropertyXml_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "java_properties", "about.xml")
	configPath := filepath.Join(tdDir, "okf_xmlstream", "java_properties", "okf_xmlstream@javaproperties.fprm")

	params := map[string]any{
		"configFile": configPath,
	}
	content, err := os.ReadFile(xmlPath)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, xmlPath, mimeType, params)
}

// okapi: PropertyXmlExtractionComparisionTest#testDoubleExtractionSingle
func TestPropertyXml_DoubleExtractionSingle(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "java_properties", "test_drive.xml")
	configPath := filepath.Join(tdDir, "okf_xmlstream", "java_properties", "okf_xmlstream@javaproperties.fprm")

	params := map[string]any{
		"configFile": configPath,
	}
	content, err := os.ReadFile(xmlPath)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, xmlPath, mimeType, params)
}

// okapi: PropertyXmlExtractionComparisionTest#testReconstructFile
func TestPropertyXml_ReconstructFile(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "java_properties", "about.xml")
	configPath := filepath.Join(tdDir, "okf_xmlstream", "java_properties", "okf_xmlstream@javaproperties.fprm")

	params := map[string]any{
		"configFile": configPath,
	}
	content, err := os.ReadFile(xmlPath)
	require.NoError(t, err)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, content, xmlPath, mimeType, params)
	require.NotEmpty(t, result.Output)
}

// okapi: PropertyXmlExtractionComparisionTest#testOpenTwice
func TestPropertyXml_OpenTwice(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "java_properties", "about.xml")
	configPath := filepath.Join(tdDir, "okf_xmlstream", "java_properties", "okf_xmlstream@javaproperties.fprm")

	params := map[string]any{
		"configFile": configPath,
	}
	content, err := os.ReadFile(xmlPath)
	require.NoError(t, err)

	parts1 := bridgetest.ReadBytes(t, pool, cfg, filterClass, content, xmlPath, mimeType, params)
	require.NotEmpty(t, parts1)

	parts2 := bridgetest.ReadBytes(t, pool, cfg, filterClass, content, xmlPath, mimeType, params)
	require.NotEmpty(t, parts2)
	require.Equal(t, len(parts1), len(parts2))
}

// okapi: PropertyXmlExtractionComparisionTest#testAsSnippetNoCdata
func TestPropertyXml_AsSnippetNoCdata(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := filepath.Join(tdDir, "okf_xmlstream", "java_properties", "okf_xmlstream@javaproperties.fprm")

	params := map[string]any{
		"configFile": configPath,
	}
	snippet := `<?xml version="1.0" encoding="UTF-8"?><properties><entry key="test">Test value</entry></properties>`
	parts := bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.xml", mimeType, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract entries from property XML")
}

// okapi: PropertyXmlExtractionComparisionTest#testAsSnippetWithCdata
func TestPropertyXml_AsSnippetWithCdata(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := filepath.Join(tdDir, "okf_xmlstream", "java_properties", "okf_xmlstream@javaproperties.fprm")

	params := map[string]any{
		"configFile": configPath,
	}
	snippet := `<?xml version="1.0" encoding="UTF-8"?><properties><entry key="test"><![CDATA[Test <b>value</b>]]></entry></properties>`
	parts := bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.xml", mimeType, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract entries from property XML with CDATA")
}
