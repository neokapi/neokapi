package format

import (
	"context"
	"io"

	"github.com/neokapi/neokapi/core/model"
)

// OriginalContentSetter is implemented by writers that need the original
// document content as a skeleton (e.g., bridge format writers, native HTML writer).
type OriginalContentSetter interface {
	SetOriginalContent(content []byte)
}

// SourcePathSetter is implemented by writers that can read the original
// document from a file path instead of receiving content bytes.
// When the file is on a shared filesystem, this avoids loading the entire
// document into memory and transferring it over gRPC.
type SourcePathSetter interface {
	SetSourcePath(path string)
}

// SourceLocaleSetter is implemented by writers that need to know the
// source (input) locale, in addition to the target locale set via
// SetLocale. Format writers that rewrite locale-bearing attributes on
// round-trip (e.g. OOXML's <w:lang>/<w:themeFontLang> w:val) need both
// the source and target locale to mirror the upstream Okapi behavior:
// "if the existing value's primary language matches the source, replace
// it with the target". Writers that don't care can simply not implement.
type SourceLocaleSetter interface {
	SetSourceLocale(locale model.LocaleID)
}

// WriterConfigurable is implemented by writers that expose
// serialization knobs (line endings, declaration emission, quote
// strategy, etc.). The returned config follows the same
// DataFormatConfig contract used by readers, so existing CLI
// introspection (formats info / formats schema), .kapi recipe loading,
// and ApplyMap plumbing all extend to writers without parallel
// machinery.
//
// Returns nil for writers with no configurable surface — callers
// should treat that as "no knobs available" rather than an error.
type WriterConfigurable interface {
	WriterConfig() DataFormatConfig
}

// StreamingWriter is a capability marker a writer implements to declare that it
// can reconstruct a document by consuming a *streaming* skeleton interleaved
// with the arriving Part stream — pulling each block referenced by a skeleton
// ref on demand rather than buffering every block into a map first. This keeps
// peak memory bounded to a small window for a streaming round-trip while
// remaining byte-identical to the buffered skeleton path (the reader emits
// skeleton refs and their blocks in the same order, so on-demand lookup yields
// the same bytes).
//
// A writer signals it took the streaming path by checking
// SkeletonStore.IsStreaming() inside Write and delegating to
// format.StreamSkeletonWrite. Writers that do not implement this keep the
// buffered (collect-all-blocks) skeleton path; the file-run path only wires a
// streaming skeleton store when both the reader (StreamingReader) and the writer
// (StreamingWriter) opt in. See
// [AD-005](../../web/docs/contribute/architecture/005-format-system.md)
// "Streaming readers and bounded-memory I/O".
type StreamingWriter interface {
	// StreamingWriter reports that this writer can consume a streaming skeleton.
	// As with StreamingReader, presence is the signal; it always returns true.
	StreamingWriter() bool
}

// DataFormatWriter reconstructs a document from Parts.
type DataFormatWriter interface {
	// Name returns the format name matching the reader.
	Name() string

	// SetOutput configures the output destination by path.
	SetOutput(path string) error

	// SetOutputWriter configures an io.Writer as output.
	SetOutputWriter(w io.Writer) error

	// SetLocale sets the target locale for writing.
	SetLocale(locale model.LocaleID)

	// SetEncoding sets the output encoding.
	SetEncoding(encoding string)

	// Write consumes Parts from a channel and writes the reconstructed document.
	// Returns when the channel is closed or context is canceled.
	Write(ctx context.Context, parts <-chan *model.Part) error

	// Close flushes and closes the output.
	Close() error
}
