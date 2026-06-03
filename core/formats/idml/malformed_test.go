package idml

import (
	"archive/zip"
	"bytes"
	"io"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// zipWith builds an in-memory ZIP archive from the given name->content map.
// It is used to construct structurally valid IDML packages (a ZIP whose
// entries are nonetheless semantically broken) so the reader's XML-parse
// error path is exercised without a real .idml fixture.
func zipWith(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		require.NoError(t, err)
		_, err = io.WriteString(w, content)
		require.NoError(t, err)
	}
	require.NoError(t, zw.Close())
	return buf.Bytes()
}

// readDoc opens a RawDocument backed by data and drains the result channel,
// reporting whether any PartResult carried an error. It asserts NotPanics
// around both Open and the full Read drain so a panic anywhere in the
// pipeline fails the test loudly rather than crashing the goroutine.
func readDoc(t *testing.T, data []byte) (openErr error, foundReadError bool) {
	t.Helper()
	ctx := t.Context()
	reader := NewReader()
	require.NotPanics(t, func() {
		openErr = reader.Open(ctx, &model.RawDocument{
			URI:          "test.idml",
			SourceLocale: model.LocaleEnglish,
			Encoding:     "UTF-8",
			MimeType:     "application/vnd.adobe.indesign-idml-package",
			Reader:       io.NopCloser(bytes.NewReader(data)),
		})
	})
	if openErr != nil {
		return openErr, false
	}
	defer reader.Close()
	require.NotPanics(t, func() {
		for result := range reader.Read(ctx) {
			if result.Error != nil {
				foundReadError = true
			}
		}
	})
	return nil, foundReadError
}

// TestReadMalformedPackage feeds a table of broken IDML inputs and asserts
// that the reader never panics and that every malformed package surfaces a
// clean error on the result channel (PartResult.Error) rather than silently
// emitting a truncated or empty document. This is the L1->L2 malformed gate;
// it must pass under -race.
func TestReadMalformedPackage(t *testing.T) {
	t.Parallel()

	// A structurally valid ZIP whose only Story entry contains malformed XML
	// (unterminated <Content> tag). The reader reaches parseStory and the XML
	// decoder must surface the error rather than panic.
	corruptStoryZip := zipWith(t, map[string]string{
		"Stories/Story_u100.xml": `<?xml version="1.0"?><Story><ParagraphStyleRange><CharacterStyleRange><Content>broken`,
	})

	tests := []struct {
		name  string
		input []byte
	}{
		{
			// Not a ZIP at all: random non-archive bytes.
			name:  "garbage bytes",
			input: []byte("this is definitely not an IDML package \x00\x01\x02\xff"),
		},
		{
			// Empty input: zip.NewReader rejects a zero-length archive.
			name:  "empty input",
			input: []byte{},
		},
		{
			// A truncated ZIP: the PK local-file header magic with nothing
			// behind it. zip.NewReader must reject it as not a valid archive.
			name:  "truncated zip header",
			input: []byte{0x50, 0x4B, 0x03, 0x04, 0x00, 0x00},
		},
		{
			// A real ZIP truncated mid-stream: take a valid archive and cut it
			// short so the central directory is gone.
			name:  "truncated valid zip",
			input: corruptStoryZip[:len(corruptStoryZip)/2],
		},
		{
			// A structurally valid ZIP whose story XML is corrupt.
			name:  "valid zip corrupt story xml",
			input: corruptStoryZip,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			openErr, foundReadError := readDoc(t, tt.input)
			require.NoError(t, openErr, "Open validates only the document/reader; parse errors must surface during Read")
			assert.True(t, foundReadError, "expected a clean error on the result channel for malformed IDML input")
		})
	}
}

// TestReadNilDocument verifies Open rejects a nil document without panicking.
func TestReadNilDocument(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := NewReader()
	var err error
	require.NotPanics(t, func() {
		err = reader.Open(ctx, nil)
	})
	require.Error(t, err)
}

// TestReadNilReader verifies Open rejects a document with a nil Reader
// without panicking. IDML needs random access to a backing byte stream;
// a nil reader cannot be unzipped.
func TestReadNilReader(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := NewReader()
	var err error
	require.NotPanics(t, func() {
		err = reader.Open(ctx, &model.RawDocument{
			URI:          "test.idml",
			SourceLocale: model.LocaleEnglish,
			Reader:       nil,
		})
	})
	require.Error(t, err)
}
