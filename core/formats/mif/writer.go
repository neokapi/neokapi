package mif

import (
	"context"
	"fmt"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for MIF files.
type Writer struct {
	format.BaseFormatWriter
	wroteVersion bool
}

// NewWriter creates a new MIF writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "mif",
		},
	}
}

// Write consumes Parts from a channel and writes reconstructed MIF.
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
		return fmt.Errorf("mif writer: expected Block resource")
	}

	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}

	// Escape the text for MIF string format.
	escaped := escapeMIF(text)

	pgfTag := block.Properties["pgf_tag"]
	if pgfTag == "" {
		pgfTag = "Body"
	}

	if _, err := fmt.Fprintf(w.Output, " <Para\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w.Output, "  <PgfTag `%s'>\n", pgfTag); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w.Output, "  <ParaLine\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w.Output, "   <String `%s'>\n", escaped); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w.Output, "  >\n"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w.Output, " >\n"); err != nil {
		return err
	}

	return nil
}

func (w *Writer) writeData(part *model.Part) error {
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return fmt.Errorf("mif writer: expected Data resource")
	}

	if data.Properties["tag"] == "MIFFile" {
		version := data.Properties["version"]
		if version == "" {
			version = "2015"
		}
		if _, err := fmt.Fprintf(w.Output, "<MIFFile %s>\n", version); err != nil {
			return err
		}
		w.wroteVersion = true
		return nil
	}

	raw := data.Properties["raw"]
	if raw != "" {
		if _, err := fmt.Fprint(w.Output, raw); err != nil {
			return err
		}
	}
	return nil
}

// escapeMIF escapes special characters for MIF string values.
func escapeMIF(s string) string {
	var out []byte
	for _, r := range s {
		switch r {
		case '`':
			out = append(out, '\\', '`')
		case '\'':
			out = append(out, '\\', '\'')
		case '\\':
			out = append(out, '\\', '\\')
		case '>':
			out = append(out, '\\', '>')
		case '\t':
			out = append(out, '\\', 't')
		case '\n':
			// Newlines in MIF strings should be represented with Char HardReturn.
			// For simplicity, we keep the newline in the string.
			out = append(out, '\\', 'n')
		default:
			out = append(out, byte(r))
		}
	}
	return string(out)
}
