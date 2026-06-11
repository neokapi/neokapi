package connector

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"time"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/httputil"
	"github.com/neokapi/neokapi/core/model"
)

// HubSpotConnector integrates with HubSpot CMS for marketing content.
type HubSpotConnector struct {
	id       string
	connName string
	apiKey   string
	client   *http.Client
	config   map[string]string
}

// hsPage represents a HubSpot CMS page.
type hsPage struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Slug      string `json:"slug"`
	HTMLTitle string `json:"htmlTitle"`
	MetaDesc  string `json:"metaDescription"`
	Updated   string `json:"updated"`
}

type hsPageList struct {
	Results []hsPage `json:"results"`
}

// NewHubSpotConnector creates a new HubSpot connector.
func NewHubSpotConnector(config map[string]string) (*HubSpotConnector, error) {
	apiKey := config["api_key"]
	if apiKey == "" {
		return nil, errors.New("hubspot connector requires 'api_key' config")
	}

	id := config["id"]
	if id == "" {
		id = "hubspot"
	}

	return &HubSpotConnector{
		id:       id,
		connName: config["name"],
		apiKey:   apiKey,
		client:   httputil.NewResilientClient(),
		config:   config,
	}, nil
}

func (c *HubSpotConnector) ID() string                  { return c.id }
func (c *HubSpotConnector) Name() string                { return c.connName }
func (c *HubSpotConnector) Category() platconn.Category { return platconn.CategoryMarketing }

func (c *HubSpotConnector) Configure(config map[string]string) error {
	maps.Copy(c.config, config)
	return nil
}

func (c *HubSpotConnector) Close() error { return nil }

func (c *HubSpotConnector) Fetch(ctx context.Context, opts platconn.FetchOptions) ([]*platconn.ContentItem, error) {
	pages, err := c.fetchPages(ctx)
	if err != nil {
		return nil, err
	}

	var items []*platconn.ContentItem
	for _, page := range pages {
		blocks := []*model.Block{
			makeBlock(fmt.Sprintf("page-%s-title", page.ID), page.HTMLTitle, "title"),
		}
		if page.MetaDesc != "" {
			blocks = append(blocks,
				makeBlock(fmt.Sprintf("page-%s-meta", page.ID), page.MetaDesc, "meta_description"))
		}

		updated, _ := time.Parse(time.RFC3339, page.Updated)
		items = append(items, &platconn.ContentItem{
			ID:          page.ID,
			Name:        page.Name,
			Path:        "pages/" + page.Slug,
			Format:      "html",
			Blocks:      blocks,
			LastChanged: updated,
			Metadata:    map[string]string{"hs_id": page.ID},
		})
	}
	return items, nil
}

func (c *HubSpotConnector) Publish(ctx context.Context, items []*platconn.ContentItem, opts platconn.PublishOptions) error {
	for _, item := range items {
		pageID := item.ID
		if pageID == "" {
			if id, ok := item.Metadata["hs_id"]; ok {
				pageID = id
			}
		}
		if pageID == "" {
			return fmt.Errorf("content item %q has no HubSpot page ID", item.Name)
		}

		// Collect title and meta description from blocks.
		update := make(map[string]string)
		for _, b := range item.Blocks {
			text := b.SourceText()
			if b.Type == "title" || strings.HasSuffix(b.ID, "-title") {
				update["htmlTitle"] = text
			} else if b.Type == "meta_description" || strings.HasSuffix(b.ID, "-meta") {
				update["metaDescription"] = text
			}
		}

		if len(update) == 0 {
			continue
		}

		body, err := json.Marshal(update)
		if err != nil {
			return fmt.Errorf("marshal update for page %s: %w", pageID, err)
		}

		url := "https://api.hubapi.com/cms/v3/pages/site-pages/" + pageID
		req, err := http.NewRequestWithContext(ctx, http.MethodPatch, url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("create request for page %s: %w", pageID, err)
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", "application/json")

		resp, err := c.client.Do(req)
		if err != nil {
			return fmt.Errorf("update page %s: %w", pageID, err)
		}
		resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("hubspot API: HTTP %d updating page %s", resp.StatusCode, pageID)
		}
	}
	return nil
}

func (c *HubSpotConnector) List(ctx context.Context) ([]*platconn.ContentItem, error) {
	return c.Fetch(ctx, platconn.FetchOptions{})
}

func (c *HubSpotConnector) Status(ctx context.Context) (*platconn.SyncStatus, error) {
	items, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	return &platconn.SyncStatus{
		ConnectorID: c.id,
		LastSync:    time.Now(),
		ItemCount:   len(items),
	}, nil
}

func (c *HubSpotConnector) fetchPages(ctx context.Context) ([]hsPage, error) {
	url := "https://api.hubapi.com/cms/v3/pages/site-pages"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch hubspot pages: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("hubspot API: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var list hsPageList
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return nil, fmt.Errorf("decode hubspot pages: %w", err)
	}
	return list.Results, nil
}
