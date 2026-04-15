package klz

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"sort"

	"github.com/neokapi/neokapi/core/klf"
)

// Writer builds a .klz archive in a single pass. Parts are added via
// AddDocument, AddTarget, AddSkeleton, AddAnnotationFile, AddAsset,
// AddVocabulary, AddMeta. Close() finalizes the manifest (including
// SHA-256 per part) and the ZIP central directory.
//
// Writer accumulates its parts in memory so the manifest can be
// stamped at the end of the archive without seeking backward. For
// typical extract output (tens of MB) this is fine; streaming-to-disk
// is a future extension tracked in RFC 0001 §Unresolved questions.
type Writer struct {
	generator ManifestGenerator
	project   ManifestProject
	created   string

	parts     []pendingPart
	partIndex map[string]bool // path → seen
}

type pendingPart struct {
	path       string
	data       []byte
	role       PartRole
	attributes map[string]any
}

// WriterOptions configure a Writer at construction.
type WriterOptions struct {
	Generator ManifestGenerator
	Project   ManifestProject
	// Created is an RFC3339 timestamp. If empty, the Writer omits
	// the created field on the manifest.
	Created string
}

// NewWriter creates an empty Writer. Call AddDocument / AddTarget /
// AddSkeleton / AddAnnotationFile / AddAsset / AddVocabulary before
// calling Write to finalize the archive.
func NewWriter(opts WriterOptions) *Writer {
	return &Writer{
		generator: opts.Generator,
		project:   opts.Project,
		created:   opts.Created,
		partIndex: make(map[string]bool),
	}
}

// AddDocument adds a .klf document part at the given archive path.
// `path` should typically start with `documents/` and end in `.klf`
// per RFC 0001; the Writer enforces no layout rules beyond path
// safety so producers can experiment with filename layouts.
func (w *Writer) AddDocument(partPath string, doc *klf.File, attrs map[string]any) error {
	data, err := klf.Marshal(doc)
	if err != nil {
		return fmt.Errorf("klz: marshal document: %w", err)
	}
	return w.addPart(partPath, data, RoleDocument, attrs)
}

// AddDocumentBytes adds a pre-marshaled .klf payload. Used when the
// caller already owns a byte buffer (e.g. round-tripping from a
// reader without re-encoding).
func (w *Writer) AddDocumentBytes(partPath string, data []byte, attrs map[string]any) error {
	return w.addPart(partPath, data, RoleDocument, attrs)
}

// AddTarget adds a per-locale sparse overlay .klf at the given path.
// Convention: `targets/{locale}/<doc>.klf`. Like AddDocument, the
// Writer does not enforce directory layout beyond path safety.
func (w *Writer) AddTarget(partPath string, doc *klf.File, attrs map[string]any) error {
	data, err := klf.Marshal(doc)
	if err != nil {
		return fmt.Errorf("klz: marshal target: %w", err)
	}
	return w.addPart(partPath, data, RoleTarget, attrs)
}

// AddSkeleton adds an opaque skeleton blob. The Writer computes a
// SHA-256 and stores it in the manifest but does not parse or
// transform the bytes — skeleton semantics belong to the owning
// extractor per RFC 0001 §Skeleton ownership.
func (w *Writer) AddSkeleton(partPath string, data []byte, attrs map[string]any) error {
	return w.addPart(partPath, data, RoleSkeleton, attrs)
}

// AddAsset adds a binary asset (image, screenshot, audio, video).
func (w *Writer) AddAsset(partPath string, data []byte, attrs map[string]any) error {
	return w.addPart(partPath, data, RoleAsset, attrs)
}

// AddVocabulary adds a vocabulary JSON override at the given path.
func (w *Writer) AddVocabulary(partPath string, data []byte, attrs map[string]any) error {
	return w.addPart(partPath, data, RoleVocabulary, attrs)
}

// AddMeta adds the optional meta.json part at the archive root.
func (w *Writer) AddMeta(data []byte) error {
	return w.addPart("meta.json", data, RoleMeta, nil)
}

// AddAnnotationFile marshals a klf.AnnotationFile to its .klfl
// JSON-Lines form and adds it as an annotation part. Convention:
// `annotations/<producer-namespace>.klfl`.
func (w *Writer) AddAnnotationFile(partPath string, f *klf.AnnotationFile, attrs map[string]any) error {
	var buf bytes.Buffer
	if err := klf.EncodeAnnotationFile(&buf, f); err != nil {
		return fmt.Errorf("klz: encode annotation file: %w", err)
	}
	return w.addPart(partPath, buf.Bytes(), RoleAnnotation, attrs)
}

// addPart is the shared ingress. Enforces path safety, detects
// duplicate paths, and files the part for later ZIP emission.
func (w *Writer) addPart(raw string, data []byte, role PartRole, attrs map[string]any) error {
	safe, err := validatePartPath(raw)
	if err != nil {
		return err
	}
	if safe == ManifestPath {
		return fmt.Errorf("klz: part path %q is reserved for the manifest", ManifestPath)
	}
	if w.partIndex[safe] {
		return fmt.Errorf("klz: duplicate part path %q", safe)
	}
	w.partIndex[safe] = true
	w.parts = append(w.parts, pendingPart{
		path: safe, data: data, role: role, attributes: attrs,
	})
	return nil
}

// Write finalizes the archive and streams it to out. The manifest
// is computed deterministically: parts are emitted in the order they
// were added (producers that want a canonical order should add in
// that order).
func (w *Writer) Write(out io.Writer) (*Manifest, error) {
	manifest := &Manifest{
		KapiLocalizationFormat: ManifestVersion,
		Created:                w.created,
		Generator:              w.generator,
		Project:                w.project,
	}
	// Build the per-part manifest entries first so we can emit the
	// manifest as the first entry in the ZIP central directory.
	for _, p := range w.parts {
		sum := sha256.Sum256(p.data)
		manifest.Parts = append(manifest.Parts, ManifestPartInfo{
			Path:       p.path,
			SHA256:     hex.EncodeToString(sum[:]),
			Size:       int64(len(p.data)),
			Role:       p.role,
			Attributes: p.attributes,
		})
	}

	manifestBytes, err := MarshalManifest(manifest)
	if err != nil {
		return nil, err
	}

	zw := zip.NewWriter(out)
	// Manifest first so lazy readers can read + verify before
	// inflating any other part.
	if err := writeZipEntry(zw, ManifestPath, manifestBytes); err != nil {
		return nil, err
	}
	for _, p := range w.parts {
		if err := writeZipEntry(zw, p.path, p.data); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("klz: close zip: %w", err)
	}
	return manifest, nil
}

func writeZipEntry(zw *zip.Writer, partPath string, data []byte) error {
	// Normalize header for deterministic output: zero the Modified
	// field (RFC 0001 wants content-addressed stable hashes, and
	// the zip headers feed into the final .klz bytes).
	hdr := &zip.FileHeader{
		Name:   partPath,
		Method: zip.Deflate,
	}
	hdr.SetMode(0o644)
	f, err := zw.CreateHeader(hdr)
	if err != nil {
		return fmt.Errorf("klz: create zip entry %q: %w", partPath, err)
	}
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("klz: write zip entry %q: %w", partPath, err)
	}
	return nil
}

// SortedPartPaths returns the current part paths in lexicographic
// order. Useful for tests and diff tools.
func (w *Writer) SortedPartPaths() []string {
	out := make([]string, 0, len(w.parts))
	for _, p := range w.parts {
		out = append(out, p.path)
	}
	sort.Strings(out)
	return out
}
