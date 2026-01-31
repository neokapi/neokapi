package registry

import (
	"context"
	"testing"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// stubReader is a minimal DataFormatReader for testing.
type stubReader struct {
	format.BaseFormatReader
}

func newStubReader(name string) *stubReader {
	return &stubReader{
		BaseFormatReader: format.BaseFormatReader{FormatName: name},
	}
}

func (s *stubReader) Signature() format.FormatSignature { return format.FormatSignature{} }
func (s *stubReader) Open(_ context.Context, _ *model.RawDocument) error {
	return nil
}
func (s *stubReader) Read(_ context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult)
	close(ch)
	return ch
}
func (s *stubReader) Close() error { return nil }

// stubWriter is a minimal DataFormatWriter for testing.
type stubWriter struct {
	format.BaseFormatWriter
}

func newStubWriter(name string) *stubWriter {
	return &stubWriter{
		BaseFormatWriter: format.BaseFormatWriter{FormatName: name},
	}
}

func (s *stubWriter) Write(_ context.Context, _ <-chan *model.Part) error { return nil }

func TestNewReaderExact(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("csv", func() format.DataFormatReader {
		return newStubReader("csv")
	})

	r, err := reg.NewReader("csv")
	require.NoError(t, err)
	assert.Equal(t, "csv", r.Name())
}

func TestNewReaderUnknown(t *testing.T) {
	reg := NewFormatRegistry()
	_, err := reg.NewReader("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown format")
}

func TestNewReaderVersionedExact(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("okapi-html@1.46.0", func() format.DataFormatReader {
		return newStubReader("okapi-html@1.46.0")
	})

	r, err := reg.NewReader("okapi-html@1.46.0")
	require.NoError(t, err)
	assert.Equal(t, "okapi-html@1.46.0", r.Name())
}

func TestNewReaderVersionedFallback(t *testing.T) {
	reg := NewFormatRegistry()
	// Register versioned entries only (no bare name).
	reg.RegisterReader("okapi-html@1.46.0", func() format.DataFormatReader {
		return newStubReader("okapi-html@1.46.0")
	})
	reg.RegisterReader("okapi-html@1.47.0", func() format.DataFormatReader {
		return newStubReader("okapi-html@1.47.0")
	})
	reg.RegisterReader("okapi-html@1.45.0", func() format.DataFormatReader {
		return newStubReader("okapi-html@1.45.0")
	})

	// Requesting bare name should fall back to latest version.
	r, err := reg.NewReader("okapi-html")
	require.NoError(t, err)
	assert.Equal(t, "okapi-html@1.47.0", r.Name())
}

func TestNewReaderBareNamePreferred(t *testing.T) {
	reg := NewFormatRegistry()
	// Register both bare name and versioned entries.
	reg.RegisterReader("okapi-html", func() format.DataFormatReader {
		return newStubReader("okapi-html-bare")
	})
	reg.RegisterReader("okapi-html@1.46.0", func() format.DataFormatReader {
		return newStubReader("okapi-html@1.46.0")
	})

	// Bare name should match directly, not fall through to versioned.
	r, err := reg.NewReader("okapi-html")
	require.NoError(t, err)
	assert.Equal(t, "okapi-html-bare", r.Name())
}

func TestNewWriterVersionedFallback(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterWriter("okapi-html@1.46.0", func() format.DataFormatWriter {
		return newStubWriter("okapi-html@1.46.0")
	})
	reg.RegisterWriter("okapi-html@1.47.0", func() format.DataFormatWriter {
		return newStubWriter("okapi-html@1.47.0")
	})

	w, err := reg.NewWriter("okapi-html")
	require.NoError(t, err)
	assert.Equal(t, "okapi-html@1.47.0", w.Name())
}

func TestNewWriterUnknown(t *testing.T) {
	reg := NewFormatRegistry()
	_, err := reg.NewWriter("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown format writer")
}

func TestInternalCompareSemver(t *testing.T) {
	assert.Equal(t, 0, compareSemver("1.0.0", "1.0.0"))
	assert.Equal(t, -1, compareSemver("1.0.0", "2.0.0"))
	assert.Equal(t, 1, compareSemver("2.0.0", "1.0.0"))
	assert.Equal(t, 1, compareSemver("1.47.0", "1.46.0"))
}
