package openxml

// okapi-filter: openxml

import (
	"archive/zip"
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The skippable-element strip mirrors upstream Okapi's RunSkippableElements
// and BlockSkippableElements, and it reaches rendered content only. A part the
// reader extracted nothing from, and the frame around an extracted paragraph,
// go back as the source wrote them. The revision strip is the other half of
// what used to be one pass: it still covers whole parts, because accepting
// revisions is a decision about the document rather than about a paragraph.

const wmlNS = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"`

func TestStripWMLSkippableElements(t *testing.T) {
	tests := []struct {
		name   string
		in     string
		strict bool
		want   string
	}{
		{
			name: "lang and noProof go, and the container they emptied goes with them",
			in:   `<w:r><w:rPr><w:noProof/><w:lang w:val="en-US"/></w:rPr><w:t>x</w:t></w:r>`,
			want: `<w:r><w:t>x</w:t></w:r>`,
		},
		{
			name: "a container with other children stays",
			in:   `<w:r><w:rPr><w:b/><w:lang w:val="en-US"/></w:rPr><w:t>x</w:t></w:r>`,
			want: `<w:r><w:rPr><w:b/></w:rPr><w:t>x</w:t></w:r>`,
		},
		{
			name: "an emptied paragraph mark collapses through both containers",
			in:   `<w:p><w:pPr><w:rPr><w:lang w:val="en-US"/></w:rPr></w:pPr><w:r><w:t>x</w:t></w:r></w:p>`,
			want: `<w:p><w:r><w:t>x</w:t></w:r></w:p>`,
		},
		{
			name: "a container the source wrote empty stays",
			in:   `<w:p><w:pPr><w:rPr></w:rPr></w:pPr><w:r><w:rPr/><w:t>x</w:t></w:r><w:r><w:rPr><w:lang w:val="en-US"/></w:rPr><w:t>y</w:t></w:r></w:p>`,
			want: `<w:p><w:pPr><w:rPr></w:rPr></w:pPr><w:r><w:rPr/><w:t>x</w:t></w:r><w:r><w:t>y</w:t></w:r></w:p>`,
		},
		{
			name: "a whitespace-padded empty container is the source's spelling too",
			in:   "<w:pPr>\n  <w:rPr>\n  </w:rPr>\n</w:pPr><w:r><w:rPr><w:lang/></w:rPr></w:r>",
			want: "<w:pPr>\n  <w:rPr>\n  </w:rPr>\n</w:pPr><w:r></w:r>",
		},
		{
			name: "an SDT's property containers keep lang and noProof",
			in:   `<w:sdt><w:sdtPr><w:rPr><w:noProof/><w:lang w:val="en-US"/></w:rPr></w:sdtPr><w:sdtEndPr><w:rPr><w:noProof/></w:rPr></w:sdtEndPr><w:sdtContent><w:r><w:rPr><w:noProof/></w:rPr><w:t>x</w:t></w:r></w:sdtContent></w:sdt>`,
			want: `<w:sdt><w:sdtPr><w:rPr><w:noProof/><w:lang w:val="en-US"/></w:rPr></w:sdtPr><w:sdtEndPr><w:rPr><w:noProof/></w:rPr></w:sdtEndPr><w:sdtContent><w:r><w:t>x</w:t></w:r></w:sdtContent></w:sdt>`,
		},
		{
			name: "bidiVisual goes from table properties",
			in:   `<w:tblPr><w:bidiVisual/><w:tblW w:w="0" w:type="auto"/></w:tblPr>`,
			want: `<w:tblPr><w:tblW w:w="0" w:type="auto"/></w:tblPr>`,
		},
		{
			name:   "a strict part keeps lang and noProof and still loses bidiVisual",
			in:     `<w:tblPr><w:bidiVisual/></w:tblPr><w:r><w:rPr><w:noProof/><w:lang w:eastAsia="ru-RU"/></w:rPr><w:t>x</w:t></w:r>`,
			strict: true,
			want:   `<w:tblPr></w:tblPr><w:r><w:rPr><w:noProof/><w:lang w:eastAsia="ru-RU"/></w:rPr><w:t>x</w:t></w:r>`,
		},
		{
			name: "revision markup is another pass's business",
			in:   `<w:pPr><w:rPr><w:ins w:id="1" w:author="a" w:date="2026-01-01T00:00:00Z"/><w:rPrChange w:id="2" w:author="a" w:date="2026-01-01T00:00:00Z"><w:rPr><w:b/></w:rPr></w:rPrChange></w:rPr></w:pPr>`,
			want: `<w:pPr><w:rPr><w:ins w:id="1" w:author="a" w:date="2026-01-01T00:00:00Z"/><w:rPrChange w:id="2" w:author="a" w:date="2026-01-01T00:00:00Z"><w:rPr><w:b/></w:rPr></w:rPrChange></w:rPr></w:pPr>`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stripWMLSkippableElements([]byte(tc.in), tc.strict)
			assert.Equal(t, tc.want, string(got))
			assert.Equal(t, tc.want, stripWMLSkippableElementsString(tc.in, tc.strict))
		})
	}
}

