package doxygen

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
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
//
// For multi-section comment groups (a single /*! ... */ comment whose
// reader emitted several Blocks — one per \param/\return/\sa
// section — to honour the spec contract that section commands extract
// as separate Blocks), the writer must emit the whole group's text
// under one comment template, not one comment per Block. Both the
// skeleton-driven and skeleton-less paths therefore buffer the full
// part stream up front and resolve sibling lookups via blocksByID.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	if w.skeletonStore != nil {
		return w.writeWithSkeleton(ctx, parts)
	}

	var collected []*model.Part
	blocksByID := make(map[string]*model.Block)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				return w.writeCollected(collected, blocksByID)
			}
			collected = append(collected, part)
			if part.Type == model.PartBlock {
				if block, ok := part.Resource.(*model.Block); ok {
					blocksByID[block.ID] = block
				}
			}
		}
	}
}

// writeCollected drains the buffered Parts in order, skipping
// non-first blocks of a comment group (they fold into the first
// block's output via writeBlockGrouped → joinedGroupText).
func (w *Writer) writeCollected(collected []*model.Part, blocksByID map[string]*model.Block) error {
	first := true
	for _, part := range collected {
		if part.Type == model.PartBlock {
			block, ok := part.Resource.(*model.Block)
			if !ok {
				return errors.New("doxygen writer: expected Block resource")
			}
			// Subsequent blocks of a multi-section group are folded
			// into the first block's comment-template output.
			if firstID := block.Properties["groupFirstID"]; firstID != "" && firstID != block.ID {
				continue
			}
			if err := w.writeBlockGrouped(block, &first, blocksByID); err != nil {
				return err
			}
			continue
		}
		if err := w.writePart(part, &first); err != nil {
			return err
		}
	}
	return nil
}

// writeBlockGrouped writes a single Block, joining sibling blocks of
// the same comment group when present.
func (w *Writer) writeBlockGrouped(block *model.Block, first *bool, blocks map[string]*model.Block) error {
	text := w.joinedGroupText(block, blocks)
	style := block.Properties["style"]
	raw := block.Properties["raw"]
	prefixes := w.joinedGroupPrefixes(block, blocks)
	layout := w.groupLayout(block, blocks)

	if !*first {
		if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
			return err
		}
	}
	*first = false

	return w.writeCommentStyled(text, style, raw, prefixes, layout)
}

