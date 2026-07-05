package format

import (
	"context"

	"github.com/neokapi/neokapi/core/model"
)

// FormatDescriptor is the identification/detection facet of a format reader:
// its unique name, human-readable name, and detection metadata. Consumers that
// only need to identify a format (listings, detection plumbing) can depend on
// this instead of the full DataFormatReader.
type FormatDescriptor interface {
	// Name returns the unique identifier for this format (e.g., "html", "xliff").
	Name() string

	// DisplayName returns a human-readable name (e.g., "HTML Filter").
	DisplayName() string

	// Signature returns detection metadata: MIME types, extensions, and content signatures.
	Signature() FormatSignature
}

// Configurable is the typed-config facet of a format reader: access to and
// replacement of its DataFormatConfig. Consumers that only apply configuration
// (project overrides, recipe config maps) can depend on this instead of the
// full DataFormatReader.
type Configurable interface {
	// Config returns the current configuration.
	Config() DataFormatConfig

	// SetConfig applies a new configuration.
	SetConfig(cfg DataFormatConfig) error
}

// PartReader is the lifecycle facet of a format reader: open a document,
// stream its Parts, release resources.
type PartReader interface {
	// Open opens a RawDocument for reading. Call Read() to stream Parts.
	Open(ctx context.Context, doc *model.RawDocument) error

	// Read returns a channel of PartResults. The channel is closed when
	// the document is fully read or an error occurs. Context cancellation
	// stops reading.
	Read(ctx context.Context) <-chan model.PartResult

	// Close releases resources.
	Close() error
}

// DataFormatReader reads a document and produces a stream of Parts. It is the
// composition of the format's identity/detection metadata (FormatDescriptor),
// its typed configuration surface (Configurable), and its read lifecycle
// (PartReader). Implementers typically embed BaseFormatReader (which covers
// Name/DisplayName/Config/SetConfig) and provide Signature plus the lifecycle
// themselves.
type DataFormatReader interface {
	FormatDescriptor
	Configurable
	PartReader
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
	// StreamingReader marks the capability. The method is a bare marker: its
	// presence is the signal, probed via IsStreamingReader; it is never called.
	StreamingReader()
}

// IsStreamingReader reports whether r declares the StreamingReader capability
// (streams its input incrementally with bounded memory). It centralizes the
// capability assertion for the file-run path.
func IsStreamingReader(r DataFormatReader) bool {
	_, ok := r.(StreamingReader)
	return ok
}

// FormatSignature describes how to detect a data format.
type FormatSignature struct {
	MIMETypes  []string          // e.g., ["text/html", "application/xhtml+xml"]
	Extensions []string          // e.g., [".html", ".htm", ".xhtml"]
	MagicBytes [][]byte          // Byte prefixes to match
	Sniff      func([]byte) bool // Custom content sniffing function (optional)
	// Binary marks a format whose reader consumes binary content (e.g. gettext
	// .mo, video containers). It tells detection NOT to decline this format for
	// binary input the way it does for a text format matched only by extension.
	// Formats that declare MagicBytes or a Sniff are already treated as binary-
	// capable; set this only for binary formats that have neither.
	Binary bool
}
