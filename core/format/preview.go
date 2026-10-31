package format

import "github.com/neokapi/neokapi/core/model"

// PreviewBuilder is an optional interface that format readers can implement
// to generate a visual HTML preview of a parsed document. The preview HTML
// contains <kat-block id="..."> markers for interactive block selection in
// the editor. A marker's id is its block's ID: editor.BuildPreview passes each
// block with its durable key as its ID, the key a venue stores the block under.
//
// Readers that do not implement this interface will get a generic fallback
// preview generated from the BlockIndex.
type PreviewBuilder interface {
	// BuildPreview generates a complete HTML document with <kat-block> markers
	// from the given parts. The returned HTML includes CSS and JavaScript
	// boilerplate for the editor iframe.
	BuildPreview(parts []*model.Part) string
}
