//go:build integration

package okf_plaintext

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.plaintext.PlainTextFilter"
const mimeType = "text/plain"

func TestExtract_SimplePlainText(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"Hello world\nThis is a test.",
		"test.txt", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from plain text")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello world")
}
