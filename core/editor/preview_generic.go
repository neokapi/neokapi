package editor

import (
	"encoding/json"
	"fmt"
	"html"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// buildGenericPreview generates a basic fallback preview for formats
// without dedicated preview builders. Each block is rendered as a
// styled paragraph with <kat-block> markers.
func buildGenericPreview(parts []*model.Part) string {
	var body strings.Builder

	body.WriteString(`<div style="font-family: monospace; font-size: 13px;">`)

	for _, part := range parts {
		if part.Type != model.PartBlock {
			continue
		}
		block, ok := part.Resource.(*model.Block)
		if !ok {
			continue
		}

		text := html.EscapeString(block.SourceText())
		fmt.Fprintf(&body,
			`<p style="margin: 4px 0; padding: 4px 8px;"><kat-block id="%s">%s</kat-block></p>`+"\n",
			block.ID, text)
	}

	body.WriteString(`</div>`)

	return PreviewBoilerplateStart() + body.String() + PreviewBoilerplateEnd()
}

// BuildPreviewFromBlockIndex generates a default preview from a stored BlockIndex
// JSON string. This is the server-side fallback when no PreviewBuilder-generated
// PreviewHTML is available. It uses DocumentOrder to maintain document structure
// and renders blocks via SourceHTML.
func BuildPreviewFromBlockIndex(blockIndexJSON string) string {
	var index BlockIndex
	if err := json.Unmarshal([]byte(blockIndexJSON), &index); err != nil {
		return ""
	}

	// Build lookup maps.
	blockMap := make(map[string]*Block, len(index.Blocks))
	for i := range index.Blocks {
		blockMap[index.Blocks[i].ID] = &index.Blocks[i]
	}
	dataMap := make(map[string]*DataPart, len(index.DataParts))
	for i := range index.DataParts {
		dataMap[index.DataParts[i].ID] = &index.DataParts[i]
	}

	var body strings.Builder
	body.WriteString(`<div style="font-family: monospace; font-size: 13px;">`)

	for _, ref := range index.DocumentOrder {
		kind, id, ok := strings.Cut(ref, ":")
		if !ok {
			continue
		}

		switch kind {
		case "block":
			b := blockMap[id]
			if b == nil {
				continue
			}
			content := b.SourceHTML
			if content == "" {
				content = html.EscapeString(b.Source)
			}
			if b.Skeleton != nil && b.Skeleton.Strategy == "fragment" {
				for _, sp := range b.Skeleton.Parts {
					switch sp.Type {
					case "text":
						body.WriteString(sp.Text)
					case "ref":
						fmt.Fprintf(&body, `<kat-block id="%s">%s</kat-block>`, b.ID, content)
					}
				}
			} else {
				fmt.Fprintf(&body,
					`<p style="margin: 4px 0; padding: 4px 8px;"><kat-block id="%s">%s</kat-block></p>`+"\n",
					b.ID, content)
			}

		case "data":
			dp := dataMap[id]
			if dp == nil {
				continue
			}
			// Render data parts that have meaningful content.
			if content, ok := dp.Properties["content"]; ok && content != "" {
				switch dp.Name {
				case "code-block":
					lang := dp.Properties[model.PropCodeLanguage]
					if lang != "" {
						fmt.Fprintf(&body, "<pre><code class=\"language-%s\">%s</code></pre>\n", lang, content)
					} else {
						fmt.Fprintf(&body, "<pre><code>%s</code></pre>\n", content)
					}
				case "thematic-break":
					body.WriteString("<hr>\n")
				case "html-block":
					body.WriteString(content)
					body.WriteString("\n")
				}
			} else if dp.Name == "thematic-break" {
				body.WriteString("<hr>\n")
			}
		}
	}

	// If no DocumentOrder, fall back to listing blocks in index order.
	if len(index.DocumentOrder) == 0 {
		for _, b := range index.Blocks {
			content := b.SourceHTML
			if content == "" {
				content = html.EscapeString(b.Source)
			}
			fmt.Fprintf(&body,
				`<p style="margin: 4px 0; padding: 4px 8px;"><kat-block id="%s">%s</kat-block></p>`+"\n",
				b.ID, content)
		}
	}

	body.WriteString(`</div>`)
	return PreviewBoilerplateStart() + body.String() + PreviewBoilerplateEnd()
}
