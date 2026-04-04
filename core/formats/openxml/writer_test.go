package openxml

import (
	"archive/zip"
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriterBasic(t *testing.T) {
	// Read original
	original, err := os.ReadFile("testdata/simple.docx")
	require.NoError(t, err)

	f, err := os.Open("testdata/simple.docx")
	require.NoError(t, err)

	reader := NewReader()
	doc := testutil.RawDocFromReader(f, "testdata/simple.docx", model.LocaleEnglish)
	err = reader.Open(context.Background(), doc)
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(context.Background()))
	reader.Close()

	// Write
	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(original)
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(context.Background(), ch)
	require.NoError(t, err)
	writer.Close()

	// Output should be a valid ZIP
	assert.True(t, buf.Len() > 0, "output should not be empty")
	assert.Equal(t, byte(0x50), buf.Bytes()[0], "should start with PK")
	assert.Equal(t, byte(0x4B), buf.Bytes()[1])
}

func TestWriterNilOriginal(t *testing.T) {
	writer := NewWriter()
	var buf bytes.Buffer
	err := writer.SetOutputWriter(&buf)
	require.NoError(t, err)

	ch := make(chan *model.Part)
	close(ch)

	err = writer.Write(context.Background(), ch)
	require.Error(t, err, "should error without original content")
}

func TestWriterMediaReplacement(t *testing.T) {
	// Build a DOCX with an embedded PNG.
	docxFile := buildDocxWithMedia(t)
	original, err := os.ReadFile(docxFile.Name())
	require.NoError(t, err)

	// Read the original.
	f, err := os.Open(docxFile.Name())
	require.NoError(t, err)

	reader := NewReader()
	doc := testutil.RawDocFromReader(f, "test.docx", model.LocaleEnglish)
	require.NoError(t, reader.Open(context.Background(), doc))
	parts := testutil.CollectParts(t, reader.Read(context.Background()))
	reader.Close()

	// Write with a media replacement.
	replacementPNG := []byte("REPLACED-IMAGE-DATA")
	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(original)
	writer.SetMediaReplacement("word/media/test.png", replacementPNG)
	require.NoError(t, writer.SetOutputWriter(&buf))

	ch := testutil.PartsToChannel(parts)
	require.NoError(t, writer.Write(context.Background(), ch))
	writer.Close()

	// Verify the output ZIP contains the replacement.
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)

	for _, f := range zr.File {
		if f.Name == "word/media/test.png" {
			data, err := readZipFile(f)
			require.NoError(t, err)
			assert.Equal(t, replacementPNG, data, "media should be replaced with locale variant")
			return
		}
	}
	t.Fatal("word/media/test.png not found in output ZIP")
}
