package markdown

import (
	"context"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for Markdown files.
type Writer struct {
	format.BaseFormatWriter
	firstBlock bool
}

// NewWriter creates a new Markdown writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "markdown",
		},
		firstBlock: true,
	}
}

// Write consumes Parts from a channel and writes reconstructed Markdown.
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
		return fmt.Errorf("markdown writer: expected Block resource")
	}

	text := w.getBlockText(block)

	if !w.firstBlock {
		if _, err := fmt.Fprint(w.Output, "\n\n"); err != nil {
			return err
		}
	}
	w.firstBlock = false

	// Reconstruct heading prefix if applicable
	if block.Type == "heading" {
		if level, ok := block.Properties["level"]; ok {
			n := 0
			_, _ = fmt.Sscanf(level, "%d", &n)
			prefix := strings.Repeat("#", n) + " "
			if _, err := fmt.Fprint(w.Output, prefix); err != nil {
				return err
			}
		}
	}

	// Reconstruct list item prefix
	if block.Type == "list-item" {
		if _, err := fmt.Fprint(w.Output, "- "); err != nil {
			return err
		}
	}

	_, err := fmt.Fprint(w.Output, text)
	return err
}

func (w *Writer) writeData(part *model.Part) error {
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return fmt.Errorf("markdown writer: expected Data resource")
	}

	switch data.Name {
	case "code-block":
		if !w.firstBlock {
			if _, err := fmt.Fprint(w.Output, "\n\n"); err != nil {
				return err
			}
		}
		w.firstBlock = false

		lang := ""
		if l, ok := data.Properties["language"]; ok {
			lang = l
		}
		content := ""
		if c, ok := data.Properties["content"]; ok {
			content = c
		}
		if _, err := fmt.Fprintf(w.Output, "```%s\n%s```", lang, content); err != nil {
			return err
		}
	case "html-block":
		if !w.firstBlock {
			if _, err := fmt.Fprint(w.Output, "\n\n"); err != nil {
				return err
			}
		}
		w.firstBlock = false

		content := ""
		if c, ok := data.Properties["content"]; ok {
			content = c
		}
		if _, err := fmt.Fprint(w.Output, strings.TrimRight(content, "\n")); err != nil {
			return err
		}
	case "thematic-break":
		if !w.firstBlock {
			if _, err := fmt.Fprint(w.Output, "\n\n"); err != nil {
				return err
			}
		}
		w.firstBlock = false
		if _, err := fmt.Fprint(w.Output, "---"); err != nil {
			return err
		}
	}

	return nil
}

func (w *Writer) getBlockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return block.TargetText(w.Locale)
	}
	return block.SourceText()
}
