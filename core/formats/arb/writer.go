package arb

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

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

// Ensure Writer satisfies StreamingWriter: paired with the streaming reader it
// consumes a streaming skeleton store interleaved with the Part stream, pulling
// each block on demand, so the writer side is bounded too.
var _ format.StreamingWriter = (*Writer)(nil)

// StreamingWriter marks the bounded-memory interleaved write path.
func (w *Writer) StreamingWriter() {}

// NewWriter creates a new ARB writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		FormatName: "arb",
		cfg:        cfg,
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
	// Streaming skeleton round-trip: interleave skeleton consumption with the
	// Part stream, pulling each message block on demand rather than buffering the
	// whole block map. Each value ref renders through the renderRef the buffered
	// path also uses.
	if w.skeletonStore != nil && w.skeletonStore.IsStreaming() {
		return format.StreamSkeletonWrite(ctx, w.skeletonStore, parts, w.Output, w.renderRef, w.renderLang)
	}

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
					if err := w.collectBlock(block, repl); err != nil {
						return err
					}
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
		return w.writeFromOriginal(original, repl, layerLocale)
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
	return format.BufferedSkeletonWrite(store, blocksByID, w.Output, w.renderRef, w.renderLang)
}

// renderLang writes the value of "@@locale" (the reader stores it as a
// language entry): the locale the writer writes, so a translation's file names
// its own language rather than its source's.
func (w *Writer) renderLang(value string) ([]byte, error) {
	return []byte(fileLocale(w.Locale, value)), nil
}

// fileLocale is the "@@locale" value of a file written for target, whose
// source file declared declared: declared when the writer writes no locale or
// the one the file declares, and otherwise target, spelled as Flutter names a
// locale (pt_BR) unless the source spelled its own with a hyphen.
func fileLocale(target model.LocaleID, declared string) string {
	if target.IsEmpty() {
		return declared
	}
	if declared != "" && model.NormalizeLocale(model.LocaleID(declared)) == model.NormalizeLocale(target) {
		return declared
	}
	name := string(model.NormalizeLocale(target))
	if !strings.Contains(declared, "-") {
		name = strings.ReplaceAll(name, "-", "_")
	}
	return name
}

// renderRef returns the bytes a value SkeletonRef contributes for the given
// block, shared by the buffered (writeFromSkeleton) and streaming
// (StreamSkeletonWrite) paths so both produce identical output. A nil block
// emits an empty JSON string rather than nothing: dropping the value would
// leave `"key":` with no value and invalidate the document. A value that
// reads as the file spelled it is written with the file's bytes, so its
// escapes stay as they were (propRaw).
func (w *Writer) renderRef(block *model.Block) ([]byte, error) {
	var value string
	if block != nil {
		var err error
		if value, err = w.blockValue(block); err != nil {
			return nil, err
		}
		if raw := block.Properties[propRaw]; raw != "" {
			var spelled string
			if json.Unmarshal([]byte(raw), &spelled) == nil && spelled == value {
				return []byte(raw), nil
			}
		}
	}
	return []byte(encodeJSONString(value)), nil
}

// blockValue resolves a block's output ARB message value: the target for the
// writer's active locale when present, otherwise the source. Runs that still
// read as the message did are written as its exact bytes, and a plural or
// select is written in the syntax it was read with (valueFromRuns). A value
// holding a plural or select that would read as another message is refused
// (checkMessage).
func (w *Writer) blockValue(block *model.Block) (string, error) {
	runs := block.SourceRuns()
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		runs = block.TargetRuns(w.Locale)
	}
	original := block.Properties[propMessage]
	value := valueFromRuns(runs, original)
	if err := checkMessage(block, runs, value, original); err != nil {
		return "", err
	}
	return value, nil
}

// Ensure Writer spells a message value from its runs.
var _ format.ValueSpeller = (*Writer)(nil)

// SpellValue returns the message value the writer writes for runs, an edition
// of b: in the shape b's message was read with when b holds a plural or
// select, else as Flutter's tools write one.
func (w *Writer) SpellValue(b *model.Block, runs []model.Run) string {
	var original string
	if b != nil {
		original = b.Properties[propMessage]
	}
	return valueFromRuns(runs, original)
}

// collectBlock records the output value for a block keyed by its ARB resource
// key: the target for the writer's locale when present, otherwise the source,
// as blockValue resolves it.
func (w *Writer) collectBlock(block *model.Block, repl *replacements) error {
	key, ok := block.Properties["arb.key"]
	if !ok {
		return nil
	}
	value, err := w.blockValue(block)
	if err != nil {
		return err
	}
	var description string
	if notes := block.Notes(); len(notes) > 0 {
		description = notes[0].Text
	}
	repl.set(key, value, description)
	return nil
}

// writeFromOriginal re-tokenizes the original bytes and rewrites changed
// message values in place.
func (w *Writer) writeFromOriginal(original []byte, repl *replacements, declared string) error {
	if locale := fileLocale(w.Locale, declared); locale != declared {
		repl.locale = &locale
	}
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
	out := buildCanonical(repl, fileLocale(w.Locale, locale))
	_, err := io.WriteString(w.Output, out)
	return err
}

// replacements accumulates resolved message values keyed by ARB resource key.
type replacements struct {
	values map[string]replValue
	// locale, when set, replaces the value of "@@locale".
	locale *string
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
