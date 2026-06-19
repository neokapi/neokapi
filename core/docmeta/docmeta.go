// Package docmeta carries document/asset metadata (a PDF's Info dictionary, an
// image's XMP/PNG text chunks, …) onto the content model. Metadata is document-
// level, not anchored to any run, so it lives on the Layer — never in a run-
// anchored overlay:
//
//   - Non-translatable fields (author, producer, dates, software) are recorded
//     as namespaced Layer.Properties. They are never sent to translation and are
//     preserved for inspection and re-application on write.
//   - Translatable fields (title, subject, description, keywords) are emitted as
//     Blocks on the metadata plane (StructureAnnotation.Layer == LayerMetadata),
//     so they localize through the normal block path and the writer folds the
//     target back into the field.
//
// This mirrors how the OOXML reader treats docProps/core.xml (translatable
// Dublin-Core fields become blocks; the rest stays skeleton), generalized to
// formats whose round-trip is a byte copy (image) or cross-format (PDF).
package docmeta

import (
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// MetadataFieldProperty is the Block.Properties key naming which metadata field a
// metadata block carries (its namespaced key, e.g. "pdf:title"), so a writer can
// fold the translated target back into the right field.
const MetadataFieldProperty = "metadata-field"

// Entry is one extracted metadata field. Key is the namespaced property key
// (e.g. "pdf:author", "xmp:dc:description"). A Translatable entry becomes a
// metadata Block (with Role, if set); a non-translatable one becomes a Layer
// property.
type Entry struct {
	Key          string
	Value        string
	Translatable bool
	Role         string // optional model.Role* for the metadata block
}

// Apply records the entries on layer and returns the translatable ones as
// metadata-plane Blocks (Translatable, role set when provided), in input order.
// Non-translatable entries (and empty values are skipped entirely) are written to
// layer.Properties under their namespaced key. Block IDs are idPrefix + "-" +
// a slug of the key, so they are stable and unique across a document.
func Apply(layer *model.Layer, entries []Entry, idPrefix string) []*model.Block {
	if layer == nil {
		return nil
	}
	var blocks []*model.Block
	for _, e := range entries {
		if e.Value == "" {
			continue
		}
		if !e.Translatable {
			if layer.Properties == nil {
				layer.Properties = map[string]string{}
			}
			layer.Properties[e.Key] = e.Value
			continue
		}
		b := model.NewBlock(idPrefix+"-"+slug(e.Key), e.Value)
		b.Name = e.Key
		if e.Role != "" {
			b.SetSemanticRole(e.Role, 0)
		}
		b.SetLayoutLayer(model.LayerMetadata)
		b.Properties[MetadataFieldProperty] = e.Key
		blocks = append(blocks, b)
	}
	return blocks
}

// slug turns a namespaced key into an ID-safe token ("xmp:dc:title" → "xmp-dc-title").
func slug(key string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return r
		default:
			return '-'
		}
	}, key)
}
