package dtd

import (
	"context"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for DTD files.
type Writer struct {
	format.BaseFormatWriter
	firstEntry bool
}

// NewWriter creates a new DTD writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "dtd",
		},
		firstEntry: true,
	}
}

// Write consumes Parts from a channel and writes reconstructed DTD.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return nil
			}
			if err := w.writePart(part); err != nil {
				return err
			}
		}
	}
}

func (w *Writer) writePart(part *model.Part) error {
	switch part.Type {
	case model.PartBlock:
		return w.writeBlock(part)
	default:
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return fmt.Errorf("dtd writer: expected Block resource")
	}

	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}

	name := block.Name
	if name == "" {
		name = block.ID
	}

	// Write comment if block has a note annotation
	if noteAnn, ok := block.Annotations["note"]; ok {
		if note, ok := noteAnn.(*model.NoteAnnotation); ok && note.Text != "" {
			if !w.firstEntry {
				if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(w.Output, "<!--%s-->\n", note.Text); err != nil {
				return err
			}
			w.firstEntry = false
		}
	}

	// Escape the value for DTD output
	escaped := escapeEntityValue(text)

	if w.firstEntry {
		w.firstEntry = false
	} else if _, hasNote := block.Annotations["note"]; !hasNote {
		if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(w.Output, "<!ENTITY %s \"%s\">\n", name, escaped); err != nil {
		return err
	}

	return nil
}

// escapeEntityValue escapes characters that need encoding in DTD entity values.
func escapeEntityValue(s string) string {
	var buf strings.Builder
	for _, r := range s {
		switch r {
		case '&':
			buf.WriteString("&amp;")
		case '"':
			buf.WriteString("&quot;")
		case '<':
			buf.WriteString("&lt;")
		case '>':
			buf.WriteString("&gt;")
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
