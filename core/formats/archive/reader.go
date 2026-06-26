package archive

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// Reader implements DataFormatReader for archive containers (ZIP/TAR/TAR.GZ).
// Each translatable entry is emitted as a child Layer whose body is the full
// part stream of the entry's own format reader (root layer included), so the
// matching sub-writer can reconstruct that entry exactly as if it had been
// processed standalone. Non-translatable entries are emitted as Data parts for
// visibility and copied byte-for-byte by the writer.
type Reader struct {
	format.BaseFormatReader
	cfg      *Config
	detector *format.Detector
	resolver format.SubfilterResolver
	data     []byte // buffered archive bytes
	layerSeq int    // counter for unique child layer IDs
}

var _ format.SubfilterAware = (*Reader)(nil)

// NewReader creates an archive reader bound to the format detector and resolver
// it uses to classify and parse entries. Both are supplied by the registration
// factory (they are the format registry).
func NewReader(detector *format.Detector, resolver format.SubfilterResolver) *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "archive",
			FormatDisplayName: "Archive (ZIP/TAR)",
			FormatMimeType:    "application/zip",
			FormatExtensions:  []string{".zip", ".tar", ".tgz", ".tar.gz"},
			Cfg:               cfg,
		},
		cfg:      cfg,
		detector: detector,
		resolver: resolver,
	}
}

// SetSubfilterResolver overrides the resolver used to parse entries. The
// resolver is normally injected at construction; this satisfies SubfilterAware
// so a caller (or test) can substitute one.
func (r *Reader) SetSubfilterResolver(resolver format.SubfilterResolver) {
	r.resolver = resolver
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes: []string{
			"application/zip", "application/x-tar",
			"application/gzip", "application/x-gzip",
		},
		Extensions: []string{".zip", ".tar", ".tgz", ".tar.gz"},
		MagicBytes: [][]byte{zipMagic, gzipMagic},
	}
}

// Open buffers the archive bytes (bounded by the shared safeio budget so an
// oversized stream fails with a typed error). Random access is needed for ZIP
// central-directory parsing, so the whole container is held in memory.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("archive: nil document or reader")
	}
	r.Doc = doc
	data, err := io.ReadAll(safeio.DefaultBudget().Reader(doc.Reader))
	if err != nil {
		return fmt.Errorf("archive: reading container: %w", err)
	}
	r.data = data
	return nil
}

// Read returns a channel of PartResults.
func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	ch := make(chan model.PartResult, 64)
	go func() {
		defer close(ch)
		r.readContent(ctx, ch)
	}()
	return ch
}

func (r *Reader) readContent(ctx context.Context, ch chan<- model.PartResult) {
	locale := r.Doc.SourceLocale
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	rootLayer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "archive",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/zip",
	}
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: rootLayer}) {
		return
	}

	switch detectKind(r.data) {
	case kindZip:
		if !r.readZip(ctx, ch, rootLayer.ID, locale) {
			return
		}
	case kindTar:
		if !r.readTar(ctx, ch, bytes.NewReader(r.data), rootLayer.ID, locale) {
			return
		}
	case kindTarGz:
		gz, err := gzip.NewReader(bytes.NewReader(r.data))
		if err != nil {
			r.emitErr(ch, fmt.Errorf("archive: opening gzip: %w", err))
			return
		}
		defer gz.Close()
		if !r.readTar(ctx, ch, safeio.DefaultBudget().Reader(gz), rootLayer.ID, locale) {
			return
		}
	default:
		r.emitErr(ch, errors.New("archive: unrecognised container (expected ZIP, TAR, or TAR.GZ)"))
		return
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: rootLayer})
}

// readZip walks the ZIP central directory, sub-filtering eligible entries and
// emitting the rest as Data. Returns false if the context was cancelled.
func (r *Reader) readZip(ctx context.Context, ch chan<- model.PartResult, rootID string, locale model.LocaleID) bool {
	zr, err := zip.NewReader(bytes.NewReader(r.data), int64(len(r.data)))
	if err != nil {
		r.emitErr(ch, fmt.Errorf("archive: opening zip: %w", err))
		return false
	}
	if err := safeio.DefaultZipLimits.CheckReader(zr); err != nil {
		r.emitErr(ch, fmt.Errorf("archive: %w", err))
		return false
	}

	guard := safeio.DefaultZipLimits.NewGuard()
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		content, err := guard.ReadEntry(f)
		if err != nil {
			r.emitErr(ch, fmt.Errorf("archive: reading %s: %w", f.Name, err))
			return false
		}
		if !r.processEntry(ctx, ch, rootID, locale, f.Name, content, f.UncompressedSize64) {
			return false
		}
	}
	return true
}

