package markdown_test

import (
	"context"
	"testing"

	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReaderReadsTabAfterBlockquoteMarker covers a blockquote marker followed
// by a tab and a fence character at the start of the document, an input
// goldmark 1.8.2 and earlier panicked on. It reads without error.
func TestReaderReadsTabAfterBlockquoteMarker(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	reader := markdown.NewReader()
	require.NoError(t, reader.Open(ctx, testutil.RawDocFromString("> \t`", model.LocaleEnglish)))
	defer reader.Close()
	var errs int
	for result := range reader.Read(ctx) {
		if result.Error != nil {
			errs++
		}
	}
	assert.Zero(t, errs, "the document reads without error")

	// The same bytes inside an indented code block are content, and the
	// rebuild path parses them while checking its output.
	assert.Equal(t, "```\n> \t`\n```\n", assertRebuildKeepsItsBlocks(t, "\t> \t`"))
}
