package doxygen

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for Doxygen/Javadoc comments in source code.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new Doxygen writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "doxygen",
		},
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed source.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		return w.writeWithSkeleton(ctx, parts)
	}

	first := true
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return nil
			}
			if err := w.writePart(part, &first); err != nil {
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
		return fmt.Errorf("doxygen writer: flush skeleton: %w", err)
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
			return fmt.Errorf("doxygen writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				text := w.blockText(block)
				style := block.Properties["style"]
				raw := block.Properties["raw"]
				if err := w.writeCommentStyled(text, style, raw); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// writeCommentStyled reconstructs a comment using the given style.
func (w *Writer) writeCommentStyled(text, style, raw string) error {
	switch style {
	case "triple":
		return w.writeTripleSlash(text, raw)
	case "exclamation":
		return w.writeExclamation(text, raw)
	case "javadoc":
		return w.writeJavadoc(text, raw)
	case "qt":
		return w.writeQt(text, raw)
	case "trailing":
		return w.writeTrailing(text, raw)
	case "trailing_qt":
		return w.writeTrailingQt(text, raw)
	default:
		return w.writeTripleSlash(text, raw)
	}
}

// blockText returns target or source text for a block.
func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return block.TargetText(w.Locale)
	}
	return block.SourceText()
}

func (w *Writer) writePart(part *model.Part, first *bool) error {
	switch part.Type {
	case model.PartData:
		return w.writeData(part, first)
	case model.PartBlock:
		return w.writeBlock(part, first)
	default:
		return nil
	}
}

func (w *Writer) writeData(part *model.Part, first *bool) error {
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return errors.New("doxygen writer: expected Data resource")
	}

	raw := data.Properties["raw"]
	if raw == "" {
		return nil
	}

	if !*first {
		if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
			return err
		}
	}
	*first = false

	_, err := fmt.Fprint(w.Output, raw)
	return err
}

func (w *Writer) writeBlock(part *model.Part, first *bool) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return errors.New("doxygen writer: expected Block resource")
	}

	text := block.SourceText()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = block.TargetText(w.Locale)
	}

	style := block.Properties["style"]
	raw := block.Properties["raw"]

	if !*first {
		if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
			return err
		}
	}
	*first = false

	// Reconstruct the comment using the original style
	switch style {
	case "triple":
		return w.writeTripleSlash(text, raw)
	case "exclamation":
		return w.writeExclamation(text, raw)
	case "javadoc":
		return w.writeJavadoc(text, raw)
	case "qt":
		return w.writeQt(text, raw)
	case "trailing":
		return w.writeTrailing(text, raw)
	case "trailing_qt":
		return w.writeTrailingQt(text, raw)
	default:
		// Fallback: write as triple-slash
		return w.writeTripleSlash(text, raw)
	}
}

// writeTripleSlash writes text as /// line comments, preserving indentation from the original.
func (w *Writer) writeTripleSlash(text, raw string) error {
	indent := extractIndent(raw)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i > 0 {
			if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w.Output, "%s/// %s", indent, line); err != nil {
			return err
		}
	}
	return nil
}

// writeExclamation writes text as //! line comments.
func (w *Writer) writeExclamation(text, raw string) error {
	indent := extractIndent(raw)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i > 0 {
			if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w.Output, "%s//! %s", indent, line); err != nil {
			return err
		}
	}
	return nil
}

// writeJavadoc writes text as a /** ... */ block comment.
func (w *Writer) writeJavadoc(text, raw string) error {
	indent := extractIndent(raw)
	rawLines := strings.Split(raw, "\n")

	// Single-line block comment
	if len(rawLines) == 1 {
		_, err := fmt.Fprintf(w.Output, "%s/** %s */", indent, text)
		return err
	}

	// Multi-line block comment
	lines := strings.Split(text, "\n")
	if _, err := fmt.Fprintf(w.Output, "%s/**", indent); err != nil {
		return err
	}
	for _, line := range lines {
		if _, err := fmt.Fprintf(w.Output, "\n%s * %s", indent, line); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w.Output, "\n%s */", indent)
	return err
}

// writeQt writes text as a /*! ... */ block comment.
func (w *Writer) writeQt(text, raw string) error {
	indent := extractIndent(raw)
	rawLines := strings.Split(raw, "\n")

	// Single-line block comment
	if len(rawLines) == 1 {
		_, err := fmt.Fprintf(w.Output, "%s/*! %s */", indent, text)
		return err
	}

	// Multi-line block comment
	lines := strings.Split(text, "\n")
	if _, err := fmt.Fprintf(w.Output, "%s/*!", indent); err != nil {
		return err
	}
	for _, line := range lines {
		if _, err := fmt.Fprintf(w.Output, "\n%s  %s", indent, line); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w.Output, "\n%s*/", indent)
	return err
}

// writeTrailing writes text as a trailing ///< comment.
func (w *Writer) writeTrailing(text, _ string) error {
	_, err := fmt.Fprintf(w.Output, "///< %s", text)
	return err
}

// writeTrailingQt writes text as a trailing /*!< ... */ comment.
func (w *Writer) writeTrailingQt(text, _ string) error {
	_, err := fmt.Fprintf(w.Output, "/*!< %s */", text)
	return err
}

// extractIndent returns the leading whitespace from the first line of raw text.
func extractIndent(raw string) string {
	firstLine := raw
	if idx := strings.IndexByte(raw, '\n'); idx >= 0 {
		firstLine = raw[:idx]
	}
	trimmed := strings.TrimLeft(firstLine, " \t")
	return firstLine[:len(firstLine)-len(trimmed)]
}
