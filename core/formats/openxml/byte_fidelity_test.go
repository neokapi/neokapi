package openxml

// okapi-filter: openxml

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The skeleton is the byte-exact half of the round trip: a part the reader
// walked past rather than extracted from goes back as its author wrote it. Go's
// xml.Decoder normalises what it parses — a self-closing element arrives as a
// start plus a synthetic end, CRLF arrives as LF (XML 1.0 §2.11), `&#39;`
// arrives as `'`, `a='1'` arrives as an attribute with the quotes gone — so a
// skeleton built by re-serialising tokens rewrote every part it touched. It
// replays the source span instead.
//
// Upstream Okapi reaches the same place by a different road: its OpenXML filter
// carries markup through as XMLEvent objects and writes them with an
// XMLEventWriter, which preserves the empty-element form and the source's
// line endings for the events it does not rewrite.

// fidelityEntry is one member of a hand-built package: the tests below author
// the XML byte for byte, so a builder cannot normalise the forms under test.
type fidelityEntry struct {
	name string
	data string
}

func buildFidelityPackage(t *testing.T, entries []fidelityEntry) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, e := range entries {
		w, err := zw.Create(e.name)
		require.NoError(t, err)
		_, err = w.Write([]byte(e.data))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

const fidelityContentTypes = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
	`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
	`<Default Extension="xml" ContentType="application/xml"/>` +
	`<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>` +
	`<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>` +
	`<Override PartName="/xl/sharedStrings.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sharedStrings+xml"/>` +
	`</Types>`

const fidelityRootRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>` +
	`</Relationships>`

const fidelityWorkbook = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">` +
	`<sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>`

const fidelityWorkbookRels = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
	`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>` +
	`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/sharedStrings" Target="sharedStrings.xml"/>` +
	`</Relationships>`

// fidelityWorksheet exercises every form encoding/xml normalises away: a
// self-closing cell and a self-closing structural element, CRLF line endings
// with indentation, single-quoted and unusually spaced attributes, a numeric
// character reference, and a CDATA section in a formula.
const fidelityWorksheet = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` + "\r\n" +
	`  <dimension ref="A1:C3"/>` + "\r\n" +
	`  <sheetViews><sheetView workbookViewId='0'/></sheetViews>` + "\r\n" +
	`  <sheetData>` + "\r\n" +
	`    <row r="1" spans="1:3">` + "\r\n" +
	`      <c r="A1" t="s"><v>0</v></c>` + "\r\n" +
	`      <c r="B1"/>` + "\r\n" +
	`      <c r='C1'   s='0'><v>42</v></c>` + "\r\n" +
	`    </row>` + "\r\n" +
	`    <row r="2"><c r="A2"><f><![CDATA[IF(A1>0,"y","n")]]></f><v>y</v></c></row>` + "\r\n" +
	`    <row r="3"><c r="A3" t="str"><v>M&#38;M&#39;s</v></c></row>` + "\r\n" +
	`  </sheetData>` + "\r\n" +
	`  <pageMargins left="0.7" right="0.7"/>` + "\r\n" +
	`</worksheet>`

const fidelitySharedStrings = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="2" uniqueCount="2">` +
	`<si><t>Header</t></si>` +
	`<si><t> </t></si>` +
	`</sst>`

func fidelityWorkbookPackage(t *testing.T) []byte {
	t.Helper()
	return buildFidelityPackage(t, []fidelityEntry{
		{"[Content_Types].xml", fidelityContentTypes},
		{"_rels/.rels", fidelityRootRels},
		{"xl/workbook.xml", fidelityWorkbook},
		{"xl/_rels/workbook.xml.rels", fidelityWorkbookRels},
		{"xl/worksheets/sheet1.xml", fidelityWorksheet},
		{"xl/sharedStrings.xml", fidelitySharedStrings},
	})
}

// TestByteFidelity_WorksheetSourceForm is the regression for the shapes Go's
// decoder normalises. Each assertion names the form it guards so a failure says
// which one moved.
func TestByteFidelity_WorksheetSourceForm(t *testing.T) {
	pkg := fidelityWorkbookPackage(t)
	out := skeletonRoundtripBytes(t, pkg, "fidelity.xlsx")
	got := string(zipPartBytes(t, out, "xl/worksheets/sheet1.xml"))

	assert.Contains(t, got, `<dimension ref="A1:C3"/>`,
		"a self-closing element must not become a start/end pair")
	assert.Contains(t, got, `<c r="B1"/>`,
		"a self-closing cell must not become a start/end pair")
	assert.Contains(t, got, "\r\n  <sheetData>",
		"CRLF line endings must survive the decoder's XML 1.0 §2.11 normalisation")
	assert.Contains(t, got, `<sheetView workbookViewId='0'/>`,
		"single-quoted attributes must keep their quote character")
	assert.Contains(t, got, `<c r='C1'   s='0'>`,
		"attribute spacing and order must survive")
	assert.Contains(t, got, `<![CDATA[IF(A1>0,"y","n")]]>`,
		"a CDATA section must not be re-escaped as text")
	assert.Contains(t, got, `M&#38;M&#39;s`,
		"a numeric character reference must keep its spelling")
	assert.Equal(t, fidelityWorksheet, got,
		"an untranslated worksheet must go back byte for byte")
}

