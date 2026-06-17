package idml

import (
	"encoding/xml"
	"testing"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A2 (structure-geometry-landscape.md §5): a TextFrame's GeometricBounds +
// ItemTransform in Spreads/ become a native GeometryAnnotation on every block
// of the referenced story — InDesign layout geometry, a format Docling can't
// read at all. No layout engine, no ML.
func TestIDMLNativeFrameGeometry(t *testing.T) {
	const story = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Story xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Story Self="u100">
    <ParagraphStyleRange><CharacterStyleRange><Content>Frame text</Content></CharacterStyleRange></ParagraphStyleRange>
  </Story>
</idPkg:Story>`

	// GeometricBounds = "y1 x1 y2 x2" → local box left=0,top=0,right=150,bottom=50
	// (w=150,h=50). ItemTransform translate (100,200) → page-space (100,200).
	const spread = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<idPkg:Spread xmlns:idPkg="http://ns.adobe.com/AdobeInDesign/idml/1.0/packaging">
  <Spread Self="uspread">
    <TextFrame Self="uframe" ParentStory="u100" PreviousTextFrame="n"
      ItemTransform="1 0 0 1 100 200" GeometricBounds="0 0 50 150"/>
  </Spread>
</idPkg:Spread>`

	data := zipWith(t, map[string]string{
		"mimetype":                   "application/vnd.adobe.indesign-idml-package",
		"designmap.xml":              `<?xml version="1.0"?><Document DOMVersion="16.0"/>`,
		"Spreads/Spread_uspread.xml": spread,
		"Stories/Story_u100.xml":     story,
	})

	parts := readIDMLBytes(t, data)
	blocks := testutil.FilterBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "Frame text", blocks[0].SourceText())

	g, ok := blocks[0].Geometry()
	require.True(t, ok, "story block should carry frame geometry")
	assert.Equal(t, 1, g.Page, "spread 1 → page 1")
	assert.InDelta(t, 100.0, g.BBox.X, 0.01)
	assert.InDelta(t, 200.0, g.BBox.Y, 0.01)
	assert.InDelta(t, 150.0, g.BBox.W, 0.01)
	assert.InDelta(t, 50.0, g.BBox.H, 0.01)
	assert.Equal(t, "top-left", g.Origin)
}

// frameBoxFromAttrs applies the ItemTransform affine to the GeometricBounds
// corners and returns the page-space AABB. A rotation/scale transform is
// covered here directly (no fixture needed).
func TestFrameBoxFromAttrsTransform(t *testing.T) {
	// 90° rotation (a=0 b=1 c=-1 d=0) of a 20×10 box at the origin, then
	// translate (300,400). Corners (0,0),(20,0),(0,10),(20,10) →
	// (-y, x)+(300,400): x∈[290,300], y∈[400,420] → box {290,400,10,20}.
	attrs := xmlAttrs(map[string]string{
		"GeometricBounds": "0 0 10 20", // y1 x1 y2 x2 → top0 left0 bottom10 right20
		"ItemTransform":   "0 1 -1 0 300 400",
	})
	fb, ok := frameBoxFromAttrs(attrs, 2)
	require.True(t, ok)
	assert.Equal(t, 2, fb.page)
	assert.InDelta(t, 290.0, fb.x, 0.01)
	assert.InDelta(t, 400.0, fb.y, 0.01)
	assert.InDelta(t, 10.0, fb.w, 0.01)
	assert.InDelta(t, 20.0, fb.h, 0.01)
}

// A frame without GeometricBounds yields no geometry.
func TestFrameBoxFromAttrsMissingBounds(t *testing.T) {
	_, ok := frameBoxFromAttrs(xmlAttrs(map[string]string{"ItemTransform": "1 0 0 1 0 0"}), 1)
	assert.False(t, ok)
}

func TestParseFloats(t *testing.T) {
	assert.Equal(t, []float64{1, 0, 0, 1, 100, 200}, parseFloats("1 0 0 1 100 200"))
	assert.Nil(t, parseFloats(""))
	assert.Nil(t, parseFloats("1 x 3"))
}

// xmlAttrs builds an unnamespaced xml.Attr slice for the frame helpers.
func xmlAttrs(m map[string]string) []xml.Attr {
	out := make([]xml.Attr, 0, len(m))
	for k, v := range m {
		out = append(out, xml.Attr{Name: xml.Name{Local: k}, Value: v})
	}
	return out
}
