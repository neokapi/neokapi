package openxml

import (
	"testing"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// docxPartStructure maps a part path to the plane / fallback note-role it
// implies (structure-geometry-landscape.md §8).
func TestDocxPartStructure(t *testing.T) {
	cases := []struct {
		path, plane, note string
	}{
		{"word/document.xml", "", ""},
		{"word/header1.xml", model.LayerFurniture, ""},
		{"word/footer2.xml", model.LayerFurniture, ""},
		{"word/footnotes.xml", "", model.RoleFootnote},
		{"word/endnotes.xml", "", model.RoleFootnote},
		{"word/glossary/footnotes.xml", "", model.RoleFootnote},
	}
	for _, c := range cases {
		plane, note := docxPartStructure(c.path)
		assert.Equal(t, c.plane, plane, "plane for %s", c.path)
		assert.Equal(t, c.note, note, "note role for %s", c.path)
	}
}

// applyPPTXPartFacets tags speaker notes as presenter-only metadata and
// masters/layouts as template furniture; slide bodies are untouched.
func TestApplyPPTXPartFacets(t *testing.T) {
	cases := []struct {
		path, plane, vis string
	}{
		{"ppt/slides/slide1.xml", "", ""},
		{"ppt/notesSlides/notesSlide1.xml", model.LayerMetadata, model.VisibilityScreenOnly},
		{"ppt/slideMasters/slideMaster1.xml", model.LayerFurniture, ""},
		{"ppt/slideLayouts/slideLayout3.xml", model.LayerFurniture, ""},
	}
	for _, c := range cases {
		b := model.NewBlock("b", "x")
		applyPPTXPartFacets(b, c.path)
		assert.Equal(t, c.plane, b.LayoutLayer(), "plane for %s", c.path)
		assert.Equal(t, c.vis, b.Visibility(), "visibility for %s", c.path)
	}
}

// A running-header part's paragraphs carry the furniture plane.
func TestNative_DocxHeaderFurniture(t *testing.T) {
	parts := readFile(t, "testdata/formatted.docx")
	var header *model.Block
	for _, b := range translatableBlocks(parts) {
		if b.SourceText() == "Header Text" {
			header = b
			break
		}
	}
	require.NotNil(t, header, "expected a 'Header Text' block from word/header1.xml")
	assert.Equal(t, model.LayerFurniture, header.LayoutLayer(), "header paragraph → furniture plane")
}

// A fully hidden (<w:vanish/>) paragraph carries visibility=hidden when hidden
// text is extracted; a visible sibling carries no visibility facet.
func TestNative_DocxHiddenVisibility(t *testing.T) {
	doc := `<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:body>` +
		`<w:p><w:r><w:rPr><w:vanish/></w:rPr><w:t>Hidden line</w:t></w:r></w:p>` +
		`<w:p><w:r><w:t>Visible line</w:t></w:r></w:p>` +
		`</w:body></w:document>`
	data := docxWithDocumentXML(t, doc)

	reader := NewReader()
	reader.cfg.TranslateHiddenText = true // so the hidden paragraph still emits a block
	rawDoc := &model.RawDocument{
		URI:          "t.docx",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(data),
	}
	require.NoError(t, reader.Open(t.Context(), rawDoc))
	blocks := translatableBlocks(testutil.CollectParts(t, reader.Read(t.Context())))
	reader.Close()

	visOf := func(text string) string {
		for _, b := range blocks {
			if b.SourceText() == text {
				return b.Visibility()
			}
		}
		t.Fatalf("no block with text %q (have: %v)", text, testutil.BlockTexts(blocks))
		return ""
	}
	assert.Equal(t, model.VisibilityHidden, visOf("Hidden line"), "vanish paragraph → hidden")
	assert.Equal(t, model.VisibilityVisible, visOf("Visible line"), "plain paragraph → no visibility facet")
}
