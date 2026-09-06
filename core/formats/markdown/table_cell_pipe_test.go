package markdown_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestTableCellEscapedPipeIsAFixedPoint is the #2501 reproducer. The rebuild
// path escaped every pipe in a cell, including one the source had already
// escaped, so the cell gained a backslash on every pass and its content key
// drifted with it.
func TestTableCellEscapedPipeIsAFixedPoint(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name    string
		input   string
		written string
	}{
		{
			"escaped pipe in a body cell",
			"x | y\n--- | ---\na \\| b | c\n",
			"| x | y |\n| --- | --- |\n| a \\| b | c |\n",
		},
		{
			"escaped pipe in a header cell",
			"a \\| b | y\n--- | ---\nc | d\n",
			"| a \\| b | y |\n| --- | --- |\n| c | d |\n",
		},
		{
			"trailing escaped pipe",
			"a | b\n--- | ---\nc \\| | d\n",
			"| a | b |\n| --- | --- |\n| c \\| | d |\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			out := roundtrip(t, tc.input)
			assert.Equal(t, tc.written, out)
			assert.Equal(t, out, roundtrip(t, out), "the rebuild is not idempotent")
			assert.Equal(t, out, roundtrip(t, roundtrip(t, out)), "the third pass drifted")
		})
	}
}
