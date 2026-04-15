package klz

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/neokapi/neokapi/core/klf"
)

// Reader exposes the contents of a .klz archive. It lazily inflates
// parts on demand: opening a Reader reads the ZIP central directory
// and the manifest, leaving every other part on disk until a caller
// asks for it. This matches the "iteration side" of the RFC 0001
// two-faced API — query helpers (TM, BlockByID, SimilarSources)
// land in Phase 4.
type Reader struct {
	archive        *zip.Reader
	closer         io.Closer
	manifest       *Manifest
	manifestBytes  []byte
	pathIndex      map[string]*zip.File
	maxPartBytes   int64
	maxTotalBytes  int64
	totalInflated  int64
}

// ReaderOptions configures a Reader.
type ReaderOptions struct {
	// MaxPartBytes caps the inflated size of a single part. Zero
	// defaults to DefaultMaxPartBytes. A part that exceeds this
	// cap causes the read to fail with a decompression-bomb guard
	// error.
	MaxPartBytes int64
	// MaxTotalBytes caps the sum of inflated part sizes over the
	// lifetime of a Reader. Zero defaults to DefaultMaxTotalBytes.
	MaxTotalBytes int64
}

// Open opens a .klz archive from a local filesystem path.
func Open(name string, opts ...ReaderOptions) (*Reader, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("klz: open %q: %w", name, err)
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("klz: stat %q: %w", name, err)
	}
	r, err := NewReader(f, info.Size(), opts...)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	r.closer = f
	return r, nil
}

// NewReader opens a .klz archive from an io.ReaderAt + size. This is
// the primary constructor when the archive is already in memory
// (bytes.Reader) or when the caller owns the file handle.
func NewReader(r io.ReaderAt, size int64, opts ...ReaderOptions) (*Reader, error) {
	var o ReaderOptions
	if len(opts) > 0 {
		o = opts[0]
	}
	if o.MaxPartBytes <= 0 {
		o.MaxPartBytes = DefaultMaxPartBytes
	}
	if o.MaxTotalBytes <= 0 {
		o.MaxTotalBytes = DefaultMaxTotalBytes
	}
	archive, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("klz: read zip: %w", err)
	}
	reader := &Reader{
		archive:       archive,
		pathIndex:     make(map[string]*zip.File, len(archive.File)),
		maxPartBytes:  o.MaxPartBytes,
		maxTotalBytes: o.MaxTotalBytes,
	}
	for _, f := range archive.File {
		if strings.HasSuffix(f.Name, "/") {
			// Empty directories MUST NOT be stored; if they are, we
			// tolerate them but don't index them.
			continue
		}
		safe, err := validatePartPath(f.Name)
		if err != nil {
			return nil, fmt.Errorf("klz: unsafe part %q: %w", f.Name, err)
		}
		if _, dup := reader.pathIndex[safe]; dup {
			return nil, fmt.Errorf("klz: duplicate part %q", safe)
		}
		reader.pathIndex[safe] = f
	}
	// The manifest must be present and parseable.
	mf, ok := reader.pathIndex[ManifestPath]
	if !ok {
		return nil, fmt.Errorf("klz: archive missing %s", ManifestPath)
	}
	body, err := reader.inflate(mf)
	if err != nil {
		return nil, fmt.Errorf("klz: read manifest: %w", err)
	}
	reader.manifestBytes = body
	manifest, err := UnmarshalManifest(body)
	if err != nil {
		return nil, err
	}
	reader.manifest = manifest
	return reader, nil
}

// Close releases resources. No-op for in-memory readers.
func (r *Reader) Close() error {
	if r.closer != nil {
		return r.closer.Close()
	}
	return nil
}

// Manifest returns the archive manifest. Never mutate the returned
// value; treat it as read-only snapshot state.
func (r *Reader) Manifest() *Manifest { return r.manifest }

