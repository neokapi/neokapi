package editor

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

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
