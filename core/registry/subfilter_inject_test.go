package registry

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// spyReader records the resolver it was handed, which is the only thing
// SubfilterAware exposes and exactly what the injection contract is about.
type spyReader struct {
	format.BaseFormatReader
	resolver format.SubfilterResolver
}

func (s *spyReader) SetSubfilterResolver(r format.SubfilterResolver) { s.resolver = r }
func (s *spyReader) Signature() format.FormatSignature               { return format.FormatSignature{} }
func (s *spyReader) Open(context.Context, *model.RawDocument) error  { return nil }
func (s *spyReader) Read(context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult)
	close(ch)
	return ch
}
func (s *spyReader) Close() error { return nil }

type spyWriter struct {
	format.BaseFormatWriter
	resolver format.SubfilterResolver
}

func (s *spyWriter) SetSubfilterResolver(r format.SubfilterResolver) { s.resolver = r }
func (s *spyWriter) Write(context.Context, <-chan *model.Part) error { return nil }

// TestNewWriterInjectsSubfilterResolver covers the writer half: a container
// writer needs the resolver to reconstruct an embedded part through the
// sub-format writer that read it.
func TestNewWriterInjectsSubfilterResolver(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterWriter("spy", func() format.DataFormatWriter {
		return &spyWriter{FormatName: "spy"}
	})

	w, err := reg.NewWriter("spy")
	require.NoError(t, err)

	spy, ok := w.(*spyWriter)
	require.True(t, ok)
	require.NotNil(t, spy.resolver, "writer was constructed without a subfilter resolver")
	assert.Same(t, reg, spy.resolver)
}

// TestNewReaderInjectsSubfilterResolver pins the contract in
// core/format/subfilter.go: a SubfilterAware reader receives a resolver before
// anything calls Open. It went unenforced for the life of the interface —
// SetSubfilterResolver was called only by per-format tests that inject by hand,
// so a reader that never got one in production looked healthy in CI.
func TestNewReaderInjectsSubfilterResolver(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("spy",
		func() format.DataFormatReader {
			return &spyReader{FormatName: "spy"}
		},
		format.FormatSignature{Extensions: []string{".spy"}}, "Spy")

	r, err := reg.NewReader("spy")
	require.NoError(t, err)

	spy, ok := r.(*spyReader)
	require.True(t, ok)
	require.NotNil(t, spy.resolver, "reader was constructed without a subfilter resolver")
	assert.Same(t, reg, spy.resolver, "the resolver should be the registry the reader came from")
}

// TestResolveReaderInjectsSubfilterResolver covers the nesting case: a reader
// obtained *through* a resolver must itself be resolver-equipped, or subfilters
// only compose one level deep (EPUB → HTML would work, HTML → embedded JSON
// would not).
func TestResolveReaderInjectsSubfilterResolver(t *testing.T) {
	reg := NewFormatRegistry()
	reg.RegisterReader("spy",
		func() format.DataFormatReader {
			return &spyReader{FormatName: "spy"}
		},
		format.FormatSignature{Extensions: []string{".spy"}}, "Spy")

	r, err := reg.ResolveReader("spy")
	require.NoError(t, err)
	spy, ok := r.(*spyReader)
	require.True(t, ok)
	assert.Same(t, reg, spy.resolver)
}
