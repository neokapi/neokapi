package connector

import (
	"context"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"time"

	platconn "github.com/neokapi/neokapi/bowrain/core/connector"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
)

// FileConnector reads and writes localization content from the filesystem
// using the format registry for format detection and parsing.
type FileConnector struct {
	id             string
	name           string
	basePath       string
	formatRegistry *registry.FormatRegistry
	config         map[string]string
}

// NewFileConnector creates a new FileConnector.
func NewFileConnector(formatReg *registry.FormatRegistry, config map[string]string) (*FileConnector, error) {
	basePath := config["path"]
	if basePath == "" {
		basePath = "."
	}
	id := config["id"]
	if id == "" {
		id = "file-" + filepath.Base(basePath)
	}
	name := config["name"]
	if name == "" {
		name = "File: " + basePath
	}

	return &FileConnector{
		id:             id,
		name:           name,
		basePath:       basePath,
		formatRegistry: formatReg,
		config:         config,
	}, nil
}

func (c *FileConnector) ID() string                  { return c.id }
func (c *FileConnector) Name() string                { return c.name }
func (c *FileConnector) Category() platconn.Category { return platconn.CategoryFile }

func (c *FileConnector) Configure(config map[string]string) error {
	maps.Copy(c.config, config)
	if p, ok := config["path"]; ok {
		c.basePath = p
	}
	return nil
}

func (c *FileConnector) Close() error { return nil }

// Fetch reads files from the filesystem, parses them with format readers,
// and returns ContentItems containing the extracted blocks.
func (c *FileConnector) Fetch(ctx context.Context, opts platconn.FetchOptions) ([]*platconn.ContentItem, error) {
	paths := opts.Paths
	if len(paths) == 0 {
		// Discover all files if no specific paths given.
		items, err := c.List(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			paths = append(paths, item.Path)
		}
	}

	var result []*platconn.ContentItem
	for _, p := range paths {
		item, err := c.fetchFile(ctx, filepath.Join(c.basePath, p))
		if err != nil {
			return nil, fmt.Errorf("fetch %s: %w", p, err)
		}
		if item != nil {
			result = append(result, item)
		}
	}
	return result, nil
}

func (c *FileConnector) fetchFile(ctx context.Context, path string) (*platconn.ContentItem, error) {
	ext := filepath.Ext(path)
	detector := c.formatRegistry.Detector()
	formatName, err := detector.DetectByExtension(ext)
	if err != nil {
		return nil, fmt.Errorf("detect format for %s: %w", path, err)
	}

	reader, err := c.formatRegistry.NewReader(registry.FormatID(formatName))
	if err != nil {
		return nil, fmt.Errorf("create reader for %s: %w", formatName, err)
	}
	defer reader.Close()

	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}

	doc := &model.RawDocument{
		URI:      path,
		FormatID: formatName,
		Reader:   f,
	}
	if err := reader.Open(ctx, doc); err != nil {
		f.Close()
		return nil, fmt.Errorf("open document %s: %w", path, err)
	}

	var blocks []*model.Block
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			return nil, fmt.Errorf("read %s: %w", path, pr.Error)
		}
		if pr.Part != nil && pr.Part.Type == model.PartBlock {
			if b, ok := pr.Part.Resource.(*model.Block); ok {
				blocks = append(blocks, b)
			}
		}
	}

	relPath, _ := filepath.Rel(c.basePath, path)
	info, _ := os.Stat(path)
	var lastChanged time.Time
	if info != nil {
		lastChanged = info.ModTime()
	}

	return &platconn.ContentItem{
		ID:          relPath,
		Name:        filepath.Base(path),
		Path:        relPath,
		Format:      formatName,
		Blocks:      blocks,
		LastChanged: lastChanged,
		Metadata:    map[string]string{"absolute_path": path},
	}, nil
}

// Publish writes translated content items back to the filesystem.
func (c *FileConnector) Publish(ctx context.Context, items []*platconn.ContentItem, opts platconn.PublishOptions) error {
	for _, item := range items {
		if err := c.publishFile(ctx, item, opts); err != nil {
			return fmt.Errorf("publish %s: %w", item.Path, err)
		}
	}
	return nil
}

func (c *FileConnector) publishFile(ctx context.Context, item *platconn.ContentItem, opts platconn.PublishOptions) error {
	path := filepath.Join(c.basePath, item.Path)

	writer, err := c.formatRegistry.NewWriter(registry.FormatID(item.Format))
	if err != nil {
		return fmt.Errorf("create writer for %s: %w", item.Format, err)
	}
	defer writer.Close()

	if err := writer.SetOutput(path); err != nil {
		return fmt.Errorf("set output %s: %w", path, err)
	}

	// Feed blocks as Parts through the writer channel.
	ch := make(chan *model.Part, len(item.Blocks))
	for _, b := range item.Blocks {
		ch <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(ch)

	return writer.Write(ctx, ch)
}

// List scans the base directory for files whose extensions match known formats.
func (c *FileConnector) List(ctx context.Context) ([]*platconn.ContentItem, error) {
	detector := c.formatRegistry.Detector()
	var items []*platconn.ContentItem

	err := filepath.Walk(c.basePath, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		ext := filepath.Ext(path)
		formatName, detectErr := detector.DetectByExtension(ext)
		if detectErr != nil {
			return nil // Skip unrecognized files.
		}
		relPath, _ := filepath.Rel(c.basePath, path)
		items = append(items, &platconn.ContentItem{
			ID:          relPath,
			Name:        info.Name(),
			Path:        relPath,
			Format:      formatName,
			LastChanged: info.ModTime(),
			Metadata:    map[string]string{"absolute_path": path},
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walk %s: %w", c.basePath, err)
	}
	return items, nil
}

// Status returns the current sync status.
func (c *FileConnector) Status(ctx context.Context) (*platconn.SyncStatus, error) {
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
