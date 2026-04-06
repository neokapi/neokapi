package pdf_test

import (
	"bytes"
	"io"
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
