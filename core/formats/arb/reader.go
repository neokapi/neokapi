package arb

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"unsafe"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Reader implements DataFormatReader for Flutter Application Resource Bundle
// (.arb) files.
type Reader struct {
	format.BaseFormatReader
	cfg *Config
}

// NewReader creates a new ARB reader.
func NewReader() *Reader {
	cfg := &Config{}
	cfg.Reset()
	return &Reader{
		BaseFormatReader: format.BaseFormatReader{
			FormatName:        "arb",
			FormatDisplayName: "Flutter ARB",
			FormatMimeType:    "application/json",
			FormatExtensions:  []string{".arb"},
			Cfg:               cfg,
		},
		cfg: cfg,
	}
}

// Signature returns detection metadata for this format.
func (r *Reader) Signature() format.FormatSignature {
	return format.FormatSignature{
		MIMETypes:  []string{"application/json"},
		Extensions: []string{".arb"},
	}
}

// Open opens a RawDocument for reading.
func (r *Reader) Open(ctx context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("arb: nil document or reader")
	}
	r.Doc = doc
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
	content, err := io.ReadAll(r.Doc.Reader)
	if err != nil {
		ch <- model.PartResult{Error: fmt.Errorf("arb: reading: %w", err)}
		return
	}

	cat, err := parseCatalog(content)
	if err != nil {
		ch <- model.PartResult{Error: err}
		return
	}

	// ARB is monolingual: the file's locale comes from "@@locale", falling back
	// to the document's declared source locale, then English. The message value
	// is treated as content in this locale.
	locale := model.LocaleID(cat.locale)
	if locale.IsEmpty() {
		locale = r.Doc.SourceLocale
	}
	if locale.IsEmpty() {
		locale = model.LocaleEnglish
	}

	layer := &model.Layer{
		ID:       "doc1",
		Name:     r.Doc.URI,
		Format:   "arb",
		Locale:   locale,
		Encoding: r.Doc.Encoding,
		MimeType: "application/json",
		Properties: map[string]string{
			"arb.locale": cat.locale,
		},
	}
	// Preserve the original document bytes so the writer can produce
	// byte-faithful output, splicing only changed values. unsafe.String shares
	// the backing array — content is not mutated after this point.
	layer.Properties["arb.original"] = unsafe.String(unsafe.SliceData(content), len(content))

	if !r.emit(ctx, ch, &model.Part{Type: model.PartLayerStart, Resource: layer}) {
		return
	}

	blockCounter := 0
	for _, key := range cat.keyOrder {
		res := cat.resources[key]
		blockCounter++
		block := r.blockFor(res, locale, blockCounter)
		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return
		}
	}

	r.emit(ctx, ch, &model.Part{Type: model.PartLayerEnd, Resource: layer})
}

// blockFor builds a Block for one ARB resource. The message value is carried as
// source content; ICU placeholders/plural/select constructs are protected as
// opaque inline placeholder runs. The sibling "@<id>" description becomes a
// developer note.
func (r *Reader) blockFor(res *resource, locale model.LocaleID, counter int) *model.Block {
	runs := runsFromValue(res.value)

	block := &model.Block{
		ID:           "tu" + strconv.Itoa(counter),
		Name:         res.id,
		Translatable: true,
		SourceLocale: locale,
		Source:       []*model.Segment{{ID: "s1", Runs: runs}},
		Targets:      make(map[model.LocaleID][]*model.Segment),
		Properties:   make(map[string]string),
		Annotations:  make(map[string]model.Annotation),
	}
	block.Properties["arb.key"] = res.id

	if res.description != "" && r.cfg.DescriptionNotes {
		block.Annotations["note"] = &model.NoteAnnotation{
			Text:      res.description,
			From:      "developer",
			Annotates: "general",
		}
	}

	return block
}

func (r *Reader) emit(ctx context.Context, ch chan<- model.PartResult, part *model.Part) bool {
	select {
	case ch <- model.PartResult{Part: part}:
		return true
	case <-ctx.Done():
		return false
	}
}

// Close releases resources.
func (r *Reader) Close() error {
	if r.Doc != nil && r.Doc.Reader != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}
