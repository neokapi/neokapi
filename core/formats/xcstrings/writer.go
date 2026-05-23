package xcstrings

import (
	"context"
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
	cfg *Config
}

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

// Write consumes Parts and writes the reconstructed string catalog.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	var original []byte
	var layerProps map[string]string
	// replacements maps a value reference to the resolved output value string.
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
					if raw, ok := layer.Properties["xcstrings.original"]; ok {
						original = []byte(raw)
					}
					layerProps = layer.Properties
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
	return w.writeFromScratch(repl, layerProps)
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
