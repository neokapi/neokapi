package openxml

// okapi-filter: openxml

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A core property's text is whatever sits between its tags, edge whitespace
// included. `<dc:title>Title 1. </dc:title>` is a title with a trailing space,
// and a reader that trims it hands a translator a different string and writes a
// different document back.
//
// The trim answered a second question in the same expression: whether the
// element held anything translatable at all. A property holding only whitespace
// still produces no block, and the empty-element branch replays its source
// form; the two questions are asked separately.
//
// One parser serves every container, so the fix reaches a document and a
// workbook as well as a deck.

// coreProps wraps a sequence of property elements in a core-properties part.
func coreProps(body string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
		`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" ` +
		`xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" ` +
		`xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` + body + `</cp:coreProperties>`
}

// corePropsWorkbook builds an xlsx carrying the given core-properties part.
func corePropsWorkbook(t *testing.T, part string) []byte {
	t.Helper()
	contentTypes := strings.Replace(fidelityContentTypes, `</Types>`,
		`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/></Types>`, 1)
	rootRels := strings.Replace(fidelityRootRels, `</Relationships>`,
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/></Relationships>`, 1)
	return buildFidelityPackage(t, []fidelityEntry{
		{"[Content_Types].xml", contentTypes},
		{"_rels/.rels", rootRels},
		{"docProps/core.xml", part},
		{"xl/workbook.xml", fidelityWorkbook},
		{"xl/_rels/workbook.xml.rels", fidelityWorkbookRels},
		{"xl/worksheets/sheet1.xml", fidelityWorksheet},
		{"xl/sharedStrings.xml", fidelitySharedStrings},
	})
}

// corePropsDeck builds a pptx carrying the given core-properties part.
func corePropsDeck(t *testing.T, part string) []byte {
	t.Helper()
	contentTypes := strings.Replace(dmlContentTypes, `</Types>`,
		`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/></Types>`, 1)
	rootRels := strings.Replace(dmlRootRels, `</Relationships>`,
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/></Relationships>`, 1)
	return buildFidelityPackage(t, []fidelityEntry{
		{"[Content_Types].xml", contentTypes},
		{"_rels/.rels", rootRels},
		{"docProps/core.xml", part},
		{"ppt/presentation.xml", dmlPresentation},
		{"ppt/_rels/presentation.xml.rels", dmlPresentationRels},
		{"ppt/slides/slide1.xml", dmlSlide(`<a:p><a:r><a:rPr lang="en-US"/><a:t>Title</a:t></a:r></a:p>`)},
	})
}

// TestCoreProperties_EdgeWhitespaceIsByteIdentical is #2535, on both containers
// the shared parser serves.
func TestCoreProperties_EdgeWhitespaceIsByteIdentical(t *testing.T) {
	part := coreProps(
		`<dc:title>Title 1. </dc:title>` +
			`<dc:subject>  Leading line breaks</dc:subject>` +
			`<dc:creator>User</dc:creator>`)

	t.Run("xlsx", func(t *testing.T) {
		out := skeletonRoundtripBytes(t, corePropsWorkbook(t, part), "props.xlsx")
		assert.Equal(t, part, string(zipPartBytes(t, out, "docProps/core.xml")))
	})
	t.Run("pptx", func(t *testing.T) {
		out := skeletonRoundtripBytes(t, corePropsDeck(t, part), "props.pptx")
		assert.Equal(t, part, string(zipPartBytes(t, out, "docProps/core.xml")))
	})
}

// TestCoreProperties_TranslatorSeesTheTextAsWritten is the other consequence:
// the trim reached the string a translator was handed.
func TestCoreProperties_TranslatorSeesTheTextAsWritten(t *testing.T) {
	part := coreProps(`<dc:title>Title 1. </dc:title><dc:subject>  Leading</dc:subject>`)
	blocks := corePropertyBlocks(t, corePropsWorkbook(t, part))
	require.Len(t, blocks, 2)
	assert.Equal(t, "Title 1. ", blocks[0].SourceText())
	assert.Equal(t, "  Leading", blocks[1].SourceText())
}