// groupLayout returns the per-line layout descriptor for the comment
// group, looked up on the first block of the group (which carries
// the lineLayout property). For ungrouped blocks it returns the
// block's own layout.
func (w *Writer) groupLayout(block *model.Block, blocks map[string]*model.Block) string {
	if layout := block.Properties["lineLayout"]; layout != "" {
		return layout
	}
	firstID := block.Properties["groupFirstID"]
	if firstID == "" {
		return ""
	}
	if first, ok := blocks[firstID]; ok {
		return first.Properties["lineLayout"]
	}
	return ""
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
				text := w.joinedGroupText(block, blocks)
				style := block.Properties["style"]
				raw := block.Properties["raw"]
				prefixes := w.joinedGroupPrefixes(block, blocks)
				layout := w.groupLayout(block, blocks)
				if err := w.writeCommentStyled(text, style, raw, prefixes, layout); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// writeCommentStyled reconstructs a comment using the given style.
//
// linePrefixes carries the per-line Doxygen command markers that the
// reader stripped on extraction (e.g. "\brief ", "\param x ",
// "\addtogroup mygroup "). The comment-style writers reattach them so
// the roundtrip output preserves the original markers instead of
// emitting bare prose. nil or empty prefixes mean "no markers" — the
// writers fall back to plain comment-line emission.
//
// layout, when non-empty, is the per-line layout descriptor produced
// by the reader's buildLineLayout. It lets the writer emit the
// original raw structural lines (blank `///` separators, excluded
// `\code...\endcode` blocks, non-translatable command lines) verbatim
// and substitute translated text into the translatable lines in
// place. Only relevant for line-comment styles (triple / exclamation).
func (w *Writer) writeCommentStyled(text, style, raw string, linePrefixes []string, layout string) error {
	switch style {
	case "triple":
		if layout != "" {
			return w.writeFromLayout(text, layout, "/// ")
		}
		return w.writeTripleSlash(text, raw, linePrefixes)
	case "exclamation":
		if layout != "" {
			return w.writeFromLayout(text, layout, "//! ")
		}
		return w.writeExclamation(text, raw, linePrefixes)
	case "javadoc":
		if layout != "" {
			return w.writeFromLayout(text, layout, "")
		}
		return w.writeJavadoc(text, raw, linePrefixes)
	case "qt":
		if layout != "" {
			return w.writeFromLayout(text, layout, "")
		}
		return w.writeQt(text, raw, linePrefixes)
	case "trailing":
		return w.writeTrailing(text, raw)
	case "trailing_qt":
		return w.writeTrailingQt(text, raw)
	case "trailing_javadoc":
		return w.writeTrailingJavadoc(text, raw)
	default:
		if layout != "" {
			return w.writeFromLayout(text, layout, "/// ")
		}
		return w.writeTripleSlash(text, raw, linePrefixes)
	}
}

// writeFromLayout emits a comment group using the per-line layout
// descriptor. Tags:
//
//	T:<prefix>   consume next text line, emit `<marker><prefix><text>`
//	             — used for /// and //! comment groups where every
//	             text line gets the same comment marker prepended.
//	B:<prefix>   consume next text line, emit `<prefix><text>` — used
//	             for /** … */ and /*! … */ block comments where each
//	             raw line embeds its own delimiter / `*` line marker
//	             in the prefix.
//	S:<raw>      emit `<raw>` verbatim.
//
// marker is the per-line comment marker for line-comment styles (e.g.
// "/// " for triple-slash). Block styles pass an empty marker since
// the prefix already includes the delimiter.
func (w *Writer) writeFromLayout(text, layout, marker string) error {
	textLines := strings.Split(text, "\n")
	textCursor := 0
	entries := strings.Split(layout, "\x01")
	for i, entry := range entries {
		if i > 0 {
			if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
				return err
			}
		}
		if len(entry) < 2 {
			continue
		}
		tag := entry[0]
		body := entry[2:]
		switch tag {
		case 'T':
			line := ""
			if textCursor < len(textLines) {
				line = textLines[textCursor]
			}
			textCursor++
			if _, err := fmt.Fprintf(w.Output, "%s%s%s", marker, body, line); err != nil {
				return err
			}
		case 'B':
			line := ""
			if textCursor < len(textLines) {
				line = textLines[textCursor]
			}
			textCursor++
			if _, err := fmt.Fprintf(w.Output, "%s%s", body, line); err != nil {
				return err
			}
		case 'S':
			if _, err := fmt.Fprint(w.Output, body); err != nil {
				return err
			}
		}
	}
	return nil
}

// blockLinePrefixes returns the per-line command markers stored on
// the block (split on \x00), or nil if the block has none.
func blockLinePrefixes(block *model.Block) []string {
	if block == nil {
		return nil
	}
	raw := block.Properties["linePrefixes"]
	if raw == "" {
		return nil
	}
	return strings.Split(raw, "\x00")
}

// joinedGroupPrefixes returns the concatenated linePrefixes for every
// sibling block in a multi-section comment group, in section order.
// For ungrouped blocks this returns just the block's own prefixes.
func (w *Writer) joinedGroupPrefixes(block *model.Block, blocks map[string]*model.Block) []string {
	sizeStr := block.Properties["groupSize"]
	if sizeStr == "" {
		return blockLinePrefixes(block)
	}
	size, err := strconv.Atoi(sizeStr)
	if err != nil || size <= 1 {
		return blockLinePrefixes(block)
	}
	firstID := block.Properties["groupFirstID"]
	if firstID == "" || !strings.HasPrefix(firstID, "tu") {
		return blockLinePrefixes(block)
	}
	startNum, err := strconv.Atoi(firstID[2:])
	if err != nil {
		return blockLinePrefixes(block)
	}
	var all []string
	hasAny := false
	for i := range size {
		id := fmt.Sprintf("tu%d", startNum+i)
		sib, ok := blocks[id]
		if !ok {
			continue
		}
		px := blockLinePrefixes(sib)
		// Each sibling's text contributes len(px) lines (or 1 if
		// no prefixes recorded but the block has text).
		if len(px) == 0 {
			n := 1
			if t := w.blockText(sib); t != "" {
				n = strings.Count(t, "\n") + 1
			}
			for range n {
				all = append(all, "")
			}
		} else {
			all = append(all, px...)
			for _, p := range px {
				if p != "" {
					hasAny = true
				}
			}
		}
	}
	if !hasAny {
		return nil
	}
	return all
}

// blockText returns target or source text for a block.
func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return block.TargetText(w.Locale)
	}
	return block.SourceText()
}

