package odf_test

import (
	"bytes"
	"strings"
	"testing"

	odf "github.com/neokapi/neokapi/core/formats/odf"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A3 (structure-geometry-landscape.md §5): ODF draw:frame svg coordinates
// become native GeometryAnnotations, per draw:page, without any layout engine.
func TestODFNativeFrameGeometry(t *testing.T) {
	content := `<?xml version="1.0" encoding="UTF-8"?>
<office:document-content
  xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0"
  xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0"
  xmlns:presentation="urn:oasis:names:tc:opendocument:xmlns:presentation:1.0"
  xmlns:draw="urn:oasis:names:tc:opendocument:xmlns:drawing:1.0"
  xmlns:svg="urn:oasis:names:tc:opendocument:xmlns:svg-compatible:1.0">
<office:body><office:presentation>
<draw:page>
  <draw:frame svg:x="2.54cm" svg:y="1in" svg:width="72pt" svg:height="36pt">
    <draw:text-box><text:p>Slide one title</text:p></draw:text-box>
  </draw:frame>
</draw:page>
<draw:page>
  <draw:frame svg:x="0cm" svg:y="0cm" svg:width="10cm" svg:height="5cm">
    <draw:text-box><text:p>Slide two body</text:p></draw:text-box>
  </draw:frame>
</draw:page>
</office:presentation></office:body></office:document-content>`

	data := makeODFZip(mimeODP, content)
	reader := odf.NewReader()
	require.NoError(t, reader.Open(t.Context(), testutil.RawDocFromReader(bytes.NewReader(data), "t.odp", model.LocaleEnglish)))
	defer reader.Close()
	blocks := testutil.CollectBlocks(t, reader.Read(t.Context()))

	geoOf := func(text string) *model.GeometryAnnotation {
		for _, b := range blocks {
			if strings.TrimSpace(b.SourceText()) == text {
				g, ok := b.Geometry()
				require.True(t, ok, "block %q should carry geometry", text)
				return g
			}
		}
		t.Fatalf("no block with text %q (have: %v)", text, testutil.BlockTexts(blocks))
		return nil
	}

	g1 := geoOf("Slide one title")
	assert.Equal(t, 1, g1.Page, "first frame is on page 1")
	assert.InDelta(t, 72.0, g1.BBox.X, 0.01, "2.54cm → 72pt")
	assert.InDelta(t, 72.0, g1.BBox.Y, 0.01, "1in → 72pt")
	assert.InDelta(t, 72.0, g1.BBox.W, 0.01, "72pt")
	assert.InDelta(t, 36.0, g1.BBox.H, 0.01, "36pt")
	assert.Equal(t, "top-left", g1.Origin)

	g2 := geoOf("Slide two body")
	assert.Equal(t, 2, g2.Page, "second frame is on page 2")
	assert.InDelta(t, 283.46, g2.BBox.W, 0.1, "10cm → ~283.46pt")
}

// A block outside any frame (a plain text document) carries no geometry.
func TestODFNoGeometryWithoutFrame(t *testing.T) {
	data := makeODFZip(mimeODT, simpleODTContent("Just a paragraph", "And another"))
	reader := odf.NewReader()
	require.NoError(t, reader.Open(t.Context(), testutil.RawDocFromReader(bytes.NewReader(data), "t.odt", model.LocaleEnglish)))
	defer reader.Close()
	for _, b := range testutil.CollectBlocks(t, reader.Read(t.Context())) {
		_, ok := b.Geometry()
		assert.False(t, ok, "plain ODT block %q should have no geometry", b.SourceText())
	}
}
