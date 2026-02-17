package html

import (
	"context"
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/format"
	"github.com/gokapi/gokapi/model"
)

// Writer implements DataFormatWriter for HTML files.
type Writer struct {
	format.BaseFormatWriter
}

// NewWriter creates a new HTML writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "html",
		},
	}
}

// Write consumes Parts from a channel and writes reconstructed HTML.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
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
	default:
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return fmt.Errorf("html writer: expected Block resource")
	}

	// Get the text to write (target if available, else source)
	var text string
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = w.getCodedText(block, w.Locale)
	} else {
		text = w.getSourceCodedText(block)
	}

	// Write using skeleton if available
	if block.Skeleton != nil && block.Skeleton.Strategy == model.SkeletonFragmentBased {
		for _, sp := range block.Skeleton.Parts {
			switch p := sp.(type) {
			case *model.SkeletonText:
				if _, err := fmt.Fprint(w.Output, p.Text); err != nil {
					return err
				}
			case *model.SkeletonRef:
				if _, err := fmt.Fprint(w.Output, text); err != nil {
					return err
				}
			}
		}
		return nil
	}

	_, err := fmt.Fprint(w.Output, text)
	return err
}

// getCodedText reconstructs the full text from a block's target including span markup.
func (w *Writer) getCodedText(block *model.Block, locale model.LocaleID) string {
	segs := block.Targets[locale]
	if len(segs) == 0 {
		return w.getSourceCodedText(block)
	}
	var buf strings.Builder
	for _, seg := range segs {
		w.renderFragment(&buf, seg.Content)
	}
	return buf.String()
}

func (w *Writer) getSourceCodedText(block *model.Block) string {
	var buf strings.Builder
	for _, seg := range block.Source {
		w.renderFragment(&buf, seg.Content)
	}
	return buf.String()
}

func (w *Writer) renderFragment(buf *strings.Builder, frag *model.Fragment) {
	if !frag.HasSpans() {
		buf.WriteString(frag.CodedText)
		return
	}

	spanIdx := 0
	for _, r := range frag.CodedText {
		if model.MarkerOpening == r || model.MarkerClosing == r || model.MarkerPlaceholder == r {
			if spanIdx < len(frag.Spans) {
				buf.WriteString(frag.Spans[spanIdx].Data)
				spanIdx++
			}
		} else {
			buf.WriteRune(r)
		}
	}
}
