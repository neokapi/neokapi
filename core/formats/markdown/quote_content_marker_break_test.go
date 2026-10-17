package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildOfAQuoteWhoseTextOpensWithAMarker covers a blockquote whose
// paragraph text begins with a ">" of its own: the parser reads a ">" after a
// bare carriage return as content. The sink took that ">" for a continuation
// prefix and held the hard break after it, spelling a backslash the pass after
// did not, so ">\r>\\\n>0" moved between its second and third rebuild.
func TestRebuildOfAQuoteWhoseTextOpensWithAMarker(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"marker then a backslash break", ">\r>\\\n>0"},
		{"marker then text", ">\r>a\n>b"},
		{"spaced marker then text", "> \r> a\n> b"},
		// A literal "<" in a quote body: the writer's parse of the body sees
		// the baked continuation marker as a nested quote unless it reads the
		// body inside the quote it will be written in.
		{"angle then a backslash break in a quote", "><\\\n>0"},
		{"angle then a soft break in a quote", "><\n>0"},
		// Controls: a marker on a continuation line is a prefix.
		{"break after a continuation marker", ">0\n>\r\\\n0"},
		{"carriage return then text", ">\ra\n>b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			assertRebuildKeepsItsBlocks(t, tc.input)
		})
	}
}

// TestRebuildOfAQuoteWhoseTextOpensWithAMarkerSpelling pins the rebuild: the
// content marker is escaped and the break is a soft one.
func TestRebuildOfAQuoteWhoseTextOpensWithAMarkerSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, ">\\>\n>0\n", assertRebuildKeepsItsBlocks(t, ">\r>\\\n>0"))
}
