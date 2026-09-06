package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBlockquoteMarkerLineKeepsItsSpace is the #2463 reproducer. A blockquote
// line that carries the marker and nothing else came back with the space after
// the ">" gone: the writer's per-line trim mirrors okapi's, which drops a
// single trailing space, and "> " looks like a word with one. CommonMark 5.1
// spells a blockquote marker as ">" plus an optional space, so that space is
// part of the marker.
func TestBlockquoteMarkerLineKeepsItsSpace(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"empty marker line first", "> \n> quoted after an empty marker\n"},
		{"empty marker line between paragraphs", "> quoted\n> \n> and more\n"},
		{"nested empty marker line", ">> \n>> deeper after an empty marker\n"},
		{"spaced nested marker line", "> > \n> > spaced deeper\n"},
		{"marker with no space", ">\n> no space on the marker line\n"},
		{"indented marker line", " > \n > indented quote\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
		})
	}
}

// TestTrailingWhitespaceSurvives covers the general rule #2463 was one case of:
// the writer reproduces the source's own bytes, trailing whitespace included
// (#2496). Two or more spaces spell a hard break and always stayed.
func TestTrailingWhitespaceSurvives(t *testing.T) {
	t.Parallel()
	for _, input := range []string{
		"a \nb\n",
		"a  \nb\n",
		"> a \n> b\n",
		"- \n",
		"* ",
		"*\t",
		"a\n\n# \n\nb\n",
		" \n0)",
		"a \n\nb \n",
	} {
		assert.Equal(t, input, roundtripWithSkeleton(t, input), "skeleton path is not byte-exact")
	}
}
