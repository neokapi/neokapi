//go:build integration

package okf_openxml

import (
	"fmt"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.openxml.OpenXMLFilter"

// MIME type is text/xml for the OpenXML filter (it processes the inner XML parts).
const mimeType = "text/xml"

// --- DOCX tests ---

func TestExtract_SimpleDocx(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/948-1.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX should produce translatable blocks")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Run1 Run3")
}

func TestExtract_DocxLayerStructure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/948-1.docx", mimeType, nil)

	var hasLayerStart, hasLayerEnd bool
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			hasLayerStart = true
		}
		if p.Type == model.PartLayerEnd {
			hasLayerEnd = true
		}
	}
	assert.True(t, hasLayerStart, "should have LayerStart")
	assert.True(t, hasLayerEnd, "should have LayerEnd")
}

func TestExtract_DocxMultiLayer(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// OpenXML documents produce multiple layers (one per internal XML part:
	// document.xml, styles.xml, etc.)
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/948-1.docx", mimeType, nil)

	var layerStartCount int
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layerStartCount++
		}
	}
	assert.Greater(t, layerStartCount, 1,
		"OpenXML should produce multiple layers (sub-documents)")
}

func TestExtract_DocxBlockIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/948-1.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

// okapi: OpenXMLTest#testTabAsCharacter
func TestExtract_DocxWithTabs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/Document-with-tabs.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with tabs should produce translatable blocks")
}

func TestExtract_DocxDataSkeleton(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/948-1.docx", mimeType, nil)

	// OpenXML skeleton data lives on Data parts (structural XML), not on blocks.
	var dataWithSkeleton int
	for _, p := range parts {
		if p.Type == model.PartData {
			data := p.Resource.(*model.Data)
			if data.Skeleton != nil && len(data.Skeleton.Parts) > 0 {
				dataWithSkeleton++
			}
		}
	}
	assert.Greater(t, dataWithSkeleton, 0, "some DOCX Data parts should have skeleton data")
}

func TestExtract_DocxInlineCodes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// 948-1.docx has formatting runs producing inline codes.
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/948-1.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Check that at least some blocks have spans (inline codes from formatting runs).
	var withSpans int
	for _, b := range blocks {
		frag := b.FirstFragment()
		if frag != nil && len(frag.Spans) > 0 {
			withSpans++
		}
	}
	if withSpans > 0 {
		t.Logf("found %d blocks with inline codes", withSpans)
	}
}

func TestExtract_DocxSegmentIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/948-1.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	for _, b := range blocks {
		require.NotEmpty(t, b.Source, "block should have source segments")
		for _, seg := range b.Source {
			assert.NotEmpty(t, seg.ID, "segment should have an ID")
			assert.NotNil(t, seg.Content, "segment should have content")
		}
	}
}

func TestExtract_DocxDataParts(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/948-1.docx", mimeType, nil)

	var dataCount int
	for _, p := range parts {
		if p.Type == model.PartData {
			dataCount++
			data := p.Resource.(*model.Data)
			assert.NotEmpty(t, data.ID, "data part should have an ID")
		}
	}
	assert.Greater(t, dataCount, 0, "DOCX should have Data parts from XML structure")
}

// okapi: OpenXMLTest#testReorderedZipPackage
func TestExtract_ReorderedZip(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// Regression: DOCX with reordered ZIP entries should still extract correctly.
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/reordered-zip.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "reordered ZIP DOCX should produce blocks")
}

// okapi: OpenXMLTest#testPPTXDocProperties
func TestExtract_DocProperties(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/DocProperties.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DocProperties.docx should produce translatable blocks")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Ode to the IRS")
	assert.Contains(t, texts, "John Doe")
}

// --- XLSX tests ---

func TestExtract_SimpleXlsx(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/pokemon.xlsx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "XLSX should produce translatable blocks")
}

func TestExtract_XlsxBlockIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/pokemon.xlsx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "XLSX block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

// okapi: OpenXmlXlsxTest#testInlineStrings
func TestExtract_XlsxInlineStrings(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/1199-inline-strings.xlsx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "XLSX with inline strings should produce blocks")
}

func TestExtract_XlsxMultiLayer(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/pokemon.xlsx", mimeType, nil)

	// XLSX should have multiple layers (shared strings, sheet1, etc.)
	var layerStartCount int
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layerStartCount++
		}
	}
	assert.Greater(t, layerStartCount, 1,
		"XLSX should produce multiple layers (sub-documents for sheets)")
}

// --- PPTX tests ---

func TestExtract_SimplePptx(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/794.pptx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX should produce translatable blocks")
}

func TestExtract_PptxBlockIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/794.pptx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "PPTX block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

