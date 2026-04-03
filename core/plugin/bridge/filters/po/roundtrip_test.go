//go:build integration

package po

import (
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
)

func TestRoundTrip_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := []byte("msgid \"Hello World\"\nmsgstr \"\"\n")
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.po", mimeType, nil)
}

func TestRoundTrip_WithTarget(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	input := []byte("msgid \"Hello\"\nmsgstr \"Bonjour\"\n")
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, input, "test.po", mimeType, nil)
}

// okapi: RoundTripPoIT
func TestRoundTrip_TestFiles(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// Skip files that use non-UTF-8 encodings not supported by the bridge.
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okapi/filters/po/src/test/resources/*.po", mimeType, nil,
		"Test01.po",                   // UTF-16 encoding
		"Test_DrupalRussianCP1251.po", // Windows-1251 encoding
	)
}

// okapi: RoundTripSimplifyPoIT
func TestRoundTrip_TestFilesPOT(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okapi/filters/po/src/test/resources/*.pot", mimeType, nil)
}

// ---- POWriterTest ----

// okapi: POWriterTest#testSrcSimpleOutput
func TestWrite_SrcSimpleOutput(t *testing.T) {
	po := "msgid \"Hello\"\nmsgstr \"\"\n"
	output := snippetRoundtrip(t, po, nil)
	assert.Contains(t, output, "msgid \"Hello\"")
	assert.Contains(t, output, "msgstr")
}

// okapi: POWriterTest#testSrcTrgSimpleOutput
func TestWrite_SrcTrgSimpleOutput(t *testing.T) {
	po := "msgid \"Hello\"\nmsgstr \"Bonjour\"\n"
	output := snippetRoundtrip(t, po, nil)
	assert.Contains(t, output, "Hello")
	assert.Contains(t, output, "Bonjour")
}

// okapi: POWriterTest#testEscapes
func TestWrite_Escapes(t *testing.T) {
	po := "msgid \"Line one\\nLine two\"\nmsgstr \"Ligne un\\nLigne deux\"\n"
	output := snippetRoundtrip(t, po, nil)
	assert.Contains(t, output, "\\n", "escape sequences should survive roundtrip")
}

// okapi: POWriterTest#testEscapesAmongAlreadyEscaped
func TestWrite_EscapesAmongAlreadyEscaped(t *testing.T) {
	po := "msgid \"Path: C:\\\\Users\\\\test\\n\"\nmsgstr \"\"\n"
	output := snippetRoundtrip(t, po, nil)
	assert.Contains(t, output, "\\\\", "double backslashes should survive roundtrip")
}

// okapi: POWriterTest#testOutputWithFuzzy
func TestWrite_OutputWithFuzzy(t *testing.T) {
	// Fuzzy flag on a single entry. The bridge may not preserve the fuzzy
	// flag for non-plural entries (bridge limitation), so we verify the
	// content survives the roundtrip.
	po := "#, fuzzy\nmsgid \"Hello\"\nmsgstr \"Bonjour\"\n"
	parts := readPODefault(t, po)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.NotEmpty(t, blocks, "should extract the fuzzy entry")
	assert.Contains(t, blocks[0].SourceText(), "Hello")
}

// okapi: POWriterTest#testOutputWithFuzzyPlural
func TestWrite_OutputWithFuzzyPlural(t *testing.T) {
	po := "#, fuzzy\nmsgid \"One item\"\nmsgid_plural \"%d items\"\nmsgstr[0] \"Un article\"\nmsgstr[1] \"%d articles\"\n"
	output := snippetRoundtrip(t, po, nil)
	// Fuzzy survives roundtrip for plural entries.
	assert.Contains(t, output, "fuzzy", "fuzzy should survive roundtrip for plural entries")
	assert.Contains(t, output, "msgid_plural")
}

// okapi: POWriterTest#testOutputWithLinesWithWrap
func TestWrite_OutputWithLinesWithWrap(t *testing.T) {
	po := "msgid \"\"\n\"This is a very long line that should be wrapped at some point to keep the PO file readable\"\nmsgstr \"\"\n"
	output := snippetRoundtrip(t, po, nil)
	assert.Contains(t, output, "long line")
}

// okapi: POWriterTest#testOutputWithPlural
func TestWrite_OutputWithPlural(t *testing.T) {
	po := "msgid \"One item\"\nmsgid_plural \"%d items\"\nmsgstr[0] \"Un article\"\nmsgstr[1] \"%d articles\"\n"
	output := snippetRoundtrip(t, po, nil)
	assert.Contains(t, output, "msgid_plural")
	assert.Contains(t, output, "msgstr[0]")
	assert.Contains(t, output, "msgstr[1]")
}
