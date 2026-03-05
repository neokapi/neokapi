//go:build integration

package okf_autoxliff

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NOTE: The AutoXLIFF filter is a read-only meta-filter that auto-detects the
// XLIFF version (1.2, 2.0, SDL XLIFF) and delegates to the correct sub-filter.
// Writing requires the delegate filter to have been initialized via open(),
// which the bridge cannot do for a write-only session. Therefore, roundtrip
// tests are not possible through the bridge for this filter.
//
// The Java tests do roundtrip by using filter.createFilterWriter() which
// internally delegates to the sub-filter writer after open(). The bridge
// does not support this pattern.

// okapi: TestAutoXLIFFFilter#testDelegateSDLXLIFF
func TestDelegate_SDLXLIFF(t *testing.T) {
	// AutoXLIFF delegates SDL XLIFF files to the XLIFF 1.2 filter with SDL config.
	// The Java test sets xliff12Config = "okf_xliff-sdl" via filter parameters.
	sdlParams := map[string]any{
		"xliff_config": "okf_xliff-sdl",
	}

	parts := readAutoXLIFFFile(t, "okf_autoxliff/sdlxliff.xlf", sdlParams)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 1, "should extract exactly 1 translatable text unit from SDL XLIFF")

	b := blocks[0]
	// The SDL XLIFF file has segmented source text. The bridge concatenates
	// segments without inter-segment whitespace, so check individual segments.
	text := b.SourceText()
	assert.Contains(t, text, "First sentence.")
	assert.Contains(t, text, "Second longer sentence.")
	assert.Contains(t, text, "Followed by a third one.")

	// Verify that the French target was extracted (SDL XLIFF has segmented targets).
	assert.True(t, b.HasTarget("fr"), "should have French target")
	assert.Contains(t, b.TargetText("fr"), "Première phrase",
		"first target segment should be extracted")
}

// okapi: TestAutoXLIFFFilter#testDelegateXLIFF12
func TestDelegate_XLIFF12(t *testing.T) {
	// AutoXLIFF auto-detects XLIFF 1.2 and delegates to the standard XLIFF filter.
	parts := readAutoXLIFFFile(t, "okf_autoxliff/xliff12.xlf", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 1, "should extract exactly 1 translatable text unit from XLIFF 1.2")

	b := blocks[0]
	assert.Equal(t, "Segment one.", b.SourceText())

	// Verify layer structure.
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")
}

// okapi: TestAutoXLIFFFilter#testDelegateXLIFF20
func TestDelegate_XLIFF20(t *testing.T) {
	// AutoXLIFF auto-detects XLIFF 2.0 and delegates to the XLIFF 2.0 filter.
	parts := readAutoXLIFFFile(t, "okf_autoxliff/xliff2.xlf", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.Len(t, blocks, 1, "should extract exactly 1 translatable text unit from XLIFF 2.0")

	b := blocks[0]
	assert.Equal(t, "Sample segment.", b.SourceText())

	// The XLIFF 2.0 file has a French target.
	assert.True(t, b.HasTarget("fr"), "should have French target")
	assert.Equal(t, "Exemple de segment.", b.TargetText("fr"))

	// Verify layer structure.
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")
}
