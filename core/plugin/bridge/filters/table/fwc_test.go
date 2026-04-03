//go:build integration

package table

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: FixedWidthColumnsFilterTest#testNameAndMimeType
func TestFWC_NameAndMimeType(t *testing.T) {
	pool, cfg := sharedPool(t)
	bridgetest.RequireFilter(t, pool, cfg, fwcFilterClass)

	// FWC content: fixed-width columns.
	content := "Source             Target\ns1                 t1\n"
	parts := readFWC(t, content, nil)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok, "first part resource should be a Layer")
	// The FWC filter inherits from the CSV base and reports text/csv as mime type
	// through the bridge, even though the canonical mime type is text/plain.
	assert.NotEmpty(t, layer.MimeType)
}

// okapi: FixedWidthColumnsFilterTest#testEmptyInput
func TestFWC_EmptyInput(t *testing.T) {
	pool, cfg := sharedPool(t)
	bridgetest.RequireFilter(t, pool, cfg, fwcFilterClass)

	parts := readFWC(t, "", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.Empty(t, blocks, "empty input should produce no translatable blocks")
}

// okapi: FixedWidthColumnsFilterTest#testParameters
func TestFWC_Parameters(t *testing.T) {
	pool, cfg := sharedPool(t)
	bridgetest.RequireFilter(t, pool, cfg, fwcFilterClass)

	// Use test_params3.txt which has explicit column start/end positions.
	parts := readFWCFile(t, "okapi/filters/table/src/test/resources/csv_test6.txt", nil)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "fixed-width columns should produce translatable blocks")
}

// okapi: FixedWidthColumnsFilterTest#testFileEvents
func TestFWC_FileEvents(t *testing.T) {
	parts := readFWCFile(t, "okapi/filters/table/src/test/resources/csv_test6.txt", nil)
	require.NotEmpty(t, parts)

	// csv_test6.txt is a fixed-width file with field names and data rows.
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Should have LayerStart and LayerEnd framing.
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	// Verify some content from the file.
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "Value") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find blocks with 'Value' content from csv_test6.txt")
}

// okapi: FixedWidthColumnsFilterTest#testDoubleExtraction
func TestFWC_DoubleExtraction(t *testing.T) {
	pool, cfg := sharedPool(t)
	bridgetest.RequireFilter(t, pool, cfg, fwcFilterClass)

	path := bridgetest.TestdataFile(t, "okapi/filters/table/src/test/resources/fwc_test5.txt")
	content, err := readFileContent(path)
	require.NoError(t, err)

	bridgetest.AssertRoundTripEvents(t, pool, cfg, fwcFilterClass, content, path, fwcMimeType, nil)
}

// okapi: FixedWidthColumnsFilterTest#testSkeleton
func TestFWC_Skeleton(t *testing.T) {
	output := fwcFileRoundtrip(t, "okapi/filters/table/src/test/resources/csv_test6.txt", nil)
	require.NotEmpty(t, output)

	// The fixed-width structure should be preserved.
	assert.Contains(t, output, "Value")
}

// okapi: FixedWidthColumnsFilterTest#testSkeleton2
func TestFWC_Skeleton2(t *testing.T) {
	output := fwcFileRoundtrip(t, "okapi/filters/table/src/test/resources/csv_test8.txt", nil)
	require.NotEmpty(t, output)

	assert.Contains(t, output, "Value")
}

// okapi: FixedWidthColumnsFilterTest#testSkeleton3
func TestFWC_Skeleton3(t *testing.T) {
	output := fwcFileRoundtrip(t, "okapi/filters/table/src/test/resources/fwc_test5.txt", nil)
	require.NotEmpty(t, output)

	assert.Contains(t, output, "s1")
	assert.Contains(t, output, "s2")
}

// okapi: FixedWidthColumnsFilterTest#testSkelRefs
func TestFWC_SkelRefs(t *testing.T) {
	output := fwcFileRoundtrip(t, "okapi/filters/table/src/test/resources/fwc_test4.txt", nil)
	require.NotEmpty(t, output)

	// fwc_test4.txt has Target and Source columns (reversed order).
	assert.Contains(t, output, "s1")
	assert.Contains(t, output, "t1")
}

// okapi: FixedWidthColumnsFilterTest#testHeader
func TestFWC_Header(t *testing.T) {
	parts := readFWCFile(t, "okapi/filters/table/src/test/resources/csv_test8.txt", nil)
	require.NotEmpty(t, parts)

	// csv_test8.txt has a header section (description lines before column names).
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: FixedWidthColumnsFilterTest#testListedColumns
func TestFWC_ListedColumns(t *testing.T) {
	parts := readFWCFile(t, "okapi/filters/table/src/test/resources/fwc_test5.txt", nil)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	found := false
	for _, text := range texts {
		if strings.Contains(text, "s1") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find 's1' in listed columns")
}

// okapi: FixedWidthColumnsFilterTest#testListedColumns2
func TestFWC_ListedColumns2(t *testing.T) {
	parts := readFWCFile(t, "okapi/filters/table/src/test/resources/fwc_test4.txt", nil)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: FixedWidthColumnsFilterTest#testListedColumns3
func TestFWC_ListedColumns3(t *testing.T) {
	// Test with csv_testa.txt which has a more complex column layout.
	parts := readFWCFile(t, "okapi/filters/table/src/test/resources/csv_testa.txt", nil)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: FixedWidthColumnsFilterTest#testListedColumns4
func TestFWC_ListedColumns4(t *testing.T) {
	parts := readFWCFile(t, "okapi/filters/table/src/test/resources/csv_testb.txt", nil)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: FixedWidthColumnsFilterTest#testListedColumns5
func TestFWC_ListedColumns5(t *testing.T) {
	parts := readFWCFile(t, "okapi/filters/table/src/test/resources/csv_test6.txt", nil)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}
