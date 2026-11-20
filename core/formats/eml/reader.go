package eml

import (
	"context"
	"errors"
	"io"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/safeio"
)

type Reader struct {
	format.BaseFormatReader
	store *format.SkeletonStore
}

func NewReader() *Reader {
	return &Reader{BaseFormatReader: format.BaseFormatReader{
		FormatName: name, FormatDisplayName: displayName, FormatMimeType: mediaType,
		FormatExtensions: Signature().Extensions, Cfg: &Config{},
	}}
}

func (*Reader) Signature() format.FormatSignature { return Signature() }

// Each MIME entity declares its own charset.
func (*Reader) DecodesInput() {}

func (r *Reader) SetSkeletonStore(store *format.SkeletonStore) { r.store = store }

func (r *Reader) Open(_ context.Context, doc *model.RawDocument) error {
	if doc == nil || doc.Reader == nil {
		return errors.New("eml: missing document reader")
	}
	r.Doc = doc
	return nil
}

func (r *Reader) Close() error {
	if r.Doc != nil {
		return r.Doc.Reader.Close()
	}
	return nil
}

func (r *Reader) Read(ctx context.Context) <-chan model.PartResult {
	return format.StreamParts(ctx, func(ctx context.Context, out chan<- model.PartResult) error {
		raw, err := io.ReadAll(safeio.NewLimitedReader(r.Doc.Reader, maxMessageBytes))
		if err != nil {
			return err
		}
		fields, err := parseMessage(ctx, raw)
		if err != nil {
			return err
		}
		emit := func(part *model.Part) error {
			select {
			case out <- model.PartResult{Part: part}:
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		layer := &model.Layer{ID: "message", Name: r.Doc.URI, Format: name, MimeType: mediaType, Locale: r.Doc.SourceLocale}
		if err := emit(&model.Part{Type: model.PartLayerStart, Resource: layer}); err != nil {
			return err
		}
		cursor := 0
		for _, f := range fields {
			if r.store != nil {
				r.store.WriteText(raw[cursor:f.start])
				r.store.WriteOriginal([]byte(f.text), raw[f.start:f.end])
				r.store.WriteRef(f.name)
				cursor = f.end
			}
			b := model.NewBlock(f.name, f.text)
			b.Name = f.name
			b.SourceLocale = r.Doc.SourceLocale
			if f.name == "subject" {
				b.SetSemanticRole(model.RoleHeading, 1)
			} else {
				b.SetSemanticRole(model.RoleParagraph, 0)
			}
			if err := emit(&model.Part{Type: model.PartBlock, Resource: b}); err != nil {
				return err
			}
		}
		if r.store != nil {
			r.store.WriteText(raw[cursor:])
		}
		return emit(&model.Part{Type: model.PartLayerEnd, Resource: layer})
	})
}