// TestByteFidelity_SharedStringsSourceForm covers the CT_Rst items the reader
// passes through: one item is extracted (so the writer projects it), and one
// holds only whitespace and goes back untouched.
func TestByteFidelity_SharedStringsSourceForm(t *testing.T) {
	pkg := fidelityWorkbookPackage(t)
	out := skeletonRoundtripBytes(t, pkg, "fidelity.xlsx")
	assert.Equal(t, fidelitySharedStrings,
		string(zipPartBytes(t, out, "xl/sharedStrings.xml")),
		"an untranslated shared-string table must go back byte for byte")
}

// TestByteFidelity_TranslatedWorksheetKeepsItsSkeleton checks that the byte
// replay does not depend on the translation being absent: a translated
// workbook still returns every byte of the worksheet the cell text does not
// occupy.
func TestByteFidelity_TranslatedWorksheetKeepsItsSkeleton(t *testing.T) {
	pkg := fidelityWorkbookPackage(t)
	out := skeletonWriteBack(t, pkg, model.LocaleID("qps"), func(b *model.Block) string {
		return "[" + b.SourceText() + "]"
	})

	sheet := string(zipPartBytes(t, out, "xl/worksheets/sheet1.xml"))
	assert.Equal(t, fidelityWorksheet, sheet,
		"a worksheet whose text lives in the shared-string table is untouched by a translation")

	sst := string(zipPartBytes(t, out, "xl/sharedStrings.xml"))
	assert.Contains(t, sst, "<si><t>[Header]</t></si>", "the translated item is written back")
	assert.Contains(t, sst, "<si><t> </t></si>", "the whitespace item is replayed unchanged")
	assert.True(t, strings.HasPrefix(sst,
		`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`+"\r\n"),
		"the declaration and its CRLF survive a translated write")
}

