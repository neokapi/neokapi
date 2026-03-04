//go:build integration

package okf_phpcontent

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
)

// ---------------------------------------------------------------------------
// Output/roundtrip tests translated from PHPContentFilterTest.java.
// These verify that read -> write produces identical output to the input.
// ---------------------------------------------------------------------------

// okapi: PHPContentFilterTest#testOutputLinefeedCodes
func TestRoundTrip_OutputLinefeedCodes(t *testing.T) {
	snippet := "$a='\\n\\n';"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: PHPContentFilterTest#testOutputSimple
func TestRoundTrip_OutputSimple(t *testing.T) {
	snippet := "$a='abc';\n$b=\"def\";"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: PHPContentFilterTest#testLineBreakType
func TestRoundTrip_LineBreakType(t *testing.T) {
	snippet := "$a='abc';\r\n$b=\"def\";\r\n"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: PHPContentFilterTest#testOutputWithNoStrings
func TestRoundTrip_OutputWithNoStrings(t *testing.T) {
	snippet := "echo $a=$b; and other dummy code"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: PHPContentFilterTest#testOutputHeredoc
func TestRoundTrip_OutputHeredoc(t *testing.T) {
	snippet := "$a=<<<EOT\ntext\nEOT \n EOT \n\nEOT;\n"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: PHPContentFilterTest#testOutputMix
func TestRoundTrip_OutputMix(t *testing.T) {
	snippet := "$a=<<<EOT\ntext\nEOT \n EOT \n\nEOT;\n" +
		"$b=\"abc\"\n// 'comments'\n$c = 'def';\n" +
		"/* $c=\"abc\" */"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: PHPContentFilterTest#testOutputArrayKeys
func TestRoundTrip_OutputArrayKeys(t *testing.T) {
	snippet := "$arr1[\"foo\"]; $arr2[  'foo' ] = 'text';"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: PHPContentFilterTest#testEmptyHeredocStringAndOutput (roundtrip part)
func TestRoundTrip_EmptyHeredocString(t *testing.T) {
	snippet := "$a=<<<EOT\n\nEOT;"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: PHPContentFilterTest#testWhiteHeredocStringAndOutput (roundtrip part)
func TestRoundTrip_WhiteHeredocString(t *testing.T) {
	snippet := "$a=<<<EOT\n  \t  \nEOT;"
	output := snippetRoundtripDefault(t, snippet)
	assert.Equal(t, snippet, output)
}

// okapi: PHPContentFilterTest#testDoubleExtraction
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_phpcontent/*.phpcnt", mimeType, nil)
}
