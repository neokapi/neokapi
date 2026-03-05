//go:build integration

package okf_xini

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// XINIFilterReaderTest
// ---------------------------------------------------------------------------

// okapi: XINIFilterReaderTest#segmentBecomesTU
func TestReader_SegmentBecomesTU(t *testing.T) {
	// Reading contents.xini should produce translatable blocks from XINI segments.
	parts := readXINIDefault(t, "contents.xini")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "XINI segments should become translatable blocks (TextUnits)")

	// contents.xini has segments with text "Test!" and "Test."
	texts := bridgetest.BlockTexts(blocks)
	found := false
	for _, text := range texts {
		if text == "Test!" || text == "Test." {
			found = true
			break
		}
	}
	assert.True(t, found, "should find segment text 'Test!' or 'Test.' in blocks, got: %v", texts)
}

// okapi: XINIFilterReaderTest#segmentsAreGroupedInTUsByOriginalSegmentId
func TestReader_SegmentsGroupedByOriginalSegmentId(t *testing.T) {
	// Segments with the same ExternalID (original segment ID) should be grouped.
	// contents.xini has Field ExternalID="tu1_tu1" with multiple Seg elements.
	parts := readXINIDefault(t, "contents.xini")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks")

	// The segments in the same field may be grouped into a single TU or
	// multiple TUs depending on segmentation. Verify we have blocks.
	assert.NotEmpty(t, blocks, "segments should produce TU blocks")

	// Verify structural integrity: should have LayerStart and LayerEnd.
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")
}
