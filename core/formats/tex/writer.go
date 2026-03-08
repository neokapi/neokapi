package tex

import (
	"context"
	"fmt"
	"io"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for TeX/LaTeX files.
type Writer struct {
	format.BaseFormatWriter
	firstPart bool
}

// NewWriter creates a new TeX/LaTeX writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "tex",
		},
		firstPart: true,
	}
}

// Write consumes Parts from a channel and writes reconstructed TeX.
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
	case model.PartData:
		return w.writeData(part)
	default:
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return fmt.Errorf("tex writer: expected Block resource")
	}

	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}

	// Reconstruct TeX structure based on block type
	switch block.Type {
	case "section", "subsection", "subsubsection", "chapter", "part",
		"paragraph", "subparagraph", "caption":
		if !w.firstPart {
			if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
				return err
			}
		}
		_, err := fmt.Fprintf(w.Output, "\\%s{%s}", block.Type, text)
		w.firstPart = false
		return err
	case "title", "author", "date":
		_, err := fmt.Fprintf(w.Output, "\\%s{%s}", block.Type, text)
		w.firstPart = false
		return err
	default:
		// Regular paragraph
		if !w.firstPart {
			if _, err := fmt.Fprint(w.Output, "\n\n"); err != nil {
				return err
			}
		}
		w.firstPart = false
		_, err := io.WriteString(w.Output, text)
		return err
	}
}

func (w *Writer) writeData(part *model.Part) error {
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return fmt.Errorf("tex writer: expected Data resource")
	}

	content := ""
	if data.Properties != nil {
		content = data.Properties["content"]
	}

	if content != "" {
		if !w.firstPart {
			if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
				return err
			}
		}
		w.firstPart = false
		_, err := io.WriteString(w.Output, content)
		return err
	}
	return nil
}
