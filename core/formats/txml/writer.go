package txml

import (
	"context"
	"fmt"
	"io"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for Trados XML (TXML) files.
type Writer struct {
	format.BaseFormatWriter
	sourceLocale string
	targetLocale string
}

// NewWriter creates a new TXML writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "txml",
		},
	}
}

// Write consumes Parts from a channel and writes reconstructed TXML.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	headerWritten := false

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				if headerWritten {
					if _, err := io.WriteString(w.Output, "</body>\n</txml>\n"); err != nil {
						return err
					}
				}
				return nil
			}
			if part.Type == model.PartLayerStart {
				layer := part.Resource.(*model.Layer)
				w.sourceLocale = string(layer.Locale)
				if tl, ok := layer.Properties["target-locale"]; ok {
					w.targetLocale = tl
				}
				if !headerWritten {
					if err := w.writeHeader(); err != nil {
						return err
					}
					headerWritten = true
				}
				continue
			}
			if !headerWritten {
				if err := w.writeHeader(); err != nil {
					return err
				}
				headerWritten = true
			}
			if err := w.writePart(part); err != nil {
				return err
			}
		}
	}
}

func (w *Writer) writeHeader() error {
	if _, err := io.WriteString(w.Output, `<?xml version="1.0" encoding="utf-8"?>`+"\n"); err != nil {
		return err
	}
	sourceLocale := w.sourceLocale
	if sourceLocale == "" {
		sourceLocale = "en-US"
	}
	targetLocale := w.targetLocale
	if targetLocale == "" && !w.Locale.IsEmpty() {
		targetLocale = string(w.Locale)
	}
	if _, err := fmt.Fprintf(w.Output, `<txml locale="%s" targetlocale="%s" version="1.0" datatype="xml">`+"\n",
		xmlEscape(sourceLocale), xmlEscape(targetLocale)); err != nil {
		return err
	}
	if _, err := io.WriteString(w.Output, "<header/>\n<body>\n"); err != nil {
		return err
	}
	return nil
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
		return fmt.Errorf("txml writer: expected Block resource")
	}

	sourceText := block.SourceText()
	targetText := ""

	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		targetText = block.TargetText(w.Locale)
	}

	segType := block.Properties["segtype"]
	if segType == "" {
		segType = "block"
	}

	if _, err := fmt.Fprintf(w.Output, `<segment segtype="%s">`+"\n", xmlEscape(segType)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w.Output, "<source>%s</source>\n", xmlEscape(sourceText)); err != nil {
		return err
	}
	if targetText != "" {
		if _, err := fmt.Fprintf(w.Output, "<target>%s</target>\n", xmlEscape(targetText)); err != nil {
			return err
		}
	}
	if _, err := io.WriteString(w.Output, "</segment>\n"); err != nil {
		return err
	}

	return nil
}

// xmlEscape escapes XML special characters.
func xmlEscape(s string) string {
	var buf []byte
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			buf = append(buf, []byte("&amp;")...)
		case '<':
			buf = append(buf, []byte("&lt;")...)
		case '>':
			buf = append(buf, []byte("&gt;")...)
		case '"':
			buf = append(buf, []byte("&quot;")...)
		default:
			buf = append(buf, s[i])
		}
	}
	return string(buf)
}
