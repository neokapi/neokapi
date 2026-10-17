package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRebuildEscapesADelimiterRowBehindACarriageReturn covers a paragraph whose
// continuation line is a bare carriage return and a dash. The parser keeps the
// "\r" inside the line as whitespace, so the line is a GFM table delimiter row
// and the line above it a header; the writer's check for such a row counted
// spaces alone, and "00\n\r-\\\n0" came back as a one-cell table once the hard
// break after the dash was spelled as a soft one.
func TestRebuildEscapesADelimiterRowBehindACarriageReturn(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		input string
	}{
		{"dash after a carriage return", "00\n\r-\\\n0"},
		{"dash after two carriage returns", "00\n\r\r-\\\n0"},
		{"three dashes after a carriage return", "0\n\r---\\\n0"},
		{"in a blockquote", ">0\n>\r-\\\n>0"},
		{"carriage return and three spaces before the dash", "0\n\r   -\\\n0"},
		{"carriage return and three spaces before an equals sign", "0\n\r   =\\\n0"},
		{"three spaces and a carriage return before the dash", "0\n   \r-\\\n0"},
		// The parser counts the indent up to the carriage return, so the
		// spaces after it do not keep the line from being a row.
		{"carriage return and four spaces before the dash", "0\n\r    -\\\n0"},
		// Controls: an equals sign there is no row, and a heading marker is
		// escaped already.
		{"equals sign after a carriage return", "00\n\r=\\\n0"},
		{"hash after a carriage return", "0\n\r#\\\n0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.input, roundtripWithSkeleton(t, tc.input), "skeleton path is not byte-exact")
			out := assertRebuildKeepsItsBlocks(t, tc.input)
			assert.Len(t, readBlockTexts(t, out), 1)
		})
	}
}

// TestRebuildEscapesADelimiterRowBehindACarriageReturnSpelling pins the escape.
func TestRebuildEscapesADelimiterRowBehindACarriageReturnSpelling(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "00\n\r\\-\n0\n", assertRebuildKeepsItsBlocks(t, "00\n\r-\\\n0"))
}
