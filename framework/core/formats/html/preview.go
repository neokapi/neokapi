package html

import (
	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

// Ensure Reader implements PreviewBuilder.
var _ format.PreviewBuilder = (*Reader)(nil)

// BuildPreview generates an HTML preview with <kat-block> markers.
// Delegates to the editor package's shared HTML preview builder.
func (r *Reader) BuildPreview(parts []*model.Part) string {
	return editor.BuildHTMLPreview(parts)
}
