package pdf_test

import (
	"errors"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/formats/pdf"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReadMalformedPDF feeds truncated, garbage, and structurally hostile byte
// sequences through Open + Read and asserts the reader never panics. PDF text
// extraction is intentionally lenient — the scanner walks untrusted bytes
// between stream/endstream and BT/ET, balancing parens and decoding hex/octal
// escapes — so a non-PDF or corrupted PDF must drain to a clean close with no
// blocks rather than crashing. Run with -race to catch goroutine-unsafe slice
// arithmetic in extractStreams / extractTextOps.
func TestReadMalformedPDF(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input []byte
	}{
		{
			name:  "empty",
			input: []byte{},
		},
		{
			name:  "nil bytes",
			input: nil,
		},
		{
			name:  "not a pdf",
			input: []byte("definitely not a pdf at all"),
		},
		{
			name:  "header only",
			input: []byte("%PDF-1.7"),
		},
		{
			name:  "truncated after stream keyword",
			input: []byte("%PDF-1.7\n4 0 obj\n<< /Length 44 >>\nstream"),
		},
		{
			name:  "stream without endstream",
			input: []byte("%PDF-1.7\nstream\nBT (Hello) Tj ET\nthen it just stops"),
		},
		{
			name:  "endstream without stream",
			input: []byte("%PDF-1.7\nendstream endstream endstream"),
		},
		{
			name:  "stream then immediate eof",
			input: []byte("stream\r"),
		},
		{
			name:  "stream newline then eof",
			input: []byte("stream\r\n"),
		},
		{
			name:  "bt without et",
			input: []byte("stream\nBT /F1 12 Tf (no end text)\nendstream"),
		},
		{
			name:  "et without bt",
			input: []byte("stream\nET ET ET\nendstream"),
		},
		{
			name:  "unbalanced open paren",
			input: []byte("stream\nBT ((((((((( Tj ET\nendstream"),
		},
		{
			name:  "unbalanced close paren",
			input: []byte("stream\nBT )))))) Tj ET\nendstream"),
		},
		{
			name:  "trailing backslash escape at eof",
			input: []byte("stream\nBT (text\\"),
		},
		{
			name:  "lone backslash inside literal then eof",
			input: []byte("stream\nBT (\\"),
		},
		{
			name:  "unterminated hex string",
			input: []byte("stream\nBT <48656C6C6F"),
		},
		{
			name:  "odd-length hex string",
			input: []byte("stream\nBT <4>Tj ET\nendstream"),
		},
		{
			name:  "incomplete octal escape at eof",
			input: []byte("stream\nBT (abc\\1"),
		},
		{
			name:  "flatedecode header but garbage stream",
			input: []byte("<< /Filter /FlateDecode >>\nstream\n\x00\x01\x02\xff\xfe\xfd\nendstream"),
		},
		{
			name:  "dictionary opener angle brackets",
			input: []byte("stream\nBT << /a 1 >> Tj ET\nendstream"),
		},
		{
			name:  "random control bytes",
			input: []byte{0x00, 0x01, 0x02, 0xff, 0xfe, 0x73, 0x74, 0x72, 0x65, 0x61, 0x6d, 0x00, 0xff},
		},
		{
			name: "stream keyword near start (lookback clamp)",
			// "stream" within the first few bytes forces headerStart-lookback to
			// clamp at zero in extractStreams; exercises the slice-arithmetic edge.
			input: []byte("stream stream\nBT (x) Tj ET\nendstream"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := t.Context()
			reader := pdf.NewReader()

			require.NotPanics(t, func() {
				require.NoError(t, reader.Open(ctx, rawDocFromBytes(tt.input, model.LocaleEnglish)))
			})
			defer reader.Close()

			require.NotPanics(t, func() {
				for result := range reader.Read(ctx) {
					// Lenient extraction: corrupted PDFs yield no error, just no
					// text. We only require that nothing panics and the channel
					// drains cleanly. If a result ever carries an error it must be
					// a real error value (never a nil-wrapped surprise).
					if result.Error != nil {
						assert.Error(t, result.Error)
					}
				}
			})
		})
	}
}

// errReader always fails, simulating an I/O error on the underlying document
// stream (e.g. a network read that drops mid-transfer).
type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, errors.New("simulated read failure")
}

// TestReadIOError verifies that a read error on the underlying document is
// surfaced on the result channel as a clean PartResult.Error rather than being
// silently swallowed. This is the one genuine error path in the reader
// (io.ReadAll failing in readContent).
func TestReadIOError(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := pdf.NewReader()
	doc := &model.RawDocument{
		URI:          "test://broken.pdf",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(errReader{}),
	}
	require.NoError(t, reader.Open(ctx, doc))
	defer reader.Close()

	var foundError bool
	require.NotPanics(t, func() {
		for result := range reader.Read(ctx) {
			if result.Error != nil {
				foundError = true
			}
		}
	})
	assert.True(t, foundError, "expected a clean PartResult.Error when the underlying reader fails")
}

// TestReadNilReader verifies Open rejects a document whose Reader is nil
// without panicking (companion to TestReadNilDocument, which covers a nil doc).
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := pdf.NewReader()
	doc := &model.RawDocument{
		URI:          "test://no-reader.pdf",
		SourceLocale: model.LocaleEnglish,
		Reader:       nil,
	}
	var err error
	require.NotPanics(t, func() {
		err = reader.Open(ctx, doc)
	})
	require.Error(t, err)
}
