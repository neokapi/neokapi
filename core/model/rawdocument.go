package model

import "io"

// RawDocument represents an unprocessed input document.
type RawDocument struct {
	URI          string
	Encoding     string
	SourceLocale LocaleID
	TargetLocale LocaleID
	MimeType     string
	FormatID     string // e.g., "html", "xliff", "docx"
	Reader       io.ReadCloser
}

// ResourceID returns the RawDocument's URI as its identifier.
func (rd *RawDocument) ResourceID() string { return rd.URI }
