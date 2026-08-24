package mdx

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Prose inside a component used to be surfaced as non-translatable, so a
// <TabItem> body rendered in the source language on a fully translated page.
//
// Docusaurus has no extraction for MDX: its own documentation says Markdown and
// MDX files are translated as complete documents. So there is no per-component
// decision in its model, and no allowlist to keep. The W3C table answers
// instead, and an element it does not classify is a container, promoted when it
// holds direct text.

func TestJSXTextFollowsTheTable(t *testing.T) {
	tests := []struct {
		name         string
		src          string
		translatable bool
		promotes     bool
	}{
		{
			name:         "unclassified component is promoted",
			src:          "<TabItem value=\"a\">\n\nProse inside a component.\n\n</TabItem>\n",
			translatable: true,
			promotes:     true,
		},
		{
			name:         "a classified element needs no inference",
			src:          "<div>\n\n<p>Prose inside a paragraph.</p>\n\n</div>\n",
			translatable: true,
		},
		{
			name:         "a non-translatable element keeps its text",
			src:          "<div>\n\n<code>kapi check</code>\n\n</div>\n",
			translatable: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// readAll returns only the blocks offered to a translator, so an
			// empty result is exactly "nothing here is translatable".
			blocks, diags := readAll(t, tc.src)
			assert.Equal(t, tc.translatable, len(blocks) > 0)

			var promoted bool
			for _, d := range diags {
				if d.Category == "structure.jsx-promoted" {
					promoted = true
				}
			}
			assert.Equal(t, tc.promotes, promoted,
				"a promotion is an inference and has to be reported")
		})
	}
}

// The promotion names the element as the author spelled it: <TabItem>
// lowercased is a component nobody can find in their source.
func TestPromotionNamesTheElementAsWritten(t *testing.T) {
	_, diags := readAll(t, "<TabItem value=\"a\">\n\nProse.\n\n</TabItem>\n")
	require.NotEmpty(t, diags)

	var msg string
	for _, d := range diags {
		if d.Category == "structure.jsx-promoted" {
			msg = d.Message
		}
	}
	assert.Contains(t, msg, "<TabItem>")
	assert.Contains(t, msg, `translate="no"`, "the message has to name the way out")
}

// One report per element, however many paragraphs it holds.
func TestPromotionReportedOncePerElement(t *testing.T) {
	_, diags := readAll(t,
		"<TabItem value=\"a\">\n\nFirst paragraph.\n\nSecond paragraph.\n\n</TabItem>\n")

	n := 0
	for _, d := range diags {
		if d.Category == "structure.jsx-promoted" {
			n++
		}
	}
	assert.Equal(t, 1, n)
}
