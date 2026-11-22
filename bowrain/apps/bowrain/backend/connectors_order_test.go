package backend

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// ListConnectors answers in connector-ID order, the same on every call.
func TestListConnectorsIsSortedByID(t *testing.T) {
	app := NewAppWithoutPlugins()
	ids := []string{"order-7", "order-2", "order-5", "order-1", "order-8", "order-3", "order-6", "order-4"}
	for _, id := range ids {
		activeConnectors[id] = &fileConnector{id: id}
	}
	t.Cleanup(func() {
		for _, id := range ids {
			delete(activeConnectors, id)
		}
	})

	want := []string{"order-1", "order-2", "order-3", "order-4", "order-5", "order-6", "order-7", "order-8"}
	for range 20 {
		var got []string
		for _, c := range app.ListConnectors() {
			got = append(got, c.ID)
		}
		assert.Equal(t, want, got)
	}
}
