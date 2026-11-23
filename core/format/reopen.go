package format

import (
	"io"
	"os"

	"github.com/neokapi/neokapi/core/model"
)

// ReopenSource returns a second reader over the document's bytes, for a
// reader that scans the document once before it streams it through
// doc.Reader. It reads through doc.ReaderAt, the random-access view the
// caller opened over the same bytes, so the scan never opens a file by the
// document's URI: a URI names a document (an archive member, a subfilter
// path, an upload's name), not a file the reader may open. It reports false
// when the document has no random-access view.
func ReopenSource(doc *model.RawDocument) (io.Reader, bool) {
	if doc == nil || doc.ReaderAt == nil {
		return nil, false
	}
	size := doc.Size
	if size <= 0 {
		f, ok := doc.ReaderAt.(*os.File)
		if !ok {
			return nil, false
		}
		info, err := f.Stat()
		if err != nil {
			return nil, false
		}
		size = info.Size()
	}
	return io.NewSectionReader(doc.ReaderAt, 0, size), true
}

// SourceFile returns the name of the file the caller opened the document
// from, for a reader that hands the document to a program that reads a path
// (an OCR engine, ffmpeg). It is the name of the descriptor behind
// doc.ReaderAt, never the document's URI, and reports false when the document
// was not opened from a file.
func SourceFile(doc *model.RawDocument) (string, bool) {
	if doc == nil {
		return "", false
	}
	f, ok := doc.ReaderAt.(*os.File)
	if !ok {
		return "", false
	}
	return f.Name(), true
}
