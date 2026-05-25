package applestrings

import (
	"context"
	"strconv"

	"github.com/neokapi/neokapi/core/model"
)

// emitStringsdict parses the .stringsdict plist content and emits one Block per
// translatable <string> leaf (the NSStringLocalizedFormatKey format string and
// each CLDR plural-category value). Like .strings, a .stringsdict is monolingual
// (one file per locale), so each leaf value is carried as the file's source
// content; a tool translating the file populates the target locale, which the
// writer splices back into the same <string>.
func (r *Reader) emitStringsdict(ctx context.Context, ch chan<- model.PartResult, content string, locale model.LocaleID) bool {
	doc, err := parseStringsdict(content)
	if err != nil {
		select {
		case ch <- model.PartResult{Error: err}:
		case <-ctx.Done():
		}
		return false
	}

	counter := 0
	for i := range doc.leafs {
		leaf := doc.leafs[i]
		counter++
		block := &model.Block{
			ID:           "tu" + strconv.Itoa(counter),
			Name:         leafName(leaf),
			Translatable: true,
			SourceLocale: locale,
			Source:       runsFromValue(leaf.value, r.cfg.ProtectPlaceholders),
			Targets:      make(map[model.VariantKey]*model.Target),
			Properties:   make(map[string]string),
			Annotations:  make(map[string]model.Annotation),
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

		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return false
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
