package model_test

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRawDocumentRandomAccess(t *testing.T) {
	content := []byte("PK\x03\x04 a package")

	t.Run("present", func(t *testing.T) {
		doc := &model.RawDocument{
			Reader:   io.NopCloser(bytes.NewReader(content)),
			ReaderAt: bytes.NewReader(content),
			Size:     int64(len(content)),
		}
		ra, size, ok := doc.RandomAccess()
		assert.True(t, ok)
		assert.Equal(t, int64(len(content)), size)

		buf := make([]byte, 2)
		n, err := ra.ReadAt(buf, 3)
		require.NoError(t, err)
		assert.Equal(t, 2, n)
		assert.Equal(t, content[3:5], buf)
	})

	t.Run("stream only", func(t *testing.T) {
		// The stdin shape: a forward-only stream, which readers must detect so
		// they fall back to buffering rather than parsing nothing.
		doc := &model.RawDocument{Reader: io.NopCloser(strings.NewReader("x"))}
		_, _, ok := doc.RandomAccess()
		assert.False(t, ok)
	})

	t.Run("incomplete", func(t *testing.T) {
		// A ReaderAt with no size is unusable — zip.NewReader needs both.
		doc := &model.RawDocument{ReaderAt: bytes.NewReader(content)}
		_, _, ok := doc.RandomAccess()
		assert.False(t, ok)

		doc = &model.RawDocument{Size: 10}
		_, _, ok = doc.RandomAccess()
		assert.False(t, ok)
	})

	t.Run("nil", func(t *testing.T) {
		var doc *model.RawDocument
		_, _, ok := doc.RandomAccess()
		assert.False(t, ok)
	})
}
