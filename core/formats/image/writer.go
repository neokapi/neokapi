package image

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer emits an image document: it writes the (single) Media part's bytes to
// the output. This is the whole-image-localization sink — the localized image
// (e.g. a pseudo-localized variant produced by the pseudo-translate tool, or a
// real per-locale replacement) is written out as-is. The Media's inline Data is
// preferred; if it carries only a URI reference, the source file is copied.
type Writer struct {
	format.BaseFormatWriter
}

// NewWriter constructs an image writer.
func NewWriter() *Writer {
	return &Writer{BaseFormatWriter: format.BaseFormatWriter{FormatName: "image"}}
}

// Write consumes the part stream and writes the first Media part's image bytes.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	var media *model.Media
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case p, ok := <-parts:
			if !ok {
				if media == nil {
					return errors.New("image: no media part to write")
				}
				return w.writeMedia(media)
			}
			if p == nil {
				continue
			}
			if m, ok := p.Resource.(*model.Media); ok && media == nil {
				media = m // first Media (the page image) wins
			}
		}
	}
}

func (w *Writer) writeMedia(m *model.Media) error {
	if w.Output == nil {
		return errors.New("image: no output configured")
	}
	data := m.Data
	if len(data) == 0 && m.URI != "" {
		b, err := os.ReadFile(m.URI)
		if err != nil {
			return fmt.Errorf("image: read source %s: %w", m.URI, err)
		}
		data = b
	}
	if len(data) == 0 {
		return errors.New("image: media part has no bytes")
	}
	_, err := w.Output.Write(data)
	return err
}
