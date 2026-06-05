package openxml

// okapi-filter: openxml

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestValidity_XMLWellFormed verifies that every XML entry in the roundtrip
// output is well-formed XML. This catches undeclared namespace prefixes,
// malformed escaping, and structural errors.
func TestValidity_XMLWellFormed(t *testing.T) {
	dir := testdataDir(t)
	files := collectTestFiles(t, dir, "*.xlsx", "*.docx", "*.pptx")

	// Also include local testdata
	local, _ := os.ReadDir("testdata")
	for _, e := range local {
		if !e.IsDir() {
			files = append(files, "testdata/"+e.Name())
		}
	}

	for _, f := range files {
		name := f
		t.Run(name, func(t *testing.T) {
			original, err := os.ReadFile(f)
			require.NoError(t, err)

			output := skeletonRoundtripBytes(t, original, name)
			assertAllXMLWellFormed(t, output)
		})
	}
}

// TestValidity_NamespacePrefixes verifies that every element and attribute
// in the output XML uses only declared namespace prefixes. Go's xml.Decoder
// won't catch this since it works with parsed namespace URIs.
func TestValidity_NamespacePrefixes(t *testing.T) {
	files := []string{"testdata/EksempelFiltrering.xlsx"}
	addIfExists(t, &files, "testdata/simple.docx", "testdata/formatted.docx")

	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			original, err := os.ReadFile(f)
			require.NoError(t, err)

			output := skeletonRoundtripBytes(t, original, f)
			assertNoPrefixViolations(t, output)
		})
	}
}

// TestValidity_XlsxFormulaPreservation verifies that formulas survive roundtrip
// without content modification (no extra escaping, no dropped elements).
// This compares raw XML bytes, not parsed content, because Excel's formula parser
// is sensitive to escaping differences (e.g., &quot; vs ") that XML parsers normalize.
func TestValidity_XlsxFormulaPreservation(t *testing.T) {
	original, err := os.ReadFile("testdata/EksempelFiltrering.xlsx")
	require.NoError(t, err)

	output := skeletonRoundtripBytes(t, original, "EksempelFiltrering.xlsx")

	// Extract raw <f>...</f> elements as byte strings (not parsed XML)
	origFormulas := extractRawFormulaElements(t, original)
	outFormulas := extractRawFormulaElements(t, output)

	require.Len(t, outFormulas, len(origFormulas),
		"formula element count should be preserved")

	for i, orig := range origFormulas {
		assert.Equal(t, orig, outFormulas[i],
			"formula[%d] raw XML should be preserved exactly", i)
	}
}

// TestValidity_XlsxTableColumnSync verifies that after translation, table
// column names match the corresponding shared string values referenced by
// header row cells. A mismatch here causes Excel to report corruption.
func TestValidity_XlsxTableColumnSync(t *testing.T) {
	original, err := os.ReadFile("testdata/EksempelFiltrering.xlsx")
	require.NoError(t, err)

	output := translateRoundtripBytes(t, original, "EksempelFiltrering.xlsx")

	zr, err := zip.NewReader(bytes.NewReader(output), int64(len(output)))
	require.NoError(t, err)

	// Parse shared strings
	sharedStrings := parseSharedStringTable(t, zr)

	// Parse table column names
	tableColumns := parseTableColumnNames(t, zr)
	require.NotEmpty(t, tableColumns, "should find table columns")

	// Parse header row cell references from sheet1
	headerRefs := parseHeaderRowSharedStringRefs(t, zr, "xl/worksheets/sheet1.xml")
	require.NotEmpty(t, headerRefs, "should find header row cells")

	// Each table column name must match the shared string it references
	for i, colName := range tableColumns {
		if i >= len(headerRefs) {
			break
		}
		ref := headerRefs[i]
		require.Less(t, ref, len(sharedStrings),
			"shared string ref %d out of range (table has %d strings)", ref, len(sharedStrings))
		assert.Equal(t, sharedStrings[ref], colName,
			"table column[%d] name %q must match shared string[%d] %q",
			i, colName, ref, sharedStrings[ref])
	}
}

