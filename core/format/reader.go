package format

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
)

// DataFormatReader reads a document and produces a stream of Parts.
type DataFormatReader interface {
	// Name returns the unique identifier for this format (e.g., "html", "xliff").
	Name() string

	// DisplayName returns a human-readable name (e.g., "HTML Filter").
	DisplayName() string

	// Signature returns detection metadata: MIME types, extensions, and content signatures.
	Signature() FormatSignature

	// Open opens a RawDocument for reading. Call Read() to stream Parts.
	Open(ctx context.Context, doc *model.RawDocument) error

	// Read returns a channel of PartResults. The channel is closed when
	// the document is fully read or an error occurs. Context cancellation
	// stops reading.
	Read(ctx context.Context) <-chan model.PartResult

	// Close releases resources.
	Close() error

	// Config returns the current configuration.
	Config() DataFormatConfig

	// SetConfig applies a new configuration.
	SetConfig(cfg DataFormatConfig) error
}

// StreamingReader is a capability marker a reader implements to declare that it
// reads its input incrementally from doc.Reader (via bufio, never io.ReadAll /
// os.ReadFile) and emits Parts incrementally — so it can be fed straight into
// the executor without the whole input or the whole Part stream being buffered.
// It also promises the reader is in-process and pure-Go (never a daemon-backed
// plugin), so its read may safely overlap a same-format writer's write: there is
// no "one Process stream at a time" ordering constraint to honour.
//
// The file-run path (core/flow.FileRunner) consults this marker to choose the
// bounded-memory streaming path; readers that do not implement it keep the
// read-fully-then-write buffered path unchanged. See
// [AD-005](../../web/docs/contribute/architecture/005-format-system.md)
// "Streaming readers and bounded-memory I/O".
type StreamingReader interface {
	// StreamingReader reports that this reader streams its input. The method
	// exists only to mark the capability; its presence (not its return value,
	// which is always true) is what the file-run path probes.
	StreamingReader() bool
}

// FormatSignature describes how to detect a data format.
type FormatSignature struct {
	MIMETypes  []string          // e.g., ["text/html", "application/xhtml+xml"]
	Extensions []string          // e.g., [".html", ".htm", ".xhtml"]
	MagicBytes [][]byte          // Byte prefixes to match
	Sniff      func([]byte) bool // Custom content sniffing function (optional)
}
