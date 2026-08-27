package main

import (
	"archive/zip"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSpreadsheetTruthCountsCellsNotTheStringTable.
//
// The first version read `xl/sharedStrings.xml` and called it ground truth. The
// table holds each distinct string once and the sheets refer to it by index, so
// a string in five hundred cells appeared once in the truth and five hundred
// times in the output. Recall is min(output, truth)/truth, so the undercount
// made the score easier — and both converters returned exactly 100.0% on .xlsx,
// which is what a metric that cannot fail looks like.
//
// Over this corpus the table holds 158,757 strings and the sheets refer to them
// 405,222 times. This test asserts the direction on one real workbook: the cells
// must yield more text than the table does.
func TestSpreadsheetTruthCountsCellsNotTheStringTable(t *testing.T) {
	root, err := repoRoot()
	require.NoError(t, err)
	// A workbook that reuses strings. Skipped rather than failed when the parity
	// corpus has not been fetched on this machine.
	f := filepath.Join(root, ".parity/okapi-testdata/1.48.0/integration-tests/okapi/src/test/resources/large.xlsx")
	if _, err := zip.OpenReader(f); err != nil {
		t.Skip("parity corpus not fetched: run `make parity-fetch`")
	}

	cells, err := xlsxCellText(f)
	require.NoError(t, err)
	require.NotEmpty(t, cells)

	table, err := textNodesFromPart(f, "xl/sharedStrings.xml", "t")
	require.NoError(t, err)
	require.NotEmpty(t, table)

	assert.Greater(t, len(cells), len(table),
		"the cells reuse the table's strings, so reading the table alone undercounts the document's text")
}

// TestSpreadsheetTruthResolvesSharedStrings: a cell of type `s` holds an index,
// and a truth that kept the index would be comparing numbers to words.
func TestSpreadsheetTruthResolvesSharedStrings(t *testing.T) {
	root, err := repoRoot()
	require.NoError(t, err)
	f := filepath.Join(root, ".parity/okapi-testdata/1.48.0/integration-tests/okapi/src/test/resources/sampleMore.xlsx")
	if _, err := zip.OpenReader(f); err != nil {
		t.Skip("parity corpus not fetched")
	}
	cells, err := xlsxCellText(f)
	require.NoError(t, err)
	require.NotEmpty(t, cells)

	letters := 0
	for _, c := range cells {
		if len(words(c)) > 0 {
			letters++
		}
	}
	assert.Positive(t, letters, "every cell resolved to a bare number, so the index was never looked up")
}

// textNodesFromPart is the old, single-part reader, kept here so the test can
// compare against what it produced.
func textNodesFromPart(file, part, element string) ([]string, error) {
	z, err := zip.OpenReader(file)
	if err != nil {
		return nil, err
	}
	defer z.Close()
	for _, f := range z.File {
		if f.Name != part {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		defer rc.Close()
		return textNodes(rc, element)
	}
	return nil, nil
}
