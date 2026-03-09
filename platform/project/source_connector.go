package project

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/gokapi/gokapi/core/format"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/registry"
	apiclient "github.com/gokapi/gokapi/platform/client"
	"github.com/gokapi/gokapi/platform/config"
	"github.com/gokapi/gokapi/platform/connector"
)

// BrainSourceConnector implements connector.SourceConnector for local bowrain-cli projects.
// It communicates with a Bowrain server via REST API.
type BrainSourceConnector struct {
	project   *Project
	client    *apiclient.BowrainClient
	formatReg *registry.FormatRegistry
	cache     *SyncCache
	maxBatch  int // Max blocks per push request
}

// NewSourceConnector creates a SourceConnector for the given project.
func NewSourceConnector(project *Project, formatReg *registry.FormatRegistry) (*BrainSourceConnector, error) {
	if project.Config.Server == nil {
		return nil, fmt.Errorf("no server configuration in .bowrain/config.yaml")
	}
	if project.Config.Server.URL == "" {
		return nil, fmt.Errorf("server URL not configured in .bowrain/config.yaml")
	}
	if project.Config.Server.ProjectID == "" {
		return nil, fmt.Errorf("server project_id not configured in .bowrain/config.yaml")
	}

	cache := LoadSyncCache(project.ConfigDir)

	var client *apiclient.BowrainClient
	srv := project.Config.Server
	switch {
	case srv.ClaimToken != "":
		client = apiclient.NewClaimTokenClient(srv.URL, srv.ProjectID, srv.ClaimToken)
	case srv.Workspace != "":
		authInfo, err := config.LoadAuth()
		if err != nil {
			return nil, fmt.Errorf("workspace sync requires authentication: run 'bowrain auth login'")
		}
		if authInfo.ServerURL != "" && authInfo.ServerURL != srv.URL {
			return nil, fmt.Errorf("auth token is for %s but project points to %s", authInfo.ServerURL, srv.URL)
		}
		client = apiclient.NewWorkspaceBowrainClient(srv.URL, srv.Workspace, srv.ProjectID, authInfo.AccessToken)
		if authInfo.RefreshToken != "" {
			client.SetRefreshToken(authInfo.RefreshToken, func(newAccess, newRefresh string) {
				authInfo.AccessToken = newAccess
				authInfo.RefreshToken = newRefresh
				_ = config.SaveAuth(*authInfo)
			})
		}
	default:
		// No workspace or claim token in config — try env-based auth (BOWRAIN_AUTH_TOKEN).
		// This supports CI scenarios where auth is provided via environment variable.
		authInfo, err := config.LoadAuth()
		if err != nil {
			return nil, fmt.Errorf("server config requires either workspace or claim_token, or set BOWRAIN_AUTH_TOKEN")
		}
		// Detect claim tokens (clm_ prefix) and route them correctly.
		if strings.HasPrefix(authInfo.AccessToken, "clm_") {
			client = apiclient.NewClaimTokenClient(srv.URL, srv.ProjectID, authInfo.AccessToken)
		} else {
			// Use flat project routes with bearer auth (works for API tokens and JWTs).
			client = apiclient.NewProjectBearerClient(srv.URL, srv.ProjectID, authInfo.AccessToken)
		}
	}

	return &BrainSourceConnector{
		project:   project,
		client:    client,
		formatReg: formatReg,
		cache:     cache,
		maxBatch:  1000,
	}, nil
}

// NewLocalConnector creates a BrainSourceConnector for local-only operations
// (listing files, scanning blocks) without requiring a server connection.
func NewLocalConnector(project *Project, formatReg *registry.FormatRegistry) *BrainSourceConnector {
	cache := LoadSyncCache(project.ConfigDir)
	return &BrainSourceConnector{
		project:   project,
		formatReg: formatReg,
		cache:     cache,
	}
}

// FileInfo describes a single tracked file with optional stats.
type FileInfo struct {
	Path       string // Relative path from project root
	Format     string // Detected format name
	BlockCount int    // Number of translatable blocks (-1 = not scanned)
	WordCount  int    // Total source word count (-1 = not scanned)
	DirtyCount int    // Blocks changed vs cache (-1 = not checked)
}

