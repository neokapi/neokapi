package markdown_test

import (
	"testing"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/stretchr/testify/require"
)

// readBlockTexts reads input through the reader and returns each block's text.
func readBlockTexts(t *testing.T, input string) []string {
	t.Helper()
	parts, hadErr := fuzzReadMarkdown(t.Context(), []byte(input))
	require.False(t, hadErr, "the reader declined %q", input)
	var texts []string
	for _, b := range testutil.FilterBlocks(parts) {
		texts = append(texts, b.SourceText())
	}
	return texts
}
