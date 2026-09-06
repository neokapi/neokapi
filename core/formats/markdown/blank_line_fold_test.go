package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuiltBlockKeepsItsBlankLinesOut is the #2503 reproducer. An inline
// construct on a line of its own is dropped by the rebuild path, which is
// accepted lossiness, and the empty line it left behind ended the paragraph:
// "a\n<a>\na" came back as "a\n\na", which re-reads as two blocks.
func TestRebuiltBlockKeepsItsBlankLinesOut(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		written string
	}{
		{"paragraph", "a\n<a>\na", "a\na\n"},
		{"paragraph opening with punctuation", ":\n<a>\na", ":\na\n"},
		{"list item", "- a\n<a>\nb", "- a\nb\n"},
		{"blockquote", "> a\n<a>\nb", "> a\nb\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := roundtrip(t, tc.input)
			assert.Equal(t, tc.written, out)
			require.Len(t, readBlocks(t, out), 1, "the rebuilt block did not stay one block")
			assert.Equal(t, out, roundtrip(t, out), "the rebuild is not idempotent")
		})
	}
}
