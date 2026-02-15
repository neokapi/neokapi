package kapiproject

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gokapi/gokapi/core/connector"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/registry"
)

// KapiSourceConnector implements connector.SourceConnector for local kapi projects.
// It communicates with a Bowrain server via REST API.
type KapiSourceConnector struct {
	project    *Project
	client     *BowrainClient
	formatReg  *registry.FormatRegistry
	cache      *SyncCache
	maxBatch   int // Max blocks per push request
}

// NewSourceConnector creates a SourceConnector for the given project.
func NewSourceConnector(project *Project, formatReg *registry.FormatRegistry) (*KapiSourceConnector, error) {
	if project.Config.Server == nil {
		return nil, fmt.Errorf("no server configuration in .kapi/config.yaml")
	}
	if project.Config.Server.URL == "" {
		return nil, fmt.Errorf("server URL not configured in .kapi/config.yaml")
	}
	if project.Config.Server.ProjectID == "" {
		return nil, fmt.Errorf("server project_id not configured in .kapi/config.yaml")
	}

	cache := LoadSyncCache(project.KapiDir)

	return &KapiSourceConnector{
		project:   project,
		client:    NewBowrainClient(project.Config.Server.URL, project.Config.Server.ProjectID),
		formatReg: formatReg,
		cache:     cache,
		maxBatch:  1000,
	}, nil
}

// ID returns the connector identifier.
func (c *KapiSourceConnector) ID() string {
	return "kapi-source"
}

// Name returns a human-readable name.
func (c *KapiSourceConnector) Name() string {
	return "Kapi Local Source"
}

// Category returns the connector category.
func (c *KapiSourceConnector) Category() connector.Category {
	return connector.CategoryFile
}

// Status reports the sync state.
func (c *KapiSourceConnector) Status(ctx context.Context) (*connector.SyncStatus, error) {
	// Count local changes by scanning files and comparing to cache.
	localBlocks, _, err := c.scanLocalBlocks(ctx, nil)
	if err != nil {
		return &connector.SyncStatus{
			ConnectorID: c.ID(),
			Errors:      []string{err.Error()},
		}, nil
	}

	pendingPush := 0
	for blockID, hash := range localBlocks {
		if cachedHash, ok := c.lookupCachedHash(blockID); !ok || cachedHash != hash {
			pendingPush++
		}
	}

	// Check remote changes by querying the server.
	pendingPull := 0
	if c.cache.SyncCursor > 0 {
		resp, err := c.client.Pull(ctx, c.cache.SyncCursor, nil, 1)
		if err == nil && len(resp.Changes) > 0 {
			pendingPull = len(resp.Changes)
			if resp.HasMore {
				pendingPull = -1 // Unknown, but there are more
			}
		}
	}

	return &connector.SyncStatus{
		ConnectorID: c.ID(),
		LastSync:    c.cache.LastSync,
		ItemCount:   len(localBlocks),
		PendingPush: pendingPush,
		PendingPull: pendingPull,
	}, nil
}

// Configure is a no-op for the kapi source connector (configured via .kapi/config.yaml).
func (c *KapiSourceConnector) Configure(config map[string]string) error {
	return nil
}

// Close saves the sync cache.
func (c *KapiSourceConnector) Close() error {
	return c.cache.Save(c.project.KapiDir)
}

// Push sends source content from local files to Bowrain.
func (c *KapiSourceConnector) Push(ctx context.Context, opts connector.PushOptions) (*connector.PushResult, error) {
	// Scan local files and extract blocks.
	blockHashes, blocks, err := c.scanLocalBlocks(ctx, opts.Paths)
	if err != nil {
		return nil, fmt.Errorf("scan local files: %w", err)
	}

	// Diff against cache to find changed blocks.
	var changed []*model.Block
	for _, b := range blocks {
		hash := blockHashes[b.ID]
		cachedHash, inCache := c.lookupCachedHash(b.ID)
		if opts.Force || !inCache || cachedHash != hash {
			changed = append(changed, b)
		}
	}

	if opts.DryRun {
		return &connector.PushResult{
			BlocksPushed: len(changed),
			FilesScanned: len(c.cache.Files),
		}, nil
	}

	if len(changed) == 0 {
		return &connector.PushResult{FilesScanned: len(blockHashes)}, nil
	}

	// Push in batches of maxBatch.
	chunkCount := 0
	totalStored := 0
	var lastCursor int64

	for i := 0; i < len(changed); i += c.maxBatch {
		end := i + c.maxBatch
		if end > len(changed) {
			end = len(changed)
		}
		batch := changed[i:end]

		inputs := make([]BlockInput, len(batch))
		for j, b := range batch {
			inputs[j] = BlockInput{
				ID:   b.ID,
				Text: b.SourceText(),
				Name: b.Name,
				Type: b.Type,
			}
		}

		resp, err := c.client.Push(ctx, inputs)
		if err != nil {
			return nil, fmt.Errorf("push batch %d: %w", chunkCount+1, err)
		}
		totalStored += resp.Stored
		lastCursor = resp.NewCursor
		chunkCount++
	}

	// Update cache with new hashes and cursor.
	for id, hash := range blockHashes {
		c.updateCachedHash(id, hash)
	}
	c.cache.SyncCursor = lastCursor
	c.cache.LastSync = time.Now().UTC()
	c.cache.ServerURL = c.project.Config.Server.URL
	c.cache.ProjectID = c.project.Config.Server.ProjectID

	if err := c.cache.Save(c.project.KapiDir); err != nil {
		return nil, fmt.Errorf("save sync cache: %w", err)
	}

	return &connector.PushResult{
		BlocksPushed: totalStored,
		FilesScanned: len(blockHashes),
		ChunkCount:   chunkCount,
	}, nil
}