// TestByteFidelity_TableColumnKeepsItsElementForm covers the one place the
// reader replaces an attribute value rather than an element: a table column's
// name. Everything else in the tag, the self-closing slash included, is replayed.
func TestByteFidelity_TableColumnKeepsItsElementForm(t *testing.T) {
	const tablePart = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
		`<table xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" id="1" name="Table1" ref="A1:A2">` +
		`<tableColumns count="1"><tableColumn id='1' name="Header"/></tableColumns>` +
		`</table>`
	contentTypes := strings.Replace(fidelityContentTypes,
		`</Types>`,
		`<Override PartName="/xl/tables/table1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.table+xml"/></Types>`, 1)
	sheetRels := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
		`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/table" Target="../tables/table1.xml"/>` +
		`</Relationships>`

	pkg := buildFidelityPackage(t, []fidelityEntry{
		{"[Content_Types].xml", contentTypes},
		{"_rels/.rels", fidelityRootRels},
		{"xl/workbook.xml", fidelityWorkbook},
		{"xl/_rels/workbook.xml.rels", fidelityWorkbookRels},
		{"xl/worksheets/sheet1.xml", fidelityWorksheet},
		{"xl/worksheets/_rels/sheet1.xml.rels", sheetRels},
		{"xl/sharedStrings.xml", fidelitySharedStrings},
		{"xl/tables/table1.xml", tablePart},
	})

	out := skeletonRoundtripBytes(t, pkg, "fidelity.xlsx")
	assert.Equal(t, tablePart, string(zipPartBytes(t, out, "xl/tables/table1.xml")),
		"an untranslated table definition must go back byte for byte")

	translated := skeletonWriteBack(t, pkg, model.LocaleID("qps"), func(b *model.Block) string {
		return "[" + b.SourceText() + "]"
	})
	got := string(zipPartBytes(t, translated, "xl/tables/table1.xml"))
	assert.Contains(t, got, `<tableColumn id='1' name="[Header]"/>`,
		"only the name value changes: the quote characters and the self-closing slash stay")
}

// TestByteFidelity_CorePropertiesSourceForm covers docProps/core.xml, whose
// parser extracts a handful of Dublin Core elements and walks past the rest.
func TestByteFidelity_CorePropertiesSourceForm(t *testing.T) {
	const coreProps = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
		`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" ` +
		`xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" ` +
		`xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">` + "\r\n" +
		`  <dc:title>A title</dc:title>` + "\r\n" +
		`  <dc:subject></dc:subject>` + "\r\n" +
		`  <cp:revision>4</cp:revision>` + "\r\n" +
		`  <dcterms:created xsi:type="dcterms:W3CDTF">2009-05-14T19:18:00Z</dcterms:created>` + "\r\n" +
		`</cp:coreProperties>`
	contentTypes := strings.Replace(fidelityContentTypes,
		`</Types>`,
		`<Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/></Types>`, 1)
	rootRels := strings.Replace(fidelityRootRels,
		`</Relationships>`,
		`<Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/></Relationships>`, 1)

	pkg := buildFidelityPackage(t, []fidelityEntry{
		{"[Content_Types].xml", contentTypes},
		{"_rels/.rels", rootRels},
		{"docProps/core.xml", coreProps},
		{"xl/workbook.xml", fidelityWorkbook},
		{"xl/_rels/workbook.xml.rels", fidelityWorkbookRels},
		{"xl/worksheets/sheet1.xml", fidelityWorksheet},
		{"xl/sharedStrings.xml", fidelitySharedStrings},
	})

	out := skeletonRoundtripBytes(t, pkg, "fidelity.xlsx")
	assert.Equal(t, coreProps, string(zipPartBytes(t, out, "docProps/core.xml")),
		"untranslated core properties must go back byte for byte")
}