// ListFiles scans all tracked files and returns per-file stats.
// It uses scanLocalBlocks for block extraction and compares against the
// sync cache for dirty detection. Results are sorted by path.
func (c *BrainSourceConnector) ListFiles(ctx context.Context, paths []string) ([]FileInfo, error) {
	_, blockMap, err := c.scanLocalBlocks(ctx, paths)
	if err != nil {
		return nil, err
	}

	var files []FileInfo
	for relPath, blocks := range blockMap {
		words := 0
		for _, b := range blocks {
			words += b.WordCount()
		}

		dirty := 0
		for _, b := range blocks {
			identity := model.ComputeIdentity(b)
			cached, found := c.lookupCachedHashForItem(relPath, b.ID)
			if !found || cached != identity.ContentHash {
				dirty++
			}
		}

		absPath := filepath.Join(c.project.Root, relPath)
		formatName := c.detectFormat(absPath)

		files = append(files, FileInfo{
			Path:       relPath,
			Format:     formatName,
			BlockCount: len(blocks),
			WordCount:  words,
			DirtyCount: dirty,
		})
	}

	sort.Slice(files, func(i, j int) bool {
		return files[i].Path < files[j].Path
	})
	return files, nil
}

// ID returns the connector identifier.
func (c *BrainSourceConnector) ID() string {
	return "brain-source"
}

// Name returns a human-readable name.
func (c *BrainSourceConnector) Name() string {
	return "Brain Local Source"
}

// Category returns the connector category.
func (c *BrainSourceConnector) Category() connector.Category {
	return connector.CategoryFile
}

