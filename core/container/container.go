// Package container treats an archive (ZIP, TAR, TAR.GZ) as a namespace of
// inner documents. It is the substrate for the "container binding" of
// AD-026: Enumerate is the source fan-out (a container expands to N inner
// entries, each its own document), and Repack is the barrier sink (collect the
// processed entries and rebuild one valid container, copying untouched members
// byte-for-byte from the original).
//
// The package is deliberately free of any dependency on the format registry,
// the flow engine, or the CLI. The per-entry *processing* (detection, reader,
// tools, writer, skeleton round-trip) is supplied by the caller. That keeps the
// same fan-out/repack shape reusable beyond files — a remote API or CMS
// "collection" that enumerates child items and batch-writes them back fits the
// same Enumerate→process→Repack contract (AD-026 §7).
package container

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"path/filepath"
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

// containerExts are the path extensions that identify a container. TAR.GZ is
// matched as a compound suffix because filepath.Ext only sees ".gz".
var containerExts = map[string]bool{
	".zip": true, ".tar": true, ".tgz": true,
}

// IsContainerPath reports whether a path names a container by its extension.
func IsContainerPath(name string) bool {
	lower := strings.ToLower(name)
	if strings.HasSuffix(lower, ".tar.gz") {
		return true
	}
	return containerExts[strings.ToLower(filepath.Ext(name))]
}

// Detect classifies container bytes by their leading signature. TAR has no
// reliable offset-0 magic (its "ustar" marker sits at offset 257), so anything
// that is neither ZIP nor gzip but carries the ustar marker is treated as TAR.
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

func looksLikeTar(data []byte) bool {
	const ustarOffset = 257
	if len(data) < ustarOffset+5 {
		return false
	}
	return bytes.HasPrefix(data[ustarOffset:], []byte("ustar"))
}

// Enumerate returns the container kind and its regular-file entries, in
// container order. Reads are bounded by the shared safeio budget (per-entry
// size, total size, entry count, zip-bomb inflate ratio).
func Enumerate(data []byte) (Kind, []Entry, error) {
	kind := Detect(data)
	switch kind {
	case KindZip:
		entries, err := enumerateZip(data)
		return kind, entries, err
	case KindTar:
		entries, err := enumerateTar(bytes.NewReader(data))
		return kind, entries, err
	case KindTarGz:
		gz, err := gzip.NewReader(bytes.NewReader(data))
		if err != nil {
			return kind, nil, fmt.Errorf("container: opening gzip: %w", err)
		}
		defer gz.Close()
		entries, err := enumerateTar(safeio.DefaultBudget().Reader(gz))
		return kind, entries, err
	default:
		return KindUnknown, nil, errors.New("container: unrecognised archive (expected ZIP, TAR, or TAR.GZ)")
	}
}

func enumerateZip(data []byte) ([]Entry, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("container: opening zip: %w", err)
	}
	if err := safeio.DefaultZipLimits.CheckReader(zr); err != nil {
		return nil, fmt.Errorf("container: %w", err)
	}
	guard := safeio.DefaultZipLimits.NewGuard()
	var entries []Entry
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		b, err := guard.ReadEntry(f)
		if err != nil {
			return nil, fmt.Errorf("container: reading %s: %w", f.Name, err)
		}
		entries = append(entries, Entry{Name: f.Name, Data: b})
	}
	return entries, nil
}

func enumerateTar(src io.Reader) ([]Entry, error) {
	tr := tar.NewReader(src)
	var entries []Entry
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return entries, nil
		}
		if err != nil {
			return nil, fmt.Errorf("container: reading tar: %w", err)
		}
		if !hdr.FileInfo().Mode().IsRegular() {
			continue
		}
		b, err := io.ReadAll(safeio.DefaultBudget().Reader(tr))
		if err != nil {
			return nil, fmt.Errorf("container: reading %s: %w", hdr.Name, err)
		}
		entries = append(entries, Entry{Name: hdr.Name, Data: b})
	}
}

// Repack rebuilds the container from its original bytes, substituting the bytes
// of any entry named in replacements and copying every other member verbatim.
// This is the barrier sink: the original is authoritative for structure, entry
// order, metadata, and untouched/binary members, so a round-trip changes only
// the entries that were actually processed.
func Repack(kind Kind, original []byte, replacements map[string][]byte, out io.Writer) error {
	switch kind {
	case KindZip:
		return repackZip(original, replacements, out)
	case KindTar:
		return repackTar(original, replacements, out)
	case KindTarGz:
		gzr, err := gzip.NewReader(bytes.NewReader(original))
		if err != nil {
			return fmt.Errorf("container: opening source gzip: %w", err)
		}
		tarData, err := io.ReadAll(gzr)
		gzr.Close()
		if err != nil {
			return fmt.Errorf("container: decompressing source gzip: %w", err)
		}
		gz := gzip.NewWriter(out)
		if err := repackTar(tarData, replacements, gz); err != nil {
			gz.Close()
			return err
		}
		return gz.Close()
	default:
		return errors.New("container: unrecognised archive for repack")
	}
}

func repackZip(data []byte, replacements map[string][]byte, out io.Writer) error {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return fmt.Errorf("container: opening source zip: %w", err)
	}
	zw := zip.NewWriter(out)
	for _, f := range zr.File {
		repl, ok := replacements[f.Name]
		if !ok {
			// Copy preserves the raw compressed bytes and metadata exactly.
			if err := zw.Copy(f); err != nil {
				return fmt.Errorf("container: copying %s: %w", f.Name, err)
			}
			continue
		}
		hdr := f.FileHeader
		hdr.CompressedSize64 = 0
		hdr.UncompressedSize64 = 0
		hdr.CRC32 = 0
		fw, err := zw.CreateHeader(&hdr)
		if err != nil {
			return fmt.Errorf("container: writing %s: %w", f.Name, err)
		}
		if _, err := fw.Write(repl); err != nil {
			return fmt.Errorf("container: writing %s: %w", f.Name, err)
		}
	}
	return zw.Close()
}

func repackTar(data []byte, replacements map[string][]byte, out io.Writer) error {
	tr := tar.NewReader(bytes.NewReader(data))
	tw := tar.NewWriter(out)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("container: reading source tar: %w", err)
		}
		repl, replace := replacements[hdr.Name]
		outHdr := *hdr
		if replace {
			outHdr.Size = int64(len(repl))
		}
		if err := tw.WriteHeader(&outHdr); err != nil {
			return fmt.Errorf("container: writing header %s: %w", hdr.Name, err)
		}
		switch {
		case replace:
			if _, err := tw.Write(repl); err != nil {
				return fmt.Errorf("container: writing %s: %w", hdr.Name, err)
			}
		case outHdr.FileInfo().Mode().IsRegular():
			if _, err := io.Copy(tw, tr); err != nil {
				return fmt.Errorf("container: copying %s: %w", hdr.Name, err)
			}
		}
	}
	return tw.Close()
}
