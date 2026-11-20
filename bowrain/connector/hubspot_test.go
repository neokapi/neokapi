package connector

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	platform "github.com/neokapi/neokapi/bowrain/core/connector"
	venueconn "github.com/neokapi/neokapi/core/venue/connector"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fixtureEndpoint http.HandlerFunc

func (f fixtureEndpoint) RoundTrip(r *http.Request) (*http.Response, error) {
	w := httptest.NewRecorder()
	f(w, r)
	return w.Result(), nil
}

func TestHubSpotExchange(t *testing.T) {
	c, err := NewHubSpotConnector(map[string]string{"api_key": "test-key"})
	require.NoError(t, err)
	var update map[string]string
	mux := http.NewServeMux()
	mux.HandleFunc("GET /cms/v3/pages/site-pages", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "api.hubapi.com", r.URL.Host)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		_, _ = w.Write([]byte(`{"results":[{"id":"123","name":"Home","slug":"home","htmlTitle":"Hello","metaDescription":"Welcome"}]}`))
	})
	mux.HandleFunc("PATCH /cms/v3/pages/site-pages/123", func(_ http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewDecoder(r.Body).Decode(&update))
	})
	c.client = &http.Client{Transport: fixtureEndpoint(mux.ServeHTTP)}
	items, err := c.Fetch(t.Context(), platform.FetchOptions{})
	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Equal(t, "pages/home", items[0].Path)
	require.Len(t, items[0].Blocks, 2)
	require.Equal(t, "Hello", items[0].Blocks[0].SourceText())
	items[0].Blocks[0] = deliveryCopy(items[0].Blocks[0], "Bonjour")
	require.NoError(t, c.Publish(t.Context(), items, platform.PublishOptions{}))
	require.Equal(t, map[string]string{"htmlTitle": "Bonjour", "metaDescription": "Welcome"}, update)
}

func TestHubSpotCategory(t *testing.T) {
	c := &HubSpotConnector{id: "test", config: map[string]string{}}
	assert.Equal(t, venueconn.CategoryMarketing, c.Category())
}
