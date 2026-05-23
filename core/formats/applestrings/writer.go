package applestrings

import (
	"context"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Writer implements DataFormatWriter for Apple Strings (.strings) and Apple
// Stringsdict (.stringsdict) files.
//
// Output strategy: the reader stores the original (UTF-8) document bytes plus a
// kind/encoding marker on the root Layer. The writer re-parses those bytes and
// rewrites only the value strings whose corresponding Block carries a changed
// translation, preserving every other byte (comments, key order, whitespace,
// escaping, the plist DOCTYPE) exactly. UTF-16 inputs are re-encoded to their
// original byte order on output. This guarantees byte-faithful round-trips for
// documents whose content was not modified. When no original is available
// (synthetic pipelines), the writer builds a canonical document from scratch.
type Writer struct {
	format.BaseFormatWriter
	cfg *Config
}

// NewWriter creates a new Apple Strings writer.
func NewWriter() *Writer {
	cfg := &Config{}
	cfg.Reset()
	return &Writer{
		BaseFormatWriter: format.BaseFormatWriter{
			FormatName: "applestrings",
		},
		cfg: cfg,
	}
}

// Config returns the writer's config for customization.
func (w *Writer) Config() *Config { return w.cfg }

// leafRef identifies one value across both file kinds. For .strings only key is
// set (leaf == leafValue). For .stringsdict leaf is "format" or "plural" and
// variable/category qualify plural leaves.
type leafRef struct {
	key      string
	leaf     string
	variable string
	category string
}

// Write consumes Parts and writes the reconstructed document.
func (w *Writer) Write(ctx context.Context, parts <-chan *model.Part) error {
	var layerProps map[string]string
	values := make(map[leafRef]string)
	var order []leafRef

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
					layerProps = layer.Properties
				}
			case model.PartBlock:
				if block, ok := part.Resource.(*model.Block); ok {
					ref, hasRef := refFromBlock(block)
					if !hasRef {
						continue
					}
					if _, seen := values[ref]; !seen {
						order = append(order, ref)
					}
					values[ref] = w.resolveValue(block, ref)
				}
			}
		}
	}
done:
	kind := "strings"
	enc := encUTF8
	original := ""
	if layerProps != nil {
		if k := layerProps[propKind]; k != "" {
			kind = k
		}
		if e := layerProps[propEncoding]; e != "" {
			enc = e
		}
		original = layerProps[propOriginal]
	}

	var out []byte
	var err error
	switch kind {
	case "stringsdict":
		if original != "" {
			out, err = rewriteStringsdict(original, values)
		} else {
			out = buildStringsdictFromScratch(order, values)
		}
	default:
		if original != "" {
			out, err = rewriteStrings(original, values)
		} else {
			out = buildStringsFromScratch(order, values)
		}
	}
	if err != nil {
		return err
	}

	encoded := encodeFromUTF8(string(out), enc)
	_, werr := w.Output.Write(encoded)
	return werr
}

// resolveValue returns the output value for a block: the target text for the
// writer's locale when present, otherwise the source text. The value is the
// markup-preserving rendering so protected placeholders re-emit verbatim.
func (w *Writer) resolveValue(block *model.Block, _ leafRef) string {
	if !w.Locale.IsEmpty() && block.HasTarget(w.Locale) {
		return valueFromRuns(block.TargetRuns(w.Locale))
	}
	return valueFromRuns(block.SourceRuns())
}

// refFromBlock reconstructs a leafRef from a Block's properties.
func refFromBlock(b *model.Block) (leafRef, bool) {
	key, ok := b.Properties[propBlockKey]
	if !ok {
		return leafRef{}, false
	}
	return leafRef{
		key:      key,
		leaf:     b.Properties[propBlockLeaf],
		variable: b.Properties[propBlockVar],
		category: b.Properties[propBlockCategory],
	}, true
}
