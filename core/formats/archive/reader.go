package archive

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"

	"github.com/neokapi/neokapi/core/container"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

// Reader is a read-only reader over an archive container. Each recognised entry
// is surfaced as a child Layer carrying that entry's own part stream (for
// inspection / analysis); unrecognised or binary entries are listed as Data.
// There is no archive writer — see the package doc.
type Reader struct {
	format.BaseFormatReader
	cfg      *Config
	resolver format.SubfilterResolver
	data     []byte
	layerSeq int
}

var _ format.SubfilterAware = (*Reader)(nil)

// errWalkStop unwinds container.Walk when the part consumer's context is done.
var errWalkStop = errors.New("archive: walk stopped")

// NewReader creates an archive reader bound to the resolver it uses to parse
// recognised entries. The resolver is supplied by the registration factory (it
// is the format registry).
func NewReader(resolver format.SubfilterResolver) *Reader {
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
		resolver: resolver,
	}
}

// SetSubfilterResolver overrides the resolver used to parse entries.
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
		MagicBytes: [][]byte{{0x50, 0x4B, 0x03, 0x04}, {0x1f, 0x8b}},
	}
}

// Open buffers the archive bytes (bounded by the shared safeio budget).
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

	// Stream entries one at a time (Walk materialises only the current entry,
	// not the full set) so inspecting a large archive does not hold every
	// decompressed member at once.
	stopped := false
	_, err := container.Walk(r.data, func(e container.Entry) error {
		if !r.processEntry(ctx, ch, rootLayer.ID, locale, e.Name, e.Data) {
			stopped = true
			return errWalkStop
		}
		return nil
	})
	if stopped {
		return
	}
	if err != nil {
		r.emitErr(ch, fmt.Errorf("archive: %w", err))
		return
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: rootLayer})
}

// processEntry surfaces one entry: parse it through its own reader when
// recognised, else list it as a Data part.
func (r *Reader) processEntry(ctx context.Context, ch chan<- model.PartResult, rootID string,
	locale model.LocaleID, name string, content []byte) bool {

	if r.resolver != nil && r.cfg.matches(name) {
		if fmtName, derr := r.detect(name, content); derr == nil && canInspect(r.resolver, fmtName) {
			return r.emitChild(ctx, ch, rootID, locale, name, fmtName, content)
		}
	}
	return r.emitData(ctx, ch, name, uint64(len(content)))
}

// detect resolves an entry's format by name+content via the registry detector,
// when the resolver exposes one.
func (r *Reader) detect(name string, content []byte) (string, error) {
	d, ok := r.resolver.(interface {
		Detector() *format.Detector
	})
	if !ok {
		return "", errors.New("archive: resolver has no detector")
	}
	return d.Detector().Detect(name, bytes.NewReader(content), "")
}

// emitChild wraps an entry's full sub-document part stream (root layer included)
// in an archive child Layer, so an inspector sees the entry exactly as if it had
// been read standalone.
func (r *Reader) emitChild(ctx context.Context, ch chan<- model.PartResult, rootID string,
	locale model.LocaleID, name, fmtName string, content []byte) bool {

	subReader, err := r.resolver.ResolveReader(fmtName)
	if err != nil {
		return r.emitData(ctx, ch, name, uint64(len(content)))
	}
	if sa, ok := subReader.(format.SubfilterAware); ok {
		sa.SetSubfilterResolver(r.resolver)
	}

	subDoc := &model.RawDocument{
		URI:          name,
		SourceLocale: locale,
		Encoding:     "UTF-8",
		Reader:       io.NopCloser(bytes.NewReader(content)),
	}
	if err := subReader.Open(ctx, subDoc); err != nil {
		return r.emitData(ctx, ch, name, uint64(len(content)))
	}

	// Buffer the entry's parts so that a sub-reader which fails partway through —
	// a binary member mis-detected as a text format (e.g. a `.dat` blob matched
	// to fixedwidth, whose scanner then hits "token too long"), a corrupt file,
	// an over-long line — falls back to listing the entry as opaque Data, exactly
	// like an Open failure, instead of aborting the WHOLE archive (and every
	// sibling archive on the command line). Only one entry's parts are held at a
	// time, preserving the archive's stream-one-member-at-a-time memory profile.
	var parts []*model.Part
	for pr := range subReader.Read(ctx) {
		if pr.Error != nil {
			subReader.Close()
			r.AddDiagnostic(format.Diagnostic{
				Severity: format.SeverityMinor,
				Category: "archive.entry-unreadable",
				Message:  fmt.Sprintf("%s: %v", name, pr.Error),
			})
			return r.emitData(ctx, ch, name, uint64(len(content)))
		}
		parts = append(parts, pr.Part)
	}
	subReader.Close()

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
	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: childLayer}) {
		return false
	}
	for _, part := range parts {
		// Stamp each block with its originating entry so downstream consumers
		// (inspect, word-count, grep) can attribute it to `<archive>!<entry>`
		// without tracking the enclosing child layer. The sub-reader's own blocks
		// carry their format-local Name/keypath, not the entry, so we add it here.
		if part != nil && part.Type == model.PartBlock {
			if b, ok := part.Resource.(*model.Block); ok && b != nil {
				if b.Properties == nil {
					b.Properties = map[string]string{}
				}
				b.Properties[model.PropContainerEntry] = name
			}
		}
		if !r.emit(ctx, ch, part) {
			return false
		}
	}
	return r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: childLayer})
}

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

// Close releases buffered bytes and the document reader.
func (r *Reader) Close() error {
	r.data = nil
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
