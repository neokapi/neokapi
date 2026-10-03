package host

import (
	"fmt"
	"io"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// A plural or a select holds its words in branches, and an XLIFF or PO unit
// holds one flat string: the unit shows the message's other branch, and a
// translation of it comes back as that branch alone. kapi extract therefore
// leaves such a message out of an XLIFF or PO file, and kapi merge refuses a
// returned translation that holds none of the source's structures, rather
// than write a message with one form where the source has several. kapi
// translate translates each branch.

// warnStructures is the advice the extract warning and the merge refusal end
// with.
const warnStructures = "kapi translate translates each branch"

// flattensStructure reports whether target, a translation of source, holds
// none of the plurals and selects source holds.
func flattensStructure(source, target []model.Run) bool {
	return model.HasStructuredRuns(source) && !model.HasStructuredRuns(target)
}

// holdsStructure reports whether a translatable block's source holds a plural
// or a select.
func holdsStructure(b *model.Block) bool {
	return b.Translatable && model.HasStructuredRuns(b.SourceRuns())
}

// blockLabel names a block in a message: its key, else its id.
func blockLabel(b *model.Block) string {
	if b.Name != "" {
		return b.Name
	}
	return b.ID
}

// writeHeldWarning reports the messages extract left out of a bilingual file.
func writeHeldWarning(w io.Writer, source string, labels []string) {
	if len(labels) == 0 {
		return
	}
	fmt.Fprintf(w, "Warning: extract: %s: left out %d message(s) holding a plural or select (%s): "+
		"an XLIFF or PO unit carries one branch of each; %s\n", source, len(labels), strings.Join(labels, ", "), warnStructures)
}
