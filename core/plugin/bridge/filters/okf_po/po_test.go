//go:build integration

package okf_po

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.po.POFilter"
const mimeType = "application/x-gettext"

func TestExtract_SimplePO(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	po := `msgid "Hello World"
msgstr ""

msgid "Goodbye"
msgstr ""
`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		po, "test.po", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from PO")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello World")
	assert.Contains(t, texts, "Goodbye")
}
