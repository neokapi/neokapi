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

// TestReaderReportsAParserFailure covers a document the parser itself fails
// on: goldmark indexes before the line when a blockquote marker is followed
// by a tab and a fence character at the start of the document. The reader
// reports the failure as a read error, and the rebuild path's own parse of
// its output treats the failure as nothing formed.
func TestReaderReportsAParserFailure(t *testing.T) {
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
	assert.Equal(t, 1, errs, "the parser failure is reported as one read error")

	// The same bytes inside an indented code block are content, and the
	// rebuild path parses them while checking its output.
	assert.Equal(t, "```\n> \t`\n```\n", assertRebuildKeepsItsBlocks(t, "\t> \t`"))
}
