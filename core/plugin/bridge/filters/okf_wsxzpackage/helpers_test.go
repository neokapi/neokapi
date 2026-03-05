//go:build integration

package okf_wsxzpackage

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
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
