package mdx

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// BlockPropLinePrefix is the markdown reader's per-block continuation-prefix
// property. The MDX reader delegates Markdown spans to the markdown reader,
// which stamps this property on multi-line paragraph/list/blockquote
// blocks; the MDX writer re-applies it the same way so those blocks keep
// their line shape on round-trip. Kept as a local constant (string value
// identical to markdown.BlockPropLinePrefix) so the MDX package needs no
// extra import surface — the markdown reader sets the property by this key.
const BlockPropLinePrefix = "md:line-prefix"

// Writer implements DataFormatWriter for MDX files.
//
// It replays the MDX skeleton (built by the reader from the spliced
// per-span markdown skeletons plus verbatim opaque MDX regions),
// substituting each block ref with the block's rendered runs (target
// locale when present, source otherwise). Because opaque MDX regions were
// captured byte-for-byte and Markdown-span blocks render back to their
// source text when untranslated, an untranslated read→write reproduces the
// document exactly.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	firstBlock    bool
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new MDX writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "mdx",
		},
		firstBlock: true,
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts and writes reconstructed MDX.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	blocksByID := make(map[string]*model.Block)
	var orderedBlocks []*model.Block
	var dataParts []*model.Data

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				goto done
			}
			switch part.Type {
			case model.PartBlock:
				if block, ok := part.Resource.(*model.Block); ok {
					blocksByID[block.ID] = block
					orderedBlocks = append(orderedBlocks, block)
				}
			case model.PartData:
				if data, ok := part.Resource.(*model.Data); ok {
					dataParts = append(dataParts, data)
				}
			}
		}
	}
done:
	// Mode 1: Skeleton store (byte-exact, streaming-friendly).
	if w.skeletonStore != nil {
		return w.writeFromSkeleton(w.skeletonStore, blocksByID, w.Output)
	}

	// Mode 2: Build from blocks + data (fallback, best-effort ordering).
	return w.writeFromParts(orderedBlocks, dataParts, w.Output)
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton(store *format.SkeletonStore, blocks map[string]*model.Block, out io.Writer) error {
	for {
		entry, err := store.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("mdx writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := out.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				if _, err := io.WriteString(out, w.blockText(block)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// writeFromParts reconstructs MDX without a skeleton store. It interleaves
// opaque MDX-region Data and code-block Data with rendered blocks in the
// order they were emitted is not tracked here, so this fallback simply
// concatenates the verbatim Data content and rendered block text. It is a
// best-effort path; the skeleton path is the faithful one and is always
// used by the pipeline (the reader registers as a SkeletonStoreEmitter).
func (w *Writer) writeFromParts(blocks []*model.Block, data []*model.Data, out io.Writer) error {
	// Without a skeleton there is no positional information linking opaque
	// regions to surrounding prose, so emit each block's text separated by
	// blank lines (mirrors the markdown writer's no-skeleton fallback) and
	// append opaque Data verbatim. This path is not exercised by the
	// pipeline but keeps the writer total.
	for _, block := range blocks {
		text := w.blockText(block)
		if !w.firstBlock {
			if _, err := fmt.Fprint(out, "\n\n"); err != nil {
				return err
			}
		}
		w.firstBlock = false
		if _, err := fmt.Fprint(out, text); err != nil {
			return err
		}
	}
	for _, d := range data {
		if content, ok := d.Properties["content"]; ok {
			if _, err := io.WriteString(out, content); err != nil {
				return err
			}
		}
	}
	return nil
}

// blockText returns the rendered text for a block, preferring the target
// locale's translation if available, falling back to source. Multi-line
// blocks whose source carried a per-line continuation prefix (set by the
// markdown reader via BlockPropLinePrefix) have that prefix re-inserted
// after every "\n" so blockquotes and indented continuations retain their
// original line shape — identical to the markdown writer's behaviour.
func (w *Writer) blockText(block *model.Block) string {
	runs := w.blockRuns(block)
	if runs == nil {
		return ""
	}
	rendered := model.RenderRunsWithData(runs)
	if prefix, ok := block.Properties[BlockPropLinePrefix]; ok && prefix != "" && strings.Contains(rendered, "\n") {
		rendered = strings.ReplaceAll(rendered, "\n", "\n"+prefix)
	}
	return rendered
}

// blockRuns returns the target Run sequence for the configured locale, or
// the source Run sequence if no target is available.
func (w *Writer) blockRuns(block *model.Block) []model.Run {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		segs := block.Targets[w.Locale]
		if len(segs) > 0 && len(segs[0].Runs) > 0 {
			return segs[0].Runs
		}
	}
	if len(block.Source) > 0 && len(block.Source[0].Runs) > 0 {
		return block.Source[0].Runs
	}
	return nil
}
