package image

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/atomicfile"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/rootpath"
)

// Writer emits an image document: it writes the (single) Media part's bytes to
// the output. This is the whole-image sink: the localized image (a
// pseudo-localized variant the pseudo-translate tool produced, or a real
// per-locale replacement) is written out as-is. The Media's inline Data is
// preferred; if it carries only a URI reference, the source file is copied.
type Writer struct {
	format.BaseFormatWriter

	// root is the project directory, when the run has one (SetProjectRoot).
	root string
}

// NewWriter constructs an image writer.
func NewWriter() *Writer {
	return &Writer{FormatName: "image", RequiresSkeleton: true, Binary: true}
}

// SetProjectRoot sets the project directory. A Media URI is then read only
// when it lies beneath it, and the alt-text sidecar is written through it.
func (w *Writer) SetProjectRoot(root string) { w.root = root }

// Write consumes the part stream and writes the first Media part's image bytes.
// A caption Block (RoleCaption, the image's alt text) is folded back into a
// per-locale "<output>.alt.txt" sidecar, using its localized target when present
// and falling back to its source text so the alt text round-trips.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	var media *model.Media
	var altText string
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case p, ok := <-parts:
			if !ok {
				if media == nil {
					return errors.New("image: no media part to write")
				}
				if err := w.writeMedia(media); err != nil {
					return err
				}
				return w.writeAltSidecar(altText)
			}
			if p == nil {
				continue
			}
			switch res := p.Resource.(type) {
			case *model.Media:
				if media == nil {
					media = res // first Media (the page image) wins
				}
			case *model.Block:
				if altText == "" && res.SemanticRole() == model.RoleCaption {
					if t := res.TargetText(w.Locale); t != "" {
						altText = t
					} else {
						altText = res.SourceText()
					}
				}
			}
		}
	}
}

// writeAltSidecar writes the (localized) caption text beside the output image as
// "<output>.alt.txt". It is a no-op when there is no caption or when the output
// is an unnamed io.Writer (no path to anchor the sidecar to).
func (w *Writer) writeAltSidecar(text string) error {
	if text == "" || w.OutputFile == nil {
		return nil
	}
	return writeBeside(w.root, altSidecarPath(w.OutputFile.Name()), []byte(text+"\n"))
}

// writeBeside writes data to p atomically: through root when p lies beneath
// it, so a link in the project cannot redirect the write, and as named when
// it lies beneath no root (an output a person named).
func writeBeside(root, p string, data []byte) error {
	write := func(out io.Writer) error {
		_, err := out.Write(data)
		return err
	}
	if !rootpath.IsBeneath(root, p) {
		_, err := atomicfile.Replace(p, write)
		return err
	}
	st, err := atomicfile.StageWithin(root, p, write)
	if err != nil {
		return err
	}
	return st.Commit()
}

func (w *Writer) writeMedia(m *model.Media) error {
	if w.Output == nil {
		return errors.New("image: no output configured")
	}
	if len(m.Data) > 0 {
		_, err := io.Copy(w.Output, bytes.NewReader(m.Data))
		return err
	}
	if m.URI == "" {
		return errors.New("image: media part has no bytes")
	}
	// The URI is content: within a project it must name a file beneath it.
	f, err := rootpath.OpenWithin(w.root, m.URI)
	if err != nil {
		return fmt.Errorf("image: read source %s: %w", m.URI, err)
	}
	defer f.Close()
	n, err := io.Copy(w.Output, f)
	if err == nil && n == 0 {
		return errors.New("image: media part has no bytes")
	}
	return err
}
