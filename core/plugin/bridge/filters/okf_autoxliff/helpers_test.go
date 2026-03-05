//go:build integration

package okf_autoxliff

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

const filterClass = "net.sf.okapi.filters.autoxliff.AutoXLIFFFilter"
const mimeType = "application/xliff+xml"

// readAutoXLIFFFile reads an XLIFF file from testdata with the given filter params.
func readAutoXLIFFFile(t *testing.T, relPath string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	return bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, filterParams)
}
