package host

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// docSource is a toolbox command's input document: an open file handle, or
// standard input buffered in memory.
//
// The toolbox used to read every input whole (os.ReadFile) before doing
// anything with it, because format detection wanted content and the readers
// wanted an io.Reader. For a package format that was two full copies of the
// file — the CLI's buffer, and the copy the reader made to get random access
// over the ZIP — so a 100 MB .docx cost several hundred megabytes to convert.
// A file source hands out the handle instead: detection reads a prefix and
// seeks back, and the reader gets both a stream and a random-access view, so
// the container stays in the page cache.
//
// Standard input has no handle and no length, so it keeps the buffered
// behaviour; a docSource over stdin is exactly what the toolbox did before.
type docSource struct {
	path string   // "" when reading standard input
	file *os.File // nil for standard input
	size int64
	buf  []byte // standard input's bytes; nil for a file source
}

// openDocSource opens path — or buffers standard input when it is "" or "-" —
// as a document source. The caller must Close it.
func openDocSource(ctx context.Context, path string) (*docSource, error) {
	if path == "" || path == StdinName {
		data, err := readContent(ctx, path)
		if err != nil {
			return nil, err
		}
		return &docSource{buf: data, size: int64(len(data))}, nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	return &docSource{path: path, file: f, size: info.Size()}, nil
}

// Close releases the source's file handle. Safe on a stdin source and safe to
// call more than once.
func (s *docSource) Close() error {
	if s == nil || s.file == nil {
		return nil
	}
	f := s.file
	s.file = nil
	return f.Close()
}

// seeker returns the whole source rewound to its start, for format detection.
// The detector reads a prefix and seeks back, so a file source is inspected
// without being read into memory.
func (s *docSource) seeker() (io.ReadSeeker, error) {
	if s.file == nil {
		return bytes.NewReader(s.buf), nil
	}
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return s.file, nil
}

// rawDocument fills in doc's input side: a byte-budgeted stream from the start
// of the source, plus a random-access view when the source has one. It must be
// called after any detection pass, since it rewinds the handle both share.
func (s *docSource) rawDocument(doc *model.RawDocument) error {
	if s.file == nil {
		doc.Reader = io.NopCloser(safeio.DefaultBudget().Reader(bytes.NewReader(s.buf)))
		doc.ReaderAt, doc.Size = bytes.NewReader(s.buf), s.size
		return nil
	}
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	// NopCloser: the handle belongs to the docSource, and a reader that closed
	// it would pull the file out from under the random-access view — which the
	// same reader may still be using, and which a deferred media accessor
	// outlives by design.
	doc.Reader = io.NopCloser(safeio.DefaultBudget().Reader(s.file))
	doc.ReaderAt, doc.Size = s.file, s.size
	return nil
}

// bindWriterSource gives a writer the original document it needs to rebuild a
// structure-preserving format, preferring the file over its bytes.
//
// A package writer (docx, odt, epub, …) reconstructs by copying the original
// archive's entries, so it needs the whole source — the one thing on this path
// that genuinely cannot be avoided. A [format.SourcePathSetter] re-opens it
// from disk during Write, which is the difference between reading a 100 MB
// .docx and holding one; only a writer that cannot do that gets the bytes.
//
// outPath is where the writer will write ("" for a stream). When it names the
// input, the source path is off the table: the output file is created — and
// truncated — before Write runs, so the writer would re-read an empty document
// and produce a broken one while reporting success. That case keeps the bytes.
func bindWriterSource(writer format.DataFormatWriter, path, outPath string, src *docSource) error {
	if sps, ok := writer.(format.SourcePathSetter); ok && src.file != nil {
		if abs, err := filepath.Abs(path); err == nil && !sameFile(abs, outPath) {
			sps.SetSourcePath(abs)
			return nil
		}
	}
	ocs, ok := writer.(format.OriginalContentSetter)
	if !ok {
		return nil
	}
	content, err := src.bytes()
	if err != nil {
		return err
	}
	ocs.SetOriginalContent(content)
	return nil
}

// sameFile reports whether outPath names the same file as the absolute input
// path abs. Path comparison, not inode comparison: the output may not exist
// yet, which is the case that matters.
func sameFile(abs, outPath string) bool {
	if outPath == "" {
		return false
	}
	outAbs, err := filepath.Abs(outPath)
	if err != nil {
		return true // cannot prove they differ — assume the worst
	}
	return outAbs == abs
}

// bytes returns the source in full. It is the escape hatch for the writers that
// need the original bytes to reconstruct a document (format.OriginalContentSetter
// on a same-format round-trip), and for in-place editing's backup copy — the
// two places the toolbox genuinely cannot avoid materializing the input.
func (s *docSource) bytes() ([]byte, error) {
	if s.file == nil {
		return s.buf, nil
	}
	if _, err := s.file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return io.ReadAll(safeio.DefaultBudget().Reader(s.file))
}
