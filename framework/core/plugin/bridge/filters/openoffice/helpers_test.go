//go:build integration

package openoffice

import (
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
)

const openOfficeFilterClass = "net.sf.okapi.filters.openoffice.OpenOfficeFilter"
const openOfficeMimeType = "application/vnd.oasis.opendocument"

const odfFilterClass = "net.sf.okapi.filters.openoffice.ODFFilter"
const odfMimeType = "application/vnd.oasis.opendocument.text"

// readODFFile reads an ODF XML file from testdata and returns parts.
func readODFFile(t *testing.T, relPath string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, odfFilterClass)
	path := bridgetest.TestdataFile(t, relPath)
	return bridgetest.ReadFile(t, pool, cfg, odfFilterClass, path, odfMimeType, filterParams)
}

// readOpenOfficeFile reads an OpenOffice archive file (odt, ods, odp, odg)
// from testdata and returns parts.
func readOpenOfficeFile(t *testing.T, relPath string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, openOfficeFilterClass)
	path := bridgetest.TestdataFile(t, relPath)
	return bridgetest.ReadFile(t, pool, cfg, openOfficeFilterClass, path, openOfficeMimeType, filterParams)
}

// allBlocks returns all blocks (translatable and non-translatable) from parts.
func allBlocks(parts []*model.Part) []*model.Block {
	return bridgetest.FilterBlocks(parts)
}

// containsAll checks if s contains all substrings.
func containsAll(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}
