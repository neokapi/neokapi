package editor

import (
	"fmt"
	"strings"

	"github.com/gokapi/gokapi/core/model"
)

// buildHTMLPreview generates an HTML preview with <kat-block> markers.
// Uses skeleton data to reconstruct the document structure, wrapping each
// block's content in a <kat-block id="..."> element.
func buildHTMLPreview(parts []*model.Part, sourceBytes []byte) string {
	var body strings.Builder

	for _, part := range parts {
		switch part.Type {
		case model.PartBlock:
			block, ok := part.Resource.(*model.Block)
			if !ok {
				continue
			}
			writeHTMLBlockPreview(&body, block)

		case model.PartData:
			data, ok := part.Resource.(*model.Data)
			if !ok {
				continue
			}
			writeHTMLDataPreview(&body, data)
		}
	}

	return previewBoilerplateStart() + body.String() + previewBoilerplateEnd()
}

// writeHTMLBlockPreview writes a single block's preview HTML.
// If the block has a fragment-based skeleton, the skeleton structure is preserved
// with the block content wrapped in <kat-block>.
func writeHTMLBlockPreview(buf *strings.Builder, block *model.Block) {
	content := renderBlockContentHTML(block)

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

// renderBlockContentHTML renders a block's source content as HTML,
// expanding inline span markers to their original markup.
func renderBlockContentHTML(block *model.Block) string {
	if len(block.Source) == 0 {
		return ""
	}

	var buf strings.Builder
	for _, seg := range block.Source {
		renderFragmentToHTML(&buf, seg.Content)
	}
	return buf.String()
}

// renderFragmentToHTML renders a Fragment to HTML, replacing Unicode markers
// with their corresponding span markup.
func renderFragmentToHTML(buf *strings.Builder, frag *model.Fragment) {
	if frag == nil {
		return
	}
	if !frag.HasSpans() {
		buf.WriteString(frag.CodedText)
		return
	}

	spanIdx := 0
	for _, r := range frag.CodedText {
		switch r {
		case model.MarkerOpening, model.MarkerClosing, model.MarkerPlaceholder:
			if spanIdx < len(frag.Spans) {
				buf.WriteString(frag.Spans[spanIdx].Data)
				spanIdx++
			}
		default:
			buf.WriteRune(r)
		}
	}
}
