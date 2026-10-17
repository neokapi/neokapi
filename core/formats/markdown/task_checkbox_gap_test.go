package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// TestTaskCheckboxFollowedByALineEndingOrFormFeed covers a task list checkbox
// whose text begins on the next line, or is followed by a form feed. The
// checkbox's spelling is read from the bytes up to the text after it, and the
// guard on that splice accepted spaces and tabs alone, so a line ending, a
// continuation prefix or a form feed in the gap fell back to the canonical
// "[ ] " and the bytes were lost on the skeleton path.
func TestTaskCheckboxFollowedByALineEndingOrFormFeed(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"text on the next line", "* [ ]\n\\"},
		{"checked, text on the next line", "- [x]\n\\"},
		{"text on an indented next line", "- [ ]\n  \\"},
		{"text on the next line of a quote", "> - [ ]\n> \\"},
		{"hard break then text", "- [ ]  \nx"},
		{"form feed at the end", "- [ ]\f"},
		{"form feed before the text", "- [ ]\fa"},
		{"tab and form feed before the text", "- [ ]\t\fa"},
		{"form feed before a blank line", "- [ ]\f\n\nx"},
		// Controls.
		{"text on the same line", "- [ ] a"},
		{"two items", "- [X] a\n- [ ] b\n"},
		{"checkbox alone", "- [ ]"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
}
