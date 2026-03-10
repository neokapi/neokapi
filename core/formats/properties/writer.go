package properties

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for Java Properties files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	firstLine     bool
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new Properties writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "properties",
		},
		firstLine: true,
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed properties content.
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
		return fmt.Errorf("properties writer: flush skeleton: %w", err)
	}
	return w.writeFromSkeleton(blocksByID)
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton(blocks map[string]*model.Block) error {
	for {
		entry, err := w.skeletonStore.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("properties writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				text := w.blockValue(block)
				if _, err := io.WriteString(w.Output, text); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// blockValue returns the appropriate value text for skeleton output.
// If the block has a target translation, it encodes it. Otherwise it
// uses the raw value stored during reading for byte-exact roundtrip.
func (w *Writer) blockValue(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return encodePropertyValue(block.TargetText(w.Locale))
	}
	// Use raw value for byte-exact roundtrip when no translation
	if raw, ok := block.Properties["rawValue"]; ok {
		return raw
	}
	return encodePropertyValue(block.SourceText())
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
		return fmt.Errorf("properties writer: expected Block resource")
	}

	// Use target text if available, otherwise source text
	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}

	// Encode unicode escapes for non-ASCII characters
	text = encodePropertyValue(text)

	sep := "="
	if s, ok := block.Properties["separator"]; ok && s != "" {
		sep = s
	}

	w.writeLine()
	_, err := fmt.Fprintf(w.Output, "%s%s%s", block.Name, sep, text)
	return err
}

func (w *Writer) writeData(part *model.Part) error {
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return fmt.Errorf("properties writer: expected Data resource")
	}

	switch data.Name {
	case "comment":
		comment := data.Properties["comment"]
		w.writeLine()
		_, err := fmt.Fprint(w.Output, comment)
		return err
	case "blank":
		w.writeLine()
		return nil
	}

	return nil
}

func (w *Writer) writeLine() {
	if !w.firstLine {
		fmt.Fprintln(w.Output)
	}
	w.firstLine = false
}

// encodePropertyValue encodes special characters in a property value:
// non-ASCII -> \uXXXX, newline -> \n, tab -> \t, CR -> \r, backslash -> \\.
func encodePropertyValue(s string) string {
	var buf strings.Builder
	for _, r := range s {
		switch {
		case r == '\\':
			buf.WriteString("\\\\")
		case r == '\n':
			buf.WriteString("\\n")
		case r == '\t':
			buf.WriteString("\\t")
		case r == '\r':
			buf.WriteString("\\r")
		case r > 127:
			buf.WriteString(fmt.Sprintf("\\u%04X", r))
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
