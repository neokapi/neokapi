// Package container treats an archive (ZIP, TAR, TAR.GZ) as a namespace of
// inner documents. It is the substrate for the "container binding" of AD-026:
// a container is a source that fans out to one inner document per entry, and a
// barrier sink that rebuilds the container around the processed entries.
//
// Memory: the package never loads the whole archive into memory. ZIP is opened
// with random access (archive/zip over the file, central directory only); TAR
// and TAR.GZ are streamed sequentially. Entries are visited one at a time, and
// an entry's bytes are materialised only when a consumer actually asks for them
// (Transform's lazy `read`), so peak memory is a single entry, never the
// archive and never the full set of entries or results. (An individual entry is
// still buffered whole — that is the format engine's whole-document contract,
// shared by every kapi reader — but only one entry is held at once.)
//
// The package is free of any dependency on the format registry, the flow
// engine, or the CLI; the per-entry processing is supplied by the caller. The
// same fan-out/repack shape is reusable beyond files — a remote API or CMS
// "collection" fits the same contract (AD-026 §7).
package container

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/safeio"
)

// Kind enumerates the container layouts the package understands.
type Kind int

const (
	KindUnknown Kind = iota
	KindZip
	KindTar
	KindTarGz
)

var (
	zipMagic  = []byte{0x50, 0x4B, 0x03, 0x04}
	gzipMagic = []byte{0x1f, 0x8b}
)

// Entry is one regular-file member of a container.
type Entry struct {
	Name string // slash-separated path within the container
	Data []byte
}

// EntryProcessor decides what to do with one entry during Transform. It receives
// the entry name and a `read` thunk that lazily materialises the (decompressed)
// entry bytes — call it ONLY when the entry is actually being processed, so
// untouched entries are never read into memory (ZIP copies them raw; TAR streams
// them). Return (bytes, true, nil) to substitute the entry, or (nil, false, nil)
// to pass it through unchanged.
type EntryProcessor func(name string, read func() ([]byte, error)) (replacement []byte, replaced bool, err error)

var containerExts = map[string]bool{".zip": true, ".tar": true, ".tgz": true}

// IsContainerPath reports whether a path names a container by its extension.
func IsContainerPath(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".tar.gz") {
		return true
	}
	return containerExts[strings.ToLower(filepath.Ext(name))]
}

// Detect classifies container bytes by their leading signature.
func Detect(data []byte) Kind {
	switch {
	case bytes.HasPrefix(data, zipMagic):
		return KindZip
	case bytes.HasPrefix(data, gzipMagic):
		return KindTarGz
	case looksLikeTar(data):
		return KindTar
	default:
		return KindUnknown
	}
}

// DetectFile classifies a container by sniffing only its header (it reads at
// most 512 bytes, never the whole file).
func DetectFile(path string) (Kind, error) {
	f, err := os.Open(path)
	if err != nil {
		return KindUnknown, err
	}
	defer f.Close()
	head := make([]byte, 512)
	n, err := io.ReadFull(f, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return KindUnknown, err
	}
	return Detect(head[:n]), nil
}

func looksLikeTar(data []byte) bool {
	const ustarOffset = 257
	if len(data) < ustarOffset+5 {
		return false
	}
	return bytes.HasPrefix(data[ustarOffset:], []byte("ustar"))
}

// unsafeEntryName reports whether an archive member name would escape a
// destination root when materialised — an absolute path, a Windows drive path,
// or any ".." traversal segment (the "zip slip" class). Walk/Transform reject
// such entries fail-closed so every consumer is protected centrally.
func unsafeEntryName(name string) bool {
	n := strings.ReplaceAll(name, "\\", "/")
	if n == "" || strings.HasPrefix(n, "/") {
		return true
	}
	if len(n) >= 2 && n[1] == ':' { // C:\... drive-absolute
		return true
	}
	return slices.Contains(strings.Split(n, "/"), "..")
}

// Walk visits each regular-file entry of an in-memory container, one at a time,
// materialising only the current entry's bytes (not all entries at once). It is
// for consumers that already hold the archive bytes (e.g. a format reader handed
// a buffered document by the engine). Reads are bounded by the shared safeio
// budget; unsafe entry names are rejected.
func Walk(data []byte, fn func(Entry) error) (Kind, error) {
	kind := Detect(data)
	switch kind {
	case KindZip:
		return kind, walkZip(data, fn)
	case KindTar:
		return kind, walkTar(bytes.NewReader(data), fn)
	case KindTarGz:
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return kind, fmt.Errorf("container: opening gzip: %w", err)
		}
		defer gz.Close()
		return kind, walkTar(safeio.DefaultBudget().Reader(gz), fn)
	default:
		return KindUnknown, errors.New("container: unrecognised archive (expected ZIP, TAR, or TAR.GZ)")
	}
}

