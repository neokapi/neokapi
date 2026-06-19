package vision

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBlocksFromOCRCarryProvenance(t *testing.T) {
	res := &OCRResult{Width: 100, Height: 50, Lines: []OCRLine{
		{Text: "clear", BBox: model.Rect{X: 1, Y: 2, W: 40, H: 8}, Confidence: 0.97},
		{Text: "fuzzy", BBox: model.Rect{X: 1, Y: 12, W: 40, H: 8}, Confidence: 0.41},
	}}
	c := 0
	blocks := BlocksFromOCR(res, 1, &c)
	require.Len(t, blocks, 2)
	for i, want := range []float64{0.97, 0.41} {
		o, ok := blocks[i].SourceOrigin()
		require.True(t, ok, "block %d should carry source origin", i)
		assert.Equal(t, model.OriginOCR, o.Kind)
		assert.InDelta(t, want, o.Confidence, 1e-9)
	}

	// Confidence survives the inverse OCRResultFromBlocks.
	rt := OCRResultFromBlocks(blocks, 100, 50)
	require.Len(t, rt.Lines, 2)
	assert.InDelta(t, 0.97, rt.Lines[0].Confidence, 1e-9)
	assert.InDelta(t, 0.41, rt.Lines[1].Confidence, 1e-9)
}

func TestPartsFromLayoutCarryProvenance(t *testing.T) {
	res := &OCRResult{Width: 200, Height: 100, Lines: []OCRLine{
		{Text: "heading", BBox: model.Rect{X: 10, Y: 10, W: 80, H: 12}, Confidence: 0.5},
	}}
	regions := []Region{{Role: model.RoleHeading, BBox: model.Rect{X: 0, Y: 0, W: 200, H: 40}}}
	c, gc := 0, 0
	parts := PartsFromLayout(regions, res, &c, &gc)

	var found bool
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		o, ok := b.SourceOrigin()
		require.True(t, ok)
		assert.Equal(t, model.OriginOCR, o.Kind)
		assert.InDelta(t, 0.5, o.Confidence, 1e-9)
		found = true
	}
	assert.True(t, found, "expected at least one block part")
}
