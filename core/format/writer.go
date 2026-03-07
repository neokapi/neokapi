package format

import (
	"context"
	"io"

	"github.com/gokapi/gokapi/core/model"
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
