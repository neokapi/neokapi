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
	notes := block.Notes()
	if len(notes) > 0 && notes[0].Text != "" {
		if !w.firstEntry {
			if _, err := fmt.Fprint(w.Output, "\n"); err != nil {
				return err
			}
		}
		if _, err := fmt.Fprintf(w.Output, "<!--%s-->\n", notes[0].Text); err != nil {
			return err
		}
		w.firstEntry = false
	}

	// Escape the value for DTD output
	escaped := w.escapedBlockValue(block)

	if w.firstEntry {
		w.firstEntry = false
	} else if len(notes) == 0 {
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
// source) with DTD escaping applied per-run.
//
// Three run categories are handled distinctly, matching okapi's DTDFilter
// (filters/dtd/DTDFilter.java lines 297-305) and DTDEncoder:
//
//   - Literal text runs get their `&`, `<`, and `"` escaped. This preserves
//     the reader's distinction between a decoded literal `&` (from `&amp;`)
//     and a structural reference: `Text of &amp;test1;` reads as the literal
//     "Text of &test1;" and must re-escape to `&amp;test1;`.
//   - Code-finder runs (SubType=="regxph") carry markup the configurable code
//     finder lifted out of the value verbatim (e.g. `<i>`, `<a name="aaa">`).
//     Their `<`, `&`, and `"` MUST be entity-escaped — emitting them raw would
//     produce invalid DTD (a bare `"` prematurely closes the quoted entity
//     value; XML 1.0 §2.3 EntityValue and §4.2). okapi re-encodes exactly
//     these codes via encoder.encode(code.getData(), EncoderContext.TEXT).
//   - Structural reference runs (named-entity / parameter-entity refs the
//     reader captured as Ph codes, e.g. `&test1;`, `%name;`) are emitted
//     byte-for-byte — they are real references, not literal markup.
func (w *Writer) escapedBlockValue(block *model.Block) string {
	runs := block.Source
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		runs = block.TargetRuns(w.Locale)
	}
	var b strings.Builder
	for _, r := range runs {
		if r.Text != nil {
			b.WriteString(escapeEntityLiteral(r.Text.Text))
			continue
		}
		// Code-finder-extracted markup (HTML tags etc.) must be
		// entity-escaped so the resulting entity value stays valid DTD.
		if r.Ph != nil && r.Ph.SubType == codeFinderTagType {
			b.WriteString(escapeEntityLiteral(r.Ph.Data))
			continue
		}
		// Structural entity / parameter references carry their original
		// DTD bytes in Data — emit verbatim.
		b.WriteString(model.RenderRunsWithData([]model.Run{r}))
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
	for i := range len(s) {
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
