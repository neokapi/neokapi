package asciidoc

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for AsciiDoc (.adoc) documents.
//
// Output strategy mirrors the reader's SkeletonStore round-trip:
//
//   - Mode 1 (skeleton): the primary, byte-exact path used by the file runner
//     and `kapi merge`. The reader's skeleton stream is replayed verbatim and
//     each Ref is resolved to its block's rendered (target-else-source) runs via
//     model.RenderRunsWithData. An untouched document round-trips byte-for-byte.
//   - Mode 2 (no skeleton): a normalized projection used when no skeleton is
//     wired (e.g. cross-format export). Blocks render with canonical AsciiDoc
//     markers from their semantic role; non-translatable Data parts replay their
//     raw bytes. This is intentionally NOT byte-exact — the byte-faithful path
//     is Mode 1.
type Writer struct {
	format.BaseFormatWriter
	cfg           *Config
	skeletonStore *format.SkeletonStore
	original      []byte
}

// Ensure Writer implements SkeletonStoreConsumer and OriginalContentSetter.
var (
	_ format.SkeletonStoreConsumer = (*Writer)(nil)
	_ format.OriginalContentSetter = (*Writer)(nil)
)

// NewWriter creates a new AsciiDoc writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "asciidoc",
		},
		cfg: cfg,
	}
}

// SetSkeletonStore wires the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) { w.skeletonStore = store }

// SetOriginalContent supplies the source document bytes so a writer with no
// externally-wired skeleton store can still produce byte-exact output: it
// regenerates the skeleton by re-reading the original (the reader assigns the
// same deterministic block ids), then splices the translated runs back in. This
// is the merge-from-source path the spec runner uses.
func (w *Writer) SetOriginalContent(content []byte) { w.original = content }

// Write consumes Parts and writes the reconstructed AsciiDoc document.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	blocksByID := make(map[string]*model.Block)
	var events []*model.Part // blocks + data + group brackets, in stream order

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
					events = append(events, part)
				}
			case model.PartData, model.PartGroupStart, model.PartGroupEnd:
				events = append(events, part)
			}
		}
	}
done:
	// Mode 1: byte-exact skeleton replay (externally-wired store, e.g. the file
	// runner / merge).
	if w.skeletonStore != nil {
		if err := w.skeletonStore.Flush(); err != nil {
			return fmt.Errorf("asciidoc writer: flush skeleton: %w", err)
		}
		return w.writeFromSkeleton(blocksByID)
	}

	// Mode 1b: byte-exact from the original source bytes — regenerate the
	// skeleton by re-reading the original, then splice translated runs in.
	if len(w.original) > 0 {
		if err := w.writeFromOriginal(ctx, blocksByID); err == nil {
			return nil
		} else if !errors.Is(err, errNoSkeleton) {
			return err
		}
		// fall through to the normalized projection on skeleton failure
	}

	// Mode 2: normalized projection from the ordered event stream.
	return w.writeFromEvents(events)
}

// errNoSkeleton signals that the original-content path could not build a
// skeleton (so the caller should fall back to the normalized projection).
var errNoSkeleton = errors.New("asciidoc writer: could not regenerate skeleton")

// writeFromOriginal regenerates the skeleton by re-reading the original source
// (the reader assigns deterministic block ids matching those in blocksByID),
// then replays it splicing the translated runs back in for a byte-exact result.
func (w *Writer) writeFromOriginal(ctx context.Context, blocksByID map[string]*model.Block) error {
	store, err := format.NewSkeletonStore()
	if err != nil {
		return errNoSkeleton
	}
	defer store.Close()

	rdr := NewReader()
	rdr.SetSkeletonStore(store)
	doc := &model.RawDocument{Encoding: "UTF-8", Reader: io.NopCloser(bytes.NewReader(w.original))}
	if err := rdr.Open(ctx, doc); err != nil {
		return errNoSkeleton
	}
	for pr := range rdr.Read(ctx) {
		if pr.Error != nil {
			_ = rdr.Close()
			return errNoSkeleton
		}
	}
	_ = rdr.Close()

	if err := store.Flush(); err != nil {
		return fmt.Errorf("asciidoc writer: flush regenerated skeleton: %w", err)
	}
	w.skeletonStore = store
	return w.writeFromSkeleton(blocksByID)
}