// TestCoreProperties_WhitespaceOnlyPropertyProducesNoBlock keeps the emptiness
// test on the trimmed value, so the element replays rather than becoming a
// block holding nothing.
func TestCoreProperties_WhitespaceOnlyPropertyProducesNoBlock(t *testing.T) {
	part := coreProps(`<dc:title>   </dc:title><dc:subject/><dc:creator>User</dc:creator>`)
	blocks := corePropertyBlocks(t, corePropsWorkbook(t, part))
	for _, b := range blocks {
		assert.NotEqual(t, "title", b.Properties["element"],
			"a property holding only whitespace produces no block")
	}
	out := skeletonRoundtripBytes(t, corePropsWorkbook(t, part), "props.xlsx")
	assert.Equal(t, part, string(zipPartBytes(t, out, "docProps/core.xml")),
		"and its source form comes back")
}

// TestCoreProperties_TrailingEmptyPropertyIsByteIdentical is #2574: an empty
// core property that is the last child of `<cp:coreProperties>` goes back in
// the form its author wrote, self-closing form included.
func TestCoreProperties_TrailingEmptyPropertyIsByteIdentical(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{"self-closing last", `<dc:title>A title</dc:title><cp:category/>`},
		{"self-closing last, newline before the root close",
			`<dc:title>A title</dc:title><cp:category/>` + "\r\n"},
		{"explicit open and close last", `<dc:title>A title</dc:title><cp:category></cp:category>`},
		{"two self-closing in a row", `<cp:category/><cp:contentStatus/>`},
		{"self-closing followed by a property Okapi leaves alone",
			`<cp:category/><cp:revision>4</cp:revision>`},
		{"every property empty and self-closing",
			`<dc:title/><dc:subject/><dc:creator/><cp:keywords/><dc:description/>` +
				`<cp:category/><cp:contentStatus/>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			part := coreProps(tc.body)
			t.Run("xlsx", func(t *testing.T) {
				out := skeletonRoundtripBytes(t, corePropsWorkbook(t, part), "props.xlsx")
				assert.Equal(t, part, string(zipPartBytes(t, out, "docProps/core.xml")))
			})
			t.Run("pptx", func(t *testing.T) {
				out := skeletonRoundtripBytes(t, corePropsDeck(t, part), "props.pptx")
				assert.Equal(t, part, string(zipPartBytes(t, out, "docProps/core.xml")))
			})
		})
	}
}

// TestCoreProperties_TrailingEmptyPropertyProducesNoBlock keeps the other half
// of the statement: the element is skeleton, so a translator is handed nothing
// for it and a target writes it back unchanged.
func TestCoreProperties_TrailingEmptyPropertyProducesNoBlock(t *testing.T) {
	part := coreProps(`<dc:title>A title</dc:title><cp:category/>`)
	blocks := corePropertyBlocks(t, corePropsWorkbook(t, part))
	require.Len(t, blocks, 1)
	assert.Equal(t, "title", blocks[0].Properties["element"])

	out := skeletonWriteBack(t, corePropsWorkbook(t, part), model.LocaleID("qps"),
		func(b *model.Block) string { return "[" + b.SourceText() + "]" })
	assert.Equal(t, coreProps(`<dc:title>[A title]</dc:title><cp:category/>`),
		string(zipPartBytes(t, out, "docProps/core.xml")))
}

// TestCoreProperties_TranslatedPropertyReplacesTheWholeText states what a
// target writes: the translation, edges and all.
func TestCoreProperties_TranslatedPropertyReplacesTheWholeText(t *testing.T) {
	part := coreProps(`<dc:title>Title 1. </dc:title>`)
	out := skeletonWriteBack(t, corePropsWorkbook(t, part), model.LocaleID("qps"),
		func(b *model.Block) string { return "[" + b.SourceText() + "]" })
	assert.Contains(t, string(zipPartBytes(t, out, "docProps/core.xml")),
		`<dc:title>[Title 1. ]</dc:title>`)
}

// corePropertyBlocks returns the blocks the reader extracted from
// docProps/core.xml, in the order it emitted them.
func corePropertyBlocks(t *testing.T, pkg []byte) []*model.Block {
	t.Helper()
	reader := NewReader()
	require.NoError(t, reader.Open(t.Context(), &model.RawDocument{
		URI:          "props.xlsx",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(pkg),
	}))
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	require.NoError(t, reader.Close())

	var out []*model.Block
	for _, b := range testutil.FilterBlocks(parts) {
		if b.Properties["partPath"] == "docProps/core.xml" {
			out = append(out, b)
		}
	}
	return out
}