// TestValidity_UntouchedPartsIdentical verifies that ZIP entries not containing
// translatable content are byte-identical to the original. The writer should
// copy these through without modification.
func TestValidity_UntouchedPartsIdentical(t *testing.T) {
	original, err := os.ReadFile("testdata/EksempelFiltrering.xlsx")
	require.NoError(t, err)

	output := skeletonRoundtripBytes(t, original, "EksempelFiltrering.xlsx")

	origZr, err := zip.NewReader(bytes.NewReader(original), int64(len(original)))
	require.NoError(t, err)
	outZr, err := zip.NewReader(bytes.NewReader(output), int64(len(output)))
	require.NoError(t, err)

	origFiles := zipEntryMap(origZr)
	outFiles := zipEntryMap(outZr)

	// These parts should not be modified during roundtrip
	untouched := []string{
		"[Content_Types].xml",
		"_rels/.rels",
		"xl/_rels/workbook.xml.rels",
		"xl/workbook.xml",
		"xl/styles.xml",
		"xl/theme/theme1.xml",
		"xl/calcChain.xml",
	}

	for _, name := range untouched {
		orig, ok1 := origFiles[name]
		out, ok2 := outFiles[name]
		if !ok1 || !ok2 {
			continue // skip if not present
		}
		assert.Equal(t, orig, out,
			"untouched part %s should be byte-identical", name)
	}
}

// --- helpers ---

// skeletonRoundtripBytes does a read→write roundtrip and returns the output bytes.
func skeletonRoundtripBytes(t *testing.T, original []byte, uri string) []byte {
	t.Helper()

	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := NewReader()
	reader.SetSkeletonStore(skelStore)
	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(original),
	}
	err = reader.Open(t.Context(), doc)
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	reader.Close()

	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(original)
	writer.SetSkeletonStore(skelStore)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(t.Context(), ch)
	require.NoError(t, err)
	writer.Close()

	require.Greater(t, buf.Len(), 0)
	return buf.Bytes()
}

// translateRoundtripBytes does a read→translate→write roundtrip.
func translateRoundtripBytes(t *testing.T, original []byte, uri string) []byte {
	t.Helper()

	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := NewReader()
	reader.SetSkeletonStore(skelStore)
	doc := &model.RawDocument{
		URI:          uri,
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(original),
	}
	err = reader.Open(t.Context(), doc)
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	reader.Close()

	// Apply pseudo-translation to all blocks
	target := model.LocaleID("qps")
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok && b.Translatable {
				b.SetTargetText(target, "["+b.SourceText()+"]")
			}
		}
	}

	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(original)
	writer.SetSkeletonStore(skelStore)
	writer.SetLocale(target)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(t.Context(), ch)
	require.NoError(t, err)
	writer.Close()

	require.Greater(t, buf.Len(), 0)
	return buf.Bytes()
}

// assertAllXMLWellFormed checks that every .xml and .rels entry in a ZIP
// parses as well-formed XML.
func assertAllXMLWellFormed(t *testing.T, data []byte) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)

	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".xml") && !strings.HasSuffix(f.Name, ".rels") {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(rc)
		rc.Close()
		require.NoError(t, err)

		d := xml.NewDecoder(bytes.NewReader(content))
		for {
			_, err := d.Token()
			if errors.Is(err, io.EOF) {
				break
			}
			assert.NoErrorf(t, err, "XML parse error in %s", f.Name)
			if err != nil {
				break
			}
		}
	}
}

// prefixRe matches element/attribute names with explicit prefixes like "x:worksheet".
var prefixRe = regexp.MustCompile(`</?([a-zA-Z][\w]*):([a-zA-Z][\w]*)`)

// assertNoPrefixViolations checks that every namespace prefix used in element
// and attribute names is actually declared in an ancestor xmlns:prefix attribute.
func assertNoPrefixViolations(t *testing.T, data []byte) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)

	for _, f := range zr.File {
		if !strings.HasSuffix(f.Name, ".xml") {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(rc)
		rc.Close()
		require.NoError(t, err)

		xmlStr := string(content)

		// Collect all declared prefixes (xmlns:prefix="...")
		declared := map[string]bool{"xml": true} // xml is always declared
		declRe := regexp.MustCompile(`xmlns:([a-zA-Z][\w]*)=`)
		for _, m := range declRe.FindAllStringSubmatch(xmlStr, -1) {
			declared[m[1]] = true
		}

		// Find all used prefixes
		for _, m := range prefixRe.FindAllStringSubmatch(xmlStr, -1) {
			prefix := m[1]
			assert.Truef(t, declared[prefix],
				"undeclared namespace prefix %q used in %s", prefix, f.Name)
		}
	}
}

// formulaElementRe matches <f ...>...</f> elements in raw XML.
var formulaElementRe = regexp.MustCompile(`<f[ >].*?</f>`)

