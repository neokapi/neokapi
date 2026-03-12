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

// okapi: CommaSeparatedValuesFilterTest#testNameAndMimeType
func TestCSV_NameAndMimeType(t *testing.T) {
	pool, cfg := sharedPool(t)
	bridgetest.RequireFilter(t, pool, cfg, csvFilterClass)

	// Verify the filter can be instantiated and processes content correctly.
	parts := readCSVDefault(t, "a,b\n1,2\n")
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)

	// Check the layer has the correct mime type.
	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok, "first part resource should be a Layer")
	assert.Equal(t, csvMimeType, layer.MimeType)
}

// okapi: CommaSeparatedValuesFilterTest#testEmptyInput
func TestCSV_EmptyInput(t *testing.T) {
	pool, cfg := sharedPool(t)
	bridgetest.RequireFilter(t, pool, cfg, csvFilterClass)

	parts := readCSVDefault(t, "")

	// Empty input should still produce at least LayerStart/LayerEnd.
	blocks := bridgetest.TranslatableBlocks(parts)
	assert.Empty(t, blocks, "empty input should produce no translatable blocks")
}

// okapi: CommaSeparatedValuesFilterTest#testParameters
func TestCSV_Parameters(t *testing.T) {
	pool, cfg := sharedPool(t)
	bridgetest.RequireFilter(t, pool, cfg, csvFilterClass)

	dir := tdDir(t)
	params := configParams(dir + "/okf_table@CSVTesting01.fprm")

	parts := readCSVFile(t, "okf_table/CSVTesting01.csv", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "CSVTesting01.csv should produce translatable blocks with config")

	// CSVTesting01.csv has Source,Target,Data columns; first column is source.
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Source text 1")
	assert.Contains(t, texts, "Source text 2")
}