// Status reports the sync state.
func (c *BrainSourceConnector) Status(ctx context.Context) (*connector.SyncStatus, error) {
	// Count local changes by scanning files and comparing to cache.
	hashMap, blockMap, err := c.scanLocalBlocks(ctx, nil)
	if err != nil {
		return &connector.SyncStatus{
			ConnectorID: c.ID(),
			Errors:      []string{err.Error()},
		}, nil
	}

	totalBlocks := 0
	pendingPush := 0
	for itemName, fileHashes := range hashMap {
		for blockID, hash := range fileHashes {
			totalBlocks++
			cachedHash, inCache := c.lookupCachedHashForItem(itemName, blockID)
			if !inCache || cachedHash != hash {
				pendingPush++
			}
		}
	}

	totalWords := 0
	for _, blocks := range blockMap {
		for _, b := range blocks {
			totalWords += b.WordCount()
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
		ItemCount:   totalBlocks,
		FileCount:   len(hashMap),
		WordCount:   totalWords,
		PendingPush: pendingPush,
		PendingPull: pendingPull,
	}, nil
}

// Configure is a no-op for the Bowrain CLI source connector (configured via .bowrain/config.yaml).
func (c *BrainSourceConnector) Configure(config map[string]string) error {
	return nil
}

// Close saves the sync cache.
func (c *BrainSourceConnector) Close() error {
	return c.cache.Save(c.project.ConfigDir)
}

// Push sends source content from local files to Bowrain.
func (c *BrainSourceConnector) Push(ctx context.Context, opts connector.PushOptions) (*connector.PushResult, error) {
	// Scan local files and extract blocks grouped by item.
	hashMap, blockMap, err := c.scanLocalBlocks(ctx, opts.Paths)
	if err != nil {
		return nil, fmt.Errorf("scan local files: %w", err)
	}

	// Diff against cache to find changed blocks, keeping item association.
	type itemBlock struct {
		itemName string
		block    *model.Block
	}
	var changed []itemBlock
	for itemName, blocks := range blockMap {
		fileHashes := hashMap[itemName]
		for _, b := range blocks {
			hash := fileHashes[b.ID]
			cachedHash, inCache := c.lookupCachedHashForItem(itemName, b.ID)
			if opts.Force || !inCache || cachedHash != hash {
				changed = append(changed, itemBlock{itemName: itemName, block: b})
			}
		}
	}

	pushWords := 0
	for _, ib := range changed {
		pushWords += ib.block.WordCount()
	}

	if opts.DryRun {
		return &connector.PushResult{
			BlocksPushed: len(changed),
			FilesScanned: len(hashMap),
			WordCount:    pushWords,
		}, nil
	}

	if len(changed) == 0 {
		return &connector.PushResult{FilesScanned: len(hashMap)}, nil
	}

	// Push in batches of maxBatch.
	chunkCount := 0
	totalStored := 0
	var lastCursor int64
	var pushID string

	for i := 0; i < len(changed); i += c.maxBatch {
		end := min(i+c.maxBatch, len(changed))
		batch := changed[i:end]

		inputs := make([]apiclient.BlockInput, len(batch))
		for j, ib := range batch {
			inputs[j] = apiclient.BlockInput{
				ID:       ib.block.ID,
				Text:     ib.block.SourceText(),
				Name:     ib.block.Name,
				Type:     ib.block.Type,
				ItemName: ib.itemName,
			}
		}

		resp, err := c.client.Push(ctx, inputs)
		if err != nil {
			return nil, fmt.Errorf("push batch %d: %w", chunkCount+1, err)
		}
		totalStored += resp.Stored
		lastCursor = resp.NewCursor
		if resp.PushID != "" {
			pushID = resp.PushID
		}
		chunkCount++
	}

	// Update cache with per-file hashes.
	for itemName, fileHashes := range hashMap {
		fc, ok := c.cache.Files[itemName]
		if !ok {
			fc = &FileCache{Blocks: map[string]string{}}
			c.cache.Files[itemName] = fc
		}
		for blockID, hash := range fileHashes {
			fc.Blocks[blockID] = hash
		}
	}
	c.cache.SyncCursor = lastCursor
	c.cache.LastSync = time.Now().UTC()
	c.cache.ServerURL = c.project.Config.Server.URL
	c.cache.ProjectID = c.project.Config.Server.ProjectID

	if err := c.cache.Save(c.project.ConfigDir); err != nil {
		return nil, fmt.Errorf("save sync cache: %w", err)
	}

	return &connector.PushResult{
		BlocksPushed: totalStored,
		FilesScanned: len(hashMap),
		ChunkCount:   chunkCount,
		WordCount:    pushWords,
		PushID:       pushID,
	}, nil
}

// Pull retrieves translated content from Bowrain.
func (c *BrainSourceConnector) Pull(ctx context.Context, opts connector.PullOptions) (*connector.PullResult, error) {
	// Default to project's target locales when none specified.
	pullLocales := opts.Locales
	if len(pullLocales) == 0 {
		pullLocales = c.project.Config.Project.TargetLocales
	}
	locales := make([]string, len(pullLocales))
	for i, l := range pullLocales {
		locales[i] = string(l)
	}

	cursor := c.cache.SyncCursor
	if opts.Force {
		cursor = 0
	}

	// Phase 1: Collect changes from the change log.
	totalPulled := 0

	for {
		resp, err := c.client.Pull(ctx, cursor, locales, 1000)
		if err != nil {
			return nil, fmt.Errorf("pull changes: %w", err)
		}

		totalPulled += len(resp.Changes)
		cursor = resp.NewCursor

		if !resp.HasMore {
			break
		}
	}

	if opts.DryRun {
		return &connector.PullResult{
			BlocksPulled: totalPulled,
			LocalesCount: len(locales),
		}, nil
	}

	// Phase 2: For each item with changes, fetch blocks and write translated files.
	filesWritten := 0

	if totalPulled > 0 && len(locales) > 0 {
		// Get all local items by scanning source files.
		hashMap, _, err := c.scanLocalBlocks(ctx, nil)
		if err != nil {
			return nil, fmt.Errorf("scan local blocks: %w", err)
		}

		for itemName := range hashMap {
			// Fetch server blocks with targets for this item.
			blocks, err := c.client.GetBlocks(ctx, itemName)
			if err != nil {
				continue // Skip items that fail.
			}
			if len(blocks) == 0 {
				continue
			}

			// Check if any blocks have targets for our locales.
			hasTargets := false
			for _, b := range blocks {
				if len(b.Targets) > 0 {
					hasTargets = true
					break
				}
			}
			if !hasTargets {
				continue
			}

			// Write a translated file for each target locale.
			for _, loc := range locales {
				// Build target map for this locale.
				targetMap := map[string]string{} // blockID → translated text
				for _, b := range blocks {
					if t, ok := b.Targets[loc]; ok {
						targetMap[b.ID] = t
					}
				}
				if len(targetMap) == 0 {
					continue
				}

				// Determine output path.
				outPath := c.resolveTargetPath(itemName, loc)
				absOut := c.project.ResolvePath(outPath)

				// Read source, inject targets, write output.
				absSource := c.project.ResolvePath(itemName)
				formatName := c.detectFormat(absSource)
				if formatName == "" {
					continue
				}

				if err := c.writeTranslatedFile(ctx, absSource, absOut, formatName, loc, targetMap); err != nil {
					continue
				}
				filesWritten++
			}
		}
	}

	// Update cursor.
	c.cache.SyncCursor = cursor
	c.cache.LastSync = time.Now().UTC()
	if err := c.cache.Save(c.project.ConfigDir); err != nil {
		return nil, fmt.Errorf("save sync cache: %w", err)
	}

	return &connector.PullResult{
		BlocksPulled: totalPulled,
		LocalesCount: len(locales),
		FilesWritten: filesWritten,
	}, nil
}

// scanLocalBlocks walks local source files, reads them with format readers,
// and extracts blocks grouped by item (file path relative to project root).
// Returns itemName→(blockID→hash) and itemName→blocks.
func (c *BrainSourceConnector) scanLocalBlocks(ctx context.Context, paths []string) (map[string]map[string]string, map[string][]*model.Block, error) {
	hashMap := map[string]map[string]string{}
	blockMap := map[string][]*model.Block{}

	// If no specific paths, use mappings to discover files.
	if len(paths) == 0 {
		for _, m := range c.project.Config.Mappings {
			relPaths, err := ExpandGlob(c.project.Root, m.Local, c.project.Config.Exclude...)
			if err != nil {
				continue
			}
			for _, rp := range relPaths {
				paths = append(paths, filepath.Join(c.project.Root, rp))
			}
		}
	}

	if len(paths) == 0 {
		return hashMap, blockMap, nil
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

		relPath, _ := c.project.RelativePath(absPath)
		fileHashes := map[string]string{}
		for _, b := range blocks {
			identity := model.ComputeIdentity(b)
			fileHashes[b.ID] = identity.ContentHash
		}
		hashMap[relPath] = fileHashes
		blockMap[relPath] = blocks
	}

	return hashMap, blockMap, nil
}

// detectFormat determines the format for a file using mappings or the registry.
func (c *BrainSourceConnector) detectFormat(absPath string) string {
	relPath, err := c.project.RelativePath(absPath)
	if err != nil {
		relPath = filepath.Base(absPath)
	}

	// Check mappings first.
	for _, m := range c.project.Config.Mappings {
		matched, err := doublestar.Match(m.Local, relPath)
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
func (c *BrainSourceConnector) readBlocks(ctx context.Context, filePath, formatName string) ([]*model.Block, error) {
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

// resolveTargetPath determines the output path for a translated file.
// It checks mappings for an explicit target_path template, falls back to
// replacing the source locale in the path, or appends the locale as a suffix.
func (c *BrainSourceConnector) resolveTargetPath(itemName, locale string) string {
	relPath := itemName

	// Check if any mapping has a target_path template.
	for _, m := range c.project.Config.Mappings {
		matched, err := doublestar.Match(m.Local, relPath)
		if err == nil && matched && m.TargetPath != "" {
			result := m.TargetPath
			result = strings.ReplaceAll(result, "{locale}", locale)

			// Extract {path} and {filename} relative to the glob's fixed prefix.
			prefix := globFixedPrefix(m.Local)
			relative := strings.TrimPrefix(relPath, prefix)
			dir := filepath.Dir(relative)
			if dir == "." {
				dir = ""
			}
			file := filepath.Base(relative)
			result = strings.ReplaceAll(result, "{filename}", file)
			result = strings.ReplaceAll(result, "{path}", dir)
			// Clean up double slashes from empty {path}.
			for strings.Contains(result, "//") {
				result = strings.ReplaceAll(result, "//", "/")
			}
			return result
		}
	}

	// Default: replace the source locale in the path with the target locale.
	srcLocale := string(c.project.Config.Project.SourceLocale)
	if srcLocale != "" && strings.Contains(relPath, srcLocale) {
		return strings.Replace(relPath, srcLocale, locale, 1)
	}

	// If we cannot determine the target path, put it next to the source with a locale suffix.
	ext := filepath.Ext(relPath)
	base := strings.TrimSuffix(relPath, ext)
	return base + "." + locale + ext
}

// writeTranslatedFile reads a source file, injects target translations into blocks,
// and writes the translated output file using the appropriate format writer.
func (c *BrainSourceConnector) writeTranslatedFile(ctx context.Context, sourcePath, outputPath, formatName, locale string, targets map[string]string) error {
	// Read source.
	reader, err := c.formatReg.NewReader(formatName)
	if err != nil {
		return fmt.Errorf("create reader for %s: %w", formatName, err)
	}

	// Create a shared skeleton store so the writer can reconstruct non-translatable
	// content (frontmatter, structural markup, etc.) byte-exactly from the source.
	skelStore, err := format.NewSkeletonStore()
	if err != nil {
		return fmt.Errorf("create skeleton store: %w", err)
	}
	defer skelStore.Close()

	if emitter, ok := reader.(format.SkeletonStoreEmitter); ok {
		emitter.SetSkeletonStore(skelStore)
	}

	f, err := os.Open(sourcePath)
	if err != nil {
		return fmt.Errorf("open source file %s: %w", sourcePath, err)
	}

	doc := &model.RawDocument{
		URI:      sourcePath,
		FormatID: formatName,
		Reader:   f,
	}
	if err := reader.Open(ctx, doc); err != nil {
		f.Close()
		return fmt.Errorf("open document %s: %w", sourcePath, err)
	}

	// Collect parts, injecting targets.
	var parts []*model.Part
	ch := reader.Read(ctx)
	for pr := range ch {
		if pr.Error != nil {
			continue
		}
		p := pr.Part
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok {
				if t, exists := targets[b.ID]; exists {
					b.SetTargetText(model.LocaleID(locale), t)
				}
			}
		}
		parts = append(parts, p)
	}

	// Write output.
	writer, err := c.formatReg.NewWriter(formatName)
	if err != nil {
		return fmt.Errorf("create writer for %s: %w", formatName, err)
	}

	if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
		consumer.SetSkeletonStore(skelStore)
	}

	// Ensure output directory exists.
	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}

	if err := writer.SetOutput(outputPath); err != nil {
		return fmt.Errorf("set output path: %w", err)
	}
	writer.SetLocale(model.LocaleID(locale))

	outCh := make(chan *model.Part, len(parts))
	for _, p := range parts {
		outCh <- p
	}
	close(outCh)

	if err := writer.Write(ctx, outCh); err != nil {
		return fmt.Errorf("write translated file: %w", err)
	}

	return writer.Close()
}

// globFixedPrefix returns the fixed directory prefix of a glob pattern,
// i.e. everything before the first glob metacharacter (*, ?, [, {).
func globFixedPrefix(pattern string) string {
	for i, c := range pattern {
		if c == '*' || c == '?' || c == '[' || c == '{' {
			// Return everything up to the last path separator before the metachar.
			return pattern[:i]
		}
	}
	// No glob chars — return the directory portion.
	dir := filepath.Dir(pattern)
	if dir == "." {
		return ""
	}
	return dir + string(filepath.Separator)
}

// lookupCachedHashForItem finds a block's cached hash for a specific item.
func (c *BrainSourceConnector) lookupCachedHashForItem(itemName, blockID string) (string, bool) {
	fc, ok := c.cache.Files[itemName]
	if !ok {
		return "", false
	}
	hash, found := fc.Blocks[blockID]
	return hash, found
}

// Client returns the underlying BowrainClient for direct API calls (e.g. PushStatus).
func (c *BrainSourceConnector) Client() *apiclient.BowrainClient {
	return c.client
}

// Ensure BrainSourceConnector implements SourceConnector at compile time.
var _ connector.SourceConnector = (*BrainSourceConnector)(nil)
