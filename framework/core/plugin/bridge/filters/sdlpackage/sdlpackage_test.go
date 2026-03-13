//go:build integration

package sdlpackage

import (
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi-unmapped: SdlPackageFilterTests#testInformation — Java-only API test (IFilter.getName/getMimeType/getDisplayName)

// okapi: SdlPackageFilterTests#testSimpleRead
func TestSimpleRead(t *testing.T) {
	// The SDLPPX has XLIFF files under locale-named folders (en-US, fr-CA, pl-PL).
	// The SDL package filter uses the target locale to select which subfolder to process.
	// Java test uses en-US -> fr-CA.
	parts := readPackageFileWithLocales(t,
		"okapi/filters/sdlpackage/src/test/resources/ts2017-test01.sdlppx", "en-US", "fr-CA")

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")

	// The SDLPPX contains sub-documents — the XLIFF files are processed as child layers.
	// Java test expects 2 sub-documents (root layer + XLIFF sub-document).
	layerStarts := countPartsByType(parts, model.PartLayerStart)
	assert.GreaterOrEqual(t, layerStarts, 2,
		"should have at least 2 layers (root + XLIFF sub-document)")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from SDLPPX")

	// Java test verifies 4 segments total.
	totalSegments := 0
	for _, b := range blocks {
		totalSegments += len(b.Source)
	}
	assert.Equal(t, 4, totalSegments,
		"should extract 4 segments from ts2017-test01.sdlppx")

	// Java test verifies one segment contains this exact text.
	segFound := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "It has several paragraphs and several sentences.") {
			segFound = true
			break
		}
	}
	assert.True(t, segFound,
		"should find segment with 'It has several paragraphs and several sentences.'")
}

// okapi: SdlPackageFilterTests#testSdlppxWithSubFolders
func TestSdlppxWithSubFolders(t *testing.T) {
	// Java test uses en-US -> zh-x-hmn-SDL (Hmong SDL custom locale).
	// The test-packages.sdlppx may use a different locale structure.
	// Try with the exact locale used in the Java test.
	parts := readPackageFileWithLocales(t,
		"okapi/filters/sdlpackage/src/test/resources/test-packages.sdlppx", "en-US", "zh-x-hmn-SDL")

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")

	// Java test expects 4 sub-documents and 3 segments.
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from SDLPPX with sub-folders")

	totalSegments := 0
	for _, b := range blocks {
		totalSegments += len(b.Source)
	}
	assert.Equal(t, 3, totalSegments,
		"should extract 3 segments from test-packages.sdlppx")

	// Java test verifies a segment with text "Text in test-in-subdir."
	segFound := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "Text in test-in-subdir.") {
			segFound = true
			break
		}
	}
	assert.True(t, segFound,
		"should find segment with 'Text in test-in-subdir.'")
}

// okapi: SdlPackageFilterTests#testSdlrpxWithSubFolders
func TestSdlrpxWithSubFolders(t *testing.T) {
	// Java test uses en-US -> zh-x-hmn-SDL and reads targets.
	parts := readPackageFileWithLocales(t,
		"okapi/filters/sdlpackage/src/test/resources/test-packages.sdlrpx", "en-US", "zh-x-hmn-SDL")

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from SDLRPX with sub-folders")

	// Java test verifies 3 segments.
	totalSegments := 0
	for _, b := range blocks {
		totalSegments += len(b.Source)
	}
	assert.Equal(t, 3, totalSegments,
		"should extract 3 segments from test-packages.sdlrpx")

	// The return package (sdlrpx) should have target translations.
	// Java test verifies target text "FR Text in test-in-subdir." for the Hmong locale.
	// The bridge maps the target locale from the RawDocument.
	hasTarget := false
	for _, b := range blocks {
		for locale := range b.Targets {
			if strings.Contains(b.TargetText(locale), "FR Text in test-in-subdir.") {
				hasTarget = true
				break
			}
		}
		if hasTarget {
			break
		}
	}
	assert.True(t, hasTarget,
		"return package should contain target translation 'FR Text in test-in-subdir.'")
}