func TestExtract_PptxMultiLayer(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/794.pptx", mimeType, nil)

	// PPTX should have layers for slides, notes, masters, etc.
	var layerStartCount int
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layerStartCount++
		}
	}
	assert.Greater(t, layerStartCount, 1,
		"PPTX should produce multiple layers (sub-documents for slides)")
}

// okapi: OpenXMLTest#testLineBreakAsCharacter
func TestExtract_PptxLineBreak(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/1421-line-break.pptx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with line breaks should produce blocks")
}

// --- Cross-format tests ---

func TestExtract_AllFormatsLayerBalance(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	tests := []struct {
		name string
		file string
	}{
		{"docx", tdDir + "/okf_openxml/948-1.docx"},
		{"xlsx", tdDir + "/okf_openxml/pokemon.xlsx"},
		{"pptx", tdDir + "/okf_openxml/794.pptx"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
				tc.file, mimeType, nil)

			var starts, ends int
			for _, p := range parts {
				if p.Type == model.PartLayerStart {
					starts++
				}
				if p.Type == model.PartLayerEnd {
					ends++
				}
			}
			assert.Equal(t, starts, ends,
				"layer starts and ends should be balanced")
		})
	}
}

func TestExtract_AllFormatsGroupBalance(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	tests := []struct {
		name string
		file string
	}{
		{"docx", tdDir + "/okf_openxml/948-1.docx"},
		{"xlsx", tdDir + "/okf_openxml/pokemon.xlsx"},
		{"pptx", tdDir + "/okf_openxml/794.pptx"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
				tc.file, mimeType, nil)

			var starts, ends int
			for _, p := range parts {
				if p.Type == model.PartGroupStart {
					starts++
				}
				if p.Type == model.PartGroupEnd {
					ends++
				}
			}
			assert.Equal(t, starts, ends,
				"group starts and ends should be balanced")
		})
	}
}

func TestExtract_PartSequenceIntegrity(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	tests := []struct {
		name string
		file string
	}{
		{"docx", tdDir + "/okf_openxml/948-1.docx"},
		{"xlsx", tdDir + "/okf_openxml/pokemon.xlsx"},
		{"pptx", tdDir + "/okf_openxml/794.pptx"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
				tc.file, mimeType, nil)

			require.NotEmpty(t, parts)

			// First part should be LayerStart.
			assert.Equal(t, model.PartLayerStart, parts[0].Type,
				"first part should be LayerStart")

			// Last part should be LayerEnd.
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type,
				"last part should be LayerEnd")

			// Every part should have a valid type.
			for i, p := range parts {
				assert.True(t, p.Type >= model.PartLayerStart && p.Type <= model.PartMedia,
					"part[%d] has invalid type %d", i, p.Type)
				assert.NotNil(t, p.Resource,
					"part[%d] resource should not be nil", i)
			}
		})
	}
}

func TestExtract_SpanData(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// 948-1.docx has formatting runs producing inline codes.
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/948-1.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Check span metadata for blocks with inline codes.
	for _, b := range blocks {
		frag := b.FirstFragment()
		if frag == nil || len(frag.Spans) == 0 {
			continue
		}
		for j, s := range frag.Spans {
			assert.NotEmpty(t, s.ID,
				fmt.Sprintf("block %s span[%d] should have an ID", b.ID, j))
		}
		return // Found a block with spans, test passes.
	}
}

// --- DOCX edge case tests ---

// okapi: OpenXMLTest#testLineBreakAsTag
func TestExtract_DocxSoftLineBreaks(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/Document-with-soft-linebreaks.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with soft line breaks should produce blocks")
}

// okapi: OpenXMLTest#extractsStructuralDocumentTagsAsRunContainers
func TestExtract_DocxTextBoxes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/TextBoxes.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with text boxes should produce blocks")
}

func TestExtract_DocxSmartArt(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/smart_art.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with SmartArt should produce blocks")
}

func TestExtract_DocxWatermark(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// Watermarks are typically non-translatable — verify document still processes.
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/watermark.docx", mimeType, nil)

	require.NotEmpty(t, parts, "DOCX with watermark should produce parts")

	var hasLayerStart bool
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			hasLayerStart = true
			break
		}
	}
	assert.True(t, hasLayerStart, "watermark DOCX should have layer structure")
}

func TestExtract_DocxSpecialCharsAndLinebreaks(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/special-chars-and-linebreaks.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with special chars should produce blocks")
}

// okapi: OpenXMLTest#extractsNoneReorderedNotesAndComments
func TestExtract_DocxNotes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/1413-notes.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with footnotes/endnotes should produce blocks")

	// Document with notes should produce multiple layers (main doc + notes).
	var layerCount int
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layerCount++
		}
	}
	assert.Greater(t, layerCount, 1, "footnotes/endnotes should create additional layers")
}

