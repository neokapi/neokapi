package exec

import (
	"context"
	"errors"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// NewReader returns a stub DataFormatReader that exists so `exec`
// appears in the kapi format registry (and therefore in UI
// surfaces like kapi-desktop's FormatSelect). The actual
// extraction pipeline for `exec` runs out-of-band in
// `kapi extract -p`, which reads the FormatSpec directly from the
// project file and spawns the declared command once per collection.
//
// A user who tries to use `exec` as a per-file reader (via a
// `--map` flag on a single-file tool command, or a flow step
// that opens a `.tsx` as if it were extractable without orchestration)
// gets a clear error pointing them at the right path.
func NewReader() format.DataFormatReader {
	return &stubReader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        FormatName,
			FormatDisplayName: "Exec (subprocess extractor)",
		},
	}
}

// stubReader implements DataFormatReader but intentionally errors on
// Open/Read. The registry entry exists for discoverability; running
// actual extraction goes through `kapi extract -p project.kapi`.
type stubReader struct {
	format.BaseFormatReader
}

func (r *stubReader) Signature() format.FormatSignature {
	return format.FormatSignature{
		// No extensions / MIMEs — `exec` is opt-in per project,
		// never auto-dispatched by file type.
	}
}

func (r *stubReader) Open(ctx context.Context, doc *model.RawDocument) error {
	return errors.New(
		"exec format is declarative — configure a collection's format: " +
			"{ name: exec, config: { command: ... } } in a .kapi project, " +
			"then run `kapi extract -p project.kapi`",
	)
}

func (r *stubReader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 1)
	ch <- model.PartResult{Error: errors.New("exec reader: Open must be called first (and will reject)")}
	close(ch)
	return ch
}

func (r *stubReader) Close() error { return nil }
