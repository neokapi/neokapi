package pdf_test

import (
	"bytes"
	"io"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/formats/pdf"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Minimal valid PDF with text "Hello World"
const minimalPDF = "%PDF-1.0\n" +
	"1 0 obj\n" +
	"<< /Type /Catalog /Pages 2 0 R >>\n" +
	"endobj\n" +
	"2 0 obj\n" +
	"<< /Type /Pages /Kids [3 0 R] /Count 1 >>\n" +
	"endobj\n" +
	"3 0 obj\n" +
	"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\n" +
	"endobj\n" +
	"4 0 obj\n" +
	"<< /Length 44 >>\n" +
	"stream\n" +
	"BT /F1 12 Tf 100 700 Td (Hello World) Tj ET\n" +
	"endstream\n" +
	"endobj\n" +
	"5 0 obj\n" +
	"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\n" +
	"endobj\n" +
	"xref\n" +
	"0 6\n" +
	"0000000000 65535 f \n" +
	"0000000009 00000 n \n" +
	"0000000058 00000 n \n" +
	"0000000115 00000 n \n" +
	"0000000266 00000 n \n" +
	"0000000360 00000 n \n" +
	"trailer\n" +
	"<< /Size 6 /Root 1 0 R >>\n" +
	"startxref\n" +
	"441\n" +
	"%%EOF\n"

// PDF with TJ array operator
const tjArrayPDF = "%PDF-1.0\n" +
	"1 0 obj\n" +
	"<< /Type /Catalog /Pages 2 0 R >>\n" +
	"endobj\n" +
	"2 0 obj\n" +
	"<< /Type /Pages /Kids [3 0 R] /Count 1 >>\n" +
	"endobj\n" +
	"3 0 obj\n" +
	"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R /Resources << /Font << /F1 5 0 R >> >> >>\n" +
	"endobj\n" +
	"4 0 obj\n" +
	"<< /Length 66 >>\n" +
	"stream\n" +
	"BT /F1 12 Tf 100 700 Td [(Hello) -10 ( ) -5 (World)] TJ ET\n" +
	"endstream\n" +
	"endobj\n" +
	"5 0 obj\n" +
	"<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>\n" +
	"endobj\n" +
	"xref\n" +
	"0 6\n" +
	"trailer\n" +
	"<< /Size 6 /Root 1 0 R >>\n" +
	"startxref\n" +
	"0\n" +
	"%%EOF\n"

func rawDocFromBytes(data []byte, sourceLocale model.LocaleID) *model.RawDocument {
	return &model.RawDocument{
		URI:          "test://input.pdf",
		SourceLocale: sourceLocale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(data)),
	}
}

