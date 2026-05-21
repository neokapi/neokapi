package properties

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for Java Properties files.
type Writer struct {
	format.BaseFormatWriter
	cfg           *Config
	skeletonStore *format.SkeletonStore
	firstLine     bool
}

// Ensure Writer implements SkeletonStoreConsumer and WriterConfigurable.
var (
	_ format.SkeletonStoreConsumer = (*Writer)(nil)
	_ format.WriterConfigurable    = (*Writer)(nil)
)

// NewWriter creates a new Properties writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "properties",
		},
		cfg:       cfg,
		firstLine: true,
	}
}

// SetConfig replaces the writer's config — used to apply serialization
// knobs such as escapeExtendedChars from parity tests, CLI introspection,
// and .kapi recipe loading.
func (w *Writer) SetConfig(cfg *Config) {
	if cfg != nil {
		w.cfg = cfg
	}
}

// WriterConfig implements format.WriterConfigurable, exposing the writer's
// Config so the flow/CLI plumbing can apply parameters (escapeExtendedChars)
// via ApplyMap.
func (w *Writer) WriterConfig() format.DataFormatConfig {
	if w.cfg == nil {
		w.cfg = &Config{}
		w.cfg.Reset()
	}
	return w.cfg
}

// escapeExtended reports whether non-ASCII characters should be encoded as
// \uXXXX on output. Defaults to true (ISO-8859-1-safe) when no config set.
func (w *Writer) escapeExtended() bool {
	if w.cfg == nil {
		return true
	}
	return w.cfg.EscapeExtendedChars
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
		if errors.Is(err, io.EOF) {
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
//
// Inline-code Ph runs (created by the reader's codeFinder for HTML tags
// and Java escapes) are spliced back into the text via
// RenderRunsWithData; plain TargetText/SourceText would drop them.
func (w *Writer) blockValue(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return encodePropertyValue(renderRunsText(block.Targets[w.Locale]), w.escapeExtended())
	}
	if raw, ok := block.Properties["rawValue"]; ok {
		return raw
	}
	return encodePropertyValue(renderRunsText(block.Source), w.escapeExtended())
}

func renderRunsText(segs []*model.Segment) string {
	var b strings.Builder
	for _, seg := range segs {
		if seg == nil {
			continue
		}
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
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return errors.New("properties writer: expected Block resource")
	}

	// Use target text if available, otherwise source text. Render with
	// inline-code Ph data spliced back in so codeFinder-extracted HTML
	// markup survives the round-trip.
	var text string
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = renderRunsText(block.Targets[w.Locale])
	} else {
		text = renderRunsText(block.Source)
	}

	// Encode unicode escapes for non-ASCII characters
	text = encodePropertyValue(text, w.escapeExtended())

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
		return errors.New("properties writer: expected Data resource")
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
// non-ASCII -> \uXXXX (when escapeExtended), newline -> \n, tab -> \t,
// CR -> \r, backslash -> \\. Leading `:` / `=` are escaped because the
// Java properties parser would otherwise treat them as a second separator
// marker (okapi mirrors this).
//
// When escapeExtended is false, non-ASCII characters are emitted verbatim
// (the output is no longer ISO-8859-1 safe but preserves the raw bytes),
// matching Okapi's setEscapeExtendedChars(false).
func encodePropertyValue(s string, escapeExtended bool) string {
	var buf strings.Builder
	for i, r := range s {
		switch {
		case r == '\\':
			buf.WriteString("\\\\")
		case r == '\n':
			buf.WriteString("\\n")
		case r == '\t':
			buf.WriteString("\\t")
		case r == '\r':
			buf.WriteString("\\r")
		case r > 127 && escapeExtended:
			buf.WriteString(fmt.Sprintf("\\u%04x", r))
		case (r == ':' || r == '=') && i == 0:
			buf.WriteByte('\\')
			buf.WriteRune(r)
		default:
			buf.WriteRune(r)
		}
	}
	return buf.String()
}
