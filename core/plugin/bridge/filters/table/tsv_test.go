//go:build integration

package table

import (
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: TabSeparatedValuesFilterTest#testFileEvents
func TestTSV_FileEvents(t *testing.T) {
	pool, cfg := sharedPool(t)
	bridgetest.RequireFilter(t, pool, cfg, tsvFilterClass)

	parts := readTSVFile(t, "okf_table/TSV_test.txt", nil)
	require.NotEmpty(t, parts)

	// TSV_test.txt has Source/Target columns with 2 data rows.
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Source text 1")
	assert.Contains(t, texts, "Source text 2")

	// Should have LayerStart and LayerEnd framing.
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// okapi: TabSeparatedValuesFilterTest#testFileEvents2
func TestTSV_FileEvents2(t *testing.T) {
	parts := readTSVFile(t, "okf_table/test_tsv_simple.txt", nil)
	require.NotEmpty(t, parts)

	// test_tsv_simple.txt has a mix: some rows with targets, some without, some with comments.
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "Source1") || strings.Contains(b.SourceText(), "Source2") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find Source entries from test_tsv_simple.txt")
}

// okapi: TabSeparatedValuesFilterTest#testSkeleton
func TestTSV_Skeleton(t *testing.T) {
	output := tsvFileRoundtrip(t, "okf_table/TSV_test.txt", nil)
	require.NotEmpty(t, output)

	assert.Contains(t, output, "Source text 1")
	assert.Contains(t, output, "Source text 2")
}

// okapi: TabSeparatedValuesFilterTest#testSkeleton2
func TestTSV_Skeleton2(t *testing.T) {
	output := tsvFileRoundtrip(t, "okf_table/test_tsv_simple.txt", nil)
	require.NotEmpty(t, output)

	assert.Contains(t, output, "Source1")
}

// okapi: TabSeparatedValuesFilterTest#testDoubleExtraction
func TestTSV_DoubleExtraction(t *testing.T) {
	pool, cfg := sharedPool(t)
	bridgetest.RequireFilter(t, pool, cfg, tsvFilterClass)

	path := bridgetest.TestdataFile(t, "okf_table/TSV_test.txt")
	content, err := readFileContent(path)
	require.NoError(t, err)

	bridgetest.AssertRoundTripEvents(t, pool, cfg, tsvFilterClass, content, path, tsvMimeType, nil)
}
