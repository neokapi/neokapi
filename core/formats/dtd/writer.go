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
				escaped := w.escapedBlockValue(block)
				if _, err := io.WriteString(w.Output, escaped); err != nil {
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
	default:
		return nil
	}
}

func (w *Writer) writeBlock(part *model.Part) error {
	block, ok := part.Resource.(*model.Block)
	if !ok {
		return errors.New("dtd writer: expected Block resource")
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
	escaped := w.escapedBlockValue(block)

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

// escapedBlockValue renders the block's value (target if translated, else
// source) with DTD escaping applied per-run. Literal text runs get their `&`,
// `<`, and `"` escaped, while inline-code runs (Ph/Pc/Sub — e.g. structural
// `&entityref;` or `%param;` references the reader captured verbatim) are
// emitted byte-for-byte. This preserves the reader's run-level distinction
// between a decoded literal `&` (from `&amp;`) and a structural entity
// reference: a value like `Text of &amp;test1;` is read as the literal text
// "Text of &test1;" and must be re-escaped to `&amp;test1;`, not left as the
// bare (and semantically different) reference `&test1;`.
func (w *Writer) escapedBlockValue(block *model.Block) string {
	segs := block.Source
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		segs = block.Targets[w.Locale]
	}
	var b strings.Builder
	for _, seg := range segs {
		if seg == nil {
			continue
		}
		for _, r := range seg.Runs {
			if r.Text != nil {
				b.WriteString(escapeEntityLiteral(r.Text.Text))
				continue
			}
			// Inline-code runs carry their original DTD bytes (entity or
			// parameter references) in Data — emit verbatim.
			b.WriteString(model.RenderRunsWithData([]model.Run{r}))
		}
	}
	return b.String()
}

// escapeEntityLiteral escapes the characters in literal entity-value text that
// must be encoded in a DTD: every `&` becomes `&amp;` (it is a literal
// ampersand, never a reference, because structural references are carried as
// inline-code runs), `<` becomes `&lt;`, and `"` becomes `&quot;`. `>` is left
// bare — it does not terminate a quoted entity value (matching okapi's
// DTDFilter and the XML 1.0 allowed-character set).
func escapeEntityLiteral(s string) string {
	var buf strings.Builder
	buf.Grow(len(s))
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			buf.WriteString("&amp;")
		case '<':
			buf.WriteString("&lt;")
		case '"':
			buf.WriteString("&quot;")
		default:
			buf.WriteByte(s[i])
		}
	}
	return buf.String()
}