// TestByteFidelity_ZipEntryOrderAndMethod checks the package level: the entries
// keep their names and their order, and an entry the writer copies keeps its
// stored/deflated form. An entry the writer rebuilds is deflated whatever it
// was, because its content is no longer the bytes whose compressed form the
// source header describes.
func TestByteFidelity_ZipEntryOrderAndMethod(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	entries := []fidelityEntry{
		{"[Content_Types].xml", fidelityContentTypes},
		{"_rels/.rels", fidelityRootRels},
		{"xl/workbook.xml", fidelityWorkbook},
		{"xl/_rels/workbook.xml.rels", fidelityWorkbookRels},
		{"xl/worksheets/sheet1.xml", fidelityWorksheet},
		{"xl/sharedStrings.xml", fidelitySharedStrings},
	}
	for _, e := range entries {
		// Store the relationship parts uncompressed so the copy path is
		// distinguishable from the rebuild path.
		method := zip.Deflate
		if strings.Contains(e.name, "_rels") {
			method = zip.Store
		}
		w, err := zw.CreateHeader(&zip.FileHeader{Name: e.name, Method: method})
		require.NoError(t, err)
		_, err = w.Write([]byte(e.data))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	pkg := buf.Bytes()

	out := skeletonRoundtripBytes(t, pkg, "fidelity.xlsx")

	origZR, err := zip.NewReader(bytes.NewReader(pkg), int64(len(pkg)))
	require.NoError(t, err)
	outZR, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
	require.NoError(t, err)

	var origNames, outNames []string
	origMethod := map[string]uint16{}
	outMethod := map[string]uint16{}
	for _, f := range origZR.File {
		origNames = append(origNames, f.Name)
		origMethod[f.Name] = f.Method
	}
	for _, f := range outZR.File {
		outNames = append(outNames, f.Name)
		outMethod[f.Name] = f.Method
	}
	assert.Equal(t, origNames, outNames, "entry names and their order must survive")
	assert.Equal(t, zip.Store, outMethod["_rels/.rels"],
		"an entry the writer copies keeps its compression method")
	assert.Equal(t, zip.Store, outMethod["xl/_rels/workbook.xml.rels"],
		"an entry the writer copies keeps its compression method")
}

// partsHoldingContent reports the parts whose skeleton carries a block ref or a
// language value, which are the parts the writer rebuilds from the content
// model rather than replaying. The skeleton brackets every part with a
// start/end marker ref, so the set is exact.
func partsHoldingContent(t *testing.T, store *format.SkeletonStore) map[string]bool {
	t.Helper()
	require.NoError(t, store.Flush())
	held := map[string]bool{}
	current := ""
	for {
		entry, err := store.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err)
		switch entry.Type {
		case format.SkeletonRef:
			id := string(entry.Data)
			if after, ok := strings.CutPrefix(id, skelPartStartPrefix); ok {
				current = after
				continue
			}
			if _, ok := strings.CutPrefix(id, skelPartEndPrefix); ok {
				current = ""
				continue
			}
			if current != "" {
				held[current] = true
			}
		case format.SkeletonLang:
			if current != "" {
				held[current] = true
			}
		}
	}
	// Rewind so the caller's writer sees the whole stream.
	require.NoError(t, store.Flush())
	return held
}

// rebuiltWithoutContent names the parts the writer rewrites even though the
// reader extracted nothing from them, so they cannot be asserted byte-identical.
// There are exactly two, and both are deliberate:
//
// word/styles.xml (and the other WordprocessingML parts shouldStripWMLLang
// covers) lose their <w:lang> and <w:noProof> elements on write: upstream
// Okapi's RunSkippableElements drops both, and the parity canon is that output.
// The strip is the writer's, not the skeleton's — the reader passes these parts
// through untouched.
//
// docProps/core.xml loses a leading UTF-8 BOM. Okapi reads the part through
// StAX, which takes the BOM as encoding metadata rather than content, and its
// XMLEventWriter emits none, so the reference output for a BOM-bearing core.xml
// is BOM-less; parseCoreProperties strips to match. 948-1.docx is the only
// fixture in the corpus that ships one.
//
// A DOCX part carrying a DrawingML paragraph loses the six run-property
// attributes Okapi's StrippableAttributes.DrawingRunProperties drops (lang,
// altLang, dirty, smtClean, err, noProof), and an <a:endParaRPr> left empty by
// that strip goes with them. The writer does this after skeleton
// reconstruction, on DOCX only; chartAmpersand.docx is the corpus case.
func rebuiltWithoutContent(name string, source []byte, isDocx bool) bool {
	if shouldStripWMLLang(name) {
		return true
	}
	if name == "docProps/core.xml" && bytes.HasPrefix(source, []byte("\xef\xbb\xbf")) {
		return true
	}
	return isDocx && stripDMLRunPropertyAttrs(string(source)) != string(source)
}

