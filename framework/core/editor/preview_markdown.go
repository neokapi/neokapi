package editor

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// buildMarkdownPreview generates an HTML preview for Markdown content.
// It converts the Part stream into styled HTML with <kat-block> markers.
func buildMarkdownPreview(parts []*model.Part, sourceBytes []byte) string {
	var body strings.Builder

	for _, part := range parts {
		switch part.Type {
		case model.PartBlock:
			block, ok := part.Resource.(*model.Block)
			if !ok {
				continue
			}
			writeMarkdownBlockPreview(&body, block)

		case model.PartData:
			data, ok := part.Resource.(*model.Data)
			if !ok {
				continue
			}
			writeMarkdownDataPreview(&body, data)
		}
	}

	return previewBoilerplateStart() + body.String() + previewBoilerplateEnd()
}

// writeMarkdownBlockPreview renders a Markdown block as HTML with <kat-block>.
func writeMarkdownBlockPreview(buf *strings.Builder, block *model.Block) {
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

// writeMarkdownDataPreview renders non-translatable Markdown data (code blocks, etc.).
func writeMarkdownDataPreview(buf *strings.Builder, data *model.Data) {
	dataType := data.Name
	content := data.Properties["content"]

	switch dataType {
	case "code-block":
		lang := data.Properties["language"]
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
