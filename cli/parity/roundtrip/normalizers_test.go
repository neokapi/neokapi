//go:build parity

package roundtrip_test

import (
	"testing"

	"github.com/neokapi/neokapi/cli/parity/roundtrip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMarkdownCanonicalFoldsTableCellPadding pins the table-row rule: the
// source's padding around each pipe (which native replays) and Okapi's
// single-space rewrite of it canonicalise to the same bytes, while a row
// that lost a cell still differs.
func TestMarkdownCanonicalFoldsTableCellPadding(t *testing.T) {
	norm := func(s string) string {
		out, err := roundtrip.MarkdownCanonical{}.Normalize([]byte(s))
		require.NoError(t, err)
		return string(out)
	}
	padded := "| Option | Description |\n| ------ | ----------- |\n| data   | path to data files. |\n| ext    | extension. |\n"
	okapi := "| Option | Description |\n| ------ | ----------- |\n| data | path to data files. |\n| ext | extension. |\n"
	compact := "|Option|Description|\n|------|-----------|\n|data|path to data files.|\n|ext|extension.|\n"
	assert.Equal(t, norm(okapi), norm(padded), "source padding and Okapi's rewrite are the same table")
	assert.Equal(t, norm(okapi), norm(compact), "compact rows are the same table")
	assert.NotEqual(t, norm(okapi), norm("| Option | Description |\n| ------ | ----------- |\n| data |\n| ext | extension. |\n"),
		"a lost cell is still a difference")

	// A row inside a list item keeps its indent atom.
	assert.Equal(t, "\t| a | b |", norm("  |  a  |  b  |"))
}
