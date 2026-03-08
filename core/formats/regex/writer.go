package regex

import (
	"context"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for regex-based text extraction.
// It reconstructs the original document by replacing source text in matched
// regions with translated text (or source text if no translation exists).
type Writer struct {
	format.BaseFormatWriter
	cfg *Config
}

// NewWriter creates a new Regex writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "regex",
		},
		cfg: cfg,
	}
}

// SetConfig applies a configuration to the writer.
func (w *Writer) SetConfig(cfg format.DataFormatConfig) error {
	if c, ok := cfg.(*Config); ok {
		w.cfg = c
		return nil
	}
	return fmt.Errorf("regex writer: invalid config type %T", cfg)
}

// Write consumes Parts from a channel and writes reconstructed output.
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
		return fmt.Errorf("regex writer: expected Block resource")
	}

	// Get the text to write (target if available, else source)
	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}

	// Re-escape the text for output
	text = w.escape(text)

	// Reconstruct the original match with the translated text
	fullMatch := block.Properties["regex.fullMatch"]
	sourceText := block.SourceText()

	if fullMatch != "" {
		// Re-escape the original source for matching against the full match string
		escapedOriginal := w.escape(sourceText)
		// Replace source text within the full match with translated text
		output := strings.Replace(fullMatch, escapedOriginal, text, 1)
		_, err := fmt.Fprint(w.Output, output)
		return err
	}

	// Fallback: write just the text
	_, err := fmt.Fprint(w.Output, text)
	return err
}

func (w *Writer) writeData(part *model.Part) error {
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return fmt.Errorf("regex writer: expected Data resource")
	}

	content := data.Properties["content"]
	if content != "" {
		_, err := fmt.Fprint(w.Output, content)
		return err
	}
	return nil
}

func (w *Writer) escape(s string) string {
	escType := w.cfg.EscapeType
	if escType == "" {
		escType = EscapeNone
	}

	switch escType {
	case EscapeBackslash:
		return escapeBackslash(s)
	case EscapeDoubleChar:
		return escapeDoubleChar(s, w.cfg.EscapeChar)
	default:
		return s
	}
}

func escapeBackslash(s string) string {
	var buf strings.Builder
	buf.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\':
			buf.WriteString("\\\\")
		case '"':
			buf.WriteString("\\\"")
		case '\n':
			buf.WriteString("\\n")
		case '\t':
			buf.WriteString("\\t")
		case '\r':
			buf.WriteString("\\r")
		default:
			buf.WriteByte(s[i])
		}
	}
	return buf.String()
}

func escapeDoubleChar(s string, escChar string) string {
	if escChar == "" {
		escChar = "\""
	}
	return strings.ReplaceAll(s, escChar, escChar+escChar)
}
