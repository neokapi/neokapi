package connector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"time"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/model"
)

// FigmaConnector integrates with the Figma API to fetch text content.
type FigmaConnector struct {
	id       string
	connName string
	fileKey  string
	token    string
	client   *http.Client
	config   map[string]string
}

// figmaFile represents a Figma file response.
type figmaFile struct {
	Name     string    `json:"name"`
	Document figmaNode `json:"document"`
}

type figmaNode struct {
	ID          string      `json:"id"`
	Name        string      `json:"name"`
	Type        string      `json:"type"`
	Characters  string      `json:"characters"`
	Children    []figmaNode `json:"children"`
	BoundingBox *figmaBBox  `json:"absoluteBoundingBox"`
}

type figmaBBox struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// NewFigmaConnector creates a new Figma connector.
func NewFigmaConnector(config map[string]string) (*FigmaConnector, error) {
	fileKey := config["file_key"]
	if fileKey == "" {
		return nil, errors.New("figma connector requires 'file_key' config")
	}
	token := config["token"]
	if token == "" {
		return nil, errors.New("figma connector requires 'token' config")
	}

	id := config["id"]
	if id == "" {
		id = "figma-" + fileKey
	}

	return &FigmaConnector{
		id:       id,
		connName: config["name"],
		fileKey:  fileKey,
		token:    token,
		client:   &http.Client{Timeout: 30 * time.Second},
		config:   config,
	}, nil
}

func (c *FigmaConnector) ID() string                  { return c.id }
func (c *FigmaConnector) Name() string                { return c.connName }
func (c *FigmaConnector) Category() platconn.Category { return platconn.CategoryDesign }

func (c *FigmaConnector) Configure(config map[string]string) error {
	maps.Copy(c.config, config)
	return nil
}

func (c *FigmaConnector) Close() error { return nil }

func (c *FigmaConnector) Fetch(ctx context.Context, opts platconn.FetchOptions) ([]*platconn.ContentItem, error) {
	file, err := c.fetchFile(ctx)
	if err != nil {
		return nil, err
	}

	var blocks []*model.Block
	c.extractTextNodes(&file.Document, &blocks)

	return []*platconn.ContentItem{{
		ID:       c.fileKey,
		Name:     file.Name,
		Path:     c.fileKey,
		Format:   "figma",
		Blocks:   blocks,
		Metadata: map[string]string{"file_key": c.fileKey},
	}}, nil
}

func (c *FigmaConnector) Publish(ctx context.Context, items []*platconn.ContentItem, opts platconn.PublishOptions) error {
	// Figma API doesn't support direct text updates in the general API.
	// This would require the Figma Plugin API or Variables API.
	return errors.New("figma publish not yet supported via REST API")
}

func (c *FigmaConnector) List(ctx context.Context) ([]*platconn.ContentItem, error) {
	return c.Fetch(ctx, platconn.FetchOptions{})
}

func (c *FigmaConnector) Status(ctx context.Context) (*platconn.SyncStatus, error) {
	items, err := c.List(ctx)
	if err != nil {
		return nil, err
	}
	count := 0
	for _, item := range items {
		count += len(item.Blocks)
	}
	return &platconn.SyncStatus{
		ConnectorID: c.id,
		LastSync:    time.Now(),
		ItemCount:   count,
	}, nil
}

func (c *FigmaConnector) fetchFile(ctx context.Context) (*figmaFile, error) {
	url := "https://api.figma.com/v1/files/" + c.fileKey
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("X-Figma-Token", c.token)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch figma file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("figma API: HTTP %d: %s", resp.StatusCode, string(body))
	}

	var file figmaFile
	if err := json.NewDecoder(resp.Body).Decode(&file); err != nil {
		return nil, fmt.Errorf("decode figma file: %w", err)
	}
	return &file, nil
}

func (c *FigmaConnector) extractTextNodes(node *figmaNode, blocks *[]*model.Block) {
	if node.Type == "TEXT" && node.Characters != "" {
		b := model.NewBlock(node.ID, node.Characters)
		b.Name = node.Name
		b.Type = "text"

		if node.BoundingBox != nil {
			b.DisplayHint = &model.DisplayHint{
				Context:     fmt.Sprintf("Figma frame at (%.0f, %.0f)", node.BoundingBox.X, node.BoundingBox.Y),
				MaxLength:   int(node.BoundingBox.Width / 6), // rough char estimate
				ContentType: "text",
			}
		}

		*blocks = append(*blocks, b)
	}

	for i := range node.Children {
		c.extractTextNodes(&node.Children[i], blocks)
	}
}
