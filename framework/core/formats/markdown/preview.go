package markdown

import (
	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Ensure Reader implements PreviewBuilder.
var _ format.PreviewBuilder = (*Reader)(nil)

// BuildPreview generates an HTML preview for Markdown content.
// Delegates to the editor package's shared Markdown preview builder.
func (r *Reader) BuildPreview(parts []*model.Part) string {
	return editor.BuildMarkdownPreview(parts)
}
