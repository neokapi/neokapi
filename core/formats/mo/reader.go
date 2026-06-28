package mo

import (
	"context"
	"errors"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader is a stub — the pipeline never reads MO as a source. It exists
// only so the format registry can answer `DetectByExtension(".mo")` and
// route `-o file.mo` output to the MO writer. Calling Open / Read
// returns an error pointing the caller at the appropriate gettext lib.
type Reader struct {
	format.BaseFormatReader
}

// NewReader constructs the stub reader.
func NewReader() *Reader {
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "mo",
			FormatDisplayName: "MO (Gettext, binary)",
			FormatExtensions:  []string{".mo"},
		},
	}
}

// Open always fails — MO is a runtime catalog format, not a pipeline
// source. Runtime consumers load MO via github.com/leonelquinteros/gotext.
func (r *Reader) Open(_ context.Context, _ *model.RawDocument) error {
	return errors.New("mo: read not supported — load compiled MO catalogs via gotext.NewMo().ParseFile at runtime, not through the pipeline")
}

// Read returns an already-closed channel so calls that slip past Open
// (e.g. test harnesses) don't hang.
func (r *Reader) Read(_ context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult)
	close(ch)
	return ch
}

// Config returns the writer's config (MO is write-only; same knobs apply).
func (r *Reader) Config() format.DataFormatConfig { return &Config{} }

// SetConfig accepts and validates a config.
func (r *Reader) SetConfig(cfg format.DataFormatConfig) error {
	if cfg == nil {
		return nil
	}
	if _, ok := cfg.(*Config); !ok {
		return errors.New("mo: expected *Config")
	}
	return nil
}

// Close is a no-op — the stub reader owns no resources.
func (r *Reader) Close() error { return nil }

// Signature returns the MO format's extension/MIME tuple so
// FormatRegistry.DetectByExtension(".mo") routes to this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/x-gettext-translation"},
		Extensions: []string{".mo"},
		Binary:     true, // gettext .mo is a binary catalog
	}
}
