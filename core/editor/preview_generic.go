package editor

import (
	"fmt"
	"html"
	"strings"

	"github.com/gokapi/gokapi/core/model"
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

	return previewBoilerplateStart() + body.String() + previewBoilerplateEnd()
}
