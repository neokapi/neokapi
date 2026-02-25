//go:build integration

package okf_xliff2

import (
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.xliff2.XLIFF2Filter"
const mimeType = "application/xliff+xml"

func TestExtract_SimpleXLIFF2(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	xliff2 := `<?xml version="1.0" encoding="UTF-8"?>
<xliff xmlns="urn:oasis:names:tc:xliff:document:2.0" version="2.0" srcLang="en" trgLang="fr">
  <file id="f1">
    <unit id="1">
      <segment>
        <source>Hello world</source>
      </segment>
    </unit>
  </file>
</xliff>`

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		xliff2, "test.xlf", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from XLIFF 2.0")
}
