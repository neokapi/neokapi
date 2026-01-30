package format

import (
	"context"

	"github.com/gokapi/gokapi/core/model"
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

// FormatSignature describes how to detect a data format.
type FormatSignature struct {
	MIMETypes  []string          // e.g., ["text/html", "application/xhtml+xml"]
	Extensions []string          // e.g., [".html", ".htm", ".xhtml"]
	MagicBytes [][]byte          // Byte prefixes to match
	Sniff      func([]byte) bool // Custom content sniffing function (optional)
}
