package openxml

// okapi-filter: openxml

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A WordprocessingML paragraph nothing changed goes back as the bytes the
// source wrote it with; a paragraph that did change reopens with the source's
// own start tag and keeps the bytes of every direct child that did not change.
// The corpus test at the end is the statement over every docx fixture upstream
// ships; the tests before it isolate the mechanisms.

const replayDocHead = `<w:document ` + wmlNS + ` xmlns:w14="http://schemas.microsoft.com/office/word/2010/wordml"` +
	` xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing"` +
	` xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main"` +
	` xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"` +
	` xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math"><w:body>`

const replayDocTail = `</w:body></w:document>`

// replaySourceParagraph holds every form the writer would spell differently:
// a self-closing element, revision-save ids on the start tag and the runs,
// a proofing mark, Word's _GoBack bookmark, a character reference, a run the
// merger would fuse with its neighbour, and pretty-printed whitespace.
const replaySourceParagraph = `<w:p w:rsidR="00A1" w14:paraId="1B8C" w14:textId="77777777">` + "\r\n" +
	`  <w:pPr><w:pStyle w:val="Normal"/><w:rPr><w:lang w:val="en-US"/></w:rPr></w:pPr>` + "\r\n" +
	`  <w:proofErr w:type="spellStart"/>` + "\r\n" +
	`  <w:r w:rsidRPr="00B2"><w:rPr><w:b/></w:rPr><w:t>Bold&#160;text</w:t></w:r>` + "\r\n" +
	`  <w:proofErr w:type="spellEnd"/>` + "\r\n" +
	`  <w:bookmarkStart w:id="0" w:name="_GoBack"/><w:bookmarkEnd w:id="0"/>` + "\r\n" +
	`  <w:r><w:t xml:space="preserve"> and </w:t></w:r><w:r><w:t>plain</w:t></w:r>` + "\r\n" +
	`  <w:r><w:tab/></w:r>` + "\r\n" +
	`</w:p>`

func replayDocx(t *testing.T, body string) []byte {
	t.Helper()
	return docxWithDocumentXML(t, replayDocHead+body+replayDocTail)
}

func TestParagraphReplay_UntranslatedParagraphGoesBackByteForByte(t *testing.T) {
	pkg := replayDocx(t, replaySourceParagraph+`<w:p w:rsidR="00C3"/>`)
	out := skeletonRoundtripBytes(t, pkg, "replay.docx")
	got := string(zipPartBytes(t, out, "word/document.xml"))
	assert.Contains(t, got, replaySourceParagraph, "the paragraph's source bytes come back whole")
	assert.Contains(t, got, `<w:p w:rsidR="00C3"/>`, "a self-closing paragraph keeps its form")
}

func TestParagraphReplay_TargetEqualToSourceStillReplays(t *testing.T) {
	pkg := replayDocx(t, replaySourceParagraph)
	out := writeBackWithTargets(t, pkg, "replay.docx", func(b *model.Block) []model.Run { return b.Source })
	got := string(zipPartBytes(t, out, "word/document.xml"))
	assert.Contains(t, got, replaySourceParagraph, "a target that says what the source said, codes included, replays")

	plain := skeletonWriteBack(t, pkg, model.LocaleID("qps"), func(b *model.Block) string { return b.SourceText() })
	assert.NotContains(t, string(zipPartBytes(t, plain, "word/document.xml")), replaySourceParagraph,
		"a target with the same text and no formatting is a change, so the paragraph is rendered")
}

func TestParagraphReplay_TranslatedParagraphKeepsItsFrameAndUnchangedChildren(t *testing.T) {
	pkg := replayDocx(t, replaySourceParagraph)
	out := translateKeepingCodes(t, pkg, "replay.docx")
	got := string(zipPartBytes(t, out, "word/document.xml"))

	assert.Contains(t, got, `<w:p w:rsidR="00A1" w14:paraId="1B8C" w14:textId="77777777">`,
		"a rendered paragraph reopens with the source's start tag")
	assert.Contains(t, got, `<w:pPr><w:pStyle w:val="Normal"/><w:rPr><w:lang w:val="en-US"/></w:rPr></w:pPr>`,
		"the paragraph properties are the source's")
	assert.Contains(t, got, "<w:t xml:space=\"preserve\">[Bold\u00a0text]</w:t>", "a translated run is rendered")
	assert.NotContains(t, got, `<w:proofErr`, "a rendered paragraph carries only what the reader read")
	assert.NotContains(t, got, `<w:r w:rsidRPr="00B2">`, "the runs the reader merged cannot be aligned with the source's, so they are rendered")
}

