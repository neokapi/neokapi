package mdx

import (
	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Ensure Reader implements PreviewBuilder.
var _ format.PreviewBuilder = (*Reader)(nil)

// BuildPreview generates an HTML preview for MDX content. MDX is Markdown
// with embedded JSX/ESM, so the shared Markdown preview builder gives a
// reasonable rendering of the translatable prose; opaque MDX regions
// (Data parts) contribute no preview text.
func (r *Reader) BuildPreview(parts []*model.Part) string {
	return editor.BuildMarkdownPreview(parts)
}