// joinedGroupText returns the text for a single comment template,
// gathering sibling Blocks that belong to the same comment-group when
// the reader split a multi-section comment (e.g. \param a … \param b …
// \return … inside one /*! … */) into multiple Blocks. Each section's
// text is joined with a newline so the comment-style writers (writeQt,
// writeJavadoc, …) emit one line per section under the comment's
// shared delimiters. For ungrouped Blocks this returns blockText
// unchanged.
func (w *Writer) joinedGroupText(block *model.Block, blocks map[string]*model.Block) string {
	sizeStr := block.Properties["groupSize"]
	if sizeStr == "" {
		return w.blockText(block)
	}
	size, err := strconv.Atoi(sizeStr)
	if err != nil || size <= 1 {
		return w.blockText(block)
	}
	firstID := block.Properties["groupFirstID"]
	if firstID == "" || !strings.HasPrefix(firstID, "tu") {
		return w.blockText(block)
	}
	startNum, err := strconv.Atoi(firstID[2:])
	if err != nil {
		return w.blockText(block)
	}
	parts := make([]string, 0, size)
	for i := range size {
		id := fmt.Sprintf("tu%d", startNum+i)
		sib, ok := blocks[id]
		if !ok {
			continue
		}
		parts = append(parts, w.blockText(sib))
	}
	return strings.Join(parts, "\n")
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
	prefixes := blockLinePrefixes(block)
	layout := block.Properties["lineLayout"]

	if !*first {
		if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
			return err
		}
	}
	*first = false

	return w.writeCommentStyled(text, style, raw, prefixes, layout)
}

// linePrefixAt returns the prefix to reattach for the i-th text line.
// Returns "" when no prefix was recorded for that line.
func linePrefixAt(prefixes []string, i int) string {
	if i < 0 || i >= len(prefixes) {
		return ""
	}
	return prefixes[i]
}

// writeTripleSlash writes text as /// line comments, preserving indentation from the original.
func (w *Writer) writeTripleSlash(text, raw string, prefixes []string) error {
	indent := extractIndent(raw)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i > 0 {
			if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
				return err
			}
		}
		px := linePrefixAt(prefixes, i)
		if _, err := fmt.Fprintf(w.Output, "%s/// %s%s", indent, px, line); err != nil {
			return err
		}
	}
	return nil
}

// writeExclamation writes text as //! line comments.
func (w *Writer) writeExclamation(text, raw string, prefixes []string) error {
	indent := extractIndent(raw)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if i > 0 {
			if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
				return err
			}
		}
		px := linePrefixAt(prefixes, i)
		if _, err := fmt.Fprintf(w.Output, "%s//! %s%s", indent, px, line); err != nil {
			return err
		}
	}
	return nil
}

// writeJavadoc writes text as a /** ... */ block comment.
func (w *Writer) writeJavadoc(text, raw string, prefixes []string) error {
	indent := extractIndent(raw)
	rawLines := strings.Split(raw, "\n")

	// Single-line block comment
	if len(rawLines) == 1 {
		px := linePrefixAt(prefixes, 0)
		_, err := fmt.Fprintf(w.Output, "%s/** %s%s */", indent, px, text)
		return err
	}

	// Multi-line block comment
	lines := strings.Split(text, "\n")
	if _, err := fmt.Fprintf(w.Output, "%s/**", indent); err != nil {
		return err
	}
	for i, line := range lines {
		px := linePrefixAt(prefixes, i)
		if _, err := fmt.Fprintf(w.Output, "\n%s * %s%s", indent, px, line); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w.Output, "\n%s */", indent)
	return err
}

// writeQt writes text as a /*! ... */ block comment.
func (w *Writer) writeQt(text, raw string, prefixes []string) error {
	indent := extractIndent(raw)
	rawLines := strings.Split(raw, "\n")

	// Single-line block comment
	if len(rawLines) == 1 {
		px := linePrefixAt(prefixes, 0)
		_, err := fmt.Fprintf(w.Output, "%s/*! %s%s */", indent, px, text)
		return err
	}

	// Multi-line block comment
	lines := strings.Split(text, "\n")
	if _, err := fmt.Fprintf(w.Output, "%s/*!", indent); err != nil {
		return err
	}
	for i, line := range lines {
		px := linePrefixAt(prefixes, i)
		if _, err := fmt.Fprintf(w.Output, "\n%s  %s%s", indent, px, line); err != nil {
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

// writeTrailingJavadoc writes text as a trailing /**< ... */ comment.
func (w *Writer) writeTrailingJavadoc(text, _ string) error {
	_, err := fmt.Fprintf(w.Output, "/**< %s */", text)
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