func TestParagraphReplay_UnchangedDrawingKeepsItsBytesInATranslatedParagraph(t *testing.T) {
	drawing := `<w:r w:rsidR="00D4"><w:drawing><wp:inline distT='0'><wp:extent cx="10" cy="10"/><a:graphic><a:graphicData uri="x"/></a:graphic></wp:inline></w:drawing></w:r>`
	pkg := replayDocx(t, `<w:p><w:r><w:t>Host</w:t></w:r>`+drawing+`</w:p>`)
	out := translateKeepingCodes(t, pkg, "drawing.docx")
	got := string(zipPartBytes(t, out, "word/document.xml"))
	assert.Contains(t, got, `<w:t xml:space="preserve">[Host]</w:t>`)
	assert.Contains(t, got, drawing, "the drawing run, unchanged, is the source's bytes: single-quoted attribute and self-closing forms included")
}

func TestParagraphReplay_UnwrappedRevisionIsNotPutBack(t *testing.T) {
	// The reader unwraps <w:ins> and keeps its run. With acceptance on the
	// paragraph is rendered, and the child list does not align the wrapper
	// with the run it became, so the wrapper stays out even where the run's
	// text did not change.
	body := `<w:p><w:r><w:t>Kept</w:t></w:r><w:ins w:id="1" w:author="a" w:date="2026-01-01T00:00:00Z"><w:r><w:t>Added</w:t></w:r></w:ins></w:p>`
	pkg := replayDocx(t, body)
	out := skeletonRoundtripBytes(t, pkg, "ins.docx")
	got := string(zipPartBytes(t, out, "word/document.xml"))
	assert.NotContains(t, got, "<w:ins", "revision markup is accepted, so the paragraph is rendered")
	assert.Contains(t, got, "Added")
}

func TestParagraphReplay_RevisionParagraphReplaysWhenAcceptanceIsOff(t *testing.T) {
	body := `<w:p><w:r><w:rPr><w:rPrChange w:id="1" w:author="a" w:date="2026-01-01T00:00:00Z"><w:rPr><w:b/></w:rPr></w:rPrChange></w:rPr><w:t>Text</w:t></w:r></w:p>`
	pkg := replayDocx(t, body)

	accepted := string(zipPartBytes(t, skeletonRoundtripBytes(t, pkg, "rev.docx"), "word/document.xml"))
	assert.NotContains(t, accepted, "rPrChange", "with acceptance on the snapshot goes")

	kept := string(zipPartBytes(t, roundtripWithConfig(t, pkg, "rev.docx", func(c *Config) {
		c.AutomaticallyAcceptRevisions = false
	}), "word/document.xml"))
	assert.Contains(t, kept, body, "with acceptance off the paragraph is the source's bytes")
}

func TestParagraphReplay_FieldAcrossParagraphsReplaysBothOrNeither(t *testing.T) {
	// A HYPERLINK field whose result runs straddle two paragraphs: the reader
	// holds the first paragraph back until the field ends. Untranslated, both
	// paragraphs and the whitespace between them come back as written;
	// translated, both are rendered.
	body := `<w:p w:rsidR="00E1"><w:r><w:fldChar w:fldCharType="begin"/></w:r><w:r><w:instrText xml:space="preserve"> HYPERLINK "http://x" </w:instrText></w:r><w:r><w:fldChar w:fldCharType="separate"/></w:r><w:r><w:t>Link one</w:t></w:r></w:p>` + "\r\n    " +
		`<w:p w:rsidR="00E2"><w:r><w:t>Link two</w:t></w:r><w:r><w:fldChar w:fldCharType="end"/></w:r></w:p>`
	pkg := replayDocx(t, body)

	same := string(zipPartBytes(t, skeletonRoundtripBytes(t, pkg, "field.docx"), "word/document.xml"))
	assert.Contains(t, same, body)

	translated := string(zipPartBytes(t, translateKeepingCodes(t, pkg, "field.docx"), "word/document.xml"))
	assert.Contains(t, translated, "[Link one")
	assert.NotContains(t, translated, `<w:r><w:t>Link two</w:t></w:r>`, "the second paragraph of the field is rendered with the first")
}

