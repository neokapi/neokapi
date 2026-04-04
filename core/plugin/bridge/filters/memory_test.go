//go:build integration

package filters

import (
	"runtime"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
)

// memoryTestEntry defines a filter to stress-test for memory leaks.
type memoryTestEntry struct {
	name        string
	filterClass string
	content     string
	uri         string
	mimeType    string
}

// memoryTestFilters lists filters to include in memory stress testing.
// Entries are added as each filter's tests are implemented.
var memoryTestFilters = []memoryTestEntry{
	{
		name:        "html",
		filterClass: "net.sf.okapi.filters.html.HtmlFilter",
		content:     "<html><body><p>Hello world</p></body></html>",
		uri:         "test.html",
		mimeType:    "text/html",
	},
	{
		name:        "json",
		filterClass: "net.sf.okapi.filters.json.JSONFilter",
		content:     `{"greeting": "Hello World"}`,
		uri:         "test.json",
		mimeType:    "application/json",
	},
	{
		name:        "properties",
		filterClass: "net.sf.okapi.filters.properties.PropertiesFilter",
		content:     "greeting=Hello World\nfarewell=Goodbye\n",
		uri:         "test.properties",
		mimeType:    "text/x-java-properties",
	},
}

// TestMemoryStress runs each filter through many extraction iterations and
// checks that memory growth stays within reasonable bounds. This catches
// JVM-side resource leaks in the bridge protocol.
func TestMemoryStress(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	const iterations = 50

	for _, entry := range memoryTestFilters {
		t.Run(entry.name, func(t *testing.T) {
			// Force GC and record baseline.
			runtime.GC()
			var baseline runtime.MemStats
			runtime.ReadMemStats(&baseline)

			for range iterations {
				parts := bridgetest.ReadString(t, pool, cfg,
					entry.filterClass, entry.content, entry.uri, entry.mimeType, nil)
				_ = parts
			}

			// Force GC and measure growth.
			runtime.GC()
			var after runtime.MemStats
			runtime.ReadMemStats(&after)

			// Allow up to 50MB growth over iterations — this is a sanity check
			// for catastrophic leaks, not a precision measurement.
			// Use signed arithmetic since HeapAlloc can decrease after GC.
			var growth int64
			if after.HeapAlloc >= baseline.HeapAlloc {
				growth = int64(after.HeapAlloc - baseline.HeapAlloc)
			} else {
				growth = -int64(baseline.HeapAlloc - after.HeapAlloc)
			}
			const maxGrowthBytes int64 = 50 * 1024 * 1024
			assert.LessOrEqual(t, growth, maxGrowthBytes,
				"memory growth of %d bytes over %d iterations exceeds %d byte threshold",
				growth, iterations, maxGrowthBytes)
		})
	}
}
