// Package exec holds the registry entry for the `exec` format name.
//
// There is no exec extractor. The package once carried a subprocess runner —
// NUL-separated paths on stdin, NDJSON block records on stdout — reachable
// from nothing: no caller outside this package, and no handling in
// core/project's FormatSpec resolution to give a recipe's `format: {name:
// exec}` any meaning. Its tests exercised only the dead runner, and its
// `#nosec G204` justification ("argv supplied by trusted project config")
// assumed a recipe is trusted, which is the assumption E-06 exists to
// refute. It was deleted rather than wired up.
//
// What remains is a registry entry so the name resolves and kapi-desktop's
// FormatSelect keeps its option, plus a reader that refuses on Open. If a
// declarative subprocess extractor is ever wanted, it arrives with a design
// for how a recipe earns the right to name one — see
// web/docs/contribute/architecture/engine/e-06-execution-trust.md.
package exec

import (
	"context"
	"errors"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// FormatName is the name this format is registered under. It stays a constant
// so the registry and the exec-class classification in core/project agree
// without stringly-typing it in two places.
const FormatName = "exec"

// NewReader returns a reader that exists so `exec` resolves in the format
// registry, and therefore in UI surfaces like kapi-desktop's FormatSelect.
// It refuses on Open, because there is nothing behind the name: no code path
// anywhere runs a subprocess for this format.
func NewReader() format.DataFormatReader {
	return &stubReader{
		FormatName:        FormatName,
		FormatDisplayName: "Exec (subprocess extractor)",
	}
}

// stubReader implements DataFormatReader and always errors on Open/Read.
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
		"the exec format has no reader — kapi does not extract content by " +
			"running a subprocess. Name the file's real format instead " +
			"(`kapi formats` lists them), or convert it to one kapi reads",
	)
}

func (r *stubReader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 1)
	ch <- model.PartResult{Error: errors.New("exec reader: Open must be called first (and will reject)")}
	close(ch)
	return ch
}

func (r *stubReader) Close() error { return nil }