// ErrEntryNotFound is returned by OpenEntry when the named entry is absent.
var ErrEntryNotFound = errors.New("container: entry not found")

// OpenEntry reads a single named entry from an on-disk container without loading
// the whole archive: ZIP is opened with random access (central directory +
// seek), TAR/TAR.GZ are scanned sequentially and reading stops at the match. The
// name is matched as stored (slash-separated; a leading "./" is ignored on both
// sides). It returns [ErrEntryNotFound] if the entry is absent. Reads are bounded
// by the shared safeio budget, and the unsafeEntryName guard rejects traversal.
func OpenEntry(path, name string) ([]byte, Kind, error) {
	if unsafeEntryName(name) {
		return nil, KindUnknown, fmt.Errorf("container: unsafe entry name %q (path traversal)", name)
	}
	want := normalizeEntry(name)
	kind, err := DetectFile(path)
	if err != nil {
		return nil, KindUnknown, fmt.Errorf("container: %w", err)
	}
	switch kind {
	case KindZip:
		b, err := openZipEntry(path, want)
		return b, kind, err
	case KindTar:
		f, err := os.Open(path)
		if err != nil {
			return nil, kind, err
		}
		defer f.Close()
		b, err := scanTarEntry(f, want)
		return b, kind, err
	case KindTarGz:
		f, err := os.Open(path)
		if err != nil {
			return nil, kind, err
		}
		defer f.Close()
		gz, err := gzip.NewReader(f)
		if err != nil {
			return nil, kind, fmt.Errorf("container: opening gzip: %w", err)
		}
		defer gz.Close()
		b, err := scanTarEntry(safeio.DefaultBudget().Reader(gz), want)
		return b, kind, err
	default:
		return nil, KindUnknown, errors.New("container: unrecognised archive (expected ZIP, TAR, or TAR.GZ)")
	}
}

// normalizeEntry canonicalises an entry path for matching: forward slashes, no
// leading "./".
func normalizeEntry(name string) string {
	return strings.TrimPrefix(filepath.ToSlash(name), "./")
}

func openZipEntry(path, want string) ([]byte, error) {
	zrc, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("container: opening zip: %w", err)
	}
	defer zrc.Close()
	if err := safeio.DefaultZipLimits.CheckReader(&zrc.Reader); err != nil {
		return nil, fmt.Errorf("container: %w", err)
	}
	guard := safeio.DefaultZipLimits.NewGuard()
	for _, f := range zrc.File {
		if f.FileInfo().IsDir() || normalizeEntry(f.Name) != want {
			continue
		}
		if unsafeEntryName(f.Name) {
			return nil, fmt.Errorf("container: unsafe entry name %q (path traversal)", f.Name)
		}
		return guard.ReadEntry(f)
	}
	return nil, fmt.Errorf("%w: %q", ErrEntryNotFound, want)
}

func scanTarEntry(src io.Reader, want string) ([]byte, error) {
	tr := tar.NewReader(src)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, fmt.Errorf("%w: %q", ErrEntryNotFound, want)
		}
		if err != nil {
			return nil, fmt.Errorf("container: reading tar: %w", err)
		}
		if !hdr.FileInfo().Mode().IsRegular() || normalizeEntry(hdr.Name) != want {
			continue
		}
		if unsafeEntryName(hdr.Name) {
			return nil, fmt.Errorf("container: unsafe entry name %q (path traversal)", hdr.Name)
		}
		return io.ReadAll(safeio.DefaultBudget().Reader(tr))
	}
}

// EntryHeader is an archive entry's metadata without its content.
type EntryHeader struct {
	Name string
	Size int64 // uncompressed size in bytes
}