// extractRawFormulaElements returns raw <f>...</f> elements from worksheet XMLs.
// Uses raw byte matching rather than XML parsing to detect escaping differences
// that XML parsers would normalize away.
func extractRawFormulaElements(t *testing.T, data []byte) []string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	require.NoError(t, err)

	var formulas []string
	for _, f := range zr.File {
		if !strings.Contains(f.Name, "sheet") || !strings.HasSuffix(f.Name, ".xml") {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(rc)
		rc.Close()
		require.NoError(t, err)

		for _, m := range formulaElementRe.FindAll(content, -1) {
			formulas = append(formulas, string(m))
		}
	}
	return formulas
}

// parseSharedStringTable returns the shared string values from xl/sharedStrings.xml.
func parseSharedStringTable(t *testing.T, zr *zip.Reader) []string {
	t.Helper()
	table, err := parseSharedStrings(zr)
	require.NoError(t, err)
	return table
}

// parseTableColumnNames returns all <tableColumn name="..."> values from table XMLs.
func parseTableColumnNames(t *testing.T, zr *zip.Reader) []string {
	t.Helper()
	var names []string
	for _, f := range zr.File {
		if !strings.Contains(f.Name, "table") || !strings.HasSuffix(f.Name, ".xml") {
			continue
		}
		rc, err := f.Open()
		require.NoError(t, err)
		content, err := io.ReadAll(rc)
		rc.Close()
		require.NoError(t, err)

		d := xml.NewDecoder(bytes.NewReader(content))
		for {
			tok, err := d.Token()
			if errors.Is(err, io.EOF) {
				break
			}
			require.NoError(t, err)
			if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "tableColumn" {
				for _, a := range se.Attr {
					if a.Name.Local == "name" {
						names = append(names, a.Value)
					}
				}
			}
		}
	}
	return names
}

// parseHeaderRowSharedStringRefs returns the shared string indices from the
// first row of a worksheet (the header row for tables).
func parseHeaderRowSharedStringRefs(t *testing.T, zr *zip.Reader, sheetPath string) []int {
	t.Helper()
	f := zipFileByName(zr, sheetPath)
	if f == nil {
		return nil
	}

	rc, err := f.Open()
	require.NoError(t, err)
	content, err := io.ReadAll(rc)
	rc.Close()
	require.NoError(t, err)

	d := xml.NewDecoder(bytes.NewReader(content))
	var refs []int
	inRow1 := false
	cellIsSharedStr := false

	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)

		switch tt := tok.(type) {
		case xml.StartElement:
			switch tt.Name.Local {
			case "row":
				for _, a := range tt.Attr {
					if a.Name.Local == "r" && a.Value == "1" {
						inRow1 = true
					}
				}
			case "c":
				if inRow1 {
					cellIsSharedStr = false
					for _, a := range tt.Attr {
						if a.Name.Local == "t" && a.Value == "s" {
							cellIsSharedStr = true
						}
					}
				}
			case "v":
				// value will be captured in CharData
			}
		case xml.CharData:
			if inRow1 && cellIsSharedStr {
				val := strings.TrimSpace(string(tt))
				if val != "" {
					var idx int
					_, err := strings.NewReader(val).Read(nil)
					_ = err
					// Parse int manually
					for _, c := range val {
						idx = idx*10 + int(c-'0')
					}
					refs = append(refs, idx)
					cellIsSharedStr = false
				}
			}
		case xml.EndElement:
			if tt.Name.Local == "row" && inRow1 {
				return refs
			}
		}
	}
	return refs
}

// zipEntryMap reads all ZIP entries into a map of name → content.
func zipEntryMap(zr *zip.Reader) map[string]string {
	m := make(map[string]string)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			continue
		}
		data, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			continue
		}
		m[f.Name] = string(data)
	}
	return m
}

// collectTestFiles returns file paths matching any of the given glob patterns.
func collectTestFiles(t *testing.T, dir string, patterns ...string) []string {
	t.Helper()
	if dir == "" {
		return nil
	}
	var files []string
	for _, p := range patterns {
		matches, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range matches {
			if !e.IsDir() && matchGlob(e.Name(), p) {
				files = append(files, dir+"/"+e.Name())
			}
		}
	}
	return files
}

// matchGlob does simple *.ext matching.
func matchGlob(name, pattern string) bool {
	if strings.HasPrefix(pattern, "*") {
		return strings.HasSuffix(name, pattern[1:])
	}
	return name == pattern
}

// addIfExists appends paths that exist on disk.
func addIfExists(t *testing.T, files *[]string, paths ...string) {
	t.Helper()
	for _, p := range paths {
		if _, err := os.Stat(p); err == nil {
			*files = append(*files, p)
		}
	}
}
