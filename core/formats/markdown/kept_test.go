package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/model"
)

// keptDocument holds one block of every type the tests below name.
const keptDocument = `---
title: Front title
---

# Heading one

A paragraph with **bold** text.

- Item one

> Quoted paragraph.

| Head A | Head B |
| --- | --- |
| Cell one | Cell two |

` + "```go\ncode := 1\n```" + `

<div>Html block text</div>

:::note Title here

Admonition body.

:::
`

func keptBlocks(t *testing.T) []*model.Block {
	t.Helper()
	return readBlocksWithConfig(t, keptDocument, func(c *markdown.Config) {
		c.TranslateCodeBlocks, c.TranslateFrontMatter, c.TranslateHTMLBlocks = true, true, true
	})
}

// TestKeptRuns_ProseHoldsNoWhiteSpaceAtItsEnds pins what the Markdown writer
// declares it keeps (format.ContentKeeper): a block of prose without the
// spaces, tabs and line breaks at its ends, and every other block whole.
func TestKeptRuns_ProseHoldsNoWhiteSpaceAtItsEnds(t *testing.T) {
	w := markdown.NewWriter()
	padded := func(b *model.Block) []model.Run {
		runs := append([]model.Run{model.TextR(" \t")}, b.SourceRuns()...)
		return append(runs, model.TextR(" \r\n"))
	}
	seen := map[string]bool{}
	for _, b := range keptBlocks(t) {
		read := model.RunsEditText(b.SourceRuns())
		given := padded(b)
		got := model.RunsEditText(w.KeptRuns(b, given))
		switch b.Type {
		case "", "heading", "list-item", "table-cell", "docusaurus-admonition-title":
			assert.Equal(t, read, got, "a %q block holds no white space at its ends", b.Type)
		case "code-block", "html-block", "front-matter":
			assert.Equal(t, model.RunsEditText(given), got, "a %q block is kept whole", b.Type)
		default:
			t.Errorf("the document holds a block of type %q this test does not name", b.Type)
		}
		seen[b.Type] = true
	}
	for _, typ := range []string{"", "heading", "list-item", "table-cell", "docusaurus-admonition-title", "code-block", "html-block", "front-matter"} {
		assert.True(t, seen[typ], "the document holds no %q block", typ)
	}

	runs := []model.Run{model.TextR(" Hello ")}
	assert.Equal(t, " Hello ", model.RunsEditText(w.KeptRuns(nil, runs)), "content of no block the reader read is kept whole")
}

// TestKeptRuns_KeepsWhatTheReaderGives pins the other half of the
// declaration: content the reader gave is content Markdown keeps.
func TestKeptRuns_KeepsWhatTheReaderGives(t *testing.T) {
	blocks := keptBlocks(t)
	require.NotEmpty(t, blocks)
	for _, b := range blocks {
		assert.Equal(t, model.RunsEditText(b.SourceRuns()), model.RunsEditText(markdown.KeptRuns(b, b.SourceRuns())),
			"block %s (%q)", b.ID, b.Type)
	}
}

// TestKeptRuns_AWrittenEdgeReadsBackAsKept writes a paragraph, a heading and
// a list item with a space or a line break at an end and reads each back as
// KeptRuns says Markdown keeps it.
func TestKeptRuns_AWrittenEdgeReadsBackAsKept(t *testing.T) {
	const doc = "# Heading one\n\nA paragraph with **bold** text.\n\n- Item one\n"
	for _, edge := range []struct{ name, before, after string }{
		{"a space at the end", "", " "},
		{"a line break at the end", "", "\n"},
		{"a space at the start", " ", ""},
	} {
		t.Run(edge.name, func(t *testing.T) {
			var want []string
			out := roundtripTranslated(t, doc, "fr", func(blocks []*model.Block) {
				for _, b := range blocks {
					runs := append([]model.Run{model.TextR(edge.before)}, b.SourceRuns()...)
					runs = append(runs, model.TextR(edge.after))
					b.SetTargetRuns("fr", runs)
					want = append(want, model.RunsEditText(markdown.KeptRuns(b, runs)))
				}
			})
			back := readBlocks(t, out)
			require.Len(t, back, len(want), "%q", out)
			for i, b := range back {
				assert.Equal(t, want[i], model.RunsEditText(b.SourceRuns()), "%q", out)
			}
		})
	}
}