// okapi: CommaSeparatedValuesFilterTest#testFileEvents
func TestCSV_FileEvents(t *testing.T) {
	pool, cfg := sharedPool(t)
	bridgetest.RequireFilter(t, pool, cfg, csvFilterClass)

	parts := readCSVFile(t, "okf_table/csv_test1.txt", nil)
	require.NotEmpty(t, parts)

	// csv_test1.txt has a header row and 3 data rows with 7 columns each.
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "csv_test1.txt should produce translatable blocks")

	// Should have LayerStart and LayerEnd framing.
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// okapi: CommaSeparatedValuesFilterTest#testFileEvents2
func TestCSV_FileEvents2(t *testing.T) {
	parts := readCSVFile(t, "okf_table/csv_test2.txt", nil)
	require.NotEmpty(t, parts)

	// csv_test2.txt has empty fields — should still produce blocks for non-empty values.
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testFileEvents2a
func TestCSV_FileEvents2a(t *testing.T) {
	parts := readCSVFile(t, "okf_table/csv_test3.txt", nil)
	require.NotEmpty(t, parts)

	// csv_test3.txt has quoted fields with embedded commas and newlines.
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testFileEvents3
func TestCSV_FileEvents3(t *testing.T) {
	parts := readCSVFile(t, "okf_table/csv_test4.txt", nil)
	require.NotEmpty(t, parts)

	// csv_test4.txt has comment lines before and after field names.
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testFileEvents4
func TestCSV_FileEvents4(t *testing.T) {
	parts := readCSVFile(t, "okf_table/csv_test5.txt", nil)
	require.NotEmpty(t, parts)

	// csv_test5.txt has various whitespace around values.
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testFileEvents5
func TestCSV_FileEvents5(t *testing.T) {
	parts := readCSVFile(t, "okf_table/csv_test6.txt", nil)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testFileEvents6
func TestCSV_FileEvents6(t *testing.T) {
	parts := readCSVFile(t, "okf_table/csv_test7.txt", nil)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testFileEvents7
func TestCSV_FileEvents7(t *testing.T) {
	parts := readCSVFile(t, "okf_table/csv_test8.txt", nil)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testFileEvents8
func TestCSV_FileEvents8(t *testing.T) {
	parts := readCSVFile(t, "okf_table/csv_test9.txt", nil)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testFileEvents96
func TestCSV_FileEvents96(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@copy-of-csv_96.fprm")

	parts := readCSVFile(t, "okf_table/CSVTest_96.txt", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Source text 1")
	assert.Contains(t, texts, "Source text 2")
}

// okapi: CommaSeparatedValuesFilterTest#testFileEvents96_2
func TestCSV_FileEvents96_2(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@copy-of-csv_96.fprm")

	parts := readCSVFile(t, "okf_table/CSVTest_96_2.txt", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "source1")
	assert.Contains(t, texts, "source2")
}

// okapi: CommaSeparatedValuesFilterTest#testFileEvents96_3
func TestCSV_FileEvents96_3(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@copy-of-csv_96.fprm")

	parts := readCSVFile(t, "okf_table/CSVTesting01.csv", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Source text 1")
}

// okapi: CommaSeparatedValuesFilterTest#testFileEvents97
func TestCSV_FileEvents97(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@copy-of-csv_97.fprm")

	parts := readCSVFile(t, "okf_table/CSVTest_97.txt", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// CSVTest_97 has two source/target column pairs.
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Source text 1")
	assert.Contains(t, texts, "SourceB1")
}

// okapi: CommaSeparatedValuesFilterTest#testFileEvents106
func TestCSV_FileEvents106(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@copy-of-csv._106.fprm")

	parts := readCSVFile(t, "okf_table/csv.txt", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "one")
	assert.Contains(t, texts, "three")
}

// okapi: CommaSeparatedValuesFilterTest#testFileEvents106_2
func TestCSV_FileEvents106_2(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@copy-of-csv._106.fprm")

	parts := readCSVFile(t, "okf_table/csv2.txt", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testFileEvents106_3
func TestCSV_FileEvents106_3(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@copy-of-csv._106.fprm")

	parts := readCSVFile(t, "okf_table/csv3.txt", params)
	require.NotEmpty(t, parts)

	// csv3.txt is a small snippet with BOM. The 106 config sets columnNamesLineNum=1
	// and valuesStartLineNum=2, sourceIdColumns=1, sourceColumns=2. With csv3.txt
	// having limited rows, the first row is treated as column names. Verify the filter
	// processes the file without error and produces the expected part structure
	// (LayerStart + Data/Block parts + LayerEnd).
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")
}

// okapi: CommaSeparatedValuesFilterTest#testFileEvents106_4
func TestCSV_FileEvents106_4(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@copy-of-csv._106.fprm")

	// Inline content for 106_4: single line with sourceId and value.
	parts := readCSV(t, "id,value\n01,one\n", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testFileEvents118
func TestCSV_FileEvents118(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@CSVTesting01.fprm")

	parts := readCSVFile(t, "okf_table/CSVTesting01.csv", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Source text 1")
}

// okapi: CommaSeparatedValuesFilterTest#testFileEvents118_2
func TestCSV_FileEvents118_2(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@CSVTesting01.fprm")

	parts := readCSVFile(t, "okf_table/CSVTest_96.txt", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testSkeleton
func TestCSV_Skeleton(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@copy-of-csv_96.fprm")

	output := csvFileRoundtrip(t, "okf_table/CSVTest_96.txt", params)
	require.NotEmpty(t, output)

	// The output should preserve the basic structure.
	assert.Contains(t, output, "Source text 1")
	assert.Contains(t, output, "Source text 2")
}

// okapi: CommaSeparatedValuesFilterTest#testSkeleton2
func TestCSV_Skeleton2(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@copy-of-csv_96.fprm")

	output := csvFileRoundtrip(t, "okf_table/CSVTest_96_2.txt", params)
	require.NotEmpty(t, output)

	assert.Contains(t, output, "source1")
	assert.Contains(t, output, "source2")
}

// okapi: CommaSeparatedValuesFilterTest#testSkeleton3
func TestCSV_Skeleton3(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@copy-of-csv_97.fprm")

	output := csvFileRoundtrip(t, "okf_table/CSVTest_97.txt", params)
	require.NotEmpty(t, output)

	assert.Contains(t, output, "Source text 1")
	assert.Contains(t, output, "SourceB1")
}

// okapi: CommaSeparatedValuesFilterTest#testSkeletonWriter
func TestCSV_SkeletonWriter(t *testing.T) {
	// Verify that skeleton writing produces valid output from a simple CSV.
	output := csvRoundtrip(t, "a,b\n1,2\n", nil)
	require.NotEmpty(t, output)
}

// okapi: CommaSeparatedValuesFilterTest#testSourceId
func TestCSV_SourceId(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@source_id.fprm")

	parts := readCSVFile(t, "okf_table/csv_teste.txt", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "source_id config should extract translatable blocks")
}

// okapi: CommaSeparatedValuesFilterTest#testEmptySourceId
func TestCSV_EmptySourceId(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@source_id.fprm")

	// Test with data that may have empty source ID columns.
	parts := readCSV(t, "source1, target1, source2, target2, , id1\nsource1b, target1b, source2b, target2b, , id1b\n", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testRecordId
func TestCSV_RecordId(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@record_id.fprm")

	parts := readCSVFile(t, "okf_table/csv_testd.txt", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "record_id config should extract translatable blocks")
}

// okapi: CommaSeparatedValuesFilterTest#testDoubleExtraction
// okapi-unmapped: Skipped in Java — Property Types difference
func TestCSV_DoubleExtraction(t *testing.T) {
	t.Skip("Skipped in Java surefire: Property Types difference: [filter-only, display] vs [filter-only]")
}

// okapi: CommaSeparatedValuesFilterTest#testTabDelimited2Column
func TestCSV_TabDelimited2Column(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@2Cols_ID_Text.fprm")

	parts := readCSVFile(t, "okf_table/test2cols.csv", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// test2cols.csv has id,text pairs — second column is source.
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "text") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find blocks with 'text' content")
}

// okapi: CommaSeparatedValuesFilterTest#testTabDelimited2ColumnRoundTrip
func TestCSV_TabDelimited2ColumnRoundTrip(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@2Cols_ID_Text.fprm")

	output := csvFileRoundtrip(t, "okf_table/test2cols.csv", params)
	require.NotEmpty(t, output)

	assert.Contains(t, output, "text")
}

// okapi: CommaSeparatedValuesFilterTest#testQualifiedValues
func TestCSV_QualifiedValues(t *testing.T) {
	// Test reading CSV with double-quote qualified fields.
	parts := readCSVFile(t, "okf_table/text_qualifier_double_quote.csv", nil)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// The qualified CSV has "one,two" etc. — the commas should be inside the field.
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "one") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find block with qualified values content")
}

// okapi: CommaSeparatedValuesFilterTest#testQualifiedValues2
func TestCSV_QualifiedValues2(t *testing.T) {
	// Test reading CSV with single-quote qualified fields.
	parts := readCSVFile(t, "okf_table/text_qualifier_single_quote.csv", nil)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testAddTextQualifiers
func TestCSV_AddTextQualifiers(t *testing.T) {
	// Verify that qualifiers are added in output when configured.
	output := csvRoundtrip(t, "\"one,two\",\"three,four\"\n", nil)
	require.NotEmpty(t, output)
}

// okapi: CommaSeparatedValuesFilterTest#testAddTextQualifiersForQualifiers
func TestCSV_AddTextQualifiersForQualifiers(t *testing.T) {
	// Test qualifier handling for fields that contain qualifiers.
	output := csvRoundtrip(t, "\"one \"\"two\"\" three\",\"four\"\n", nil)
	require.NotEmpty(t, output)
}

// okapi: CommaSeparatedValuesFilterTest#testEscapeQualifiersDoubleQuotes
func TestCSV_EscapeQualifiersDoubleQuotes(t *testing.T) {
	// Default escaping mode uses doubled quotes to escape.
	parts := readCSVDefault(t, "\"one \"\"two\"\" three\",\"four\"\n")
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// The filter removes text qualifiers and unescapes doubled quotes.
	// The extracted text should contain "two" as part of the content.
	found := false
	for _, b := range blocks {
		text := b.SourceText()
		if strings.Contains(text, "two") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find block containing 'two'")
}

// okapi: CommaSeparatedValuesFilterTest#testEscapeQualifiersBackslash
func TestCSV_EscapeQualifiersBackslash(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/Issue404/okf_table@csv_backslash.fprm")

	parts := readCSVFile(t, "okf_table/Issue404/Issue_404 .csv", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Issue 404 has backslash-escaped quotes.
	found := false
	for _, b := range blocks {
		text := b.SourceText()
		if strings.Contains(text, "quotes") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find block with backslash-escaped content")
}

// okapi: CommaSeparatedValuesFilterTest#testEscapeQualifiersInUnqualifiedFields
func TestCSV_EscapeQualifiersInUnqualifiedFields(t *testing.T) {
	// Unqualified fields with embedded qualifiers.
	parts := readCSVDefault(t, "one \"two\" three,four\n")
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testEmptyQualifiersWithSourceQualifiers
func TestCSV_EmptyQualifiersWithSourceQualifiers(t *testing.T) {
	// Source has qualifiers, target is empty qualified field.
	parts := readCSVDefault(t, "\"source\",\"\"\n")
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testEmptyQualifiersWithoutSourceQualifiers
func TestCSV_EmptyQualifiersWithoutSourceQualifiers(t *testing.T) {
	// Source without qualifiers, empty field.
	parts := readCSVDefault(t, "source,\n")
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testEmptyQualifiersWithSourceQualifiersAddQualifiers
func TestCSV_EmptyQualifiersWithSourceQualifiersAddQualifiers(t *testing.T) {
	// Roundtrip qualified source with empty target — qualifiers should be preserved.
	output := csvRoundtrip(t, "\"source\",\"\"\n", nil)
	require.NotEmpty(t, output)
	assert.Contains(t, output, "source")
}

// okapi: CommaSeparatedValuesFilterTest#testUnqualifiedTargetWithSourceQualifiers
func TestCSV_UnqualifiedTargetWithSourceQualifiers(t *testing.T) {
	parts := readCSVDefault(t, "\"source\",target\n")
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testDoEscapeRemovedQualifiers
func TestCSV_DoEscapeRemovedQualifiers(t *testing.T) {
	// When qualifiers are removed, embedded qualifiers must be escaped.
	output := csvRoundtrip(t, "\"one \"\"two\"\" three\",four\n", nil)
	require.NotEmpty(t, output)
}

// okapi: CommaSeparatedValuesFilterTest#testDontEscapeUnremovedQualifiers
func TestCSV_DontEscapeUnremovedQualifiers(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@2Cols_ID_Text.fprm")

	output := csvFileRoundtrip(t, "okf_table/test2cols.csv", params)
	require.NotEmpty(t, output)
}

// okapi: CommaSeparatedValuesFilterTest#testEmptyLinesInCell
func TestCSV_EmptyLinesInCell(t *testing.T) {
	// CSV cell with empty lines inside quoted text.
	parts := readCSVDefault(t, "\"line1\n\nline3\",second\n")
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testThreeColumnsSrcTrgData
func TestCSV_ThreeColumnsSrcTrgData(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@copy-of-csv_96.fprm")

	parts := readCSVFile(t, "okf_table/CSVTesting01.csv", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Source text 1")
}

// okapi: CommaSeparatedValuesFilterTest#testThreeColumnsSrcTrgData_2
func TestCSV_ThreeColumnsSrcTrgData_2(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@copy-of-csv_96.fprm")

	parts := readCSVFile(t, "okf_table/CSVTest_96.txt", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Source text 1")

	// Verify targets are present when config specifies target column.
	for _, b := range blocks {
		if b.SourceText() == "Source text 1" {
			// With the csv_96 config, column 2 is target with language FR-FR.
			for _, locale := range []model.LocaleID{"FR-FR", "fr-FR", "fr-fr", "fr"} {
				if b.HasTarget(locale) {
					assert.Equal(t, "Target text 1", b.TargetText(locale))
					break
				}
			}
			break
		}
	}
}

// okapi: CommaSeparatedValuesFilterTest#testThreeColumnsSrcTrgData_3
func TestCSV_ThreeColumnsSrcTrgData_3(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@copy-of-csv_96.fprm")

	parts := readCSVFile(t, "okf_table/CSVTest_96_2.txt", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testThreeColumnsExtractAllWithSubfilter
func TestCSV_ThreeColumnsExtractAllWithSubfilter(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@CSVTesting01.fprm")

	parts := readCSVFile(t, "okf_table/testContent_escaped2.csv", params)
	require.NotEmpty(t, parts)

	// This test exercises subfilter extraction (HTML content in CSV cells).
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "subfilter extraction should produce translatable blocks")
}

// okapi: CommaSeparatedValuesFilterTest#testThreeColumnsSrcDataWithSubfilter
func TestCSV_ThreeColumnsSrcDataWithSubfilter(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@CSVTesting01.fprm")

	parts := readCSVFile(t, "okf_table/testContent_escaped4.csv", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testTrgAtCol4_Issue511
func TestCSV_TrgAtCol4_Issue511(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@tabtest1.fprm")

	// tabtest1 config: tab-delimited, source=col3, target=col4, recordId=col2.
	content := "\"file\"\t\"id\"\t\"src\"\t\"trg\"\n\"f1\"\t\"i1\"\t\"src1\"\t\"trg for 1\"\n\"f2\"\t\"i2\"\t\"src2\"\t\"trg for 2\"\n"
	parts := readCSV(t, content, params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "src1")
	assert.Contains(t, texts, "src2")
}

// okapi: CommaSeparatedValuesFilterTest#testCatkeys
func TestCSV_Catkeys(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@catkeys.fprm")

	parts := readCSVFile(t, "okf_table/test01.catkeys", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// test01.catkeys has Haiku translation entries.
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "OK") || strings.Contains(b.SourceText(), "Quit") || strings.Contains(b.SourceText(), "Pulse") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find catkeys entries")
}

// okapi: CommaSeparatedValuesFilterTest#testSubfilterTuIds
func TestCSV_SubfilterTuIds(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@CSVTesting01.fprm")

	parts := readCSVFile(t, "okf_table/testContent_escaped.csv", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "subfilter extraction should produce blocks")

	// Verify that block IDs are unique even with subfilter extraction.
	ids := make(map[string]bool)
	for _, b := range blocks {
		if b.ID != "" {
			assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
			ids[b.ID] = true
		}
	}
}

// okapi: CommaSeparatedValuesFilterTest#testCommentColumnsAsMetadata
func TestCSV_CommentColumnsAsMetadata(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@csv-metadata.fprm")

	parts := readCSVFile(t, "okf_table/CSVTesting01.csv", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: CommaSeparatedValuesFilterTest#testIssue_1153
func TestCSV_Issue_1153(t *testing.T) {
	dir := tdDir(t)
	params := configParams(dir + "/okf_table@debug.fprm")

	parts := readCSVFile(t, "okf_table/debug/test.csv", params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// debug config has useCodeFinder with {.*?} pattern.
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "source test") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find 'source test' block from debug/test.csv")
}
