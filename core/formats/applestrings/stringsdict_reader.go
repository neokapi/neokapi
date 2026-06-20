package applestrings

import (
	"context"
	"strconv"

	"github.com/neokapi/neokapi/core/model"
)

// emitStringsdict emits one Block per translatable <string> leaf of the
// already-parsed .stringsdict doc (the NSStringLocalizedFormatKey format string
// and each CLDR plural-category value). Like .strings, a .stringsdict is
// monolingual (one file per locale), so each leaf value is carried as the file's
// source content; a tool translating the file populates the target locale, which
// the writer splices back into the same <string>. The parse happens up front in
// readContent so any XML comments can be attached to the layer beforehand.
func (r *Reader) emitStringsdict(ctx context.Context, ch chan<- model.PartResult, locale model.LocaleID, doc *stringsdictDoc) bool {
	// Skeleton token cursor: the index of the next token whose raw bytes have not
	// yet been emitted as SkeletonText. Leaves are walked in document order, so
	// their <string> start-tag indices (strStart) are strictly ascending; for
	// each leaf we emit every token up to and including its <string> start tag,
	// a SkeletonRef for the value, the </string> end tag, then skip the inner
	// value tokens. Everything else (DOCTYPE, keys, whitespace, entities) is
	// replayed verbatim.
	tokCursor := 0

	counter := 0
	for i := range doc.leafs {
		leaf := doc.leafs[i]
		counter++
		blockID := "tu" + strconv.Itoa(counter)
		block := &model.Block{
			ID:           blockID,
			Name:         leafName(leaf),
			Translatable: true,
			SourceLocale: locale,
			Source:       runsFromValue(leaf.value, r.cfg.ProtectPlaceholders),
			Targets:      make(map[model.VariantKey]*model.Target),
			Properties:   make(map[string]string),
		}
		block.Properties[propBlockKey] = leaf.topKey
		block.Properties[propBlockLeaf] = string(leaf.kind)
		if leaf.variable != "" {
			block.Properties[propBlockVar] = leaf.variable
		}
		if leaf.category != "" {
			block.Properties[propBlockCategory] = leaf.category
		}
		if leaf.specType != "" {
			block.Properties[propBlockSpecType] = leaf.specType
		}
		if leaf.valueType != "" {
			block.Properties[propBlockValType] = leaf.valueType
		}

		// Skeleton: structure up to and including the <string> start tag → Text;
		// the value → Ref; the </string> end tag → Text; skip inner value tokens.
		if r.skeletonStore != nil {
			for t := tokCursor; t <= leaf.strStart; t++ {
				r.skelText(doc.toks[t].raw)
			}
			r.skelRef(blockID)
			r.skelText(doc.toks[leaf.strEnd].raw)
			tokCursor = leaf.strEnd + 1
		}

		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return false
		}
	}

	// Skeleton: trailing tokens after the last leaf's </string> (closing dicts,
	// </plist>, trailing whitespace/EOF). skelFlush in readContent emits it.
	if r.skeletonStore != nil {
		for t := tokCursor; t < len(doc.toks); t++ {
			r.skelText(doc.toks[t].raw)
		}
	}
	return true
}

// leafName builds a stable, human-readable Block name for a stringsdict leaf:
//
//	<topKey>                              (format key)
//	<topKey>/<variable>/<category>        (plural value)
func leafName(leaf dictLeaf) string {
	if leaf.kind == leafFormatKey {
		return leaf.topKey
	}
	return leaf.topKey + "/" + leaf.variable + "/" + leaf.category
}
