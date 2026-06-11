package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"time"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/model"
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
		return nil, errors.New("wordpress connector requires 'url' config")
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

func (c *WordPressConnector) ID() string                  { return c.id }
func (c *WordPressConnector) Name() string                { return c.connName }
func (c *WordPressConnector) Category() platconn.Category { return platconn.CategoryCMS }

func (c *WordPressConnector) Configure(config map[string]string) error {
	maps.Copy(c.config, config)
	return nil
}

func (c *WordPressConnector) Close() error { return nil }

func (c *WordPressConnector) Fetch(ctx context.Context, opts platconn.FetchOptions) ([]*platconn.ContentItem, error) {
	posts, err := c.fetchPosts(ctx)
	if err != nil {
		return nil, err
	}

	var items []*platconn.ContentItem
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
		items = append(items, &platconn.ContentItem{
			ID:          fmt.Sprintf("post-%d", post.ID),
			Name:        post.Title.Rendered,
			Path:        "posts/" + post.Slug,
			Format:      "html",
			Blocks:      blocks,
			LastChanged: modified,
			Metadata:    map[string]string{"wp_id": strconv.Itoa(post.ID)},
		})
	}
	return items, nil
}

func (c *WordPressConnector) Publish(ctx context.Context, items []*platconn.ContentItem, opts platconn.PublishOptions) error {
	for _, item := range items {
		wpID := item.Metadata["wp_id"]
		if wpID == "" {
			slog.Warn("wordpress: skipping item with no wp_id", "item", item.Name)
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
			default:
				slog.Warn("wordpress: skipping block with unsupported type", "block_id", b.ID, "type", b.Type, "post_id", wpID)
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

func (c *WordPressConnector) List(ctx context.Context) ([]*platconn.ContentItem, error) {
	return c.Fetch(ctx, platconn.FetchOptions{})
}

func (c *WordPressConnector) Status(ctx context.Context) (*platconn.SyncStatus, error) {
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

func (c *WordPressConnector) fetchPosts(ctx context.Context) ([]wpPost, error) {
	url := c.baseURL + "/wp-json/wp/v2/posts?per_page=100"
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
