package wiki

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

// Writer implements DataFormatWriter for Wiki files.
type Writer struct {
	format.BaseFormatWriter
	cfg           *Config
	firstBlock    bool
	skeletonStore *format.SkeletonStore
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new wiki writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "wiki",
		},
		cfg:        cfg,
		firstBlock: true,
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Config returns the writer's configuration.
func (w *Writer) Config() *Config { return w.cfg }

// Write consumes Parts from a channel and writes reconstructed wiki markup.
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
		return fmt.Errorf("wiki writer: flush skeleton: %w", err)
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
			return fmt.Errorf("wiki writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				text := w.blockText(block)
				// Reconstruct header lines from the delimiter layout
				// captured at read time (level + exact spacing), not by
				// re-parsing a stored raw source line.
				if block.Name == "header" {
					if line, ok := w.headerLine(block, text); ok {
						if _, err := io.WriteString(w.Output, line); err != nil {
							return err
						}
						continue
					}
				}
				if _, err := io.WriteString(w.Output, text); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// headerLine rebuilds a wiki header line from the delimiter layout the
// reader captured (level + surrounding whitespace + closing delimiters)
// and the supplied (possibly translated) title text. It returns false if
// the block carries no header-level metadata, in which case the caller
// falls back to writing the plain text.
//
// The layout is reconstructed byte-for-byte: leading "=" run of the
// recorded level, the recorded whitespace before the title, the title,
// the recorded whitespace after the title, the recorded closing "=" run,
// and any recorded trailing whitespace. storeHeaderLayout stamps every key
// together, so once headerLevel is present the whitespace captures are
// used verbatim — including empty strings (e.g. "==No Spacing==").
func (w *Writer) headerLine(block *model.Block, text string) (string, bool) {
	levelStr, ok := block.Properties[headerLevelKey]
	if !ok {
		return "", false
	}
	level, err := strconv.Atoi(levelStr)
	if err != nil || level < 1 {
		return "", false
	}

	// closeRun falls back to a matching "=" run only if the key is
	// genuinely absent; an empty value cannot occur since the recognizer
	// requires 2-6 closing delimiters.
	closeRun, ok := block.Properties[headerCloseKey]
	if !ok || closeRun == "" {
		closeRun = strings.Repeat("=", level)
	}

	var b strings.Builder
	b.WriteString(strings.Repeat("=", level))
	b.WriteString(block.Properties[headerPrefixWS]) // verbatim, may be ""
	b.WriteString(text)
	b.WriteString(block.Properties[headerSuffixWS]) // verbatim, may be ""
	b.WriteString(closeRun)
	b.WriteString(block.Properties[headerTrailerWS]) // verbatim, may be ""
	return b.String(), true
}

// blockText returns target or source text for a block, splicing inline
// PlaceholderRun / PcOpen / PcClose data back into the stream so
// `[[link]]`, `[[link|alt]]`, and `{{image}}` constructs round-trip
// verbatim. Plain SourceText/TargetText drops the inline-code Data
// payloads, which would erase the markup the reader carved out under
// tokenizeDokuWikiInlineCodes.
func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		segs := block.Targets[w.Locale]
		var b strings.Builder
		for _, seg := range segs {
			b.WriteString(model.RenderRunsWithData(seg.Runs))
		}
		return b.String()
	}
	var b strings.Builder
	for _, seg := range block.Source {
		b.WriteString(model.RenderRunsWithData(seg.Runs))
	}
	return b.String()
}

func (w *Writer) writePart(part *model.Part) error {
	switch part.Type {
	case model.PartBlock:
		return w.writeBlock(part)
	case model.PartData:
		return w.writeData(part)
	default:
		// Skip layer start/end and other structural parts
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return errors.New("wiki writer: expected Block resource")
	}

	// Use the markup-preserving renderer so inline `[[link]]`,
	// `[[link|alt]]`, and `{{image}}` placeholder runs survive the
	// no-skeleton write path too (e.g. when the writer is fed parts
	// directly by a tool, not driven by skeleton replay).
	text := w.blockText(block)

	if !w.firstBlock {
		if _, err := fmt.Fprintln(w.Output); err != nil {
			return err
		}
	}
	w.firstBlock = false

	// Reconstruct wiki markup based on block name
	switch block.Name {
	case "header":
		if line, ok := w.headerLine(block, text); ok {
			_, err := io.WriteString(w.Output, line)
			return err
		}
		_, err := fmt.Fprintf(w.Output, "== %s ==", text)
		return err
	case "table-header":
		_, err := fmt.Fprintf(w.Output, "! %s", text)
		return err
	case "table-cell":
		_, err := fmt.Fprintf(w.Output, "| %s", text)
		return err
	case "image-caption":
		// Captions are complex to reconstruct; write as plain text
		_, err := fmt.Fprint(w.Output, text)
		return err
	default:
		_, err := fmt.Fprint(w.Output, text)
		return err
	}
}

func (w *Writer) writeData(part *model.Part) error {
	// Data parts represent structural separators (blank lines, table markers, etc.)
	if !w.firstBlock {
		if _, err := fmt.Fprintln(w.Output); err != nil {
			return err
		}
	}
	w.firstBlock = false
	return nil
}
