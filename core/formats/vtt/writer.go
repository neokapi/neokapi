package vtt

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for WebVTT subtitle files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	wroteHeader   bool
	firstCue      bool
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new VTT writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "vtt",
		},
		firstCue: true,
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed VTT.
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
		return fmt.Errorf("vtt writer: flush skeleton: %w", err)
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
			return fmt.Errorf("vtt writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				text := w.blockText(block)
				if _, err := io.WriteString(w.Output, text); err != nil {
					return err
				}
			}
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

func (w *Writer) writeData(part *model.Part) error {
	data, ok := part.Resource.(*model.Data)
	if !ok {
		return errors.New("vtt writer: expected Data resource")
	}

	if data.Name == "vtt-header" {
		header := data.Properties["content"]
		if header == "" {
			header = "WEBVTT"
		}
		if _, err := fmt.Fprint(w.Output, header); err != nil {
			return err
		}
		w.wroteHeader = true
	}
	// Cue identifiers are handled inline with blocks
	return nil
}

func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return block.TargetText(w.Locale)
	}
	return block.SourceText()
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return errors.New("vtt writer: expected Block resource")
	}

	text := w.blockText(block)

	// Cue timing is the format-agnostic TimingAnnotation (AD-002); format it to
	// VTT syntax. Fall back to a legacy timecode property if no anchor is set.
	timecode := block.Properties["timecode"]
	if t, ok := block.Timing(); ok && t != nil {
		timecode = formatVTTTimecode(t.StartMS, t.EndMS)
		if settings := block.Properties["cue-settings"]; settings != "" {
			timecode += " " + settings
		}
	}
	cueID := block.Properties["cue-id"]

	// A valid WebVTT file must start with the WEBVTT signature. When writing from
	// a non-VTT source (e.g. audio → ASR → VTT) no header Data part arrives, so
	// emit a default one before the first cue.
	if !w.wroteHeader {
		if _, err := io.WriteString(w.Output, "WEBVTT"); err != nil {
			return err
		}
		w.wroteHeader = true
	}

	// Blank line separator before cue
	if _, err := fmt.Fprint(w.Output, "\n\n"); err != nil {
		return err
	}

	// Write cue identifier if present
	if cueID != "" {
		if _, err := fmt.Fprintf(w.Output, "%s\n", cueID); err != nil {
			return err
		}
	}

	// Write timecode
	if _, err := fmt.Fprintf(w.Output, "%s\n", timecode); err != nil {
		return err
	}

	// Write subtitle text
	if _, err := fmt.Fprint(w.Output, text); err != nil {
		return err
	}

	w.firstCue = false
	return nil
}
