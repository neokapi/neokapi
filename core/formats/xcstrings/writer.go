package xcstrings

import (
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for Apple String Catalog (.xcstrings) files.
//
// Output strategy: the reader stores the original document bytes on the root
// Layer. The writer re-tokenizes those bytes and rewrites only the leaf
// "value" strings whose corresponding Block carries a changed translation,
// preserving every other byte (key order, whitespace, escaping) exactly. This
// guarantees byte-faithful round-trips for catalogs whose content was not
// modified. When no original is available (synthetic pipelines), the writer
// builds a catalog from scratch using Apple's canonical formatting.
type Writer struct {
	format.BaseFormatWriter
	cfg           *Config
	skeletonStore *format.SkeletonStore
}

// Ensure Writer implements SkeletonStoreConsumer.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// NewWriter creates a new Apple String Catalog writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "xcstrings",
		},
		cfg: cfg,
	}
}

// Config returns the writer's config for customization.
func (w *Writer) Config() *Config { return w.cfg }

// SetSkeletonStore sets the skeleton store for byte-exact output. When set, the
// writer reconstructs the document from the skeleton stream emitted by the
// reader (SkeletonText replayed verbatim, SkeletonRef resolved to the block's
// translated-or-source value, JSON-escaped), splicing in only changed values.
// This is the path `kapi merge` uses: the source-captured skeleton plus the
// freshly-read blocks.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
}

// Write consumes Parts and writes the reconstructed string catalog.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	var original []byte
	var layerProps map[string]string
	// replacements maps a value reference to the resolved output value string.
	repl := newReplacements()
	// blocksByID indexes blocks for the skeleton path, which resolves Refs by
	// block ID rather than by value reference.
	blocksByID := make(map[string]*model.Block)

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
					if raw, ok := layer.Properties["xcstrings.original"]; ok {
						original = []byte(raw)
					}
					layerProps = layer.Properties
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
	// Mode 1: Skeleton store (byte-exact, streaming-friendly). This is the
	// merge path — the original-bytes property is intentionally absent when a
	// skeleton store is wired, so this branch must come first.
	if w.skeletonStore != nil {
		if err := w.skeletonStore.Flush(); err != nil {
			return fmt.Errorf("xcstrings writer: flush skeleton: %w", err)
		}
		return w.writeFromSkeleton(w.skeletonStore, blocksByID)
	}

	// Mode 2: re-tokenize the original bytes and splice changed leaf values.
	if original != nil {
		return w.writeFromOriginal(original, repl)
	}
	// Mode 3: build a canonical catalog from scratch (synthetic pipelines).
	return w.writeFromScratch(repl, layerProps)
}

// writeFromSkeleton replays the skeleton stream, writing each SkeletonText
// entry verbatim and resolving each SkeletonRef to the referenced block's
// output value (target for the active locale, else source), JSON-escaped the
// way Apple's encoder does. An untranslated roundtrip is byte-for-byte exact.
func (w *Writer) writeFromSkeleton(store *format.SkeletonStore, blocks map[string]*model.Block) error {
	for {
		entry, err := store.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return fmt.Errorf("xcstrings writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			if _, err := w.Output.Write(entry.Data); err != nil {
				return err
			}
		case format.SkeletonRef:
			block, ok := blocks[string(entry.Data)]
			if !ok {
				// No block for this Ref — emit an empty JSON string rather than
				// dropping the value (which would produce invalid JSON). This
				// should not happen for skeletons emitted by this reader.
				if _, err := io.WriteString(w.Output, encodeJSONString("")); err != nil {
					return err
				}
				continue
			}
			if _, err := io.WriteString(w.Output, encodeJSONString(w.blockValue(block))); err != nil {
				return err
			}
		}
	}
	return nil
}

