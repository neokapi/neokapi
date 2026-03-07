package markdown

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
)

// Writer implements DataFormatWriter for Markdown files.
type Writer struct {
	format.BaseFormatWriter
	cfg           *Config
	skeletonStore *format.SkeletonStore
	firstBlock    bool
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new Markdown writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "markdown",
		},
		cfg:        cfg,
		firstBlock: true,
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed Markdown.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	blocksByID := make(map[string]*model.Block)
	var orderedBlocks []*model.Block

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
			}
		}
	}
done:
	// Mode 1: Skeleton store (byte-exact, streaming-friendly).
	if w.skeletonStore != nil {
		return w.writeFromSkeleton(w.skeletonStore, blocksByID)
	}

	// Mode 2: Build from blocks (fallback).
	return w.writeFromBlocks(orderedBlocks)
}

// writeFromSkeleton reads skeleton entries and fills in block content.
func (w *Writer) writeFromSkeleton(store *format.SkeletonStore, blocks map[string]*model.Block) error {
	for {
		entry, err := store.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("markdown writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			refID := string(entry.Data)
			if block, ok := blocks[refID]; ok {
				text := w.blockText(block)
				if _, err := io.WriteString(w.Output, text); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// writeFromBlocks reconstructs markdown from blocks without a skeleton store.
func (w *Writer) writeFromBlocks(blocks []*model.Block) error {
	for _, block := range blocks {
		text := w.blockText(block)

		if !w.firstBlock {
			if _, err := fmt.Fprint(w.Output, "\n\n"); err != nil {
				return err
			}
		}
		w.firstBlock = false

		// Reconstruct heading prefix
		if block.Type == "heading" {
			if level, ok := block.Properties["level"]; ok {
				n := 0
				_, _ = fmt.Sscanf(level, "%d", &n)
				prefix := strings.Repeat("#", n) + " "
				if _, err := fmt.Fprint(w.Output, prefix); err != nil {
					return err
				}
			}
		}

		// Reconstruct list item prefix
		if block.Type == "list-item" {
			if _, err := fmt.Fprint(w.Output, "- "); err != nil {
				return err
			}
		}

		if _, err := fmt.Fprint(w.Output, text); err != nil {
			return err
		}
	}
	return nil
}

// blockText returns the rendered text for a block, preferring the target
// locale's translation if available, falling back to source.
func (w *Writer) blockText(block *model.Block) string {
	frag := w.getFragment(block)
	if frag == nil {
		return ""
	}
	return renderFragment(frag)
}

// getFragment returns the target fragment for the configured locale,
// or the source fragment if no target is available.
func (w *Writer) getFragment(block *model.Block) *model.Fragment {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		segs := block.Targets[w.Locale]
		if len(segs) > 0 && segs[0].Content != nil {
			return segs[0].Content
		}
	}
	if len(block.Source) > 0 && block.Source[0].Content != nil {
		return block.Source[0].Content
	}
	return nil
}

// renderFragment renders a Fragment by iterating CodedText and emitting
// Span.Data at marker positions. This preserves the original markup.
func renderFragment(frag *model.Fragment) string {
	if !frag.HasSpans() {
		return frag.Text()
	}
	var buf strings.Builder
	spanIdx := 0
	for _, r := range frag.CodedText {
		if isMarker(r) && spanIdx < len(frag.Spans) {
			buf.WriteString(frag.Spans[spanIdx].Data)
			spanIdx++
		} else {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// isMarker returns true if the rune is a span marker character.
func isMarker(r rune) bool {
	return r >= '\uE001' && r <= '\uE003'
}
