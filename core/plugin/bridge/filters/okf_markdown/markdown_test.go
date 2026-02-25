//go:build integration

package okf_markdown

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.markdown.MarkdownFilter"
const mimeType = "text/markdown"

func TestExtract_SimpleMarkdown(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	md := "# Hello World\n\nThis is a paragraph.\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		md, "test.md", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from Markdown")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello World")
}