// blockValue resolves the output value for a block on the skeleton path: the
// target text for the writer's active locale when present, otherwise the value
// the reader recorded for this leaf (xcstrings.value), otherwise the locale-
// appropriate runs. This mirrors collectBlock's resolution so the two write
// paths agree.
func (w *Writer) blockValue(block *model.Block) string {
	vr, _ := valueRefFromBlock(block)
	switch {
	case !w.Locale.IsEmpty() && block.HasTarget(w.Locale):
		return valueFromRuns(block.TargetRuns(w.Locale))
	case w.Locale.IsEmpty() && model.LocaleID(vr.Lang) == block.SourceLocale:
		return valueFromRuns(block.SourceRuns())
	case !w.Locale.IsEmpty() && w.Locale == block.SourceLocale:
		return valueFromRuns(block.SourceRuns())
	default:
		// No translation for the active locale — keep the leaf's original
		// value so it round-trips unchanged.
		if v, ok := block.Properties["xcstrings.value"]; ok {
			return v
		}
		if model.LocaleID(vr.Lang) == block.SourceLocale {
			return valueFromRuns(block.SourceRuns())
		}
		return valueFromRuns(block.TargetRuns(model.LocaleID(vr.Lang)))
	}
}

// collectBlock records the output value for a block keyed by its value
// reference. The output value is the target text for the writer's locale when
// present, otherwise the source text — mirroring how other native writers
// resolve the active locale.
func (w *Writer) collectBlock(block *model.Block, repl *replacements) {
	vr, ok := valueRefFromBlock(block)
	if !ok {
		return
	}

	var value string
	switch {
	case !w.Locale.IsEmpty() && block.HasTarget(w.Locale):
		value = valueFromRuns(block.TargetRuns(w.Locale))
	case w.Locale.IsEmpty() && model.LocaleID(vr.Lang) == block.SourceLocale:
		value = valueFromRuns(block.SourceRuns())
	case !w.Locale.IsEmpty() && w.Locale == block.SourceLocale:
		value = valueFromRuns(block.SourceRuns())
	default:
		// No translation for the active locale on this block — keep the
		// original value (recorded by the reader) so the leaf round-trips
		// unchanged.
		if v, ok := block.Properties["xcstrings.value"]; ok {
			value = v
		} else if model.LocaleID(vr.Lang) == block.SourceLocale {
			value = valueFromRuns(block.SourceRuns())
		} else {
			value = valueFromRuns(block.TargetRuns(model.LocaleID(vr.Lang)))
		}
	}

	state := block.Properties["state"]
	repl.set(vr, value, state)
}

// writeFromOriginal re-tokenizes the original bytes and rewrites changed leaf
// values in place.
func (w *Writer) writeFromOriginal(original []byte, repl *replacements) error {
	out, err := rewriteCatalog(original, repl)
	if err != nil {
		return err
	}
	_, werr := w.Output.Write(out)
	return werr
}

// writeFromScratch builds a canonical Apple-formatted catalog from the
// collected replacements when no original document is available.
func (w *Writer) writeFromScratch(repl *replacements, layerProps map[string]string) error {
	srcLang := "en"
	version := "1.0"
	if layerProps != nil {
		if s := layerProps["xcstrings.sourceLanguage"]; s != "" {
			srcLang = s
		}
		if v := layerProps["xcstrings.version"]; v != "" {
			version = v
		}
	}
	out := buildCanonical(repl, srcLang, version)
	_, err := io.WriteString(w.Output, out)
	return err
}

// replacements accumulates resolved leaf values keyed by value reference.
type replacements struct {
	values map[refKey]replValue
}

type refKey struct {
	key      string
	lang     string
	kind     valueKind
	sub      string
	category string
}

type replValue struct {
	value string
	state string
	set   bool
}

func newReplacements() *replacements {
	return &replacements{values: make(map[refKey]replValue)}
}

func keyOf(vr valueRef) refKey {
	return refKey{
		key:      vr.Key,
		lang:     vr.Lang,
		kind:     vr.Kind,
		sub:      vr.Sub,
		category: vr.Category,
	}
}

func (r *replacements) set(vr valueRef, value, state string) {
	r.values[keyOf(vr)] = replValue{value: value, state: state, set: true}
}

func (r *replacements) lookup(vr valueRef) (replValue, bool) {
	v, ok := r.values[keyOf(vr)]
	return v, ok
}