func TestStripWMLSkippableElementsReturnsInputWhenNothingMatches(t *testing.T) {
	in := []byte(`<w:p><w:pPr><w:rPr></w:rPr></w:pPr><w:r><w:rPr/><w:t>x</w:t></w:r></w:p>`)
	got := stripWMLSkippableElements(in, false)
	assert.Equal(t, string(in), string(got))
	assert.Same(t, &in[0], &got[0], "nothing to strip means the caller's slice comes back")
}

func TestStripWMLRevisionElements(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "a property-change snapshot goes, and the container it emptied goes with it",
			in:   `<w:r><w:rPr><w:rPrChange w:id="1" w:author="a" w:date="2026-01-01T00:00:00Z"><w:rPr><w:b/></w:rPr></w:rPrChange></w:rPr><w:t>x</w:t></w:r>`,
			want: `<w:r><w:t>x</w:t></w:r>`,
		},
		{
			name: "an empty-body paragraph-mark marker goes through both containers",
			in:   `<w:p><w:pPr><w:rPr><w:ins w:id="1" w:author="a" w:date="2026-01-01T00:00:00Z"/></w:rPr></w:pPr><w:r><w:t>x</w:t></w:r></w:p>`,
			want: `<w:p><w:r><w:t>x</w:t></w:r></w:p>`,
		},
		{
			name: "a content-wrapping insertion keeps its children and its wrapper",
			in:   `<w:ins w:id="1" w:author="a" w:date="2026-01-01T00:00:00Z"><w:r><w:t>x</w:t></w:r></w:ins>`,
			want: `<w:ins w:id="1" w:author="a" w:date="2026-01-01T00:00:00Z"><w:r><w:t>x</w:t></w:r></w:ins>`,
		},
		{
			name: "move-range markers go",
			in:   `<w:moveFromRangeStart w:id="1" w:name="m"/><w:p/><w:moveFromRangeEnd w:id="1"/><w:moveToRangeStart w:id="2" w:name="m"/><w:moveToRangeEnd w:id="2"/>`,
			want: `<w:p/>`,
		},
		{
			name: "a table property change goes and the container stays with its other children",
			in:   `<w:tblPr><w:tblStyle w:val="T"/><w:tblPrChange w:id="1" w:author="a" w:date="2026-01-01T00:00:00Z"><w:tblPr><w:tblStyle w:val="U"/></w:tblPr></w:tblPrChange></w:tblPr>`,
			want: `<w:tblPr><w:tblStyle w:val="T"/></w:tblPr>`,
		},
		{
			name: "a container the source wrote empty stays",
			in:   `<w:p><w:pPr><w:rPr></w:rPr></w:pPr><w:r><w:rPr><w:rPrChange w:id="1"><w:rPr/></w:rPrChange></w:rPr><w:t>x</w:t></w:r></w:p>`,
			want: `<w:p><w:pPr><w:rPr></w:rPr></w:pPr><w:r><w:t>x</w:t></w:r></w:p>`,
		},
		{
			name: "lang and noProof are another pass's business",
			in:   `<w:r><w:rPr><w:noProof/><w:lang w:val="en-US"/></w:rPr><w:t>x</w:t></w:r>`,
			want: `<w:r><w:rPr><w:noProof/><w:lang w:val="en-US"/></w:rPr><w:t>x</w:t></w:r>`,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stripWMLRevisionElements([]byte(tc.in))
			assert.Equal(t, tc.want, string(got))
		})
	}
}