// okapi: SdlPackageFilterTests#testSimpleReadWrite
func TestSimpleReadWrite(t *testing.T) {
	// Java test uses en-US -> pl-PL for the read-write test.
	result := roundtripPackageFile(t,
		"okapi/filters/sdlpackage/src/test/resources/ts2017-test01.sdlppx", "en-US", "pl-PL")
	require.NotEmpty(t, result.Parts, "roundtrip should produce parts")
	require.NotEmpty(t, result.Output, "roundtrip should produce output")

	blocks1 := bridgetest.TranslatableBlocks(result.Parts)
	require.NotEmpty(t, blocks1, "first read should produce translatable blocks")

	// Re-read the output to verify content is preserved.
	// Write the output to a temp file since the SDL package filter needs a file URI.
	tmpFile, err := os.CreateTemp("", "sdlpackage-roundtrip-*.sdlppx")
	require.NoError(t, err)
	defer os.Remove(tmpFile.Name())

	_, err = tmpFile.Write(result.Output)
	require.NoError(t, err)
	require.NoError(t, tmpFile.Close())

	// Re-read the roundtripped file with the same locales.
	rereadParts := readPackageFileFromPath(t, tmpFile.Name(), "en-US", "pl-PL")
	blocks2 := bridgetest.TranslatableBlocks(rereadParts)
	require.NotEmpty(t, blocks2, "re-read after roundtrip should produce translatable blocks")

	// The block count should match between first read and re-read.
	assert.Equal(t, len(blocks1), len(blocks2),
		"roundtrip should preserve the same number of translatable blocks")

	// Verify segment count matches.
	segCount1 := 0
	for _, b := range blocks1 {
		segCount1 += len(b.Source)
	}
	segCount2 := 0
	for _, b := range blocks2 {
		segCount2 += len(b.Source)
	}
	assert.Equal(t, segCount1, segCount2,
		"roundtrip should preserve the same number of segments")
}

// TestLayerBalance verifies that layer start/end parts are balanced.
func TestLayerBalance(t *testing.T) {
	files := []struct {
		name      string
		path      string
		srcLocale model.LocaleID
		tgtLocale model.LocaleID
	}{
		{"sdlppx-frCA", "okapi/filters/sdlpackage/src/test/resources/ts2017-test01.sdlppx", "en-US", "fr-CA"},
		{"sdlppx-subfolders", "okapi/filters/sdlpackage/src/test/resources/test-packages.sdlppx", "en-US", "zh-x-hmn-SDL"},
		{"sdlrpx-subfolders", "okapi/filters/sdlpackage/src/test/resources/test-packages.sdlrpx", "en-US", "zh-x-hmn-SDL"},
	}

	for _, tc := range files {
		t.Run(tc.name, func(t *testing.T) {
			parts := readPackageFileWithLocales(t, tc.path, tc.srcLocale, tc.tgtLocale)

			starts := countPartsByType(parts, model.PartLayerStart)
			ends := countPartsByType(parts, model.PartLayerEnd)
			assert.Equal(t, starts, ends,
				"layer starts (%d) and ends (%d) should be balanced", starts, ends)
		})
	}
}

// TestGroupBalance verifies that group start/end parts are balanced.
func TestGroupBalance(t *testing.T) {
	files := []struct {
		name      string
		path      string
		srcLocale model.LocaleID
		tgtLocale model.LocaleID
	}{
		{"sdlppx-frCA", "okapi/filters/sdlpackage/src/test/resources/ts2017-test01.sdlppx", "en-US", "fr-CA"},
		{"sdlppx-subfolders", "okapi/filters/sdlpackage/src/test/resources/test-packages.sdlppx", "en-US", "zh-x-hmn-SDL"},
		{"sdlrpx-subfolders", "okapi/filters/sdlpackage/src/test/resources/test-packages.sdlrpx", "en-US", "zh-x-hmn-SDL"},
	}

	for _, tc := range files {
		t.Run(tc.name, func(t *testing.T) {
			parts := readPackageFileWithLocales(t, tc.path, tc.srcLocale, tc.tgtLocale)

			starts := countPartsByType(parts, model.PartGroupStart)
			ends := countPartsByType(parts, model.PartGroupEnd)
			assert.Equal(t, starts, ends,
				"group starts (%d) and ends (%d) should be balanced", starts, ends)
		})
	}
}

// TestBlockIDs verifies that all blocks have unique, non-empty IDs.
func TestBlockIDs(t *testing.T) {
	parts := readPackageFileWithLocales(t,
		"okapi/filters/sdlpackage/src/test/resources/ts2017-test01.sdlppx", "en-US", "fr-CA")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

// TestSegmentIDs verifies that all segments have non-empty IDs.
func TestSegmentIDs(t *testing.T) {
	parts := readPackageFileWithLocales(t,
		"okapi/filters/sdlpackage/src/test/resources/ts2017-test01.sdlppx", "en-US", "fr-CA")

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
