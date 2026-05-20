package regex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for regex-based text extraction.
// It reconstructs the original document by replacing source text in matched
// regions with translated text (or source text if no translation exists).
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	cfg           *Config
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

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

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed output.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		return w.writeWithSkeleton(ctx, parts)
	}

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
		return errors.New("regex writer: expected Block resource")
	}

	// Reconstruct the match by pure assembly: prefix + escape(value) + suffix.
	// The prefix and suffix are the raw document bytes recorded by the reader
	// around the translatable capture, so no string replacement over
	// reconstructed full-match text is needed.
	_, err := io.WriteString(w.Output, w.renderBlock(block))
	return err
}

// renderBlock builds the output text for a block by assembling the raw prefix,
// the escaped value (target if available, else source), and the raw suffix.
func (w *Writer) renderBlock(block *model.Block) string {
	// Get the text to write (target if available, else source)
	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}

	// Re-escape the value (benign per-value escaping; never touches the
	// surrounding prefix/suffix which are already raw document bytes).
	text = w.escape(text)

	prefix, hasPrefix := block.Properties["regex.prefix"]
	suffix := block.Properties["regex.suffix"]
	if hasPrefix {
		return prefix + text + suffix
	}

	// Fallback for blocks lacking recorded offsets: write just the value.
	return text
}

func (w *Writer) writeData(part *model.Part) error {
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return errors.New("regex writer: expected Data resource")
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
	for i := range len(s) {
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

// writeWithSkeleton collects all blocks, then reconstructs output from skeleton entries.
func (w *Writer) writeWithSkeleton(ctx context.Context, parts <-chan *model.Part) error {
	blocksByID := make(map[string]*model.Block)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				goto done
			}
			if part.Type == model.PartBlock {
				if block, ok := part.Resource.(*model.Block); ok {
					blocksByID[block.ID] = block
				}
			}
		}
	}
done:
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("regex writer: flush skeleton: %w", err)
	}
	return w.writeFromSkeleton(blocksByID)
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton(blocks map[string]*model.Block) error {
	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("regex writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				// Reconstruct the match by pure assembly (prefix + value + suffix),
				// identical to the non-skeleton path.
				if _, err := io.WriteString(w.Output, w.renderBlock(block)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