// Pull retrieves translated content from Bowrain.
func (c *KapiSourceConnector) Pull(ctx context.Context, opts connector.PullOptions) (*connector.PullResult, error) {
	locales := make([]string, len(opts.Locales))
	for i, l := range opts.Locales {
		locales[i] = string(l)
	}

	totalPulled := 0
	cursor := c.cache.SyncCursor
	if opts.Force {
		cursor = 0
	}

	for {
		resp, err := c.client.Pull(ctx, cursor, locales, 1000)
		if err != nil {
			return nil, fmt.Errorf("pull changes: %w", err)
		}

		if opts.DryRun {
			totalPulled += len(resp.Changes)
			if !resp.HasMore {
				break
			}
			cursor = resp.NewCursor
			continue
		}

		totalPulled += len(resp.Changes)
		cursor = resp.NewCursor

		if !resp.HasMore {
			break
		}
	}

	if !opts.DryRun {
		c.cache.SyncCursor = cursor
		c.cache.LastSync = time.Now().UTC()
		if err := c.cache.Save(c.project.KapiDir); err != nil {
			return nil, fmt.Errorf("save sync cache: %w", err)
		}
	}

	return &connector.PullResult{
		BlocksPulled: totalPulled,
		LocalesCount: len(opts.Locales),
	}, nil
}

// scanLocalBlocks walks local source files, reads them with format readers,
// and extracts blocks. Returns blockID→contentHash map and the blocks themselves.
func (c *KapiSourceConnector) scanLocalBlocks(ctx context.Context, paths []string) (map[string]string, []*model.Block, error) {
	hashes := map[string]string{}
	var allBlocks []*model.Block

	// If no specific paths, use mappings to discover files.
	if len(paths) == 0 {
		for _, m := range c.project.Config.Mappings {
			matched, err := filepath.Glob(filepath.Join(c.project.Root, m.Local))
			if err != nil {
				continue
			}
			paths = append(paths, matched...)
		}
	}

	if len(paths) == 0 {
		return hashes, allBlocks, nil
	}

	for _, p := range paths {
		absPath := c.project.ResolvePath(p)

		// Skip if file doesn't exist.
		info, err := os.Stat(absPath)
		if err != nil || info.IsDir() {
			continue
		}

		// Determine format from config mappings or registry detection.
		formatName := c.detectFormat(absPath)
		if formatName == "" {
			continue
		}

		blocks, err := c.readBlocks(ctx, absPath, formatName)
		if err != nil {
			continue // Skip files that can't be parsed.
		}

		for _, b := range blocks {
			identity := model.ComputeIdentity(b)
			hashes[b.ID] = identity.ContentHash
			allBlocks = append(allBlocks, b)
		}
	}

	return hashes, allBlocks, nil
}

// detectFormat determines the format for a file using mappings or the registry.
func (c *KapiSourceConnector) detectFormat(absPath string) string {
	relPath, err := c.project.RelativePath(absPath)
	if err != nil {
		relPath = filepath.Base(absPath)
	}

	// Check mappings first.
	for _, m := range c.project.Config.Mappings {
		matched, err := filepath.Match(m.Local, relPath)
		if err == nil && matched && m.Format != "" {
			return m.Format
		}
	}

	// Fall back to registry detection by file extension.
	ext := filepath.Ext(absPath)
	if ext == "" {
		return ""
	}
	name, err := c.formatReg.Detector().DetectByExtension(ext)
	if err != nil {
		return ""
	}
	return name
}

// readBlocks reads a file and extracts blocks using the format reader.
func (c *KapiSourceConnector) readBlocks(ctx context.Context, filePath, formatName string) ([]*model.Block, error) {
	reader, err := c.formatReg.NewReader(formatName)
	if err != nil {
		return nil, fmt.Errorf("create reader for %s: %w", formatName, err)
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("open file %s: %w", filePath, err)
	}

	doc := &model.RawDocument{
		URI:      filePath,
		FormatID: formatName,
		Reader:   f,
	}

	if err := reader.Open(ctx, doc); err != nil {
		f.Close()
		return nil, fmt.Errorf("open document %s: %w", filePath, err)
	}

	var blocks []*model.Block
	ch := reader.Read(ctx)
	for pr := range ch {
		if pr.Error != nil {
			continue
		}
		if pr.Part.Type == model.PartBlock {
			if b, ok := pr.Part.Resource.(*model.Block); ok {
				blocks = append(blocks, b)
			}
		}
	}

	return blocks, nil
}

// lookupCachedHash finds a block's cached hash across all file caches.
func (c *KapiSourceConnector) lookupCachedHash(blockID string) (string, bool) {
	for _, fc := range c.cache.Files {
		if hash, ok := fc.Blocks[blockID]; ok {
			return hash, true
		}
	}
	return "", false
}

// updateCachedHash updates the hash for a block in a flat "blocks" entry.
func (c *KapiSourceConnector) updateCachedHash(blockID, hash string) {
	const globalKey = "_blocks"
	fc, ok := c.cache.Files[globalKey]
	if !ok {
		fc = &FileCache{Blocks: map[string]string{}}
		c.cache.Files[globalKey] = fc
	}
	fc.Blocks[blockID] = hash
}

// Ensure KapiSourceConnector implements SourceConnector at compile time.
var _ connector.SourceConnector = (*KapiSourceConnector)(nil)
