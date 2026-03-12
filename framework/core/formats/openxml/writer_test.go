package openxml

import (
	"bytes"
	"context"
	"os"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/testutil"
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
	assert.Error(t, err, "should error without original content")
}
