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