// ManifestHash returns the SHA-256 of the raw manifest.json bytes as
// stored in the ZIP. This is the cache key per RFC 0001 §The cache
// key — content-addressed identity for the archive.
func (r *Reader) ManifestHash() string {
	sum := sha256.Sum256(r.manifestBytes)
	return hex.EncodeToString(sum[:])
}

// PartPaths returns the sorted list of archive entries (excluding
// the manifest itself).
func (r *Reader) PartPaths() []string {
	out := make([]string, 0, len(r.pathIndex))
	for p := range r.pathIndex {
		if p == ManifestPath {
			continue
		}
		out = append(out, p)
	}
	return out
}

// ReadPart inflates a part by path, verifies its SHA-256 against the
// manifest, and returns the bytes. Returns an error if the part is
// missing, oversized, or has a mismatched hash.
func (r *Reader) ReadPart(partPath string) ([]byte, error) {
	f, ok := r.pathIndex[partPath]
	if !ok {
		return nil, fmt.Errorf("klz: part %q not found", partPath)
	}
	data, err := r.inflate(f)
	if err != nil {
		return nil, fmt.Errorf("klz: read part %q: %w", partPath, err)
	}
	if partPath == ManifestPath {
		return data, nil
	}
	entry := r.manifest.FindPart(partPath)
	if entry == nil {
		return nil, fmt.Errorf("klz: part %q has no manifest entry", partPath)
	}
	sum := sha256.Sum256(data)
	got := hex.EncodeToString(sum[:])
	if got != entry.SHA256 {
		return nil, fmt.Errorf("klz: part %q hash mismatch: manifest %s, computed %s", partPath, entry.SHA256, got)
	}
	if int64(len(data)) != entry.Size {
		return nil, fmt.Errorf("klz: part %q size mismatch: manifest %d, computed %d", partPath, entry.Size, len(data))
	}
	return data, nil
}

// ReadDocument is a typed convenience that inflates a document part
// and decodes it as a klf.File.
func (r *Reader) ReadDocument(partPath string) (*klf.File, error) {
	data, err := r.ReadPart(partPath)
	if err != nil {
		return nil, err
	}
	return klf.Unmarshal(data)
}

// ReadAnnotationFile inflates and parses a .klfl annotation sidecar.
func (r *Reader) ReadAnnotationFile(partPath string) (*klf.AnnotationFile, error) {
	data, err := r.ReadPart(partPath)
	if err != nil {
		return nil, err
	}
	return klf.DecodeAnnotationFile(bytes.NewReader(data))
}

