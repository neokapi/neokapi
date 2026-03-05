//go:build integration

package okf_rtf

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// RTFFilterTest
// ---------------------------------------------------------------------------

// okapi: RTFFilterTest#testSimpleTU
func TestExtract_SimpleTU(t *testing.T) {
	// Test01.rtf is a bilingual RTF file (en/fr) with translatable text units.
	// The Java test checks:
	//   TU1 source: "Text (to) translate."  target: "Texte \u00e0 traduire."
	//   TU2 source: "Text with <bold>bold</bold>."  target: "Texte avec du <bold>gras</bold>."
	parts := readRTFFile(t, "okf_rtf/Test01.rtf", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "Test01.rtf should produce translatable blocks")

	// Find the block containing "Text (to) translate." or similar source text.
	var found bool
	for _, b := range blocks {
		text := b.SourceText()
		if strings.Contains(text, "translate") {
			found = true
			// The source should contain the translatable text.
			assert.Contains(t, text, "Text")
			break
		}
	}
	assert.True(t, found, "should find a block containing 'translate' in Test01.rtf")

	// Verify there are at least 2 translatable blocks (TU1 and TU2 from Java test).
	assert.GreaterOrEqual(t, len(blocks), 2,
		"Test01.rtf should have at least 2 translatable blocks")
}

// okapi: RTFFilterTest#testBasicProcessing
func TestExtract_BasicProcessing(t *testing.T) {
	// The Java test opens Test01.rtf through FilterTestDriver.process() and
	// verifies no exception occurs. We verify the file can be fully extracted
	// through the bridge without errors.
	parts := readRTFFile(t, "okf_rtf/Test01.rtf", nil)

	require.NotEmpty(t, parts, "Test01.rtf should produce parts")
	assert.Equal(t, model.PartLayerStart, parts[0].Type,
		"first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type,
		"last part should be LayerEnd")

	// Should extract blocks without error.
	blocks := bridgetest.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "Test01.rtf should produce blocks")
}

// ---------------------------------------------------------------------------
// RtfEventTest
// ---------------------------------------------------------------------------

// okapi: RtfEventTest#testStartDoc
func TestExtract_StartDoc(t *testing.T) {
	// The Java test parses an RTF snippet and checks that the first event is
	// a StartDocument. We verify the bridge produces a LayerStart as the first
	// part when reading a minimal RTF document.
	snippet := "{\\rtf1\\ansi\\ansicpg1252\\deff0\\deflang1033{\\fonttbl{\\f0\\fnil\\fcharset0 Courier New;}}" +
		"\\uc1\\pard\\f0\\fs22 t\\b e\\b0 st\\par }"

	parts := readRTFSnippet(t, snippet, nil)

	require.NotEmpty(t, parts, "RTF snippet should produce parts")
	assert.Equal(t, model.PartLayerStart, parts[0].Type,
		"first part should be LayerStart (maps to StartDocument)")

	// Verify the layer has expected properties.
	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok, "LayerStart resource should be a *model.Layer")
	assert.NotEmpty(t, layer.ID, "layer should have an ID")
}

// ---------------------------------------------------------------------------
// RtfFullFileTest
// ---------------------------------------------------------------------------

// okapi: RtfFullFileTest#testAllExternalFiles
func TestExtract_AllExternalFiles(t *testing.T) {
	// The Java test iterates over all .rtf files in the test resources
	// directory and processes each one to completion. We do the same:
	// read every .rtf file from testdata/okf_rtf/ and verify no errors.
	tdDir := bridgetest.TestdataDir(t)
	rtfDir := filepath.Join(tdDir, "okf_rtf")

	entries, err := os.ReadDir(rtfDir)
	require.NoError(t, err, "reading okf_rtf testdata directory")

	var rtfFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".rtf") {
			rtfFiles = append(rtfFiles, e.Name())
		}
	}
	require.NotEmpty(t, rtfFiles, "should find .rtf test files in okf_rtf/")

	pool, cfg := bridgetest.SharedBridge(t)

	for _, f := range rtfFiles {
		t.Run(f, func(t *testing.T) {
			path := filepath.Join(rtfDir, f)
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

			require.NotEmpty(t, parts, "%s should produce parts", f)
			assert.Equal(t, model.PartLayerStart, parts[0].Type,
				"%s: first part should be LayerStart", f)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type,
				"%s: last part should be LayerEnd", f)
		})
	}
}

// ---------------------------------------------------------------------------
// RtfSnippetsTest
// ---------------------------------------------------------------------------

// okapi: RtfSnippetsTest#testBold
func TestExtract_Bold(t *testing.T) {
	// The Java testBold is entirely commented out (no-op). The RTF filter
	// processes monolingual RTF snippets as document structure without
	// producing translatable text units (unlike bilingual RTF files like
	// Test01.rtf). We verify the snippet parses without error and produces
	// the expected layer structure.
	snippet := "{\\rtf1\\ansi\\ansicpg1252\\deff0{\\fonttbl{\\f0\\fnil\\fcharset0 Courier New;}}" +
		"\\uc1\\pard\\f0\\fs22 Normal \\b Bold\\b0  text.\\par }"

	parts := readRTFSnippet(t, snippet, nil)
	require.NotEmpty(t, parts, "bold snippet should produce parts")

	// Monolingual RTF snippets produce LayerStart + LayerEnd with no
	// translatable blocks, which is correct behavior for the RTF filter.
	assert.Equal(t, model.PartLayerStart, parts[0].Type,
		"first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type,
		"last part should be LayerEnd")
}

// ---------------------------------------------------------------------------
// ExtractionComparisionTest
// ---------------------------------------------------------------------------

// okapi: ExtractionComparisionTest#testDoubleExtraction
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	// The Java testDoubleExtraction is commented out (TODO: implement RTF
	// Filter as a filter). The RTF filter writer produces plain text output
	// (not reconstructed RTF), so we verify that:
	// 1. The roundtrip completes without error.
	// 2. The output contains the target translations from the bilingual RTF.
	// 3. The first extraction produces the expected translatable blocks.
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okf_rtf/Test01.rtf")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, nil)
	require.NotEmpty(t, result.Output, "roundtrip should produce output")

	// Verify first extraction produced translatable blocks.
	blocks := bridgetest.TranslatableBlocks(result.Parts)
	require.NotEmpty(t, blocks, "extraction should produce translatable blocks")

	// Verify the output contains the French target translations.
	output := string(result.Output)
	assert.Contains(t, output, "traduire",
		"output should contain French translation text")
}