// ListEntries returns the regular-file entries' names and uncompressed sizes
// without reading their content — ZIP from the central directory, TAR by scanning
// headers (bodies are skipped, not buffered). Unsafe (traversal) names are
// skipped. This is the cheap listing the desktop archive tree uses.
func ListEntries(path string) (Kind, []EntryHeader, error) {
	kind, err := DetectFile(path)
	if err != nil {
		return KindUnknown, nil, fmt.Errorf("container: %w", err)
	}
	switch kind {
	case KindZip:
		zrc, err := zip.OpenReader(path)
		if err != nil {
			return kind, nil, fmt.Errorf("container: opening zip: %w", err)
		}
		defer zrc.Close()
		var out []EntryHeader
		for _, f := range zrc.File {
			if f.FileInfo().IsDir() || unsafeEntryName(f.Name) {
				continue
			}
			out = append(out, EntryHeader{Name: f.Name, Size: int64(f.UncompressedSize64)})
		}
		return kind, out, nil
	case KindTar:
		f, err := os.Open(path)
		if err != nil {
			return kind, nil, err
		}
		defer f.Close()
		out, err := listTar(f)
		return kind, out, err
	case KindTarGz:
		f, err := os.Open(path)
		if err != nil {
			return kind, nil, err
		}
		defer f.Close()
		gz, err := gzip.NewReader(f)
		if err != nil {
			return kind, nil, fmt.Errorf("container: opening gzip: %w", err)
		}
		defer gz.Close()
		out, err := listTar(gz)
		return kind, out, err
	default:
		return KindUnknown, nil, errors.New("container: unrecognised archive (expected ZIP, TAR, or TAR.GZ)")
	}
}

func listTar(src io.Reader) ([]EntryHeader, error) {
	tr := tar.NewReader(src)
	var out []EntryHeader
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return out, nil
		}
		if err != nil {
			return nil, fmt.Errorf("container: reading tar: %w", err)
		}
		if !hdr.FileInfo().Mode().IsRegular() || unsafeEntryName(hdr.Name) {
			continue
		}
		out = append(out, EntryHeader{Name: hdr.Name, Size: hdr.Size})
	}
}

// Enumerate is a convenience wrapper over Walk that collects every entry. It
// holds all entries at once and so is intended for tests and small archives;
// streaming consumers should use Walk or Transform.
func Enumerate(data []byte) (Kind, []Entry, error) {
	var entries []Entry
	kind, err := Walk(data, func(e Entry) error {
		entries = append(entries, e)
		return nil
	})
	return kind, entries, err
}

func walkZip(data []byte, fn func(Entry) error) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("container: opening zip: %w", err)
	}
	if err := safeio.DefaultZipLimits.CheckReader(zr); err != nil {
		return fmt.Errorf("container: %w", err)
	}
	guard := safeio.DefaultZipLimits.NewGuard()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if unsafeEntryName(f.Name) {
			return fmt.Errorf("container: unsafe entry name %q (path traversal)", f.Name)
		}
		b, err := guard.ReadEntry(f)
		if err != nil {
			return fmt.Errorf("container: reading %s: %w", f.Name, err)
		}
		if err := fn(Entry{Name: f.Name, Data: b}); err != nil {
			return err
		}
	}
	return nil
}

func walkTar(src io.Reader, fn func(Entry) error) error {
	tr := tar.NewReader(src)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("container: reading tar: %w", err)
		}
		if !hdr.FileInfo().Mode().IsRegular() {
			continue
		}
		if unsafeEntryName(hdr.Name) {
			return fmt.Errorf("container: unsafe entry name %q (path traversal)", hdr.Name)
		}
		b, err := io.ReadAll(safeio.DefaultBudget().Reader(tr))
		if err != nil {
			return fmt.Errorf("container: reading %s: %w", hdr.Name, err)
		}
		if err := fn(Entry{Name: hdr.Name, Data: b}); err != nil {
			return err
		}
	}
}

// Transform streams an on-disk container to out, replacing the entries the
// processor chooses and copying the rest. It never loads the whole archive into
// memory: ZIP is opened with random access and untouched entries are raw-copied
// (no decompress, byte-for-byte fidelity); TAR/TAR.GZ are streamed and untouched
// entries are piped straight through. Only an entry the processor actually reads
// is materialised, one at a time.
func Transform(path string, out io.Writer, proc EntryProcessor) error {
	kind, err := DetectFile(path)
	if err != nil {
		return fmt.Errorf("container: %w", err)
	}
	switch kind {
	case KindZip:
		return transformZip(path, out, proc)
	case KindTar:
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		return transformTar(f, out, proc)
	case KindTarGz:
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		gz, err := gzip.NewReader(f)
		if err != nil {
			return fmt.Errorf("container: opening gzip: %w", err)
		}
		defer gz.Close()
		gzw := gzip.NewWriter(out)
		if err := transformTar(gz, gzw, proc); err != nil {
			gzw.Close()
			return err
		}
		return gzw.Close()
	default:
		return errors.New("container: unrecognised archive for transform")
	}
}

