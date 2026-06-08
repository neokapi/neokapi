package applestrings

import (
	"context"
	"strconv"

	"github.com/neokapi/neokapi/core/model"
)

// Block property keys shared by both file kinds. The writer reads these to
// locate the leaf value to splice.
const (
	propBlockKey      = "applestrings.key"       // entry key (.strings) or top-level key (.stringsdict)
	propBlockVar      = "applestrings.var"       // variable name (.stringsdict plural leaf)
	propBlockCategory = "applestrings.category"  // CLDR plural category (.stringsdict)
	propBlockLeaf     = "applestrings.leaf"      // "value" | "format" | "plural"
	propBlockSpecType = "applestrings.specType"  // NSStringFormatSpecTypeKey
	propBlockValType  = "applestrings.valueType" // NSStringFormatValueTypeKey
)

const (
	leafValue = "value" // a plain .strings "key" = "value"; entry
)

// emitStrings parses the .strings content and emits one Block per entry.
func (r *Reader) emitStrings(ctx context.Context, ch chan<- model.PartResult, content string, locale model.LocaleID) bool {
	doc, err := parseStringsFile(content)
	if err != nil {
		select {
		case ch <- model.PartResult{Error: err}:
		case <-ctx.Done():
		}
		return false
	}

	// Skeleton cursor: the byte offset up to which the file structure has been
	// emitted as SkeletonText. For each entry we emit the prefix structure
	// (comments, the quoted key, ` = `, the opening quote) up to the value's
	// inner span, then a SkeletonRef standing in for the DECODED value text
	// (the writer re-encodes the quoting/escaping), then advance past the value.
	cursor := 0

	counter := 0
	for i := range doc.entries {
		e := doc.entries[i]
		counter++
		blockID := "tu" + strconv.Itoa(counter)
		block := &model.Block{
			ID:           blockID,
			Name:         e.key,
			Translatable: true,
			SourceLocale: locale,
			Source:       runsFromValue(e.value, r.cfg.ProtectPlaceholders),
			Targets:      make(map[model.VariantKey]*model.Target),
			Properties:   make(map[string]string),
		}
		block.Properties[propBlockKey] = e.key
		block.Properties[propBlockLeaf] = leafValue

		// The preceding comment (/* */ or //) becomes a translator note.
		if e.hasComment && r.cfg.ExtractComments && e.comment != "" {
			block.SetAnno("note", &model.NoteAnnotation{
				Text:      e.comment,
				From:      "developer",
				Annotates: "general",
			})
		}

		// Skeleton: structure before the value's inner content → Text; the value
		// itself → Ref; resume after the value's inner content.
		if r.skeletonStore != nil {
			r.skelText(content[cursor:e.valStart])
			r.skelRef(blockID)
			cursor = e.valEnd
		}

		if !r.emit(ctx, ch, &model.Part{Type: model.PartBlock, Resource: block}) {
			return false
		}
	}

	// Skeleton: trailing structure after the last value (closing quote, ';',
	// trailing whitespace/EOF). skelFlush in readContent emits the final buffer.
	if r.skeletonStore != nil {
		r.skelText(content[cursor:])
	}
	return true
}
