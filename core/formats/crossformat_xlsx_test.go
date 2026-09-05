package formats_test

import (
	"bytes"
	"context"
	"flag"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/formats/asciidoc"
	"github.com/neokapi/neokapi/core/formats/doclang"
	htmlfmt "github.com/neokapi/neokapi/core/formats/html"
	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/formats/openxml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/structure"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var updateGolden = flag.Bool("update", false, "rewrite the golden export files under testdata/")

// exportXLSX converts a workbook to a structural writer the way a cross-format
// export does: the worksheet's cell geometry becomes table groups
// (structure.SpreadsheetGridToTables, as the flow runner applies it) and the
// writer renders the stream on its no-skeleton path.
func exportXLSX(t *testing.T, w xfWriter, workbook []byte) string {
	t.Helper()
	ctx := context.Background()
	r := openxml.NewReader()
	require.NoError(t, r.Open(ctx, testutil.RawDocFromReader(bytes.NewReader(workbook), "numfmt.xlsx", model.LocaleEnglish)))
	parts := testutil.CollectParts(t, r.Read(ctx))
	require.NoError(t, r.Close())

	counter := 0
	parts = structure.SpreadsheetGridToTables(parts, &counter)

	var buf bytes.Buffer
	require.NoError(t, w.SetOutputWriter(&buf))
	require.NoError(t, w.Write(ctx, testutil.PartsToChannel(parts)))
	return buf.String()
}

// assertGolden compares an export against its committed golden file, or
// rewrites the file under -update.
func assertGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *updateGolden {
		require.NoError(t, os.WriteFile(path, []byte(got), 0o644))
		return
	}
	want, err := os.ReadFile(path)
	require.NoError(t, err, "golden %s is missing; run with -update to write it", path)
	assert.Equal(t, string(want), got, "export differs from %s; run with -update if the change is intended", path)
}

// A spreadsheet exported to a structural format reads the way the sheet
// does: a date cell shows its date, a share its percentage, a price its
// currency, and a boolean TRUE or FALSE, while the stored serials and
// fractions never reach the output.
func TestCrossFormat_SpreadsheetCellsShowTheirNumberFormat(t *testing.T) {
	workbook := testutil.XLSXNumberFormats(t, false)

	md := exportXLSX(t, markdown.NewWriter(), workbook)
	assertGolden(t, "xlsx-number-formats.md", md)
	html := exportXLSX(t, htmlfmt.NewWriter(), workbook)
	assertGolden(t, "xlsx-number-formats.html", html)

	outputs := map[string]string{
		"markdown": md,
		"html":     html,
		"asciidoc": exportXLSX(t, asciidoc.NewWriter(), workbook),
		"doclang":  exportXLSX(t, doclang.NewWriter(), workbook),
	}
	for name, out := range outputs {
		for _, shown := range []string{"01-01-21", "12.5%", "$1,234.50", "($1,234.50)", "1,234,567", "TRUE", "FALSE", "1900-02-29", "hello"} {
			assert.Contains(t, out, shown, "%s export shows %q", name, shown)
		}
		for _, stored := range []string{"44197", "0.125", "1234567", ">60<", "| 60 |"} {
			assert.NotContains(t, out, stored, "%s export never shows the stored value %q", name, stored)
		}
	}
}
