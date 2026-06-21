package editor

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// buildMarkdownPreview generates an HTML preview for Markdown content.
// It converts the Part stream into styled HTML with <kat-block> markers.
// BuildMarkdownPreview generates an HTML preview for Markdown content.
// Exported for use by the Markdown format reader's PreviewBuilder implementation.
func BuildMarkdownPreview(parts []*model.Part) string {
	var body strings.Builder

	for _, part := range parts {
		switch part.Type {
		case model.PartBlock:
			block, ok := part.Resource.(*model.Block)
			if !ok {
				continue
			}
			WriteMarkdownBlockPreview(&body, block)

		case model.PartData:
			data, ok := part.Resource.(*model.Data)
			if !ok {
				continue
			}
			WriteMarkdownDataPreview(&body, data)
		}
	}

	return PreviewBoilerplateStart() + body.String() + PreviewBoilerplateEnd()
}

// WriteMarkdownBlockPreview renders a Markdown block as HTML with <kat-block>.
// Exported for use by the Markdown format reader's PreviewBuilder implementation.
func WriteMarkdownBlockPreview(buf *strings.Builder, block *model.Block) {
	text := block.SourceText()
	blockType := block.Type
	level := block.Properties["level"]

	switch blockType {
	case "heading":
		tag := "h2"
		switch level {
		case "1":
			tag = "h1"
		case "2":
			tag = "h2"
		case "3":
			tag = "h3"
		case "4":
			tag = "h4"
		case "5":
			tag = "h5"
		case "6":
			tag = "h6"
		}
		fmt.Fprintf(buf, "<%s><kat-block id=\"%s\">%s</kat-block></%s>\n", tag, block.ID, text, tag)
	case "list-item":
		fmt.Fprintf(buf, "<li><kat-block id=\"%s\">%s</kat-block></li>\n", block.ID, text)
	default:
		fmt.Fprintf(buf, "<p><kat-block id=\"%s\">%s</kat-block></p>\n", block.ID, text)
	}
}

// WriteMarkdownDataPreview renders non-translatable Markdown data (code blocks, etc.).
// Exported for use by the Markdown format reader's PreviewBuilder implementation.
func WriteMarkdownDataPreview(buf *strings.Builder, data *model.Data) {
	dataType := data.Name
	content := data.Properties["content"]

	switch dataType {
	case "code-block":
		lang := data.Properties[model.PropCodeLanguage]
		if lang != "" {
			fmt.Fprintf(buf, "<pre><code class=\"language-%s\">%s</code></pre>\n", lang, content)
		} else {
			fmt.Fprintf(buf, "<pre><code>%s</code></pre>\n", content)
		}
	case "thematic-break":
		buf.WriteString("<hr>\n")
	case "html-block":
		buf.WriteString(content)
		buf.WriteString("\n")
	}
}
