//go:build integration

package archive

import (
	"bytes"
	"context"
	"io"
	"os"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: ArchiveFilterTest#testNoTUs
func TestExtract_NoTUs(t *testing.T) {
	// Archive with unknown file types (stuff.txt) produces no translatable blocks.
	parts := readArchiveFile(t, "test2_unknownfiles.archive", xliffOnlyParams())

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.Empty(t, blocks, "archive with unknown files should produce no translatable blocks")
}

// okapi: ArchiveFilterTest#testNoExtraction
func TestExtract_NoExtraction(t *testing.T) {
	// With no matching file patterns, the archive filter should not extract
	// translatable blocks. We use a pattern that matches nothing in the archive.
	params := map[string]any{
		"fileNames": "*.nonexistent",
		"configIds": "okf_xliff",
	}
	parts := readArchiveFile(t, "test3_es.archive", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.Empty(t, blocks, "non-matching file pattern should extract no translatable blocks")
}

// okapi: ArchiveFilterTest#testMimeType
func TestExtract_MimeType(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	// Read with default params to verify the filter opens successfully.
	parts := readArchiveFile(t, "test3_es.archive", xliffOnlyParams())
	require.NotEmpty(t, parts, "should produce parts from archive")

	// The first part should be a LayerStart with archive mime type info.
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
}

// okapi: ArchiveFilterTest#testExtractXLIFFOnly
func TestExtract_XLIFFOnly(t *testing.T) {
	// fileNames="*.xlf", configIds="okf_xliff": extracts only XLIFF TU "About..."
	parts := readArchiveFile(t, "test3_es.archive", xliffOnlyParams())

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract XLIFF blocks from archive")

	// test3_es.archive contains SF-12-Test02.xlf with "About..." text.
	found := findBlockContaining(blocks, "About")
	assert.NotNil(t, found, "should find block containing 'About' from XLIFF sub-document")
}

// okapi: ArchiveFilterTest#testExtractTMXOnly
func TestExtract_TMXOnly(t *testing.T) {
	// fileNames="*.tmx", configIds="okf_tmx": extracts only TMX TU "test en"
	parts := readArchiveFile(t, "test3_es.archive", tmxOnlyParams())

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract TMX blocks from archive")

	// test3_es.archive contains test1.tmx with "test en" text.
	found := findBlockContaining(blocks, "test en")
	assert.NotNil(t, found, "should find block containing 'test en' from TMX sub-document")
}

// okapi: ArchiveFilterTest#testExtractXLIFFandTMX
func TestExtract_XLIFFandTMX(t *testing.T) {
	// Combined fileNames/configIds extracts both XLIFF and TMX TUs.
	parts := readArchiveFile(t, "test3_es.archive", xliffAndTMXParams())

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks from both XLIFF and TMX sub-documents")

	// Should have both "About" from XLIFF and "test en" from TMX.
	foundAbout := findBlockContaining(blocks, "About")
	foundTestEn := findBlockContaining(blocks, "test en")
	assert.NotNil(t, foundAbout, "should find XLIFF block containing 'About'")
	assert.NotNil(t, foundTestEn, "should find TMX block containing 'test en'")
}

// okapi: ArchiveFilterTest#testMissingFilter
func TestExtract_MissingFilter(t *testing.T) {
	// Missing filter config should cause an error during open or read.
	// In Java this throws OkapiIOException.
	params := map[string]any{
		"fileNames": "*.xlf",
		"configIds": "okf_nonexistent",
	}

	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_archive/test3_es.archive")
	content, err := os.ReadFile(path)
	require.NoError(t, err)

	reader := bridge.NewBridgeFormatReader(pool, cfg, filterClass)
	reader.SetFilterParams(params)

	doc := &model.RawDocument{
		URI:          path,
		SourceLocale: "en",
		TargetLocale: "fr",
		Encoding:     "UTF-8",
		MimeType:     mimeType,
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}

	ctx := context.Background()
	openErr := reader.Open(ctx, doc)
	if openErr != nil {
		// Error during open — expected for missing filter config.
		t.Logf("open error (expected): %v", openErr)
		return
	}
	defer reader.Close()

	// If open succeeded, check that reading produces an error or no blocks.
	var parts []*model.Part
	var readErr error
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			readErr = pr.Error
			break
		}
		parts = append(parts, pr.Part)
	}

	if readErr != nil {
		// Error during read — expected for missing filter config.
		t.Logf("read error (expected): %v", readErr)
		return
	}

	// If no error, there should be no translatable blocks.
	blocks := bridgetest.TranslatableBlocks(parts)
	assert.Empty(t, blocks, "missing filter config should not produce translatable blocks")
}

// okapi: ArchiveFilterTest#testSubFilterOpen
func TestExtract_SubFilterOpen(t *testing.T) {
	// Opens ZIP entry from test1_es.archive with XLIFF subfilter.
	parts := readArchiveFile(t, "test1_es.archive", xliffOnlyParams())

	// Should have sub-document layers for the embedded XLIFF content.
	layerStarts := countPartsByType(parts, model.PartLayerStart)
	assert.GreaterOrEqual(t, layerStarts, 1, "should have at least one LayerStart for sub-document")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from XLIFF in archive")
}

// okapi: ArchiveFilterTest#testFilterOpen
func TestExtract_FilterOpen(t *testing.T) {
	// Opens ZIP entry from test1_es.archive via filter pipeline.
	parts := readArchiveFile(t, "test1_es.archive", xliffAndTMXParams())

	require.NotEmpty(t, parts, "should produce parts from archive")
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")

	// Should have blocks from both XLIFF and TMX in the archive.
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from archive")
}

// okapi: ArchiveFilterTest#testWithStream
func TestExtract_WithStream(t *testing.T) {
	// Stream-based read/write pipeline with archive filter.
	// Verify both TMX and XLIFF content can be extracted.
	parts := readArchiveFile(t, "test3_es.archive", xliffAndTMXParams())

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "stream-based extraction should produce translatable blocks")

	// Verify specific content from both sub-formats.
	foundAbout := findBlockContaining(blocks, "About")
	foundTestEn := findBlockContaining(blocks, "test en")
	assert.NotNil(t, foundAbout, "should have XLIFF content (About)")
	assert.NotNil(t, foundTestEn, "should have TMX content (test en)")
}

// okapi: ArchiveFilterTest#testDoubelextraction
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	// Double extraction roundtrip for 3 archive files (matching Java test).
	testFiles := []struct {
		name   string
		params map[string]any
	}{
		{"test1_es.archive", xliffAndTMXParams()},
		{"test2_unknownfiles.archive", xliffOnlyParams()},
		{"test3_es.archive", xliffAndTMXParams()},
	}

	for _, tt := range testFiles {
		t.Run(tt.name, func(t *testing.T) {
			path := bridgetest.TestdataFile(t, "okf_archive/"+tt.name)
			content, err := os.ReadFile(path)
			require.NoError(t, err)

			// First extraction.
			result := bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, tt.params)

			// Second extraction from the roundtrip output.
			parts2 := bridgetest.ReadBytes(t, pool, cfg, filterClass, result.Output, path, mimeType, tt.params)
			blocks1 := bridgetest.TranslatableBlocks(result.Parts)
			blocks2 := bridgetest.TranslatableBlocks(parts2)

			assert.Equal(t, len(blocks1), len(blocks2),
				"double extraction should produce same block count for %s", tt.name)
		})
	}
}
