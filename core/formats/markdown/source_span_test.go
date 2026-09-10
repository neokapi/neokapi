package markdown_test

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/markdown"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReaderSourceSpansLocateRepeatedUTF8Blocks(t *testing.T) {
	source := "\ufeff# Café\r\n\r\nRepeat **text**.\r\n\r\n- First\r\n  - Child\r\n\r\nRepeat **text**.\r\n\r\n## Code\r\n\r\n```go\r\nrun()\r\n```\r\n"
	blocks := readBlocksWithConfig(t, source, func(config *markdown.Config) { config.TranslateCodeBlocks = true })
	require.Len(t, blocks, 7)
	starts := []int{}
	for _, block := range blocks {
		span, ok := block.SourceSpan()
		require.True(t, ok, block.ID+": "+block.SourceText())
		require.Empty(t, span.Part)
		require.Greater(t, span.End, span.Start)
		require.LessOrEqual(t, span.End, len(source))
		raw := source[span.Start:span.End]
		assert.NotEmpty(t, strings.TrimSpace(raw))
		starts = append(starts, span.Start)
	}
	assert.IsIncreasing(t, starts)
	first, _ := blocks[0].SourceSpan()
	assert.Equal(t, "# Café", source[first.Start:first.End])
	second, _ := blocks[1].SourceSpan()
	repeated, _ := blocks[4].SourceSpan()
	assert.Equal(t, source[second.Start:second.End], source[repeated.Start:repeated.End])
	assert.NotEqual(t, second.Start, repeated.Start)
	code, _ := blocks[6].SourceSpan()
	assert.Contains(t, source[code.Start:code.End], "```go\r\nrun()")
}
