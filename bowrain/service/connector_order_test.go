package service

import (
	"testing"

	venueconn "github.com/neokapi/neokapi/core/venue/connector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/connector"
)

// ListActive answers in connector-ID order, the same on every call; the
// workspace connector list in the API is built from it.
func TestListActiveIsSortedByID(t *testing.T) {
	reg := connector.NewRegistry()
	reg.Register("test", venueconn.CategoryFile, func(config map[string]string) (connector.IntegrationConnector, error) {
		return &testConnector{id: config["id"]}, nil
	})
	svc := NewConnectorService(newTestStore(t), reg)
	for _, id := range []string{"c-7", "c-2", "c-5", "c-1", "c-8", "c-3", "c-6", "c-4"} {
		_, err := svc.AddConnector("ws-a", "test", map[string]string{"id": id})
		require.NoError(t, err)
	}

	want := []string{"c-1", "c-2", "c-3", "c-4", "c-5", "c-6", "c-7", "c-8"}
	for range 20 {
		var got []string
		for _, c := range svc.ListActive("ws-a") {
			got = append(got, c.ID())
		}
		assert.Equal(t, want, got)
	}
}
