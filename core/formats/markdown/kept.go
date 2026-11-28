package markdown

import (
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
)

var _ format.ContentKeeper = (*Writer)(nil)

// edgeSpace is the white space no block of prose holds at an end of its
// content: Markdown reads a paragraph, a heading, a list item or a table cell
// without the spaces, tabs and line breaks around it.
const edgeSpace = " \t\r\n"

// proseTypes are the block types whose content holds no edgeSpace at its
// ends. The empty type is a paragraph, in a block quote and an admonition as
// elsewhere. jsx-text is the text child of a JSX element in MDX.
var proseTypes = map[string]bool{
	"":                            true,
	"heading":                     true,
	"list-item":                   true,
	"table-cell":                  true,
	"docusaurus-admonition-title": true,
	"jsx-text":                    true,
}

// KeptRuns implements format.ContentKeeper.
func (w *Writer) KeptRuns(b *model.Block, runs []model.Run) []model.Run {
	return KeptRuns(b, runs)
}

// KeptRuns returns runs as Markdown keeps them when they are the content of
// b: a block of prose (proseTypes) without the white space at its ends. Every
// other block is returned whole, as the content of a code block, an HTML
// block and a front matter value is kept. The MDX writer answers with it too:
// it reads its prose through the Markdown reader.
func KeptRuns(b *model.Block, runs []model.Run) []model.Run {
	if b == nil || !proseTypes[b.Type] {
		return runs
	}
	return format.TrimRunEdges(runs, edgeSpace)
}