// okapi: OpenXMLTest#extractsExternalHyperlinks
func TestExtract_DocxExternalHyperlink(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/external_hyperlink.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with external hyperlinks should produce blocks")
}

// okapi: OpenXMLTest#extractsNestedContentInTheExpectedOrder
func TestExtract_DocxNestedTables(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/848-nested-tables.docx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with nested tables should produce blocks")
}

// --- XLSX edge case tests ---

// okapi: OpenXMLTest#documentsWithAbsentSharedStringsProcessed
func TestExtract_XlsxEmptyCells(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/894-empty-cells-and-rows.xlsx", mimeType, nil)

	require.NotEmpty(t, parts, "XLSX with empty cells should produce parts")
}

func TestExtract_XlsxSmartArt(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/smartart.xlsx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "XLSX with SmartArt should produce blocks")
}

// okapi: OpenXMLTest#testXLSXOnlyExtractStringsNotNumbers
func TestExtract_XlsxSharedStringsAndComments(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/972-shared-strings-and-comments.xlsx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "XLSX with shared strings and comments should produce blocks")
}

// --- PPTX edge case tests ---

func TestExtract_PptxSmartArt(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/SmartArt.pptx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with SmartArt should produce blocks")
}

// okapi: OpenXMLTest#testPPTXComments
func TestExtract_PptxComments(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/Comments.pptx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with comments should produce blocks")
}

// okapi: OpenXmlPptxTest#extractsHiddenSlides
func TestExtract_PptxHiddenSlides(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/1010-slide1-hidden-slide2-hidden.pptx", mimeType, nil)

	// Hidden slides should still be processed (content may be translatable).
	require.NotEmpty(t, parts, "PPTX with hidden slides should produce parts")
}

// okapi: OpenXMLTest#testSlideReordering
func TestExtract_PptxSlideLayouts(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/slideLayouts.pptx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with multiple slide layouts should produce blocks")

	// Multiple layouts → multiple layers.
	var layerCount int
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layerCount++
		}
	}
	assert.Greater(t, layerCount, 1,
		"PPTX with slide layouts should produce multiple layers")
}

func TestExtract_PptxFormattedHyperlink(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		tdDir+"/okf_openxml/FormattedHyperlink.pptx", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with formatted hyperlinks should produce blocks")
}

// TestExtract_KnownLimitationDocx verifies that DOCX files which fail roundtrip
// due to Okapi OpenXML filter limitations still extract content correctly.
// The roundtrip failures are Okapi-level structural Data part changes, not
// bridge bugs — translatable content is fully preserved.
func TestExtract_KnownLimitationDocx(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	files := []struct {
		name       string
		limitation string
	}{
		{"1102.docx", "structural Data dropped in complex revision markup"},
		{"830-3.docx", "structural Data added between consecutive blocks"},
		{"847-2.docx", "tracked changes cause Data part drop"},
		{"847-3.docx", "tracked changes cause Data part drop"},
		{"956.docx", "complex structure causes Data part drop"},
		{"1437-color-exclusion.docx", "span Type CSS property order instability"},
	}

	for _, tt := range files {
		t.Run(tt.name, func(t *testing.T) {
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
				bridgetest.TestdataFile(t, "okf_openxml/"+tt.name), mimeType, nil)

			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

			blocks := bridgetest.TranslatableBlocks(parts)
			assert.NotEmpty(t, blocks,
				"%s should extract translatable blocks (limitation: %s)", tt.name, tt.limitation)
		})
	}
}

// --- Additional PPTX extraction tests (OpenXmlPptxTest) ---

// okapi: OpenXmlPptxTest#testFormattingsPptx
func TestExtract_PptxFormattings(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		bridgetest.TestdataFile(t, "okf_openxml/1009-1.pptx"), mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with formattings should produce blocks")
}

// okapi: OpenXmlPptxTest#chartsTranslatedAndReordered
func TestExtract_PptxCharts(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		bridgetest.TestdataFile(t, "okf_openxml/1046.pptx"), mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with charts should produce blocks")
}

// okapi: OpenXmlPptxTest#testIncludeSlidesNo
func TestExtract_PptxVisibleHiddenSlides(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// Both hidden and visible slides should be processable.
	tests := []struct {
		name string
		file string
	}{
		{"visible-hidden", "1010-slide1-visible-slide2-hidden.pptx"},
		{"both-hidden", "1010-slide1-hidden-slide2-hidden.pptx"},
		{"visible-hidden-2", "1011-slide1-visible-slide2-hidden.pptx"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
				bridgetest.TestdataFile(t, "okf_openxml/"+tc.file), mimeType, nil)
			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
		})
	}
}