func transformZip(path string, out io.Writer, proc EntryProcessor) error {
	// zip.OpenReader uses the central directory + seeks; it does not read the
	// whole archive into memory.
	zrc, err := zip.OpenReader(path)
	if err != nil {
		return fmt.Errorf("container: opening zip: %w", err)
	}
	defer zrc.Close()
	if err := safeio.DefaultZipLimits.CheckReader(&zrc.Reader); err != nil {
		return fmt.Errorf("container: %w", err)
	}
	zw := zip.NewWriter(out)
	guard := safeio.DefaultZipLimits.NewGuard()
	for _, f := range zrc.File {
		if f.FileInfo().IsDir() {
			if err := zw.Copy(f); err != nil {
				return fmt.Errorf("container: copying %s: %w", f.Name, err)
			}
			continue
		}
		if unsafeEntryName(f.Name) {
			return fmt.Errorf("container: unsafe entry name %q (path traversal)", f.Name)
		}

		var cached []byte
		var readErr error
		read := false
		repl, replaced, perr := proc(f.Name, func() ([]byte, error) {
			if !read {
				cached, readErr = guard.ReadEntry(f)
				read = true
			}
			return cached, readErr
		})
		if perr != nil {
			return perr
		}
		if readErr != nil {
			return fmt.Errorf("container: reading %s: %w", f.Name, readErr)
		}

		switch {
		case replaced:
			if err := writeZipEntry(zw, &f.FileHeader, repl); err != nil {
				return fmt.Errorf("container: writing %s: %w", f.Name, err)
			}
		case read:
			// Processor read the entry but declined to replace it (e.g. a failed
			// run): write the original bytes back rather than re-streaming.
			if err := writeZipEntry(zw, &f.FileHeader, cached); err != nil {
				return fmt.Errorf("container: writing %s: %w", f.Name, err)
			}
		default:
			// Untouched: raw-copy preserves the exact compressed bytes + metadata.
			if err := zw.Copy(f); err != nil {
				return fmt.Errorf("container: copying %s: %w", f.Name, err)
			}
		}
	}
	return zw.Close()
}

func writeZipEntry(zw *zip.Writer, src *zip.FileHeader, data []byte) error {
	hdr := *src
	hdr.CompressedSize64 = 0
	hdr.UncompressedSize64 = 0
	hdr.CRC32 = 0
	w, err := zw.CreateHeader(&hdr)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

func transformTar(src io.Reader, out io.Writer, proc EntryProcessor) error {
	tr := tar.NewReader(src)
	tw := tar.NewWriter(out)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("container: reading tar: %w", err)
		}
		if !hdr.FileInfo().Mode().IsRegular() {
			if err := tw.WriteHeader(hdr); err != nil {
				return fmt.Errorf("container: writing header %s: %w", hdr.Name, err)
			}
			continue
		}
		if unsafeEntryName(hdr.Name) {
			return fmt.Errorf("container: unsafe entry name %q (path traversal)", hdr.Name)
		}

		var cached []byte
		var readErr error
		read := false
		repl, replaced, perr := proc(hdr.Name, func() ([]byte, error) {
			if !read {
				cached, readErr = io.ReadAll(safeio.DefaultBudget().Reader(tr))
				read = true
			}
			return cached, readErr
		})
		if perr != nil {
			return perr
		}
		if readErr != nil {
			return fmt.Errorf("container: reading %s: %w", hdr.Name, readErr)
		}

		switch {
		case replaced:
			outHdr := *hdr
			outHdr.Size = int64(len(repl))
			if err := tw.WriteHeader(&outHdr); err != nil {
				return fmt.Errorf("container: writing header %s: %w", hdr.Name, err)
			}
			if _, err := tw.Write(repl); err != nil {
				return fmt.Errorf("container: writing %s: %w", hdr.Name, err)
			}
		case read:
			if err := tw.WriteHeader(hdr); err != nil {
				return fmt.Errorf("container: writing header %s: %w", hdr.Name, err)
			}
			if _, err := tw.Write(cached); err != nil {
				return fmt.Errorf("container: writing %s: %w", hdr.Name, err)
			}
		default:
			// Untouched: stream the body straight through without buffering it.
			if err := tw.WriteHeader(hdr); err != nil {
				return fmt.Errorf("container: writing header %s: %w", hdr.Name, err)
			}
			if _, err := io.Copy(tw, safeio.DefaultBudget().Reader(tr)); err != nil {
				return fmt.Errorf("container: copying %s: %w", hdr.Name, err)
			}
		}
	}
	return tw.Close()
}
