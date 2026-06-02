package applestrings

import (
	"context"
	"errors"
	"fmt"
	"io"

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
	cfg           *Config
	skeletonStore *format.SkeletonStore
}

// Ensure Writer implements SkeletonStoreConsumer so kapi merge can replay the
// source skeleton captured at extract, reconstructing the file byte-exactly with
// only changed values re-encoded. This is the merge byte-exactness guarantee.
var _ format.SkeletonStoreConsumer = (*Writer)(nil)

// SetSkeletonStore sets the skeleton store for byte-exact output.
func (w *Writer) SetSkeletonStore(store *format.SkeletonStore) {
	w.skeletonStore = store
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
					layerProps = layer.Properties
				}
			case model.PartBlock:
				if block, ok := part.Resource.(*model.Block); ok {
					blocksByID[block.ID] = block
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

	// Skeleton path (kapi merge): the source skeleton replays every structural
	// byte verbatim; only changed values are re-encoded. This runs before the
	// original/scratch paths because merge feeds a synthetic layer with no
	// propOriginal — the skeleton is the sole source of structure. The UTF-8
	// output still flows through the final encode step below so UTF-16 byte
	// order and any UTF-8 BOM are reproduced exactly.
	if w.skeletonStore != nil {
		out, err := w.writeFromSkeleton(blocksByID, kind)
		if err != nil {
			return err
		}
		encoded := encodeFromUTF8(string(out), enc)
		_, werr := w.Output.Write(encoded)
		return werr
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

// writeFromSkeleton reconstructs the UTF-8 document from the skeleton stream:
// SkeletonText entries are emitted verbatim; each SkeletonRef is replaced by the
// re-encoded value of the referenced block. The .strings and .stringsdict value
// encoders differ (NeXTSTEP escaping vs XML entity encoding), selected per block
// via its leaf kind property — falling back to the document kind. The returned
// bytes are UTF-8; the caller applies the final UTF-16/BOM re-encode.
func (w *Writer) writeFromSkeleton(blocks map[string]*model.Block, kind string) ([]byte, error) {
	if err := w.skeletonStore.Flush(); err != nil {
		return nil, fmt.Errorf("applestrings writer: flush skeleton: %w", err)
	}
	var out []byte
	for {
		entry, err := w.skeletonStore.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("applestrings writer: read skeleton: %w", err)
		}
		switch entry.Type {
		case format.SkeletonText:
			out = append(out, entry.Data...)
		case format.SkeletonRef:
			block, ok := blocks[string(entry.Data)]
			if !ok {
				continue // unknown ref — emit nothing (value dropped)
			}
			ref, _ := refFromBlock(block)
			value := w.resolveValue(block, ref)
			out = append(out, w.encodeValue(value, block, kind)...)
		}
	}
	return out, nil
}

// encodeValue escapes a decoded value for its sub-format: a .stringsdict value
// uses XML entity encoding (encodePlistText); a .strings value uses NeXTSTEP
// quoted-string escaping (encodeStringsValue). The block's leaf-kind property
// distinguishes the two — "format"/"plural" are stringsdict leaves, "value" is a
// .strings entry — with the document kind as the fallback.
func (w *Writer) encodeValue(value string, block *model.Block, kind string) string {
	leaf := block.Properties[propBlockLeaf]
	isDict := leaf == string(leafFormatKey) || leaf == string(leafPlural)
	if leaf == "" {
		isDict = kind == "stringsdict"
	}
	if isDict {
		return encodePlistText(value)
	}
	return encodeStringsValue(value)
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
