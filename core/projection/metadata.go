package projection

import "github.com/neokapi/neokapi/core/model"

// IsMetadata reports whether a block carries a value the format keeps about the
// document rather than a line of it: a drawing's non-visual properties (its
// object name, alt text and title, which the image run carries into the output
// on its own) and a document core property (dc:title, dc:creator, …). Readers
// emit these as "property" blocks so they can be edited and translated; the
// render tree and every generative writer leave them out of the flow. The test
// is on the reader's own discriminators rather than on the Type alone, because
// "property" also covers visible text (a VML textpath string, an
// mc:AlternateContent fallback).
func IsMetadata(b *model.Block) bool {
	if b.Type != "property" {
		return false
	}
	switch b.Properties["element"] {
	case "drawing-name", "drawing-descr", "drawing-title":
		return true
	}
	return b.Properties["partPath"] == "docProps/core.xml"
}