// TestByteFidelity_CorpusUntouchedParts is the corpus-wide statement of the
// contract: over every OpenXML fixture upstream ships, a part the reader did not
// extract from comes back byte for byte.
//
// Parts that do hold extracted content are excluded, because the writer
// projects those from the content model: a paragraph's runs are rebuilt from
// the block, so their bytes are the writer's, not the source's.
func TestByteFidelity_CorpusUntouchedParts(t *testing.T) {
	dir := testdataDir(t)
	var files []string
	for _, pattern := range []string{"*.docx", "*.xlsx", "*.pptx"} {
		matches, err := filepath.Glob(filepath.Join(dir, pattern))
		require.NoError(t, err)
		files = append(files, matches...)
	}
	require.NotEmpty(t, files, "no OpenXML fixtures in %s", dir)
	sort.Strings(files)

	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			original, err := os.ReadFile(path)
			require.NoError(t, err)

			skelStore, err := format.NewSkeletonStore()
			require.NoError(t, err)
			defer skelStore.Close()

			reader := NewReader()
			reader.SetSkeletonStore(skelStore)
			require.NoError(t, reader.Open(t.Context(), &model.RawDocument{
				URI:          filepath.Base(path),
				SourceLocale: model.LocaleEnglish,
				Encoding:     "UTF-8",
				Reader:       readCloserFromBytes(original),
			}))
			parts := testutil.CollectParts(t, reader.Read(t.Context()))
			require.NoError(t, reader.Close())

			held := partsHoldingContent(t, skelStore)

			var buf bytes.Buffer
			writer := NewWriter()
			writer.SetOriginalContent(original)
			writer.SetSkeletonStore(skelStore)
			require.NoError(t, writer.SetOutputWriter(&buf))
			require.NoError(t, writer.Write(t.Context(), testutil.PartsToChannel(parts)))
			require.NoError(t, writer.Close())

			origZR, err := zip.NewReader(bytes.NewReader(original), int64(len(original)))
			require.NoError(t, err)
			outZR, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
			require.NoError(t, err)

			out := map[string][]byte{}
			var origNames, outNames []string
			for _, f := range outZR.File {
				outNames = append(outNames, f.Name)
				rc, err := f.Open()
				require.NoError(t, err)
				data, err := io.ReadAll(rc)
				require.NoError(t, rc.Close())
				require.NoError(t, err)
				out[f.Name] = data
			}
			for _, f := range origZR.File {
				origNames = append(origNames, f.Name)
			}
			assert.Equal(t, origNames, outNames, "entry names and their order must survive")

			for _, f := range origZR.File {
				if held[f.Name] {
					continue
				}
				rc, err := f.Open()
				require.NoError(t, err)
				data, err := io.ReadAll(rc)
				require.NoError(t, rc.Close())
				require.NoError(t, err)
				if rebuiltWithoutContent(f.Name, data, strings.HasSuffix(path, ".docx")) {
					continue
				}
				if !bytes.Equal(data, out[f.Name]) {
					assert.Failf(t, "part rewritten",
						"%s: the reader extracted nothing from this part, so it must go back byte for byte\n%s",
						f.Name, firstByteDifference(data, out[f.Name]))
				}
			}
		})
	}
}

// firstByteDifference renders the neighbourhood of the first differing byte,
// which is what a reader of a failure needs to see.
func firstByteDifference(want, got []byte) string {
	n := min(len(want), len(got))
	i := 0
	for i < n && want[i] == got[i] {
		i++
	}
	lo := max(0, i-60)
	return fmt.Sprintf("at byte %d\nwant: %q\ngot:  %q", i, window(want, lo, i+60), window(got, lo, i+60))
}

func window(b []byte, lo, hi int) string {
	hi = min(hi, len(b))
	lo = min(lo, hi)
	return string(b[lo:hi])
}