// readTar walks a (possibly gzip-wrapped) TAR stream sequentially. Per-entry
// size is bounded by the shared byte budget.
func (r *Reader) readTar(ctx context.Context, ch chan<- model.PartResult, src io.Reader, rootID string, locale model.LocaleID) bool {
	tr := tar.NewReader(src)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return true
		}
		if err != nil {
			r.emitErr(ch, fmt.Errorf("archive: reading tar: %w", err))
			return false
		}
		if !hdr.FileInfo().Mode().IsRegular() {
			continue
		}
		content, err := io.ReadAll(safeio.DefaultBudget().Reader(tr))
		if err != nil {
			r.emitErr(ch, fmt.Errorf("archive: reading %s: %w", hdr.Name, err))
			return false
		}
		if !r.processEntry(ctx, ch, rootID, locale, hdr.Name, content, uint64(len(content))) {
			return false
		}
	}
}

// processEntry routes one entry: sub-filter it when eligible, otherwise emit it
// as a Data part. Returns false if the context was cancelled.
func (r *Reader) processEntry(ctx context.Context, ch chan<- model.PartResult, rootID string,
	locale model.LocaleID, name string, content []byte, size uint64) bool {

	if r.resolver != nil && r.detector != nil && r.cfg.matches(name) {
		if fmtName, err := r.detector.Detect(name, bytes.NewReader(content), ""); err == nil &&
			canSubfilter(r.resolver, fmtName) {
			return r.emitChild(ctx, ch, rootID, locale, name, fmtName, content)
		}
	}
	return r.emitData(ctx, ch, name, size)
}

// emitChild wraps an entry's full sub-document part stream in an archive child
// Layer. The sub-reader's own root layer is preserved (not skipped) so the
// matching sub-writer reconstructs the entry exactly — e.g. the JSON reader's
// "json.original" property rides through for byte-faithful JSON output.
func (r *Reader) emitChild(ctx context.Context, ch chan<- model.PartResult, rootID string,
	locale model.LocaleID, name, fmtName string, content []byte) bool {

	subReader, err := r.resolver.ResolveReader(fmtName)
	if err != nil {
		return r.emitData(ctx, ch, name, uint64(len(content)))
	}
	if sa, ok := subReader.(format.SubfilterAware); ok {
		sa.SetSubfilterResolver(r.resolver)
	}

	r.layerSeq++
	childLayer := &model.Layer{
		ID:       fmt.Sprintf("ar%d", r.layerSeq),
		Name:     name,
		Format:   fmtName,
		Locale:   locale,
		ParentID: rootID,
		Properties: map[string]string{
			"subfilter.source": "archive",
			"entry":            name,
		},
	}

	subDoc := &model.RawDocument{
		URI:          name,
		SourceLocale: locale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}
	if err := subReader.Open(ctx, subDoc); err != nil {
		// Treat a sub-reader open failure as a pass-through rather than failing
		// the whole archive.
		return r.emitData(ctx, ch, name, uint64(len(content)))
	}

	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: childLayer}) {
		subReader.Close()
		return false
	}
	for pr := range subReader.Read(ctx) {
		if pr.Error != nil {
			r.emitErr(ch, fmt.Errorf("archive: parsing %s: %w", name, pr.Error))
			subReader.Close()
			return false
		}
		if !r.emit(ctx, ch, pr.Part) {
			subReader.Close()
			return false
		}
	}
	subReader.Close()
	return r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
}

// emitData emits a non-translatable entry as a Data part (for inspection); the
// writer copies its bytes from the original archive.
func (r *Reader) emitData(ctx context.Context, ch chan<- model.PartResult, name string, size uint64) bool {
	data := &model.Data{
		ID:   name,
		Name: name,
		Properties: map[string]string{
			"entry": name,
			"size":  strconv.FormatUint(size, 10),
		},
	}
	return r.emit(ctx, ch, &model.Part{Type: model.PartData, Resource: data})
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

func (r *Reader) emitErr(ch chan<- model.PartResult, err error) {
	ch <- model.PartResult{Error: err}
}

// Close releases the underlying document reader.
func (r *Reader) Close() error {
	r.data = nil
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