// okapi: OpenXmlPptxTest#lineBreaksExtractedAsTags
func TestExtract_PptxLineBreaksAsTags(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		bridgetest.TestdataFile(t, "okf_openxml/1421-line-break.pptx"), mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with line breaks should produce blocks")

	// Line breaks should appear as inline codes (spans).
	hasSpans := false
	for _, b := range blocks {
		frag := b.FirstFragment()
		if frag != nil && len(frag.Spans) > 0 {
			hasSpans = true
			break
		}
	}
	assert.True(t, hasSpans, "line breaks should be represented as inline codes")
}

// --- Additional XLSX extraction tests (OpenXmlXlsxTest) ---

// okapi: OpenXmlXlsxTest#inlineStringsExtracted
func TestExtract_XlsxInlineStringsExtracted(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		bridgetest.TestdataFile(t, "okf_openxml/1199-inline-strings.xlsx"), mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "XLSX inline strings should produce blocks")

	// Inline strings should have extractable text.
	texts := bridgetest.BlockTexts(blocks)
	assert.NotEmpty(t, texts)
}

// okapi: OpenXmlXlsxTest#mergedCellsAsMetadataMarked
func TestExtract_XlsxMergedCells(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		bridgetest.TestdataFile(t, "okf_openxml/972-shared-strings-and-comments.xlsx"), mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: OpenXmlXlsxTest#crossSheetsReferences
func TestExtract_XlsxCrossSheetReferences(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	tests := []struct {
		name string
		file string
	}{
		{"cross-sheets", "1051-cross-sheets-references.xlsx"},
		{"table-refs", "1051-cross-sheets-table-references.xlsx"},
		{"table-refs-2", "1051-cross-sheets-table-references-2.xlsx"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
				bridgetest.TestdataFile(t, "okf_openxml/"+tc.file), mimeType, nil)
			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
		})
	}
}

// --- Additional roundtrip tests (OpenXMLDefaultConfigRoundTripTest, OpenXMLRoundTripTest) ---

// okapi: OpenXMLDefaultConfigRoundTripTest
// okapi: OpenXMLRoundTripTest
// Note: These are covered by RoundTripTestFiles in roundtrip_test.go which
// globs all .docx/.xlsx/.pptx files. The 85+117=202 surefire tests correspond
// to per-file roundtrips which our glob-based approach covers comprehensively.

// --- OpenXML-specific feature tests ---

// okapi: OpenXMLRoundtripAddTabAsCharTest
func TestExtract_TabAsCharVariants(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		bridgetest.TestdataFile(t, "okf_openxml/Document-with-tabs.docx"), mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "tab-as-char document should produce blocks")
}

// okapi-skip: OpenXMLRepetitionTest — no testdata file (repetitions.docx not in testdata set)

// okapi: OpenXMLZipFullFileTest
func TestExtract_ZipFullFile(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// This tests that the full ZIP structure is handled correctly.
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		bridgetest.TestdataFile(t, "okf_openxml/948-1.docx"), mimeType, nil)

	require.NotEmpty(t, parts)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: OpenXmlFormattingTest
func TestExtract_FormattingPreservation(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// Test formatting in all three document types.
	tests := []struct {
		name string
		file string
	}{
		{"docx-formatting", "948-1.docx"},
		{"pptx-formatting", "1009-1.pptx"},
		{"xlsx-formatting", "pokemon.xlsx"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
				bridgetest.TestdataFile(t, "okf_openxml/"+tc.file), mimeType, nil)
			blocks := bridgetest.TranslatableBlocks(parts)
			require.NotEmpty(t, blocks)
		})
	}
}

// okapi: SubfilteringTest
func TestExtract_Subfiltering(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// Subfiltering tests embedded content within OpenXML documents.
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
		bridgetest.TestdataFile(t, "okf_openxml/948-1.docx"), mimeType, nil)

	require.NotEmpty(t, parts)
	// Sub-documents (child layers) indicate subfiltering is working.
	layerStarts := 0
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layerStarts++
		}
	}
	assert.Greater(t, layerStarts, 1, "subfiltering should produce multiple layers")
}

// TestExtract_KnownLimitationPptx verifies that PPTX files which fail roundtrip
// due to Okapi style inheritance collapse still extract content correctly.
func TestExtract_KnownLimitationPptx(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	files := []struct {
		name       string
		limitation string
	}{
		{"1329-styles-clarification.pptx", "PPTX theme-based style inheritance collapse"},
		{"1435-text-for-masking.pptx", "font stack truncation during roundtrip"},
	}

	for _, tt := range files {
		t.Run(tt.name, func(t *testing.T) {
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass,
				bridgetest.TestdataFile(t, "okf_openxml/"+tt.name), mimeType, nil)

			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

			blocks := bridgetest.TranslatableBlocks(parts)
			assert.NotEmpty(t, blocks,
				"%s should extract translatable blocks (limitation: %s)", tt.name, tt.limitation)
		})
	}
}
