package model

import "io"

// RawDocument represents an unprocessed input document.
type RawDocument struct {
	URI string
	// Encoding is the charset the document is in. A caller declares it (the
	// --encoding flag, the recipe's defaults.encoding) and the ingestion seam
	// (format.OpenDocument) settles it against the bytes, so by the time a
	// reader sees the document it names the encoding the document was read as.
	Encoding string
	// EncodingSettled reports that the ingestion seam settled Encoding from a
	// byte-order mark or a declaration and, where that was not UTF-8,
	// transcoded Reader to UTF-8. A reader then takes the bytes as UTF-8 and
	// applies no charset heuristic of its own (an XML prolog, a PO header).
	// When it is false the bytes are as the source holds them, and a reader
	// with such a heuristic may still apply it.
	EncodingSettled bool
	SourceLocale    LocaleID
	TargetLocale    LocaleID
	MimeType        string
	FormatID        string // e.g., "html", "xliff", "docx"
	Reader          io.ReadCloser

	// ReaderAt, with Size, is an optional random-access view over the same
	// bytes as Reader. A container format (ZIP-backed: OOXML, ODF, EPUB, IDML)
	// cannot be parsed in one forward pass — the central directory is at the
	// end of the file, and OOXML needs [Content_Types].xml and the relationship
	// parts to interpret entries stored before them — so without random access
	// such a reader has to buffer the whole container first.
	//
	// Random access is not the same as resident memory: an *os.File is already
	// an io.ReaderAt, and a zip read over the file handle lets the page cache
	// hold the container instead of the heap. Sources that genuinely have no
	// random access (standard input, an archive entry, a plugin stream) leave
	// these zero and readers fall back to buffering Reader.
	//
	// Both bytes must describe the same content as Reader, and Reader must be
	// positioned at offset 0 when the document is opened.
	ReaderAt io.ReaderAt
	Size     int64
}

// RandomAccess returns the document's random-access view and its size, and
// reports whether one is available. Readers that can exploit random access
// should route through this rather than testing the fields, so the "both set,
// non-empty" contract lives in one place.
func (rd *RawDocument) RandomAccess() (io.ReaderAt, int64, bool) {
	if rd == nil || rd.ReaderAt == nil || rd.Size <= 0 {
		return nil, 0, false
	}
	return rd.ReaderAt, rd.Size, true
}

// ResourceID returns the RawDocument's URI as its identifier.
func (rd *RawDocument) ResourceID() string { return rd.URI }
