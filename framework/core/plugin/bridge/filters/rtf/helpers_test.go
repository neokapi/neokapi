//go:build integration

package rtf

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

const filterClass = "net.sf.okapi.filters.rtf.RTFFilter"
const mimeType = "application/rtf"

// readRTFFile reads an RTF file from testdata and returns parts.
func readRTFFile(t *testing.T, relPath string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	return bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, filterParams)
}

// readRTFSnippet parses an RTF snippet string and returns the parts.
func readRTFSnippet(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.rtf", mimeType, filterParams)
}
