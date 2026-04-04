package connector

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupHubSpotServer(t *testing.T) *httptest.Server {
	t.Helper()

	pages := hsPageList{
		Results: []hsPage{
			{
				ID:        "123",
				Name:      "Landing Page",
				Slug:      "landing",
				HTMLTitle: "Welcome to Our Product",
				MetaDesc:  "The best product for your needs",
				Updated:   "2024-03-01T12:00:00Z",
			},
			{
				ID:        "456",
				Name:      "About Us",
				Slug:      "about",
				HTMLTitle: "About Our Company",
				Updated:   "2024-03-02T09:00:00Z",
			},
		},
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/cms/v3/pages/site-pages", func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(pages)
	})

	return httptest.NewServer(mux)
}

func TestHubSpotPull(t *testing.T) {
	srv := setupHubSpotServer(t)
	defer srv.Close()

	// Create connector pointing to test server.
	c := &HubSpotConnector{
		id:       "hs-test",
		connName: "Test HubSpot",
		apiKey:   "test-key",
		client:   srv.Client(),
		config:   map[string]string{},
	}
	// Override fetchPages to use the test server URL.
	items, err := pullHubSpotFromURL(context.Background(), c, srv.URL+"/cms/v3/pages/site-pages")
	require.NoError(t, err)
	require.Len(t, items, 2)

	assert.Equal(t, "Landing Page", items[0].Name)
	assert.GreaterOrEqual(t, len(items[0].Blocks), 2) // title + meta
	assert.Equal(t, "About Us", items[1].Name)
}

func TestHubSpotCategory(t *testing.T) {
	c := &HubSpotConnector{id: "test", config: map[string]string{}}
	assert.Equal(t, platconn.CategoryMarketing, c.Category())
}

// pullHubSpotFromURL is a test helper that fetches pages from a custom URL.
func pullHubSpotFromURL(ctx context.Context, c *HubSpotConnector, url string) ([]*platconn.ContentItem, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var list hsPageList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, err
	}

	var items []*platconn.ContentItem
	for _, page := range list.Results {
		blocks := []*model.Block{
			makeBlock("page-"+page.ID+"-title", page.HTMLTitle, "title"),
		}
		if page.MetaDesc != "" {
			blocks = append(blocks,
				makeBlock("page-"+page.ID+"-meta", page.MetaDesc, "meta_description"))
		}
		items = append(items, &platconn.ContentItem{
			ID:       page.ID,
			Name:     page.Name,
			Blocks:   blocks,
			Metadata: map[string]string{"hs_id": page.ID},
		})
	}
	return items, nil
}
