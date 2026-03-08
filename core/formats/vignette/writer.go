package vignette

import (
	"context"
	"fmt"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for R Vignette files.
type Writer struct {
	format.BaseFormatWriter
	firstPart bool
}

// NewWriter creates a new R Vignette writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "vignette",
		},
		firstPart: true,
	}
}

// Write consumes Parts from a channel and writes reconstructed vignette content.
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
		return fmt.Errorf("vignette writer: expected Block resource")
	}

	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}

	if !w.firstPart {
		if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
			return err
		}
	}
	w.firstPart = false

	_, err := fmt.Fprint(w.Output, text)
	return err
}

func (w *Writer) writeData(part *model.Part) error {
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return nil
	}

	if !w.firstPart {
		if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
			return err
		}
	}
	w.firstPart = false

	typ := data.Properties["type"]
	line := data.Properties["line"]

	switch typ {
	case "yaml-frontmatter":
		_, err := fmt.Fprint(w.Output, "---")
		return err
	case "yaml-content", "code", "rmd-code", "rnw-code":
		_, err := fmt.Fprint(w.Output, line)
		return err
	default:
		// Whitespace or other structural data
		return nil
	}
}
