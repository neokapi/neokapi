package openxml

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A standalone equation paragraph with a math run, an <m:nor/> prose run
// ("where"), and another math run. The OMML is namespace-less inside the run
// (the m: prefix is bound on <w:document>) — exactly as captured from a .docx.
const norEquationBody = `<w:p><w:r><w:t>Intro</w:t></w:r></w:p>` +
	`<w:p><m:oMathPara><m:oMath>` +
	`<m:r><m:t>x</m:t></m:r>` +
	`<m:r><m:rPr><m:nor/></m:rPr><m:t>where</m:t></m:r>` +
	`<m:r><m:t>y</m:t></m:r>` +
	`</m:oMath></m:oMathPara></w:p>`

func norEquationDocx(t *testing.T) []byte {
	doc := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"` +
		` xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math"><w:body>` +
		norEquationBody + `</w:body></w:document>`
	return zipBytes(t, [][2]string{
		{"[Content_Types].xml", validContentTypes},
		{"_rels/.rels", validRootRels},
		{"word/document.xml", doc},
	})
}

func docxDocumentXML(t *testing.T, docx []byte) string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	require.NoError(t, err)
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			rc, err := f.Open()
			require.NoError(t, err)
			b, _ := io.ReadAll(rc)
			rc.Close()
			return string(b)
		}
	}
	t.Fatal("no word/document.xml in output")
	return ""
}

// writeBackNor reads the nor-equation docx, applies translate[sourceText] to the
// matching omml-nor block, and writes the docx back, returning the output bytes.
func writeBackNor(t *testing.T, docx []byte, translate map[string]string) []byte {
	t.Helper()
	skel, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skel.Close()

	reader := NewReader()
	reader.SetSkeletonStore(skel)
	require.NoError(t, reader.Open(t.Context(), &model.RawDocument{
		URI: "t.docx", SourceLocale: model.LocaleEnglish, Encoding: "UTF-8",
		Reader: readCloserFromBytes(docx),
	}))
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	reader.Close()

	tgt := model.LocaleID("nb-NO")
	for _, p := range parts {
		if p.Type != model.PartBlock {
			continue
		}
		b := p.Resource.(*model.Block)
		if b.Type == ommlNorBlockType {
			if tr, ok := translate[b.SourceText()]; ok {
				b.SetTargetText(tgt, tr)
			}
		}
	}

	var buf bytes.Buffer
	w := NewWriter()
	w.SetOriginalContent(docx)
	w.SetSkeletonStore(skel)
	if len(translate) > 0 {
		w.SetLocale(tgt)
	}
	require.NoError(t, w.SetOutputWriter(&buf))
	require.NoError(t, w.Write(t.Context(), testutil.PartsToChannel(parts)))
	w.Close()
	return buf.Bytes()
}

// The equation's <m:nor/> prose surfaces as a translatable block.
func TestOMMLNorBlockSurfaced(t *testing.T) {
	skel, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skel.Close()
	reader := NewReader()
	reader.SetSkeletonStore(skel)
	require.NoError(t, reader.Open(t.Context(), &model.RawDocument{
		URI: "t.docx", SourceLocale: model.LocaleEnglish, Encoding: "UTF-8",
		Reader: readCloserFromBytes(norEquationDocx(t)),
	}))
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	reader.Close()

	var nor *model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b := p.Resource.(*model.Block); b.Type == ommlNorBlockType {
				nor = b
			}
		}
	}
	require.NotNil(t, nor, "the <m:nor/> prose should surface as a translatable block")
	assert.True(t, nor.Translatable)
	assert.Equal(t, "where", nor.SourceText())
}

// An untranslated equation round-trips byte-exact — the sub-skeleton's verbatim
// segments + the nor ref rendering its source reproduce the original OMML.
func TestOMMLNorUntranslatedByteExact(t *testing.T) {
	out := writeBackNor(t, norEquationDocx(t), nil)
	xml := docxDocumentXML(t, out)
	// captureRawElement re-serializes the empty <m:nor/> as <m:nor></m:nor>; the
	// untranslated nor ref resolves to its source text, reproducing the rest verbatim.
	assert.Contains(t, xml,
		`<m:oMath><m:r><m:t>x</m:t></m:r><m:r><m:rPr><m:nor></m:nor></m:rPr><m:t>where</m:t></m:r><m:r><m:t>y</m:t></m:r></m:oMath>`,
		"untranslated equation round-trips through the sub-skeleton")
}

// Translating the prose splices the translation into the equation's OMML while
// the math structure is preserved.
func TestOMMLNorTranslatedSplice(t *testing.T) {
	out := writeBackNor(t, norEquationDocx(t), map[string]string{"where": "der"})
	xml := docxDocumentXML(t, out)
	assert.Contains(t, xml, `<m:rPr><m:nor></m:nor></m:rPr><m:t>der</m:t>`, "prose translated in place")
	assert.NotContains(t, xml, `<m:t>where</m:t>`, "original prose replaced")
	assert.Contains(t, xml, `<m:r><m:t>x</m:t></m:r>`, "math preserved")
	assert.Contains(t, xml, `<m:r><m:t>y</m:t></m:r>`, "math preserved")
	assert.Contains(t, xml, `<m:oMathPara><m:oMath>`, "equation structure preserved")
}
