package designtokens

import (
	"context"
	"io"

	"github.com/neokapi/neokapi/core/format"
	jsonfmt "github.com/neokapi/neokapi/core/formats/json"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for W3C DTCG design-token files. Writing is
// delegated to the generic JSON writer: design-token blocks ($description
// values) carry the same json.keypath / json.original bookkeeping as plain JSON
// blocks, so the JSON writer's token-level value replacement reproduces the
// document byte-faithfully. Token values and structure arrive as
// non-translatable Data and are replayed verbatim from the original tokens.
//
// Unlike the i18next wrapper, no inline-code flattening is needed: the
// design-tokens reader does not enable the JSON code-finder ($description is
// plain prose with no protected interpolation syntax), so blocks are already
// flat text and pass straight through to the inner writer.
type Writer struct {
	format.BaseFormatWriter
	cfg   *Config
	inner *jsonfmt.Writer
}

// Ensure Writer consumes a byte-exact skeleton by forwarding the store to the
// inner JSON writer, whose writeFromSkeleton path reproduces the document.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new design-tokens writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	w := &Writer{
		BaseFormatWriter: format.BaseFormatWriter{FormatName: formatID},
		cfg:              cfg,
	}
	w.inner = jsonfmt.NewWriter()
	// Apply the design-tokens-derived settings (notably escapeForwardSlashes) to
	// the inner writer's live config. Config() returns the writer's own pointer,
	// so mutating it in place is what actually takes effect.
	cfg.applyToJSON(w.inner.Config())
	return w
}

// SetOutput configures the output destination by path on the inner writer.
func (w *Writer) SetOutput(path string) error { return w.inner.SetOutput(path) }

// SetOutputWriter configures an io.Writer as output on the inner writer.
func (w *Writer) SetOutputWriter(out io.Writer) error { return w.inner.SetOutputWriter(out) }

// SetLocale sets the target locale on the inner writer.
func (w *Writer) SetLocale(locale model.LocaleID) {
	w.Locale = locale
	w.inner.SetLocale(locale)
}

// SetEncoding sets the output encoding on the inner writer.
func (w *Writer) SetEncoding(encoding string) { w.inner.SetEncoding(encoding) }

// SetSkeletonStore forwards the skeleton store to the inner JSON writer, whose
// writeFromSkeleton path reproduces the document byte-for-byte, splicing only
// translated $description values at each block reference.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.inner.SetSkeletonStore(store)
}

// Write delegates serialization to the inner JSON writer. Parts pass through
// unchanged; the JSON writer splices translated $description values into the
// original token stream and replays everything else byte-for-byte.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	return w.inner.Write(ctx, parts)
}

// Close flushes and closes the inner writer.
func (w *Writer) Close() error { return w.inner.Close() }
