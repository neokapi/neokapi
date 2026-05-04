package dtd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for DTD files.
type Writer struct {
	format.BaseFormatWriter
	skeletonStore *format.SkeletonStore
	firstEntry    bool
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new DTD writer.
func NewWriter() *Writer {
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "dtd",
		},
		firstEntry: true,
	}
}

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts from a channel and writes reconstructed DTD.
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
		return fmt.Errorf("dtd writer: flush skeleton: %w", err)
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
			return fmt.Errorf("dtd writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				text := w.blockText(block)
				escaped := escapeEntityValue(text)
				if _, err := io.WriteString(w.Output, escaped); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (w *Writer) blockText(block *model.Block) string {
	// Render via RenderRunsWithData so codeFinder-extracted Ph runs
	// (e.g. `&entityref;`, HTML markup) round-trip with their original
	// Data verbatim instead of being dropped by SourceText / TargetText.
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return renderSegments(block.Targets[w.Locale])
	}
	return renderSegments(block.Source)
}

func renderSegments(segs []*model.Segment) string {
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
	default:
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return errors.New("dtd writer: expected Block resource")
	}

	text := renderSegments(block.Source)
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		text = renderSegments(block.Targets[w.Locale])
	}

	name := block.Name
	if name == "" {
		name = block.ID
	}

	// Write comment if block has a note annotation
	if noteAnn, ok := block.Annotations["note"]; ok {
		if note, ok := noteAnn.(*model.NoteAnnotation); ok && note.Text != "" {
			if !w.firstEntry {
				if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintf(w.Output, "<!--%s-->\n", note.Text); err != nil {
				return err
			}
			w.firstEntry = false
		}
	}

	// Escape the value for DTD output
	escaped := escapeEntityValue(text)

	if w.firstEntry {
		w.firstEntry = false
	} else if _, hasNote := block.Annotations["note"]; !hasNote {
		if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
			return err
		}
	}

	if _, err := fmt.Fprintf(w.Output, "<!ENTITY %s \"%s\">\n", name, escaped); err != nil {
		return err
	}

	return nil
}

// escapeEntityValue escapes characters that need encoding in DTD entity
// values. Pre-existing entity references (named, numeric/hex, parameter)
// pass through unchanged — their `&` (or `%`) is part of a syntactic
// reference, not a literal that needs escaping. Bare `&` not followed by
// a valid reference still gets escaped to `&amp;`.
func escapeEntityValue(s string) string {
	var buf strings.Builder
	i := 0
	for i < len(s) {
		c := s[i]
		switch c {
		case '&':
			// Look ahead for a closing `;` within reasonable distance.
			// Accept if the chars between form a valid entity-name or
			// numeric/hex reference. Otherwise escape the `&` itself.
			if end := indexByteUpTo(s[i+1:], ';', 64); end >= 0 {
				ref := s[i+1 : i+1+end]
				if isValidEntityRef(ref) {
					buf.WriteString(s[i : i+1+end+1])
					i += 1 + end + 1
					continue
				}
			}
			buf.WriteString("&amp;")
			i++
		case '"':
			buf.WriteString("&quot;")
			i++
		case '<':
			buf.WriteString("&lt;")
			i++
		// `>` is intentionally not escaped — it doesn't terminate a
		// quoted entity value, so leaving it bare matches okapi's
		// DTDFilter output (and the spec's allowed-character set).
		default:
			buf.WriteByte(c)
			i++
		}
	}
	return buf.String()
}

// indexByteUpTo returns the index of c in s within the first n bytes,
// or -1 if not found.
func indexByteUpTo(s string, c byte, n int) int {
	if n > len(s) {
		n = len(s)
	}
	for i := range n {
		if s[i] == c {
			return i
		}
	}
	return -1
}

// isValidEntityRef reports whether s (the content between `&` and `;`)
// is a syntactically valid entity reference: a Name (per XML 1.0 §2.3),
// a decimal NCR (`#NNN`), or a hex NCR (`#xHH`).
func isValidEntityRef(s string) bool {
	if s == "" {
		return false
	}
	if s[0] == '#' {
		// Numeric character reference.
		if len(s) > 2 && (s[1] == 'x' || s[1] == 'X') {
			for _, c := range s[2:] {
				if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F') {
					return false
				}
			}
			return len(s) > 2
		}
		for _, c := range s[1:] {
			if c < '0' || c > '9' {
				return false
			}
		}
		return len(s) > 1
	}
	// Named reference: NameStartChar followed by NameChar*. Use the
	// ASCII-safe subset (covers all common entity names).
	c := s[0]
	if !(c == '_' || c == ':' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
		return false
	}
	for i := 1; i < len(s); i++ {
		c := s[i]
		if !(c == '_' || c == ':' || c == '-' || c == '.' ||
			(c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z')) {
			return false
		}
	}
	return true
}