// translateKeepingCodes round-trips a package with every translatable block
// given a target that brackets each text run and keeps every other run as the
// source had it, so a rendered paragraph still carries its formatting codes
// and its placeholders.
func translateKeepingCodes(t *testing.T, original []byte, uri string) []byte {
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
		b, ok := p.Resource.(*model.Block)
		if !ok || !b.Translatable {
			continue
		}
		runs := make([]model.Run, len(b.Source))
		for i, r := range b.Source {
			if r.Text != nil {
				text := *r.Text
				text.Text = "[" + text.Text + "]"
				r.Text = &text
			}
			runs[i] = r
		}
		b.SetTargetRuns(target, runs)
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

// docxWithParts clones testdata/simple.docx and replaces or adds the named
// entries, so a test can author a part byte for byte.
func docxWithParts(t *testing.T, parts map[string]string) []byte {
	t.Helper()
	src, err := os.ReadFile("testdata/simple.docx")
	require.NoError(t, err)
	zr, err := zip.NewReader(bytes.NewReader(src), int64(len(src)))
	require.NoError(t, err)

	var out bytes.Buffer
	zw := zip.NewWriter(&out)
	written := map[string]bool{}
	for _, f := range zr.File {
		if data, ok := parts[f.Name]; ok {
			w, err := zw.Create(f.Name)
			require.NoError(t, err)
			_, err = w.Write([]byte(data))
			require.NoError(t, err)
			written[f.Name] = true
			continue
		}
		require.NoError(t, zw.Copy(f))
	}
	for name, data := range parts {
		if written[name] {
			continue
		}
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = w.Write([]byte(data))
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return out.Bytes()
}

const skippableStylesPart = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<w:styles ` + wmlNS + `>` +
	`<w:docDefaults><w:rPrDefault><w:rPr><w:lang w:val="en-US" w:eastAsia="en-US" w:bidi="ar-SA"/></w:rPr></w:rPrDefault>` +
	`<w:pPrDefault><w:pPr><w:rPr><w:noProof/></w:rPr></w:pPr></w:pPrDefault></w:docDefaults>` +
	`<w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:rPr><w:lang w:val="en-US"/></w:rPr></w:style>` +
	`<w:style w:type="table" w:styleId="T"><w:name w:val="T"/><w:tblPr><w:bidiVisual/></w:tblPr></w:style>` +
	`</w:styles>`

// The styles part is the single most common place a docx lost bytes: every
// <w:lang> in every style went, and the containers with them.
func TestSkippableStrip_StylesPartGoesBackByteForByte(t *testing.T) {
	pkg := docxWithParts(t, map[string]string{"word/styles.xml": skippableStylesPart})

	for name, roundtrip := range map[string]func(*testing.T, []byte, string) []byte{
		"untranslated": skeletonRoundtripBytes,
		"translated":   translateRoundtripBytes,
	} {
		t.Run(name, func(t *testing.T) {
			out := roundtrip(t, pkg, "styles.docx")
			assert.Equal(t, skippableStylesPart, string(zipPartBytes(t, out, "word/styles.xml")))

			outZR, err := zip.NewReader(bytes.NewReader(out), int64(len(out)))
			require.NoError(t, err)
			for _, f := range outZR.File {
				if f.Name == "word/styles.xml" {
					assert.Equal(t, zip.Deflate, f.Method, "a part copied through keeps its entry header")
				}
			}
		})
	}
}

// The frame around an extracted paragraph is skeleton, so the paragraph mark's
// <w:lang> stays even when the paragraph is translated; the run the writer
// renders carries the reader's run properties, which never held the element.
func TestSkippableStrip_ParagraphFrameKeepsLang(t *testing.T) {
	doc := `<w:document ` + wmlNS + `><w:body>` +
		`<w:p w:rsidR="00A1"><w:pPr><w:pStyle w:val="Normal"/><w:rPr><w:lang w:val="en-US"/></w:rPr></w:pPr>` +
		`<w:r><w:rPr><w:b/><w:lang w:val="en-US"/></w:rPr><w:t>Hello</w:t></w:r></w:p>` +
		`<w:tbl><w:tblPr><w:bidiVisual/></w:tblPr><w:tblGrid><w:gridCol w:w="1"/></w:tblGrid>` +
		`<w:tr><w:tc><w:p><w:r><w:t>Cell</w:t></w:r></w:p></w:tc></w:tr></w:tbl>` +
		`</w:body></w:document>`
	pkg := docxWithDocumentXML(t, doc)

	out := translateKeepingCodes(t, pkg, "frame.docx")
	got := string(zipPartBytes(t, out, "word/document.xml"))

	assert.Contains(t, got, `<w:pPr><w:pStyle w:val="Normal"/><w:rPr><w:lang w:val="en-US"/></w:rPr></w:pPr>`,
		"the paragraph mark keeps its language")
	assert.Contains(t, got, `<w:tblPr><w:bidiVisual/></w:tblPr>`,
		"table properties in the skeleton keep bidiVisual")
	assert.Contains(t, got, `<w:r><w:rPr><w:b/></w:rPr><w:t xml:space="preserve">[Hello]</w:t></w:r>`,
		"a rendered run carries the reader's properties, which drop lang the way RunSkippableElements does")
}

// A properties container the source wrote empty is the source's spelling and
// survives; only a container the strip empties is removed.
func TestSkippableStrip_SourceEmptyContainerSurvives(t *testing.T) {
	doc := `<w:document ` + wmlNS + `><w:body>` +
		`<w:p><w:pPr><w:pStyle w:val="Normal"/><w:rPr></w:rPr></w:pPr><w:r><w:t>Hello</w:t></w:r></w:p>` +
		`</w:body></w:document>`
	pkg := docxWithDocumentXML(t, doc)

	out := translateRoundtripBytes(t, pkg, "empty.docx")
	got := string(zipPartBytes(t, out, "word/document.xml"))
	assert.Contains(t, got, `<w:pPr><w:pStyle w:val="Normal"/><w:rPr></w:rPr></w:pPr>`)
}

// A payload the writer renders from a block, such as the paragraphs of a
// textbox travelling on the host run, is rendered content and takes the strip.
func TestSkippableStrip_RenderedPayloadIsStripped(t *testing.T) {
	doc := `<w:document ` + wmlNS + ` ` +
		`xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" ` +
		`xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
		`xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"><w:body>` +
		`<w:p><w:r><w:t>Host</w:t></w:r><w:r><w:drawing><wp:inline><a:graphic><a:graphicData uri="http://schemas.microsoft.com/office/word/2010/wordprocessingShape">` +
		`<wps:wsp><wps:txbx><w:txbxContent>` +
		`<w:p><w:pPr><w:rPr><w:lang w:val="en-US"/></w:rPr></w:pPr><w:r><w:rPr><w:lang w:val="en-US"/></w:rPr><w:t>Box</w:t></w:r></w:p>` +
		`</w:txbxContent></wps:txbx></wps:wsp>` +
		`</a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>` +
		`</w:body></w:document>`
	pkg := docxWithDocumentXML(t, doc)

	out := translateKeepingCodes(t, pkg, "textbox.docx")
	got := string(zipPartBytes(t, out, "word/document.xml"))
	require.Contains(t, got, "<w:txbxContent>")
	box := got[strings.Index(got, "<w:txbxContent>"):strings.Index(got, "</w:txbxContent>")]
	assert.NotContains(t, box, "<w:lang", "the textbox paragraph is rendered from its block, so the strip reaches it")
	assert.Contains(t, box, "[Box]")
}

// The revision strip still covers a whole part: a property-change snapshot in
// a paragraph the reader extracted nothing from goes, as it does upstream when
// revisions are accepted.
func TestRevisionStrip_CoversTheWholePart(t *testing.T) {
	doc := `<w:document ` + wmlNS + `><w:body>` +
		`<w:tbl><w:tblPr><w:tblStyle w:val="T"/><w:tblPrChange w:id="1" w:author="a" w:date="2026-01-01T00:00:00Z"><w:tblPr><w:tblStyle w:val="U"/></w:tblPr></w:tblPrChange></w:tblPr>` +
		`<w:tblGrid><w:gridCol w:w="1"/></w:tblGrid>` +
		`<w:tr><w:tc><w:p><w:pPr><w:rPr><w:ins w:id="2" w:author="a" w:date="2026-01-01T00:00:00Z"/></w:rPr></w:pPr></w:p></w:tc></w:tr></w:tbl>` +
		`</w:body></w:document>`
	pkg := docxWithDocumentXML(t, doc)

	out := skeletonRoundtripBytes(t, pkg, "revisions.docx")
	got := string(zipPartBytes(t, out, "word/document.xml"))
	assert.Contains(t, got, `<w:tblPr><w:tblStyle w:val="T"/></w:tblPr>`)
	assert.NotContains(t, got, "<w:ins")
	assert.NotContains(t, got, "<w:pPr>", "the paragraph mark's container went with its only child")
}
