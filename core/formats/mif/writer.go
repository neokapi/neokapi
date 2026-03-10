package mif

import (
	"context"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for MIF files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	wroteVersion  bool
	blocks        []*model.Block
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new MIF writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "mif",
		},
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed MIF.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		// Collect all blocks, then write from skeleton
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case part, ok := <-parts:
				if !ok {
					return w.writeFromSkeleton()
				}
				if part.Type == model.PartBlock {
					if block, ok := part.Resource.(*model.Block); ok {
						w.blocks = append(w.blocks, block)
					}
				}
			}
		}
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

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton() error {
	if err := w.skeletonStore.Flush(); err != nil {
		return fmt.Errorf("mif writer: flush skeleton: %w", err)
	}

	for {
		entry, err := w.skeletonStore.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("mif writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			// Ref ID is "blockIdx:stringIdx"
			refID := string(entry.Data)
			parts := strings.SplitN(refID, ":", 2)
			if len(parts) != 2 {
				continue
			}
			blockIdx, err := strconv.Atoi(parts[0])
			if err != nil || blockIdx < 0 || blockIdx >= len(w.blocks) {
				continue
			}
			stringIdx, err := strconv.Atoi(parts[1])
			if err != nil {
				continue
			}

			block := w.blocks[blockIdx]
			text := block.SourceText()

			// For skeleton roundtrip, each String in the para is a separate ref.
			// If this is the only string (stringIdx==0 and it's the full text),
			// just write the text. For multi-string paras, we need to split.
			// Since the original strings were concatenated, and we only have the
			// combined text, for roundtrip we write the original text at stringIdx==0
			// and empty for others. For translated content, put all text in first string.
			if stringIdx == 0 {
				if _, err := io.WriteString(w.Output, escapeMIF(text)); err != nil {
					return err
				}
			}
			// For stringIdx > 0, write nothing (the text was combined into block text)
			// This preserves byte-exactness for single-string paras (the common case)
			// and puts translated text in the first string for multi-string paras.
		}
	}
	return nil
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
