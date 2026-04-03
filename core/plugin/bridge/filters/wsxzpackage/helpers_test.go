//go:build integration

package wsxzpackage

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
)

const filterClass = "net.sf.okapi.filters.wsxzpackage.WsxzPackageFilter"
const mimeType = "application/x-wsxz"

func readFile(t *testing.T, relPath string) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadFile(t, pool, cfg, filterClass,
		bridgetest.TestdataFile(t, relPath), mimeType, nil)
}

func countPartsByType(parts []*model.Part, pt model.PartType) int {
	n := 0
	for _, p := range parts {
		if p.Type == pt {
			n++
		}
	}
	return n
}
