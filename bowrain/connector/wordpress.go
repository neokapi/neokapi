package connector

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gokapi/gokapi/core/model"
)

// WordPressConnector integrates with WordPress sites via the REST API.
type WordPressConnector struct {
	id       string
	connName string
	baseURL  string
	username string
	password string
	client   *http.Client
	config   map[string]string
}

// wpPost represents a WordPress post from the REST API.
type wpPost struct {
	ID       int       `json:"id"`
	Slug     string    `json:"slug"`
	Title    wpContent `json:"title"`
	Content  wpContent `json:"content"`
	Excerpt  wpContent `json:"excerpt"`
	Modified string    `json:"modified"`
}

type wpContent struct {
	Rendered string `json:"rendered"`
	Raw      string `json:"raw"`
}

// NewWordPressConnector creates a new WordPress connector.
func NewWordPressConnector(config map[string]string) (*WordPressConnector, error) {
	baseURL := config["url"]
	if baseURL == "" {
		return nil, fmt.Errorf("wordpress connector requires 'url' config")
	}
	baseURL = strings.TrimRight(baseURL, "/")

	id := config["id"]
	if id == "" {
		id = "wp-" + strings.ReplaceAll(baseURL, "/", "-")
	}

	return &WordPressConnector{
		id:       id,
		connName: config["name"],
		baseURL:  baseURL,
		username: config["username"],
		password: config["password"],
		client:   &http.Client{Timeout: 30 * time.Second},
		config:   config,
	}, nil
}

func (c *WordPressConnector) ID() string         { return c.id }
func (c *WordPressConnector) Name() string       { return c.connName }
func (c *WordPressConnector) Category() Category { return CategoryCMS }

func (c *WordPressConnector) Configure(config map[string]string) error {
	for k, v := range config {
		c.config[k] = v
	}
	return nil
}

func (c *WordPressConnector) Close() error { return nil }

func (c *WordPressConnector) Fetch(ctx context.Context, opts FetchOptions) ([]*ContentItem, error) {
	posts, err := c.fetchPosts(ctx)
	if err != nil {
		return nil, err
	}

	var items []*ContentItem
	for _, post := range posts {
		blocks := []*model.Block{
			makeBlock(fmt.Sprintf("post-%d-title", post.ID), post.Title.Rendered, "title"),
			makeBlock(fmt.Sprintf("post-%d-content", post.ID), post.Content.Rendered, "content"),
		}
		if post.Excerpt.Rendered != "" {
			blocks = append(blocks,
				makeBlock(fmt.Sprintf("post-%d-excerpt", post.ID), post.Excerpt.Rendered, "excerpt"))
		}

		modified, _ := time.Parse("2006-01-02T15:04:05", post.Modified)
		items = append(items, &ContentItem{
			ID:          fmt.Sprintf("post-%d", post.ID),
			Name:        post.Title.Rendered,
			Path:        fmt.Sprintf("posts/%s", post.Slug),
			Format:      "html",
			Blocks:      blocks,
			LastChanged: modified,
			Metadata:    map[string]string{"wp_id": fmt.Sprintf("%d", post.ID)},
		})
	}
	return items, nil
}

func (c *WordPressConnector) Publish(ctx context.Context, items []*ContentItem, opts PublishOptions) error {
	for _, item := range items {
		wpID := item.Metadata["wp_id"]
		if wpID == "" {
			continue
		}

		payload := map[string]string{}
		for _, b := range item.Blocks {
			switch b.Type {
			case "title":
				payload["title"] = b.SourceText()
			case "content":
				payload["content"] = b.SourceText()
			case "excerpt":
				payload["excerpt"] = b.SourceText()
			}
		}

		body, _ := json.Marshal(payload)
		url := fmt.Sprintf("%s/wp-json/wp/v2/posts/%s", c.baseURL, wpID)
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(body)))
		if err != nil {
			return err
		}
		req.Header.Set("Content-Type", "application/json")
		if c.username != "" {
			req.SetBasicAuth(c.username, c.password)
		}

		resp, err := c.client.Do(req)
		if err != nil {
			return fmt.Errorf("update post %s: %w", wpID, err)
		}
		resp.Body.Close()
		if resp.StatusCode >= 400 {
			return fmt.Errorf("update post %s: HTTP %d", wpID, resp.StatusCode)
		}
	}
	return nil
}

func (c *WordPressConnector) List(ctx context.Context) ([]*ContentItem, error) {
	return c.Fetch(ctx, FetchOptions{})
}

func (c *WordPressConnector) Status(ctx context.Context) (*SyncStatus, error) {
	items, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	return &SyncStatus{
		ConnectorID: c.id,
		LastSync:    time.Now(),
		ItemCount:   len(items),
	}, nil
}

func (c *WordPressConnector) fetchPosts(ctx context.Context) ([]wpPost, error) {
	url := fmt.Sprintf("%s/wp-json/wp/v2/posts?per_page=100", c.baseURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if c.username != "" {
		req.SetBasicAuth(c.username, c.password)
	}

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch posts: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("fetch posts: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var posts []wpPost
	if err := json.NewDecoder(resp.Body).Decode(&posts); err != nil {
		return nil, fmt.Errorf("decode posts: %w", err)
	}
	return posts, nil
}

func makeBlock(id, text, typ string) *model.Block {
	b := model.NewBlock(id, text)
	b.Type = typ
	return b
}
