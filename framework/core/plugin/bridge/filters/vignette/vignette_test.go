//go:build integration

package vignette

import (
	"os"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Tests ported from VignetteFilterTest.java (7 tests)
// ---------------------------------------------------------------------------

// okapi: VignetteFilterTest#testSimpleEntry
func TestExtract_SimpleEntry(t *testing.T) {
	snippet := createSimpleDoc()
	parts := readVignetteDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// The simple doc has one source entry with body "<p>ENtext</p>" from the
	// en_US locale. The Vignette filter extracts through an HTML subfilter,
	// so the extracted source text should be "ENtext".
	found := false
	for _, b := range blocks {
		if b.SourceText() == "ENtext" {
			found = true
			break
		}
	}
	assert.True(t, found, "should find block with source text 'ENtext'")
}

// okapi: VignetteFilterTest#testStartDocument
func TestExtract_StartDocument(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_vignette/Test01.xml")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	parts := readVignetteBytes(t, pool, cfg, content, path, nil)

	// Should produce a LayerStart at the beginning.
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
}

// okapi-unmapped: VignetteFilterTest#testDefaultInfo — Java-only API test (IFilter.getDisplayName/getConfigurations)

// okapi: VignetteFilterTest#testDoubleExtraction
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_vignette/Test01.xml")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	// First pass: read → write.
	result := bridgetest.RoundTripWithLocales(t, pool, cfg, filterClass,
		content, path, mimeType, nil,
		"en-us", "es-es")

	// Second pass: re-read the output.
	parts2 := readVignetteBytes(t, pool, cfg, result.Output, "test.xml", nil)
	blocks2 := bridgetest.TranslatableBlocks(parts2)
	require.NotEmpty(t, blocks2, "double extraction should produce blocks")

	// The block count from both passes should match.
	blocks1 := bridgetest.TranslatableBlocks(result.Parts)
	assert.Equal(t, len(blocks1), len(blocks2), "double extraction should produce same block count")
}

// okapi: VignetteFilterTest#testComplexEntry
func TestExtract_ComplexEntry(t *testing.T) {
	snippet := createComplexDoc()
	parts := readVignetteDefault(t, snippet)

	// The complex doc has two source entries (EN-id1, EN-id2) from en_US locale.
	// Order is driven by the targets (es_ES entries come first in the doc).
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "EN-id1", "should extract EN-id1 source text")
	assert.Contains(t, texts, "EN-id2", "should extract EN-id2 source text")
}

// okapi: VignetteFilterTest#testComplexEntryOutput
func TestRoundTrip_ComplexEntryOutput(t *testing.T) {
	snippet := createComplexDoc()
	output := snippetRoundtrip(t, snippet, nil)

	// The output should preserve the content instances with their structure.
	// Verify key attribute values survive the roundtrip.
	assert.Contains(t, output, "EN-id1", "roundtrip should preserve EN-id1 content")
	assert.Contains(t, output, "EN-id2", "roundtrip should preserve EN-id2 content")
	assert.Contains(t, output, "es_ES", "roundtrip should preserve es_ES locale")
	assert.Contains(t, output, "en_US", "roundtrip should preserve en_US locale")
	assert.Contains(t, output, "SOURCE_ID", "roundtrip should preserve SOURCE_ID attribute")
	assert.Contains(t, output, "id1", "roundtrip should preserve source id1")
	assert.Contains(t, output, "id2", "roundtrip should preserve source id2")
}

// okapi: VignetteFilterTest#testSimpleEntryOutput
func TestRoundTrip_SimpleEntryOutput(t *testing.T) {
	snippet := createSimpleDoc()
	output := snippetRoundtrip(t, snippet, nil)

	// The simple doc roundtrip should preserve the key structure:
	// - The en_US content instance with "<p>ENtext</p>" body
	// - The es_ES content instance
	// - The <stuff/> element between them
	assert.Contains(t, output, "ENtext", "roundtrip should preserve ENtext content")
	assert.Contains(t, output, "es_ES", "roundtrip should preserve es_ES locale")
	assert.Contains(t, output, "en_US", "roundtrip should preserve en_US locale")
	assert.Contains(t, output, "SOURCE_ID", "roundtrip should preserve SOURCE_ID attribute")
	assert.Contains(t, output, "id1", "roundtrip should preserve source id1")
}
