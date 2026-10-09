package openxml

import (
	"archive/zip"
	"bytes"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A deck's relationship lists reach the same part from several places: a slide
// from presentation.xml and from its notes slide, a layout from every slide
// that uses it. The part walk collects each once, plays the slides in the order
// the presentation does, and treats layouts as the furniture masters are.
func TestBuildPPTXParts_ShowOrderOnceEach(t *testing.T) {
	rels := map[string][]relationship{
		"ppt/_rels/presentation.xml.rels": {
			{ID: "rId1", Type: relTypeSlideMaster, Target: "slideMasters/slideMaster1.xml"},
			{ID: "rId2", Type: relTypeSlide, Target: "slides/slide1.xml"},
			{ID: "rId3", Type: relTypeSlide, Target: "slides/slide2.xml"},
			{ID: "rId4", Type: relTypeSlide, Target: "slides/slide10.xml"},
		},
		"ppt/slides/_rels/slide1.xml.rels": {
			{ID: "rId1", Type: relTypeSlideLayout, Target: "../slideLayouts/slideLayout1.xml"},
			{ID: "rId2", Type: relTypeNotesSlide, Target: "../notesSlides/notesSlide1.xml"},
		},
		"ppt/slides/_rels/slide2.xml.rels": {
			{ID: "rId1", Type: relTypeSlideLayout, Target: "../slideLayouts/slideLayout2.xml"},
		},
		"ppt/slides/_rels/slide10.xml.rels": {
			{ID: "rId1", Type: relTypeSlideLayout, Target: "../slideLayouts/slideLayout1.xml"},
			{ID: "rId2", Type: relTypeNotesSlide, Target: "../notesSlides/notesSlide2.xml"},
		},
		"ppt/notesSlides/_rels/notesSlide1.xml.rels": {
			{ID: "rId1", Type: relTypeSlide, Target: "../slides/slide1.xml"},
		},
		"ppt/notesSlides/_rels/notesSlide2.xml.rels": {
			{ID: "rId1", Type: relTypeSlide, Target: "../slides/slide10.xml"},
		},
		"ppt/slideLayouts/_rels/slideLayout1.xml.rels": {
			{ID: "rId1", Type: relTypeSlideMaster, Target: "../slideMasters/slideMaster1.xml"},
		},
		"ppt/slideLayouts/_rels/slideLayout2.xml.rels": {
			{ID: "rId1", Type: relTypeSlideMaster, Target: "../slideMasters/slideMaster1.xml"},
		},
	}
	info := &containerInfo{
		docType:          docTypePPTX,
		mainDocumentPart: "ppt/presentation.xml",
		relationships:    rels,
		// The deck plays slide 10 first: part names follow creation order.
		slideOrder: []string{"ppt/slides/slide10.xml", "ppt/slides/slide1.xml", "ppt/slides/slide2.xml"},
	}
	cfg := &Config{}
	cfg.Reset()

	assert.Equal(t, []string{
		"ppt/slides/slide10.xml", "ppt/notesSlides/notesSlide2.xml",
		"ppt/slides/slide1.xml", "ppt/notesSlides/notesSlide1.xml",
		"ppt/slides/slide2.xml",
		"docProps/core.xml",
	}, buildPPTXParts(info, cfg), "slides once each in show order, each followed by its notes, no layouts")

	cfg.TranslateSlideMasters = true
	parts := buildPPTXParts(info, cfg)
	assert.Equal(t, []string{
		"ppt/slides/slide10.xml", "ppt/notesSlides/notesSlide2.xml",
		"ppt/slides/slide1.xml", "ppt/notesSlides/notesSlide1.xml",
		"ppt/slides/slide2.xml",
		"ppt/slideMasters/slideMaster1.xml",
		"ppt/slideLayouts/slideLayout1.xml", "ppt/slideLayouts/slideLayout2.xml",
		"docProps/core.xml",
	}, parts, "the masters option brings the layouts with the masters")

	cfg.Reset()
	cfg.TranslateSlideNotes = false
	assert.Equal(t, []string{
		"ppt/slides/slide10.xml", "ppt/slides/slide1.xml", "ppt/slides/slide2.xml",
		"docProps/core.xml",
	}, buildPPTXParts(info, cfg), "notes off")

	cfg.Reset()
	info.slideOrder = nil
	assert.Equal(t, []string{
		"ppt/slides/slide1.xml", "ppt/notesSlides/notesSlide1.xml",
		"ppt/slides/slide2.xml",
		"ppt/slides/slide10.xml", "ppt/notesSlides/notesSlide2.xml",
		"docProps/core.xml",
	}, buildPPTXParts(info, cfg), "without a show order the parts sort by number, not lexically")

	// A notes part no slide claims follows the deck.
	info.relationships["ppt/notesSlides/_rels/notesSlide9.xml.rels"] = []relationship{
		{ID: "rId1", Type: relTypeNotesSlide, Target: "notesSlide9.xml"},
	}
	parts = buildPPTXParts(info, cfg)
	assert.Equal(t, "ppt/notesSlides/notesSlide9.xml", parts[len(parts)-2])
}

func TestParseSlideOrder(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("ppt/presentation.xml")
	require.NoError(t, err)
	_, err = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<p:presentation xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main"
  xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <p:sldMasterIdLst><p:sldMasterId id="2147483648" r:id="rId1"/></p:sldMasterIdLst>
  <p:sldIdLst>
    <p:sldId id="258" r:id="rId4"/>
    <p:sldId id="256" r:id="rId2"/>
    <p:sldId id="257" r:id="rId3"/>
  </p:sldIdLst>
</p:presentation>`))
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)

	info := &containerInfo{
		mainDocumentPart: "ppt/presentation.xml",
		relationships: map[string][]relationship{
			"ppt/_rels/presentation.xml.rels": {
				{ID: "rId1", Type: relTypeSlideMaster, Target: "slideMasters/slideMaster1.xml"},
				{ID: "rId2", Type: relTypeSlide, Target: "slides/slide1.xml"},
				{ID: "rId3", Type: relTypeSlide, Target: "slides/slide2.xml"},
				{ID: "rId4", Type: relTypeSlide, Target: "slides/slide10.xml"},
			},
		},
	}
	assert.Equal(t,
		[]string{"ppt/slides/slide10.xml", "ppt/slides/slide1.xml", "ppt/slides/slide2.xml"},
		parseSlideOrder(zr, info))

	assert.Nil(t, parseSlideOrder(zr, &containerInfo{}), "no main part, no order")
}

func TestNaturalCompare(t *testing.T) {
	assert.Negative(t, naturalCompare("ppt/slides/slide2.xml", "ppt/slides/slide10.xml"))
	assert.Positive(t, naturalCompare("ppt/slides/slide10.xml", "ppt/slides/slide2.xml"))
	assert.Zero(t, naturalCompare("ppt/slides/slide7.xml", "ppt/slides/slide7.xml"))
	assert.Negative(t, naturalCompare("a", "b"))
	assert.Negative(t, naturalCompare("slide", "slide1"))
}
