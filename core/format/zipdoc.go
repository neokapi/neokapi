package format

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// OpenZipDocument opens a RawDocument's bytes as a ZIP archive, ready for
// random-access entry reads. formatName prefixes the errors so the caller's
// diagnostics keep reading as its own ("openxml: …", "odf: …").
//
// A document that offers random access ([model.RawDocument.RandomAccess] — a
// file handle, typically) is read in place: the container never becomes
// resident, and the page cache absorbs it. That matters because a package
// format is large precisely when it embeds media, which is already-compressed
// and stores roughly 1:1, so buffering the container means holding every
// embedded image on the heap for the whole run. A document without random
// access (standard input, an archive entry, a plugin stream) is buffered, and
// the shared safeio byte budget bounds the read.
//
// Either way the archive is validated against the shared zip limits — entry
// count, declared per-entry and total uncompressed sizes, inflate ratio —
// before any entry is read.
func OpenZipDocument(doc *model.RawDocument, formatName string) (*zip.Reader, error) {
	if doc == nil {
		return nil, fmt.Errorf("%s: nil document", formatName)
	}
	ra, size, ok := doc.RandomAccess()
	if ok {
		// The size is known up front, so the byte budget is one comparison
		// rather than a counted stream (which would mean buffering it).
		if err := safeio.DefaultBudget().CheckSize(size); err != nil {
			return nil, fmt.Errorf("%s: %w", formatName, err)
		}
	} else {
		if doc.Reader == nil {
			return nil, fmt.Errorf("%s: nil reader", formatName)
		}
		data, err := io.ReadAll(safeio.DefaultBudget().Reader(doc.Reader))
		if err != nil {
			return nil, fmt.Errorf("%s: reading: %w", formatName, err)
		}
		ra, size = bytes.NewReader(data), int64(len(data))
	}

	zr, err := zip.NewReader(ra, size)
	if err != nil {
		return nil, fmt.Errorf("%s: not a valid ZIP archive: %w", formatName, err)
	}
	if err := safeio.DefaultZipLimits.CheckReader(zr); err != nil {
		return nil, fmt.Errorf("%s: %w", formatName, err)
	}
	return zr, nil
}