func TestParagraphReplay_TextNeedingSpacePreserveIsRepaired(t *testing.T) {
	body := `<w:p><w:r><w:t>Trailing </w:t></w:r><w:r><w:t>space</w:t></w:r></w:p>`
	pkg := replayDocx(t, body)
	got := string(zipPartBytes(t, skeletonRoundtripBytes(t, pkg, "space.docx"), "word/document.xml"))
	assert.NotContains(t, got, body)
	assert.Contains(t, got, `<w:t xml:space="preserve">Trailing space</w:t>`,
		"a <w:t> whose whitespace a consumer may drop is rendered with the attribute, as SpreadsheetML does")
}

func TestParagraphReplay_NarrowNoBreakSpaceIsNotXMLWhitespace(t *testing.T) {
	body := "<w:p><w:r><w:t>Figure </w:t></w:r></w:p>"
	pkg := replayDocx(t, body)
	got := string(zipPartBytes(t, skeletonRoundtripBytes(t, pkg, "nnbsp.docx"), "word/document.xml"))
	assert.Contains(t, got, body, "XML collapses only space, tab, CR and LF; U+202F is content")
}

func TestParagraphReplay_TextboxTranslationRendersTheHostParagraph(t *testing.T) {
	host := `<w:p w:rsidR="00F1"><w:r><w:drawing><wp:inline><a:graphic><a:graphicData uri="http://schemas.microsoft.com/office/word/2010/wordprocessingShape">` +
		`<wps:wsp><wps:txbx><w:txbxContent><w:p><w:r><w:t>Box</w:t></w:r></w:p></w:txbxContent></wps:txbx></wps:wsp>` +
		`</a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`
	pkg := replayDocx(t, host)

	same := string(zipPartBytes(t, skeletonRoundtripBytes(t, pkg, "box.docx"), "word/document.xml"))
	assert.Contains(t, same, host, "an untranslated textbox leaves its host paragraph replayed")

	translated := string(zipPartBytes(t, translateKeepingCodes(t, pkg, "box.docx"), "word/document.xml"))
	assert.Contains(t, translated, "[Box]", "a translated textbox reaches the output")
	assert.NotContains(t, translated, "<w:t>Box</w:t>")
}

func TestParagraphReplay_CorePropertiesKeepTheirByteOrderMark(t *testing.T) {
	core := "\xef\xbb\xbf" + `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/">` +
		`<dc:title>Title</dc:title></cp:coreProperties>`
	pkg := docxWithParts(t, map[string]string{"docProps/core.xml": core})
	got := zipPartBytes(t, skeletonRoundtripBytes(t, pkg, "bom.docx"), "docProps/core.xml")
	assert.Equal(t, core, string(got))
}

func TestWMLParagraphChildren(t *testing.T) {
	span := []byte(`<w:p><w:pPr><w:jc w:val="both"/></w:pPr><w:proofErr w:type="spellStart"/><w:r><w:t>a</w:t></w:r>` +
		`<w:bookmarkStart w:id="0" w:name="_GoBack"/><w:bookmarkEnd w:id="0"/><w:bookmarkStart w:id="1" w:name="real"/>` +
		`<w:hyperlink r:id="rId1"><w:r><w:t>b</w:t></w:r></w:hyperlink><w:customXml/><w:r><w:tab/></w:r></w:p>`)
	children, ok := wmlParagraphChildren(span)
	require.True(t, ok)
	var names []string
	for _, c := range children {
		names = append(names, c.name+":"+string(span[c.start:c.end]))
	}
	assert.Equal(t, []string{
		`r:<w:r><w:t>a</w:t></w:r>`,
		`bookmarkStart:<w:bookmarkStart w:id="1" w:name="real"/>`,
		`hyperlink:<w:hyperlink r:id="rId1"><w:r><w:t>b</w:t></w:r></w:hyperlink>`,
		`r:<w:r><w:tab/></w:r>`,
	}, names)

	_, ok = wmlParagraphChildren([]byte(`<w:p><w:r>`))
	assert.False(t, ok, "bytes that do not close cannot be walked")
}