func TestReadMinimalPDF(t *testing.T) {
	ctx := t.Context()
	reader := pdf.NewReader()
	err := reader.Open(ctx, rawDocFromBytes([]byte(minimalPDF), model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(blocks), 1)
	assert.Contains(t, blocks[0].SourceText(), "Hello World")
}

func TestReadTJArray(t *testing.T) {
	ctx := t.Context()
	reader := pdf.NewReader()
	err := reader.Open(ctx, rawDocFromBytes([]byte(tjArrayPDF), model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(blocks), 1)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Hello")
	assert.Contains(t, text, "World")
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := pdf.NewReader()
	err := reader.Open(ctx, rawDocFromBytes([]byte(minimalPDF), model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	// Should have at least: doc layer start, page layer start, block, page layer end, doc layer end
	require.GreaterOrEqual(t, len(parts), 5)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "pdf", layer.Format)
}

func TestPageLayers(t *testing.T) {
	ctx := t.Context()
	reader := pdf.NewReader()
	err := reader.Open(ctx, rawDocFromBytes([]byte(minimalPDF), model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	// Find page layers
	var pageLayers []*model.Layer
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			l := p.Resource.(*model.Layer)
			if l.Properties != nil {
				if _, ok := l.Properties["page-number"]; ok {
					pageLayers = append(pageLayers, l)
				}
			}
		}
	}

	require.GreaterOrEqual(t, len(pageLayers), 1)
	assert.Equal(t, "1", pageLayers[0].Properties["page-number"])
}

func TestReaderSignature(t *testing.T) {
	reader := pdf.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "application/pdf")
	assert.Contains(t, sig.Extensions, ".pdf")
	assert.Equal(t, [][]byte{[]byte("%PDF-")}, sig.MagicBytes)
}

func TestReaderMetadata(t *testing.T) {
	reader := pdf.NewReader()
	assert.Equal(t, "pdf", reader.Name())
	assert.Equal(t, "PDF Text Extraction", reader.DisplayName())
}

// okapi: PdfFilterTest#testDefaultInfo
// Okapi's testDefaultInfo asserts the filter exposes non-null parameters, a
// non-null name, and a non-empty configuration list. The native analog: the
// reader has a non-nil Config, a non-empty Name, and a Signature that
// advertises the .pdf extension and application/pdf MIME type.
func TestDefaultInfo(t *testing.T) {
	reader := pdf.NewReader()
	assert.NotNil(t, reader.Config())
	assert.NotEmpty(t, reader.Name())
	sig := reader.Signature()
	assert.Contains(t, sig.Extensions, ".pdf")
	assert.Contains(t, sig.MIMETypes, "application/pdf")
}

// okapi: PdfFilterTest#testStartDocument
// Okapi's testStartDocument opens a real-world PDF (OmegaT_documentation_en.PDF)
// and asserts a well-formed StartDocument event. The native analog opens a real
// PDF fixture and asserts the reader emits a well-formed document layer-start
// (Format "pdf") with no read error. NOTE: the native PDF reader is a
// regex-based text extractor with known limits (#510) and does not recover
// translatable text from this font-encoded, compressed document — but opening
// and starting the document is well-defined and is what this contract checks.
func TestStartDocument(t *testing.T) {
	ctx := t.Context()
	data, err := os.ReadFile("testdata/TAUS-QualityDashboard-September.pdf")
	require.NoError(t, err)

	reader := pdf.NewReader()
	require.NoError(t, reader.Open(ctx, rawDocFromBytes(data, model.LocaleEnglish)))
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx)) // fails on any read error
	require.NotEmpty(t, parts, "real PDF should emit at least a document layer-start")
	require.Equal(t, model.PartLayerStart, parts[0].Type)
	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "pdf", layer.Format)
}

// firstTextUnit and firstParagraphTextUnit assert exact extracted text from
// three real-world PDFs (e.g. the first unit equals "TAUS Quality Dashboard",
// the third paragraph starts with "Abstract: In large computer-aided
// translation"). neokapi's native PDF reader is a regex-based text extractor
// (#510) that handles uncompressed text streams only; it recovers zero text
// units from these font-encoded/compressed documents (verified: 0 blocks
// extracted), and it has no lineSeparator/paragraphSeparator parameters. These
// contracts are honest gaps in the native reader, skip-classified rather than
// fake-passed:
//
// okapi-skip: PdfFilterTest#firstTextUnit — native regex-based PDF reader (#510) extracts no text from these compressed/font-encoded real-world PDFs; cannot assert the first text unit's content
// okapi-skip: PdfFilterTest#firstParagraphTextUnit — native regex-based PDF reader (#510) extracts no paragraphs from these compressed real-world PDFs, and has no lineSeparator/paragraphSeparator config to drive paragraph segmentation

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := pdf.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadEmptyPDF(t *testing.T) {
	ctx := t.Context()
	reader := pdf.NewReader()
	err := reader.Open(ctx, rawDocFromBytes([]byte("%PDF-1.0\n%%EOF\n"), model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks)
}

func TestWriterOutputsPlainText(t *testing.T) {
	ctx := t.Context()

	reader := pdf.NewReader()
	err := reader.Open(ctx, rawDocFromBytes([]byte(minimalPDF), model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := pdf.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	output := buf.String()
	assert.Contains(t, output, "Hello World")
	// Writer should output plain text, not PDF format
	assert.NotContains(t, output, "%PDF")
}

func TestWriterName(t *testing.T) {
	writer := pdf.NewWriter()
	assert.Equal(t, "pdf", writer.Name())
}