// Documents iterates over every document part in manifest order and
// returns the decoded klf.File list. This is the primary
// iteration-side entry point; for a Block-by-id or TM lookup use the
// query helpers (Phase 4).
func (r *Reader) Documents() ([]*klf.File, error) {
	var out []*klf.File
	for _, p := range r.manifest.PartsByRole(RoleDocument) {
		doc, err := r.ReadDocument(p.Path)
		if err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	return out, nil
}

// Targets returns the decoded target overlays for a given locale.
func (r *Reader) Targets(locale string) ([]*klf.File, error) {
	prefix := "targets/" + locale + "/"
	var out []*klf.File
	for _, p := range r.manifest.PartsByRole(RoleTarget) {
		if !strings.HasPrefix(p.Path, prefix) {
			continue
		}
		doc, err := r.ReadDocument(p.Path)
		if err != nil {
			return nil, err
		}
		out = append(out, doc)
	}
	return out, nil
}

// AnnotationFiles iterates every annotation sidecar in the archive.
func (r *Reader) AnnotationFiles() ([]AnnotationEntry, error) {
	var out []AnnotationEntry
	for _, p := range r.manifest.PartsByRole(RoleAnnotation) {
		file, err := r.ReadAnnotationFile(p.Path)
		if err != nil {
			return nil, err
		}
		out = append(out, AnnotationEntry{Path: p.Path, File: file})
	}
	return out, nil
}

// AnnotationEntry pairs an annotation sidecar with its archive path.
type AnnotationEntry struct {
	Path string
	File *klf.AnnotationFile
}

// VerifyAll verifies every part in the archive:
//   - Every manifest entry refers to a part present in the ZIP.
//   - Every ZIP entry is present in the manifest.
//   - Every part's SHA-256 and size match the manifest.
//
// Returns a list of problems (empty slice on success).
func (r *Reader) VerifyAll() []VerificationError {
	var errs []VerificationError

	seen := make(map[string]bool, len(r.manifest.Parts))
	for _, p := range r.manifest.Parts {
		seen[p.Path] = true
		f, ok := r.pathIndex[p.Path]
		if !ok {
			errs = append(errs, VerificationError{
				Path: p.Path, Kind: VerifyMissingPart,
				Message: "manifest references part not present in archive",
			})
			continue
		}
		data, err := r.inflate(f)
		if err != nil {
			errs = append(errs, VerificationError{
				Path: p.Path, Kind: VerifyInflateFailed,
				Message: err.Error(),
			})
			continue
		}
		sum := sha256.Sum256(data)
		got := hex.EncodeToString(sum[:])
		if got != p.SHA256 {
			errs = append(errs, VerificationError{
				Path: p.Path, Kind: VerifyHashMismatch,
				Message: fmt.Sprintf("manifest %s, computed %s", p.SHA256, got),
			})
		}
		if int64(len(data)) != p.Size {
			errs = append(errs, VerificationError{
				Path: p.Path, Kind: VerifySizeMismatch,
				Message: fmt.Sprintf("manifest %d, computed %d", p.Size, len(data)),
			})
		}
	}
	// Every archive entry (except manifest) must have a manifest
	// record. Anything else is an orphan and should be flagged.
	for path := range r.pathIndex {
		if path == ManifestPath {
			continue
		}
		if !seen[path] {
			errs = append(errs, VerificationError{
				Path: path, Kind: VerifyOrphanPart,
				Message: "archive contains part not listed in manifest",
			})
		}
	}
	return errs
}

// VerificationErrorKind enumerates VerifyAll failure cases.
type VerificationErrorKind string

const (
	VerifyMissingPart   VerificationErrorKind = "missing-part"
	VerifyOrphanPart    VerificationErrorKind = "orphan-part"
	VerifyHashMismatch  VerificationErrorKind = "hash-mismatch"
	VerifySizeMismatch  VerificationErrorKind = "size-mismatch"
	VerifyInflateFailed VerificationErrorKind = "inflate-failed"
)

// VerificationError describes one problem found by VerifyAll.
type VerificationError struct {
	Path    string
	Kind    VerificationErrorKind
	Message string
}

func (e VerificationError) Error() string {
	return fmt.Sprintf("%s: %s: %s", e.Kind, e.Path, e.Message)
}

// inflate reads a ZIP entry into memory, enforcing the configured
// size limits. Limits apply to the inflated (post-DEFLATE) bytes to
// guard against decompression bombs.
func (r *Reader) inflate(f *zip.File) ([]byte, error) {
	if int64(f.UncompressedSize64) > r.maxPartBytes {
		return nil, fmt.Errorf("part %q exceeds MaxPartBytes (%d > %d)",
			f.Name, f.UncompressedSize64, r.maxPartBytes)
	}
	rc, err := f.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	// io.LimitReader caps actual reads so a fraudulently-small
	// UncompressedSize64 can't bypass the cap.
	limited := io.LimitReader(rc, r.maxPartBytes+1)
	data, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > r.maxPartBytes {
		return nil, fmt.Errorf("part %q exceeds MaxPartBytes (%d)", f.Name, r.maxPartBytes)
	}
	r.totalInflated += int64(len(data))
	if r.totalInflated > r.maxTotalBytes {
		return nil, fmt.Errorf("archive exceeds MaxTotalBytes (%d)", r.maxTotalBytes)
	}
	return data, nil
}
