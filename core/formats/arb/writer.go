package arb

import (
	"context"
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
	cfg *Config
}

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

// Write consumes Parts and writes the reconstructed ARB document.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	var original []byte
	var layerLocale string
	repl := newReplacements()

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
					w.collectBlock(block, repl)
				}
			}
		}
	}
done:
	if original != nil {
		return w.writeFromOriginal(original, repl)
	}
	return w.writeFromScratch(repl, layerLocale)
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
	if note, ok := block.Annotations["note"].(*model.NoteAnnotation); ok {
		description = note.Text
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