func TestReplayUnchangedChildren(t *testing.T) {
	span := []byte(`<w:p><w:r w:rsidR="1"><w:t>Hello</w:t></w:r><w:r w:rsidR="2"><w:tab/></w:r></w:p>`)
	source := `<w:r><w:t>Hello</w:t></w:r><w:r><w:tab/></w:r>`
	target := `<w:r><w:t>Hallo</w:t></w:r><w:r><w:tab/></w:r>`
	assert.Equal(t, `<w:r><w:t>Hallo</w:t></w:r><w:r w:rsidR="2"><w:tab/></w:r>`,
		replayUnchangedChildren(target, source, span))

	assert.Equal(t, target, replayUnchangedChildren(target, `<w:r><w:t>Hello</w:t><w:tab/></w:r>`, span),
		"a source rendering with fewer children than the span cannot be aligned")

	wrapped := []byte(`<w:p><w:ins w:id="1"><w:r><w:t>Hello</w:t></w:r></w:ins><w:r w:rsidR="3"><w:tab/></w:r></w:p>`)
	assert.Equal(t, `<w:r><w:t>Hello</w:t></w:r><w:r w:rsidR="3"><w:tab/></w:r>`,
		replayUnchangedChildren(`<w:r><w:t>Hello</w:t></w:r><w:r><w:tab/></w:r>`, source, wrapped),
		"a child whose name differs from the rendered child's is rendered, even when the run inside it did not change")
}

// writeBackWithTargets round-trips a package with every translatable block
// given the target runs translate returns for it.
func writeBackWithTargets(t *testing.T, original []byte, uri string, translate func(*model.Block) []model.Run) []byte {
	t.Helper()
	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := NewReader()
	reader.SetSkeletonStore(skelStore)
	require.NoError(t, reader.Open(t.Context(), &model.RawDocument{
		URI:          uri,
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(original),
	}))
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	require.NoError(t, reader.Close())

	target := model.LocaleID("qps")
	for _, p := range parts {
		if b, ok := p.Resource.(*model.Block); ok && b.Translatable {
			b.SetTargetRuns(target, translate(b))
		}
	}

	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(original)
	writer.SetSkeletonStore(skelStore)
	writer.SetLocale(target)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(t.Context(), testutil.PartsToChannel(parts)))
	require.NoError(t, writer.Close())
	return buf.Bytes()
}

// roundtripWithConfig round-trips a package untranslated with the reader
// configured by configure.
func roundtripWithConfig(t *testing.T, original []byte, uri string, configure func(*Config)) []byte {
	t.Helper()
	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	reader := NewReader()
	configure(reader.cfg)
	reader.SetSkeletonStore(skelStore)
	require.NoError(t, reader.Open(t.Context(), &model.RawDocument{
		URI:          uri,
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(original),
	}))
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	require.NoError(t, reader.Close())

	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(original)
	writer.SetSkeletonStore(skelStore)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(t.Context(), testutil.PartsToChannel(parts)))
	require.NoError(t, writer.Close())
	return buf.Bytes()
}

// The reasons a fixture is not byte-identical, one per group.
const (
	replayExcludedRevision = "revision markup is accepted on write (AutomaticallyAcceptRevisions is on), " +
		"so a paragraph holding it is rendered rather than replayed and the accepted document differs from the source"
	replayExcludedDrawingML = "a DrawingML paragraph in a chart or diagram part is rebuilt from the model, " +
		"and a DOCX package takes Okapi's StrippableAttributes.DrawingRunProperties strip on write"
	replayExcludedSpace = "a <w:t> with edge whitespace and no xml:space=\"preserve\": the writer adds the attribute " +
		"rather than replaying a spelling that invites a consumer to drop the space (wmlTextIsSpaceSafe)"
)

