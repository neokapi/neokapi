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

func hubSpotFixture(t *testing.T, update *map[string]string) http.RoundTripper {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /cms/v3/pages/site-pages", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "api.hubapi.com", r.URL.Host)
		if r.Header.Get("Authorization") != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_, _ = w.Write([]byte(`{"results":[` +
			`{"id":"123","name":"Home","slug":"home","htmlTitle":"Hello","metaDescription":"Welcome"},` +
			`{"id":"456","name":"About","slug":"about","htmlTitle":"About us"}]}`))
	})
	mux.HandleFunc("/cms/v3/pages/site-pages/123", func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPatch, r.Method)
		assert.Equal(t, "Bearer test-key", r.Header.Get("Authorization"))
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))
		assert.NoError(t, json.NewDecoder(r.Body).Decode(update))
	})
	return fixtureEndpoint(mux.ServeHTTP)
}

func TestHubSpotExchange(t *testing.T) {
	c, err := NewHubSpotConnector(map[string]string{"api_key": "test-key"})
	require.NoError(t, err)
	var update map[string]string
	c.client = &http.Client{Transport: hubSpotFixture(t, &update)}
	items, err := c.Fetch(t.Context(), platform.FetchOptions{})
	require.NoError(t, err)
	require.Len(t, items, 2)
	require.Equal(t, "pages/home", items[0].Path)
	require.Len(t, items[0].Blocks, 2)
	require.Equal(t, "Hello", items[0].Blocks[0].SourceText())
	require.Equal(t, "Welcome", items[0].Blocks[1].SourceText())
	// A page without a meta description yields its title alone.
	require.Equal(t, "pages/about", items[1].Path)
	require.Len(t, items[1].Blocks, 1)
	require.Equal(t, "About us", items[1].Blocks[0].SourceText())
	items[0].Blocks[0] = deliveryCopy(items[0].Blocks[0], "Bonjour")
	require.NoError(t, c.Publish(t.Context(), items[:1], platform.PublishOptions{}))
	require.Equal(t, map[string]string{"htmlTitle": "Bonjour", "metaDescription": "Welcome"}, update)
}

func TestHubSpotFetchUnauthorized(t *testing.T) {
	c, err := NewHubSpotConnector(map[string]string{"api_key": "wrong-key"})
	require.NoError(t, err)
	c.client = &http.Client{Transport: hubSpotFixture(t, new(map[string]string))}
	_, err = c.Fetch(t.Context(), platform.FetchOptions{})
	require.ErrorContains(t, err, "HTTP 401")
}

func TestHubSpotCategory(t *testing.T) {
	c := &HubSpotConnector{id: "test", config: map[string]string{}}
	assert.Equal(t, venueconn.CategoryMarketing, c.Category())
}
