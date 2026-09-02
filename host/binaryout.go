package host

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/mattn/go-isatty"
)

// The binary-output guard.
//
// `ksed` and `kconv` write a whole reconstructed document to standard output
// when nothing else is asked of them — which is sed's own contract, and the
// right one. For the formats the toolbox exists to serve, that document is
// binary: `ksed 's/Inc\./LLC/' report.docx` streams a zip at the terminal,
// wedging it in whatever escape state the deflate stream happens to spell.
//
// The input-side guard in binary.go cannot catch this. It only fires when no
// format claimed the file, so the formats kapi understands *best* — .docx,
// .pptx, .idml, .epub — are precisely the ones it waves through. This is the
// output-side half: the same NUL rule, applied to the bytes on their way out.
//
// The convention is gzip's. `gzip`, `xz`, `zstd` and `bzip2` all refuse to
// write compressed data to a terminal and name a way around it; `curl` warns
// and points at `--output -`. Two things about that shape are load-bearing:
//
//   - It is a refusal, not a substitution. isatty may decide presentation
//     (colour, columns, a pager) or stop the write outright, but it must never
//     change the bytes a command would otherwise emit — otherwise `ksed … >
//     out.docx` and `ksed … | cat > out.docx` produce different documents, and
//     the pipeline stops being a pipeline.
//   - It guards the destination, not the format id. Sniffing the payload
//     catches every writer, including plugin formats that no capability flag
//     in the registry would have declared.
//
// So the guard buffers the first binarySniffLen bytes, decides once, and either
// releases them and streams the rest untouched or fails having emitted nothing.
// Redirected or piped output never constructs a guard at all: the byte path off
// a terminal is exactly what it was.

// ErrBinaryStdout reports a document that was about to be written to a terminal
// as raw bytes. Callers wrap it with the escape hatch that fits their command.
var ErrBinaryStdout = errors.New("binary output not written to a terminal")

// errBinaryStdout builds the per-command error, naming the way out — the same
// shape as errBinaryInput on the read side.
func errBinaryStdout(hint string) error {
	return fmt.Errorf("%w: %s", ErrBinaryStdout, hint)
}

// stdoutIsTerminal reports whether standard output is attached to a terminal
// (so a document written there would land in front of a human). A var, like
// stdinIsPipe, so tests can drive both sides of the branch.
var stdoutIsTerminal = func() bool {
	fd := os.Stdout.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// guardedStdout returns the writer a toolbox command hands to a document
// writer, and the flush that completes it — which must be called once the
// writer is closed, since a document shorter than the sniff length is still
// sitting in the buffer at that point.
//
// Off a terminal, or when --encoding names a Unicode form whose text carries
// NUL bytes legitimately, this is os.Stdout and a no-op: no buffering, no
// sniffing, no behaviour to explain.
func (a *App) guardedStdout() (io.Writer, func() error) {
	if a.encodingAllowsNUL() || !stdoutIsTerminal() {
		return os.Stdout, func() error { return nil }
	}
	g := &binaryGuard{w: os.Stdout}
	return g, g.Flush
}

// binaryGuard withholds a document's opening bytes until it knows whether they
// are text. Once decided it is a pass-through in one direction and a hard stop
// in the other; it never emits a partial document.
type binaryGuard struct {
	w       io.Writer
	buf     []byte
	passing bool // decided text: buf released, writes go straight through
	failed  bool // decided binary: nothing was written, nothing will be
}

func (g *binaryGuard) Write(p []byte) (int, error) {
	switch {
	case g.failed:
		return 0, ErrBinaryStdout
	case g.passing:
		return g.w.Write(p)
	}
	g.buf = append(g.buf, p...)
	if len(g.buf) < binarySniffLen {
		// Reported as written: it is buffered, and the writer upstream has no
		// short-write recovery to offer that would help here.
		return len(p), nil
	}
	if err := g.decide(); err != nil {
		return 0, err
	}
	return len(p), nil
}

// Flush completes a document that never reached the sniff length, and reports
// the refusal for one that did — so a format writer that swallows write errors
// still cannot turn a blocked document into a successful command.
func (g *binaryGuard) Flush() error {
	switch {
	case g.failed:
		return ErrBinaryStdout
	case g.passing:
		return nil
	}
	return g.decide()
}

// decide inspects the buffered prefix once and settles the guard's direction.
func (g *binaryGuard) decide() error {
	if looksBinary(g.buf[:min(len(g.buf), binarySniffLen)]) {
		g.failed = true
		g.buf = nil // the whole point: these bytes never reach the terminal
		return ErrBinaryStdout
	}
	g.passing = true
	buf := g.buf
	g.buf = nil
	if len(buf) == 0 {
		return nil
	}
	_, err := g.w.Write(buf)
	return err
}
