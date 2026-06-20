package transtable

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for Okapi TransTable v1 files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	headerWritten bool
	sourceLocale  model.LocaleID
	lineEnd       string
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new TransTable v1 writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName:  "transtable",
			Interchange: true,
		},
		lineEnd: "\n",
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes a reconstructed
// TransTable v1 document.
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

// writeWithSkeleton collects all blocks, then reconstructs output from
// skeleton entries — the reader emits one ref per block, so this just
// substitutes the rendered rows in document order.
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
			if part.Type == model.PartLayerStart {
				if layer, ok := part.Resource.(*model.Layer); ok && !layer.Locale.IsEmpty() {
					w.sourceLocale = layer.Locale
				}
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
		return fmt.Errorf("transtable writer: flush skeleton: %w", err)
	}
	return w.writeFromSkeleton(blocksByID)
}

// writeFromSkeleton reads skeleton entries and substitutes block content.
// For TransTable, each ref corresponds to one block; the writer emits
// every segment of that block as its own row.
func (w *Writer) writeFromSkeleton(blocks map[string]*model.Block) error {
	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("transtable writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			block, ok := blocks[string(entry.Data)]
			if !ok {
				continue
			}
			if err := w.writeBlockRows(block); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *Writer) writePart(part *model.Part) error {
	switch part.Type {
	case model.PartLayerStart:
		if layer, ok := part.Resource.(*model.Layer); ok && !layer.Locale.IsEmpty() {
			w.sourceLocale = layer.Locale
		}
		return w.writeHeaderIfNeeded()
	case model.PartBlock:
		if err := w.writeHeaderIfNeeded(); err != nil {
			return err
		}
		return w.writeBlock(part)
	case model.PartData:
		// Non-translatable data (we don't currently emit any from the
		// reader without a skeleton store) — pass through silently.
		return nil
	default:
		return nil
	}
}

func (w *Writer) writeHeaderIfNeeded() error {
	if w.headerWritten {
		return nil
	}
	w.headerWritten = true
	src := w.sourceLocale
	if src.IsEmpty() {
		src = model.LocaleEnglish
	}
	trg := w.Locale
	_, err := fmt.Fprintf(w.Output, "TransTableV1\t%s\t%s%s", src, trg, w.lineEnd)
	return err
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return errors.New("transtable writer: expected Block resource")
	}
	return w.writeBlockRows(block)
}

func (w *Writer) writeBlockRows(block *model.Block) error {
	if block == nil {
		return nil
	}
	tuID := block.Properties["tu_id"]
	if tuID == "" {
		tuID = block.ID
	}

	// Reconstruct the former structural segments by walking the source
	// segmentation overlay. With no overlay the whole source is one
	// segment (SourceSegmentCount/SourceSegmentRuns handle this).
	srcSeg := block.SourceSegmentation()
	srcCount := block.SourceSegmentCount()

	// Resolve the target overlay (if any) so we can split the target
	// runs by the same segment spans the reader recorded.
	var (
		trgRuns []model.Run
		trgOv   *model.Overlay
	)
	if !w.Locale.IsEmpty() {
		trgRuns = block.TargetRuns(w.Locale)
		key := model.Variant(w.Locale)
		trgOv = block.SegmentationFor(&key)
	}

	// One row per source segment. When there is more than one segment
	// we emit `:s=<seg-id>` suffixes so the round-trip preserves the
	// segmentation; with exactly one segment the suffix is dropped to
	// match the upstream "unsegmented" wire shape.
	multi := srcCount > 1
	for i := range srcCount {
		var crumb string
		if multi {
			segID := fmt.Sprintf("s%d", i+1)
			if srcSeg != nil && i < len(srcSeg.Spans) && srcSeg.Spans[i].ID != "" {
				segID = srcSeg.Spans[i].ID
			}
			crumb = fmt.Sprintf(`"okpCtx:tu=%s:s=%s"`, tuID, segID)
		} else {
			crumb = fmt.Sprintf(`"okpCtx:tu=%s"`, tuID)
		}

		sourceCell := quote(escape(model.RunsText(block.SourceSegmentRuns(i))))

		var targetCell string
		hasTarget := false
		if !w.Locale.IsEmpty() && trgRuns != nil {
			// Always render the target column when a target locale is set
			// so the wire shape matches what the upstream writer emits
			// (third cell present, possibly empty).
			targetCell = quote(escape(model.RunsText(targetSegmentRuns(trgRuns, trgOv, i))))
			hasTarget = true
		} else if !w.Locale.IsEmpty() {
			targetCell = `""`
			hasTarget = true
		}

		var line string
		if hasTarget {
			line = crumb + "\t" + sourceCell + "\t" + targetCell + w.lineEnd
		} else {
			line = crumb + "\t" + sourceCell + w.lineEnd
		}
		if _, err := io.WriteString(w.Output, line); err != nil {
			return err
		}
	}
	return nil
}

// targetSegmentRuns returns the runs of the idx-th target segment. With
// a target segmentation overlay it extracts the matching span; without
// one (whole target = one segment) idx 0 returns all target runs and
// any other index returns nil.
func targetSegmentRuns(runs []model.Run, ov *model.Overlay, idx int) []model.Run {
	if ov == nil {
		if idx == 0 {
			return runs
		}
		return nil
	}
	if idx < 0 || idx >= len(ov.Spans) {
		return nil
	}
	return ov.Spans[idx].Range.ExtractRuns(runs)
}

// quote wraps s in double quotes. Mirrors the upstream writer which
// always emits cells quoted.
func quote(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	b.WriteString(s)
	b.WriteByte('"')
	return b.String()
}
