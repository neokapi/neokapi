package editor

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// The preview writers below reproduce document markup verbatim: skeleton text
// is the source document's own HTML, and block content is rendered rather than
// escaped, because a preview that escaped the document would not be a preview
// of it. The output is therefore exactly as trustworthy as the document it came
// from — which, for an upload, a translator's target text, or content pulled
// through a connector from an external CMS, is not at all.
//
// That is a deliberate property of the format rather than a defect to fix here,
// and it puts an obligation on whoever serves the result: never as HTML on an
// origin that matters. Bowrain's preview routes send
// Content-Security-Policy: sandbox, and the editor embeds the markup through an
// iframe's srcDoc with sandbox="allow-scripts" and no allow-same-origin. A new
// consumer has to arrange something equivalent.

// buildHTMLPreview generates an HTML preview with <kat-block> markers.
// Uses skeleton data to reconstruct the document structure, wrapping each
// block's content in a <kat-block id="..."> element.
// BuildHTMLPreview generates an HTML preview with <kat-block> markers.
// Exported for use by the HTML format reader's PreviewBuilder implementation.
func BuildHTMLPreview(parts []*model.Part) string {
	var body strings.Builder

	for _, part := range parts {
		switch part.Type {
		case model.PartBlock:
			block, ok := part.Resource.(*model.Block)
			if !ok {
				continue
			}
			WriteHTMLBlockPreview(&body, block)

		case model.PartData:
			data, ok := part.Resource.(*model.Data)
			if !ok {
				continue
			}
			writeHTMLDataPreview(&body, data)
		}
	}

	return PreviewBoilerplateStart() + body.String() + PreviewBoilerplateEnd()
}

// WriteHTMLBlockPreview writes a single block's preview HTML.
// If the block has a fragment-based skeleton, the skeleton structure is preserved
// with the block content wrapped in <kat-block>.
// Exported for use by format reader PreviewBuilder implementations.
func WriteHTMLBlockPreview(buf *strings.Builder, block *model.Block) {
	content := RenderBlockContentHTML(block)

	if block.Skeleton != nil && block.Skeleton.Strategy == model.SkeletonFragmentBased {
		for _, sp := range block.Skeleton.Parts {
			switch p := sp.(type) {
			case *model.SkeletonText:
				buf.WriteString(p.Text)
			case *model.SkeletonRef:
				fmt.Fprintf(buf, `<kat-block id="%s">%s</kat-block>`, block.ID, content)
			}
		}
		return
	}

	// No skeleton: wrap content directly
	fmt.Fprintf(buf, `<kat-block id="%s">%s</kat-block>`, block.ID, content)
}

// writeHTMLDataPreview writes non-translatable data parts.
// We skip structural data like DOCTYPE in preview since the boilerplate handles that.
func writeHTMLDataPreview(buf *strings.Builder, data *model.Data) {
	// Data parts are non-translatable structure; skip in preview
}

// RenderBlockContentHTML renders a block's source content as HTML,
// expanding inline codes to their original markup.
// Exported for use by format reader PreviewBuilder implementations.
func RenderBlockContentHTML(block *model.Block) string {
	if len(block.Source) == 0 {
		return ""
	}
	return model.RenderRunsWithData(block.Source)
}