// writeFromSkeleton replays the skeleton stream, writing each SkeletonText entry
// verbatim and resolving each SkeletonRef to its block's rendered runs.
func (w *Writer) writeFromSkeleton(blocks map[string]*model.Block) error {
	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("asciidoc writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			if block, ok := blocks[string(entry.Data)]; ok {
				if _, err := io.WriteString(w.Output, w.blockText(block)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// blockText renders a block's content: the target runs for the active locale
// when present, otherwise the source runs, with inline markup spliced back via
// RenderRunsWithData.
func (w *Writer) blockText(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return model.RenderRunsWithData(block.TargetRuns(w.Locale))
	}
	return model.RenderRunsWithData(block.Source)
}

// writeFromEvents renders a normalized AsciiDoc document from the ordered block
// + data + group event stream. Not byte-exact (see the Mode 2 note on Write).
func (w *Writer) writeFromEvents(events []*model.Part) error {
	var groupStack []string
	first := true
	sep := func() error {
		if !first {
			if _, err := io.WriteString(w.Output, "\n"); err != nil {
				return err
			}
		}
		first = false
		return nil
	}

	for _, part := range events {
		switch part.Type {
		case model.PartGroupStart:
			g, _ := part.Resource.(*model.GroupStart)
			typ := ""
			if g != nil {
				typ = g.Type
			}
			groupStack = append(groupStack, typ)
			if typ == "table" {
				if err := sep(); err != nil {
					return err
				}
				if _, err := io.WriteString(w.Output, "|===\n"); err != nil {
					return err
				}
			}
		case model.PartGroupEnd:
			if n := len(groupStack); n > 0 {
				typ := groupStack[n-1]
				groupStack = groupStack[:n-1]
				if typ == "table" {
					if _, err := io.WriteString(w.Output, "|===\n"); err != nil {
						return err
					}
				}
			}
		case model.PartData:
			if d, ok := part.Resource.(*model.Data); ok {
				if raw := d.Properties["raw"]; raw != "" {
					if _, err := io.WriteString(w.Output, raw); err != nil {
						return err
					}
					first = false
				}
			}
		case model.PartBlock:
			block, ok := part.Resource.(*model.Block)
			if !ok {
				continue
			}
			if err := w.writeBlockNormalized(block, sep); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeBlockNormalized renders one block with canonical AsciiDoc markers keyed
// on its semantic role.
func (w *Writer) writeBlockNormalized(block *model.Block, sep func() error) error {
	text := w.blockText(block)
	role := block.SemanticRole()

	switch role {
	case model.RoleTableHeader, model.RoleTableCell:
		_, err := fmt.Fprintf(w.Output, "| %s\n", text)
		return err
	case model.RoleListItem:
		level := max(headingLevel(block), 1)
		_, err := fmt.Fprintf(w.Output, "%s %s\n", strings.Repeat("*", level), text)
		return err
	}

	if err := sep(); err != nil {
		return err
	}
	var prefix string
	switch role {
	case model.RoleHeading:
		level := max(headingLevel(block), 1)
		prefix = strings.Repeat("=", level) + " "
	case model.RoleCaption:
		prefix = "."
	}
	_, err := fmt.Fprintf(w.Output, "%s%s\n", prefix, text)
	return err
}

// headingLevel returns a block's level from the structural annotation, falling
// back to the legacy "level" property; 0 when neither is present.
func headingLevel(block *model.Block) int {
	if s, ok := block.Structure(); ok && s != nil && s.Level > 0 {
		return s.Level
	}
	if v, ok := block.Properties["level"]; ok {
		n := 0
		_, _ = fmt.Sscanf(v, "%d", &n)
		return n
	}
	return 0
}
