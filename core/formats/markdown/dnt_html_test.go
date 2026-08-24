package markdown

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The other half of the scope: raw HTML. The element name decides, not the
// vocabulary id, because <code> and <span> both arrive as fmt:html.
func TestCodeishHTMLElementsProtectTheirText(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		protected string
		// after is text following the closing tag. It must be translatable
		// again, which is the assertion that catches a depth counter that
		// never comes back down.
		after string
	}{
		{name: "code", src: "Run <code>kapi check</code> now.\n", protected: "kapi check", after: " now."},
		{name: "kbd", src: "Press <kbd>Ctrl+C</kbd> now.\n", protected: "Ctrl+C", after: " now."},
		{name: "samp", src: "See <samp>exit 3</samp> now.\n", protected: "exit 3", after: " now."},
		{name: "var", src: "Set <var>PATH</var> now.\n", protected: "PATH", after: " now."},
		// A span is not a code element, whatever its class says.
		{name: "span", src: "Run <span class=\"code\">kapi check</span> now.\n"},
		{name: "bold", src: "Run <b>kapi check</b> now.\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			blocks := blocksOf(t, tc.src)
			require.NotEmpty(t, blocks)
			translatable, protected := runText(blocks[0].Source)

			if tc.protected == "" {
				assert.Empty(t, protected, "%s is not a code element", tc.name)
				assert.Contains(t, translatable, "kapi check")
				return
			}
			assert.Equal(t, tc.protected, protected)
			assert.Contains(t, translatable, tc.after,
				"text after the closing tag must be translatable again")
		})
	}
}

// Nesting has to unwind, or one <code> leaves the rest of the paragraph
// protected and the page stops being translated after it.
func TestNestedCodeishElementsUnwind(t *testing.T) {
	blocks := blocksOf(t, "A <code>one <var>two</var> three</code> and then prose.\n")
	require.NotEmpty(t, blocks)

	translatable, protected := runText(blocks[0].Source)
	assert.Contains(t, protected, "one ")
	assert.Contains(t, protected, "two")
	assert.Contains(t, translatable, " and then prose.")
}
