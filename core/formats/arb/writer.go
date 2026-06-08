package arb

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for Flutter Application Resource Bundle
// (.arb) files.
//
// Output strategy: the reader stores the original document bytes on the root
// Layer. The writer re-tokenizes those bytes and rewrites only the message
// value strings whose corresponding Block carries a changed value, preserving
// every other byte (key order, whitespace, JSON escaping, @/@@ metadata)
// exactly. This guarantees byte-faithful round-trips for files whose content
// was not modified. When no original is available (synthetic pipelines), the
// writer builds a document from scratch using Dart's canonical indentation.
type Writer struct {
	format.BaseFormatWriter
	cfg           *Config
	skeletonStore *format.SkeletonStore
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new ARB writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "arb",
		},
		cfg: cfg,
	}
}

// Config returns the writer's config for customization.
func (w *Writer) Config() *Config { return w.cfg }

// SetSkeletonStore sets the skeleton store for byte-exact output. When set, the
// writer reconstructs the document from the skeleton (the kapi merge path)
// rather than re-tokenizing the original bytes.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts and writes the reconstructed ARB document.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	var original []byte
	var layerLocale string
	repl := newReplacements()
	blocksByID := make(map[string]*model.Block) // block.ID → block (for skeleton store)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case part, ok := <-parts:
			if !ok {
				goto done
			}
			switch part.Type {
			case model.PartLayerStart:
				if layer, ok := part.Resource.(*model.Layer); ok && layer.IsRoot() {
					if raw, ok := layer.Properties["arb.original"]; ok {
						original = []byte(raw)
					}
					layerLocale = layer.Properties["arb.locale"]
				}
			case model.PartBlock:
				if block, ok := part.Resource.(*model.Block); ok {
					blocksByID[block.ID] = block
					w.collectBlock(block, repl)
				}
			}
		}
	}
done:
	// Skeleton path (byte-exact, the kapi merge path): rebuild from the
	// captured skeleton, splicing in each block's resolved value. Takes
	// precedence over the original/scratch fallbacks.
	if w.skeletonStore != nil {
		if err := w.skeletonStore.Flush(); err != nil {
			return fmt.Errorf("arb writer: flush skeleton: %w", err)
		}
		return w.writeFromSkeleton(w.skeletonStore, blocksByID)
	}

	if original != nil {
		return w.writeFromOriginal(original, repl)
	}
	return w.writeFromScratch(repl, layerLocale)
}

// writeFromSkeleton reads skeleton entries and reconstructs the document. Text
// entries are written verbatim; each Ref is replaced with the referenced
// block's resolved value (target for the writer's locale, else source),
// JSON-escaped the way Dart's JsonEncoder does. This produces byte-exact output
// for unchanged messages and changes only the message values that were
// translated.
func (w *Writer) writeFromSkeleton(store *format.SkeletonStore, blocksByID map[string]*model.Block) error {
	for {
		entry, err := store.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("arb writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			var value string
			if block, ok := blocksByID[string(entry.Data)]; ok {
				value = w.blockValue(block)
			}
			if _, err := io.WriteString(w.Output, encodeJSONString(value)); err != nil {
				return err
			}
		}
	}
	return nil
}

// blockValue resolves a block's output ARB message value: the target text for
// the writer's active locale when present, otherwise the source — mirroring
// collectBlock. ICU placeholders re-emit their captured source via
// valueFromRuns, so unchanged messages reproduce their exact bytes.
func (w *Writer) blockValue(block *model.Block) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return valueFromRuns(block.TargetRuns(w.Locale))
	}
	return valueFromRuns(block.SourceRuns())
}

// collectBlock records the output value for a block keyed by its ARB resource
// key. The output value is the target text for the writer's locale when present,
// otherwise the source text — mirroring how other native writers resolve the
// active locale. ICU placeholders re-emit their captured source via
// valueFromRuns, so unchanged messages reproduce their exact bytes.
func (w *Writer) collectBlock(block *model.Block, repl *replacements) {
	key, ok := block.Properties["arb.key"]
	if !ok {
		return
	}

	var value string
	switch {
	case !w.Locale.IsEmpty() && block.HasTarget(w.Locale):
		value = valueFromRuns(block.TargetRuns(w.Locale))
	case !w.Locale.IsEmpty() && w.Locale == block.SourceLocale:
		value = valueFromRuns(block.SourceRuns())
	case w.Locale.IsEmpty():
		value = valueFromRuns(block.SourceRuns())
	default:
		// No translation for the active locale — keep the source so the message
		// round-trips unchanged.
		value = valueFromRuns(block.SourceRuns())
	}

	var description string
	if av, ok := block.Anno("note"); ok {
		if note, ok := av.(*model.NoteAnnotation); ok {
			description = note.Text
		}
	}
	repl.set(key, value, description)
}

// writeFromOriginal re-tokenizes the original bytes and rewrites changed
// message values in place.
func (w *Writer) writeFromOriginal(original []byte, repl *replacements) error {
	out, err := rewriteCatalog(original, repl)
	if err != nil {
		return err
	}
	_, werr := w.Output.Write(out)
	return werr
}

// writeFromScratch builds a canonical Dart-formatted ARB document from the
// collected replacements when no original document is available.
func (w *Writer) writeFromScratch(repl *replacements, locale string) error {
	if locale == "" && !w.Locale.IsEmpty() {
		locale = string(w.Locale)
	}
	out := buildCanonical(repl, locale)
	_, err := io.WriteString(w.Output, out)
	return err
}

// replacements accumulates resolved message values keyed by ARB resource key.
type replacements struct {
	values map[string]replValue
}

type replValue struct {
	value       string
	description string
	set         bool
}

func newReplacements() *replacements {
	return &replacements{values: make(map[string]replValue)}
}

func (r *replacements) set(key, value, description string) {
	r.values[key] = replValue{value: value, description: description, set: true}
}

func (r *replacements) lookup(key string) (replValue, bool) {
	v, ok := r.values[key]
	return v, ok
}
