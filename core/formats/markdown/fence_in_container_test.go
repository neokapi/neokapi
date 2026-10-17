package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/model"
)

// TestFenceInAContainerKeepsItsLines is the #3203 reproducer. goldmark's line
// segments leave out the prefix a container strips from each line, and the
// skeleton refers to the fence's content for the whole body, so every content
// line after the first came back without its list item's indent or its
// blockquote's marker. A fence that is never closed runs to the end of its
// container, and the reader took the line after it for the closing fence, so
// a paragraph's first line was written twice.
func TestFenceInAContainerKeepsItsLines(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"unterminated fence after a blank line", "- ```\n  \n  code\n"},
		{"unterminated fence after an empty line", "- ```\n\n  text\n"},
		{"unterminated fence then a paragraph", "- ```\n  \n\\]</b>"},
		{"unterminated fence with two lines", "- ```\n  code\n  more\n"},
		{"unterminated fence then a blank line and a paragraph", "- ```\n  code\n\ntext\n"},
		{"unterminated fence then a fence of its own", "- ```\n  code\n\n```\nx\n```\n"},
		{"closed fence with two lines", "- ```\n  code\n  more\n  ```\n"},
		{"closed fence with a blank line", "- ```\n  \n  code\n  ```\n"},
		{"closed fence with a language", "- ```js\n  const a = 1;\n\n  const b = 2;\n  ```\n"},
		{"ordered item", "1. ```\n   a\n   b\n   ```\n"},
		{"nested item", "- a\n  - ```\n    x\n    y\n    ```\n"},
		{"fence on the item's second line", "- Item\n\n  ```\n  a\n  b\n  ```\n\n- Next\n"},
		{"tab-indented item", "-\t```\n\ta\n\tb\n\t```\n"},
		{"blockquote with a blank line", "> ```\n> \n> code\n"},
		{"blockquote closed", "> ```\n> a\n> b\n> ```\n"},
		{"blockquote unterminated then a paragraph", "> ```\n> a\n\nafter\n"},
		{"indented fence at the top level", " ```\n  a\n  b\n ```\n"},
		{"closer indented inside the item", "- ```\n  code\n    ```\n"},
		{"closer longer than the opener", "- ```\n  code\n  `````\n"},
		{"tilde fence in an item", "- ~~~\n  a\n  b\n  ~~~\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			translated := roundtripWithSkeletonConfig(t, tc.input, func(c *markdown.Config) { c.TranslateCodeBlocks = true })
			require.Equal(t, tc.input, translated, "skeleton path is not byte-exact with translated code blocks")
		})
	}
}

// The line after an unterminated fence is the paragraph's, so it is read once:
// as that paragraph's block, and never as the fence's closer.
func TestUnterminatedFenceLeavesTheNextLineToItsBlock(t *testing.T) {
	t.Parallel()
	blocks := readBlocks(t, "- ```\n  \n\\]</b>")
	require.Len(t, blocks, 2)
	require.Equal(t, "code-block", blocks[0].Type)
	require.Equal(t, "\n", blocks[0].SourceText())
	require.Equal(t, "\\]</b>", model.RenderRunsWithData(blocks[1].SourceRuns()))
}