// docxReplayExclusions names the fixtures whose untranslated round trip is
// deliberately not byte-identical, and why.
var docxReplayExclusions = map[string]string{
	// Revision acceptance: move ranges, inserted and deleted content and
	// paragraph marks, property-change snapshots.
	"1080-1.docx":                           replayExcludedRevision,
	"1080-2.docx":                           replayExcludedRevision,
	"1080-3.docx":                           replayExcludedRevision,
	"1080-4.docx":                           replayExcludedRevision,
	"1102.docx":                             replayExcludedRevision,
	"1370-same-nested-revisions.docx":       replayExcludedRevision,
	"768-2.docx":                            replayExcludedRevision,
	"768.docx":                              replayExcludedRevision,
	"843-1.docx":                            replayExcludedRevision,
	"843-2.docx":                            replayExcludedRevision,
	"843-31.docx":                           replayExcludedRevision,
	"843-32.docx":                           replayExcludedRevision,
	"843-33.docx":                           replayExcludedRevision,
	"843-34.docx":                           replayExcludedRevision,
	"847-1.docx":                            replayExcludedRevision,
	"847-2.docx":                            replayExcludedRevision,
	"847-3.docx":                            replayExcludedRevision,
	"848-nested-tables-with-revisions.docx": replayExcludedRevision,
	"848.docx":                              replayExcludedRevision,
	"859.docx":                              replayExcludedRevision,
	"956.docx":                              replayExcludedRevision,
	"Addcomments.docx":                      replayExcludedRevision,
	"Deli.docx":                             replayExcludedRevision,
	"delTextAmp.docx":                       replayExcludedRevision,
	"document-revision-information-stripping.docx": replayExcludedRevision,
	"hyperlink.docx":                   replayExcludedRevision,
	"numbering-revisions.docx":         replayExcludedRevision,
	"OpenXML_text_reference_v1_2.docx": replayExcludedRevision,
	"table_truncation.docx":            replayExcludedRevision,
	"table-grid-revisions.docx":        replayExcludedRevision,
	// DrawingML paragraphs in chart and diagram parts.
	"chartAmpersand.docx": replayExcludedDrawingML,
	"simple_chart.docx":   replayExcludedDrawingML,
	"smart_art.docx":      replayExcludedDrawingML,
	// The xml:space repair.
	"952-1.docx": replayExcludedSpace,
}

// TestWMLParagraphReplay_CorpusIsByteIdentical is the corpus-wide statement:
// every docx fixture upstream ships goes back byte for byte on an untranslated
// round trip, except the ones named above.
func TestWMLParagraphReplay_CorpusIsByteIdentical(t *testing.T) {
	files, err := filepath.Glob(filepath.Join(testdataDir(t), "*.docx"))
	require.NoError(t, err)
	require.NotEmpty(t, files)
	sort.Strings(files)

	for _, path := range files {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if why, skip := docxReplayExclusions[filepath.Base(path)]; skip {
				t.Skip(why)
			}
			original, err := os.ReadFile(path)
			require.NoError(t, err)
			out := skeletonRoundtripBytes(t, original, filepath.Base(path))
			assertSamePackageBytes(t, original, out)
		})
	}
}

// TestWMLParagraphReplay_ExclusionsAreStillNeeded keeps the list above honest:
// a fixture that comes back byte for byte no longer belongs on it.
func TestWMLParagraphReplay_ExclusionsAreStillNeeded(t *testing.T) {
	dir := testdataDir(t)
	for name := range docxReplayExclusions {
		t.Run(name, func(t *testing.T) {
			original, err := os.ReadFile(filepath.Join(dir, name))
			require.NoError(t, err)
			out := skeletonRoundtripBytes(t, original, name)
			assert.False(t, samePackageBytes(t, original, out),
				"%s round-trips byte for byte and should leave docxReplayExclusions", name)
		})
	}
}

// samePackageBytes reports whether every entry of want has the same bytes in
// got.
func samePackageBytes(t *testing.T, want, got []byte) bool {
	t.Helper()
	wantZR, err := zip.NewReader(bytes.NewReader(want), int64(len(want)))
	require.NoError(t, err)
	gotZR, err := zip.NewReader(bytes.NewReader(got), int64(len(got)))
	require.NoError(t, err)
	gotParts := map[string][]byte{}
	for _, f := range gotZR.File {
		gotParts[f.Name] = zipEntryBytes(t, f)
	}
	for _, f := range wantZR.File {
		if !bytes.Equal(zipEntryBytes(t, f), gotParts[f.Name]) {
			return false
		}
	}
	return true
}
