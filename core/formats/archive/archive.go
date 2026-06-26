// Package archive implements a format that treats an archive container
// (ZIP, TAR, TAR.GZ) as a folder of sub-documents. Each entry whose content
// kapi recognises and can faithfully rewrite is parsed through its own format
// reader and emitted as a child Layer; every other entry (binary assets,
// skeleton-bound formats such as DOCX/EPUB, nested archives, unrecognised
// files) rides through unchanged so the round-trip is byte-for-byte.
//
// The reader and writer are constructed with the format registry (captured in
// the registration factory) so they can detect each entry's format and resolve
// the matching sub-reader / sub-writer. This is deliberate: the SubfilterResolver
// is not wired into the generic file-run path, so a container format must carry
// its own resolver rather than rely on the pipeline to inject one.
package archive

import (
	"bytes"

	"github.com/neokapi/neokapi/core/format"
)

// kind enumerates the archive container layouts the format understands.
type kind int

const (
	kindUnknown kind = iota
	kindZip
	kindTar
	kindTarGz
)

// zipMagic / gzipMagic are the offset-0 signatures used to tell the container
// layouts apart. ZIP shares its PK signature with OOXML/ODF/IDML/EPUB, which is
// why the registry detector defers those to content sniffing; here we already
// know the document resolved to "archive", so the signature only has to pick
// between ZIP and TAR.GZ.
var (
	zipMagic  = []byte{0x50, 0x4B, 0x03, 0x04}
	gzipMagic = []byte{0x1f, 0x8b}
)

// detectKind classifies archive bytes by their leading signature. TAR has no
// reliable offset-0 magic (its "ustar" marker sits at offset 257), so anything
// that is neither ZIP nor gzip is assumed to be an uncompressed TAR.
func detectKind(data []byte) kind {
	switch {
	case bytes.HasPrefix(data, zipMagic):
		return kindZip
	case bytes.HasPrefix(data, gzipMagic):
		return kindTarGz
	case looksLikeTar(data):
		return kindTar
	default:
		return kindUnknown
	}
}

// looksLikeTar reports whether the buffer carries the POSIX "ustar" marker at
// offset 257, the one stable structural signal an uncompressed TAR exposes.
func looksLikeTar(data []byte) bool {
	const ustarOffset = 257
	if len(data) < ustarOffset+5 {
		return false
	}
	return bytes.HasPrefix(data[ustarOffset:], []byte("ustar"))
}

// skipFormats are formats that must never be sub-filtered even though their
// writer is technically generative: binary assets (whole-asset localisation,
// pointless to re-encode inside an archive) and the archive format itself
// (nested archives ride through rather than recursing). Skeleton-bound formats
// (DOCX/EPUB/ODF/IDML/…) are already excluded by the Generative() probe in
// canSubfilter, but binary assets reconstruct from the content model and would
// otherwise be re-encoded.
var skipFormats = map[string]bool{
	"image": true,
	"audio": true,
	"video": true,
	"mo":    true, // binary gettext catalog (reader is a stub)
	"pdf":   true, // read-only
	"archive": true,
}

// canSubfilter decides whether an entry of the given detected format should be
// parsed through its own reader/writer (true) or pass through unchanged (false).
// An entry is sub-filtered only when the registry can resolve both a reader and
// a writer for it, the writer is generative (can reconstruct from the content
// model without a byte skeleton — DOCX/EPUB/ODF/… fail this), the format is not
// a bilingual interchange format (XLIFF/PO/TMX belong to extract→merge, not to
// in-place rewriting), and it is not on the binary/self skip list.
func canSubfilter(resolver format.SubfilterResolver, fmtName string) bool {
	if fmtName == "" || skipFormats[fmtName] {
		return false
	}
	if _, err := resolver.ResolveReader(fmtName); err != nil {
		return false
	}
	w, err := resolver.ResolveWriter(fmtName)
	if err != nil {
		return false
	}
	if gw, ok := w.(format.GenerativeWriter); ok && !gw.Generative() {
		return false
	}
	if iw, ok := w.(format.InterchangeWriter); ok && iw.IsInterchange() {
		return false
	}
	return true
}
