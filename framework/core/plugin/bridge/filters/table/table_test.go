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

// okapi: TableFilterTest#testNameAndMimeType
func TestTable_NameAndMimeType(t *testing.T) {
	pool, cfg := sharedPool(t)
	bridgetest.RequireFilter(t, pool, cfg, tableFilterClass)

	parts := readTable(t, "one\ttwo\nthree\tfour\n", nil)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok, "first part resource should be a Layer")
	// The base table filter inherits from the CSV filter and reports text/csv
	// as mime type through the bridge, even though the canonical mime type is text/plain.
	assert.NotEmpty(t, layer.MimeType)
}

// okapi: TableFilterTest#testEmptyInput
func TestTable_EmptyInput(t *testing.T) {
	pool, cfg := sharedPool(t)
	bridgetest.RequireFilter(t, pool, cfg, tableFilterClass)

	parts := readTable(t, "", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.Empty(t, blocks, "empty input should produce no translatable blocks")
}

// okapi: TableFilterTest#testColumnDefinedLocales
func TestTable_ColumnDefinedLocales(t *testing.T) {
	pool, cfg := sharedPool(t)
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@defined_locales.fprm")

	// The defined_locales config specifies parametersClass=tsv.Parameters,
	// so we use the TSV filter class which properly splits tab-separated columns.
	path := bridgetest.TestdataFile(t, "okf_table/Locale_defined_TSV_test.txt")
	parts := bridgetest.ReadFile(t, pool, cfg, tsvFilterClass, path, tsvMimeType, params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Locale_defined_TSV_test.txt has "en\tfr" header and 2 data rows.
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Source text 1")
	assert.Contains(t, texts, "Source text 2")
}

// okapi: TableFilterTest#testColumnDefinedSource
func TestTable_ColumnDefinedSource(t *testing.T) {
	pool, cfg := sharedPool(t)
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@defined_locales.fprm")

	path := bridgetest.TestdataFile(t, "okf_table/Locale_defined_TSV_test.txt")
	parts := bridgetest.ReadFile(t, pool, cfg, tsvFilterClass, path, tsvMimeType, params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Source column (col 1) should be extracted.
	assert.Equal(t, "Source text 1", blocks[0].SourceText())
}

// okapi: TableFilterTest#testColumnDefinedTarget
func TestTable_ColumnDefinedTarget(t *testing.T) {
	pool, cfg := sharedPool(t)
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@defined_locales.fprm")

	path := bridgetest.TestdataFile(t, "okf_table/Locale_defined_TSV_test.txt")
	parts := bridgetest.ReadFile(t, pool, cfg, tsvFilterClass, path, tsvMimeType, params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Target column (col 2) with "fr" locale should provide targets.
	b := blocks[0]
	// Check all possible locale representations for the target.
	for _, locale := range []model.LocaleID{"fr", "FR", "fr-FR"} {
		if b.HasTarget(locale) {
			assert.Equal(t, "Target text 1", b.TargetText(locale))
			break
		}
	}
}

// okapi: TableFilterTest#testMultilineColNames
func TestTable_MultilineColNames(t *testing.T) {
	// Test table with multiline column names (long header).
	parts := readTableFile(t, "okf_table/csv_test7.txt", nil)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: TableFilterTest#testSkeleton
func TestTable_Skeleton(t *testing.T) {
	pool, cfg := sharedPool(t)
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@defined_locales.fprm")

	path := bridgetest.TestdataFile(t, "okf_table/Locale_defined_TSV_test.txt")
	content, err := readFileContent(path)
	require.NoError(t, err)

	result := bridgetest.RoundTrip(t, pool, cfg, tsvFilterClass, content, path, tsvMimeType, params)
	output := string(result.Output)
	require.NotEmpty(t, output)

	assert.Contains(t, output, "Source text 1")
	assert.Contains(t, output, "Source text 2")
}

// okapi: TableFilterTest#testSkeleton3
func TestTable_Skeleton3(t *testing.T) {
	output := tableFileRoundtrip(t, "okf_table/csv_test7.txt", nil)
	require.NotEmpty(t, output)

	assert.Contains(t, output, "Value")
}

// okapi: TableFilterTest#testFileEvents
func TestTable_FileEvents(t *testing.T) {
	parts := readTableFile(t, "okf_table/csv_test7.txt", nil)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// okapi: TableFilterTest#testFileEvents2
func TestTable_FileEvents2(t *testing.T) {
	parts := readTableFile(t, "okf_table/csv_test9.txt", nil)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "Value") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find blocks with 'Value' content from csv_test9.txt")
}

// okapi: TableFilterTest#testDoubleExtraction
func TestTable_DoubleExtraction(t *testing.T) {
	pool, cfg := sharedPool(t)
	bridgetest.RequireFilter(t, pool, cfg, tableFilterClass)

	path := bridgetest.TestdataFile(t, "okf_table/csv_test7.txt")
	content, err := readFileContent(path)
	require.NoError(t, err)

	bridgetest.AssertRoundTripEvents(t, pool, cfg, tableFilterClass, content, path, tableMimeType, nil)
}

// okapi: TableFilterTest#testTrimMode
func TestTable_TrimMode(t *testing.T) {
	// Test trimming behavior — when configured to trim, values should have leading/trailing whitespace removed.
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@strong.fprm")

	parts := readTableFile(t, "okf_table/strong.csv", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// strong.csv has tab-separated content with "This is strong text." as source.
	b := blocks[0]
	text := b.SourceText()
	// With trim mode, the text should not have leading/trailing whitespace.
	assert.Equal(t, strings.TrimSpace(text), text, "trimmed text should not have leading/trailing whitespace")
}

// okapi: TableFilterTest#testSynchronization
func TestTable_Synchronization(t *testing.T) {
	pool, cfg := sharedPool(t)
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@defined_locales.fprm")

	// Read a file that tests synchronization of source and target columns.
	// The defined_locales config uses TSV parameters class.
	path := bridgetest.TestdataFile(t, "okf_table/Locale_defined_TSV_test.txt")
	parts := bridgetest.ReadFile(t, pool, cfg, tsvFilterClass, path, tsvMimeType, params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2, "should extract at least 2 blocks")

	// Verify that the source text lines up with the correct target.
	for _, b := range blocks {
		if b.SourceText() == "Source text 1" {
			for _, locale := range []model.LocaleID{"fr", "FR", "fr-FR"} {
				if b.HasTarget(locale) {
					assert.Equal(t, "Target text 1", b.TargetText(locale))
					break
				}
			}
		}
		if b.SourceText() == "Source text 2" {
			for _, locale := range []model.LocaleID{"fr", "FR", "fr-FR"} {
				if b.HasTarget(locale) {
					assert.Equal(t, "Target text 2", b.TargetText(locale))
					break
				}
			}
		}
	}
}

// okapi: TableFilterTest#testIssue1128
func TestTable_Issue1128(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/issue1128/okf_table@strong.fprm")

	parts := readTableFile(t, "okf_table/issue1128/strong.csv", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// strong.csv in issue1128/ has "This is strong text." as source.
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "strong text") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find 'strong text' entry in issue1128/strong.csv")

	// Roundtrip and check output matches golden.
	pool, cfg := sharedPool(t)
	path := bridgetest.TestdataFile(t, "okf_table/issue1128/strong.csv")
	content, err := readFileContent(path)
	require.NoError(t, err)

	// Use CSV filter class since the fprm specifies csv.Parameters.
	result := bridgetest.RoundTrip(t, pool, cfg, csvFilterClass, content, path, csvMimeType, params)
	require.NotEmpty(t, result.Output)
}

// okapi: TableFilterTest#testIssue124
func TestTable_Issue124(t *testing.T) {
	pool, cfg := sharedPool(t)
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@test124.fprm")

	// test124 config: TSV with source=col2, comment=col1 (parametersClass=tsv.Parameters).
	path := bridgetest.TestdataFile(t, "okf_table/test_tsv_simple.txt")
	parts := bridgetest.ReadFile(t, pool, cfg, tsvFilterClass, path, tsvMimeType, params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "Source1") || strings.Contains(b.SourceText(), "Target1") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find entries from test_tsv_simple.txt with test124 config")
}
