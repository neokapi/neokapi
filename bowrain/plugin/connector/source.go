// Package connector provides the bowrain Source connector implementation
// (push/pull/status against a Bowrain server). It is the heavy in-process
// integration layer that turns local files into synced server state.
package connector

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	apiclient "github.com/neokapi/neokapi/bowrain/core/client"
	"github.com/neokapi/neokapi/bowrain/core/config"
	bowrainconn "github.com/neokapi/neokapi/bowrain/core/connector"
	bproject "github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/bowrain/plugin/schema"
	"github.com/neokapi/neokapi/core/editor"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
)

// Project is re-exported so callers can keep typing connector.Project.
// In practice everyone constructs their own *bproject.Project and passes
// it through.
type Project = bproject.Project

// Recipe is re-exported for symmetry.
type Recipe = bproject.Recipe

// SyncCache is re-exported so this package can talk about the cache
// owned by bowrain/core/project.
type SyncCache = bproject.SyncCache

// FileCache is re-exported.
type FileCache = bproject.FileCache

// CachedProjectMeta is re-exported.
type CachedProjectMeta = bproject.CachedProjectMeta

// LoadSyncCache is re-exported.
var LoadSyncCache = bproject.LoadSyncCache

// ResolveStream is re-exported so connector code can stay
// self-contained.
var ResolveStream = bproject.ResolveStream

// formatNameOf returns the format ID from a FormatSpec, or "" when unset.
func formatNameOf(spec *coreproj.FormatSpec) string {
	if spec == nil {
		return ""
	}
	return spec.Name
}

// streamFor returns the recipe's configured stream name (server.stream),
// or "" when the recipe has no server block.
func streamFor(recipe *Recipe) string {
	if recipe == nil || recipe.Server == nil {
		return ""
	}
	return recipe.Server.Stream
}

// BowrainSourceConnector implements bowrainconn.SourceConnector for local bowrain-cli projects.
// It communicates with a Bowrain server via REST API.
type BowrainSourceConnector struct {
	project   *Project
	client    *apiclient.BowrainClient
	formatReg *registry.FormatRegistry
	cache     *SyncCache
	stream    string // resolved stream name
	maxBatch  int    // Max blocks per push request
}

// itemBlock associates a block with its source item name.
type itemBlock struct {
	itemName string
	block    *model.Block
}

// NewSourceConnector creates a SourceConnector for the given project.
func NewSourceConnector(project *Project, formatReg *registry.FormatRegistry) (*BowrainSourceConnector, error) {
	recipe := project.Recipe
	if !recipe.HasServer() {
		return nil, errors.New("no server configuration in the kapi recipe (add a `server:` block)")
	}

	serverURL := recipe.Server.ServerURL()
	projectID := recipe.Server.ProjectID()
	workspace := recipe.Server.Workspace()

	if serverURL == "" {
		return nil, errors.New("server URL not configured in the recipe's `server:` block")
	}
	if projectID == "" {
		return nil, errors.New("server project ID not configured in the recipe's `server:` block")
	}

	cache := LoadSyncCache(project.Layout)

	var client *apiclient.BowrainClient
	switch {
	case cache.ClaimToken != "":
		client = apiclient.NewClaimTokenClient(serverURL, projectID, cache.ClaimToken)
	case workspace != "":
		authInfo, err := config.LoadAuth()
		if err != nil {
			return nil, errors.New("workspace sync requires authentication: run 'kapi auth login'")
		}
		if authInfo.ServerURL != "" && authInfo.ServerURL != serverURL {
			return nil, fmt.Errorf("auth token is for %s but project points to %s", authInfo.ServerURL, serverURL)
		}
		client = apiclient.NewWorkspaceBowrainClient(serverURL, workspace, projectID, authInfo.AccessToken)
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
			return nil, errors.New("server config requires either workspace or claim_token, or set BOWRAIN_AUTH_TOKEN")
		}
		// Detect claim tokens (clm_ prefix) and route them correctly.
		if strings.HasPrefix(authInfo.AccessToken, "clm_") {
			client = apiclient.NewClaimTokenClient(serverURL, projectID, authInfo.AccessToken)
		} else {
			// Use flat project routes with bearer auth (works for API tokens and JWTs).
			client = apiclient.NewProjectBearerClient(serverURL, projectID, authInfo.AccessToken)
		}
	}

	// Resolve the active stream and set it on the client.
	stream := ResolveStream("", streamFor(recipe))
	client.SetStream(stream)

	return &BowrainSourceConnector{
		project:   project,
		client:    client,
		formatReg: formatReg,
		cache:     cache,
		stream:    stream,
		maxBatch:  1000,
	}, nil
}

// NewLocalConnector creates a BowrainSourceConnector for local-only operations
// (listing files, scanning blocks) without requiring a server connection.
func NewLocalConnector(project *Project, formatReg *registry.FormatRegistry) *BowrainSourceConnector {
	cache := LoadSyncCache(project.Layout)
	return &BowrainSourceConnector{
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
func (c *BowrainSourceConnector) ListFiles(ctx context.Context, paths []string) ([]FileInfo, error) {
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

	slices.SortFunc(files, func(a, b FileInfo) int {
		return cmp.Compare(a.Path, b.Path)
	})
	return files, nil
}

// ID returns the connector identifier.
func (c *BowrainSourceConnector) ID() string {
	return "bowrain-source"
}

// Name returns a human-readable name.
func (c *BowrainSourceConnector) Name() string {
	return "Bowrain Local Source"
}

// Category returns the connector category.
func (c *BowrainSourceConnector) Category() bowrainconn.Category {
	return bowrainconn.CategoryFile
}

// Status reports the sync state.
func (c *BowrainSourceConnector) Status(ctx context.Context) (*bowrainconn.SyncStatus, error) {
	// Count local changes by scanning files and comparing to cache.
	hashMap, blockMap, err := c.scanLocalBlocks(ctx, nil)
	if err != nil {
		return &bowrainconn.SyncStatus{
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
	if c.cache.GetStreamCursor(c.stream) > 0 {
		resp, err := c.client.Pull(ctx, c.cache.GetStreamCursor(c.stream), nil, 1)
		if err == nil && len(resp.Blocks) > 0 {
			pendingPull = len(resp.Blocks)
			if resp.HasMore {
				pendingPull = -1 // Unknown, but there are more
			}
		}
	}

	return &bowrainconn.SyncStatus{
		ConnectorID: c.ID(),
		LastSync:    c.cache.LastSync,
		ItemCount:   totalBlocks,
		FileCount:   len(hashMap),
		WordCount:   totalWords,
		PendingPush: pendingPush,
		PendingPull: pendingPull,
	}, nil
}

// Configure is a no-op for the Bowrain CLI source connector (configured via the kapi recipe).
func (c *BowrainSourceConnector) Configure(config map[string]string) error {
	return nil
}

// Close saves the sync cache.
func (c *BowrainSourceConnector) Close() error {
	return c.cache.Save(c.project.Layout)
}

// Push sends source content from local files to Bowrain.
func (c *BowrainSourceConnector) Push(ctx context.Context, opts bowrainconn.PushOptions) (*bowrainconn.PushResult, error) {
	// Scan local files and extract blocks and media grouped by item.
	hashMap, blockMap, mediaHashMap, mediaMap, err := c.scanLocalBlocksAndMedia(ctx, opts.Paths)
	if err != nil {
		return nil, fmt.Errorf("scan local files: %w", err)
	}
	_, _ = mediaHashMap, mediaMap // used below after block push

	// Diff against cache to find changed blocks, keeping item association.
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
		return &bowrainconn.PushResult{
			BlocksPushed: len(changed),
			FilesScanned: len(hashMap),
			WordCount:    pushWords,
		}, nil
	}

	if len(changed) == 0 {
		// Verify server still has our data. If the server was reset/rebuilt,
		// the cached cursor will be stale and we need to force re-push.
		if c.client != nil && len(blockMap) > 0 {
			cursor := c.cache.GetStreamCursor(c.stream)
			if cursor > 0 {
				// Quick probe: pull with cursor=0, limit=1 to check if server has data.
				resp, err := c.client.Pull(ctx, 0, nil, 1)
				if err == nil && resp.Cursor == 0 {
					// Server is empty but cache says we've pushed — re-push everything.
					for itemName, blocks := range blockMap {
						for _, b := range blocks {
							changed = append(changed, itemBlock{itemName: itemName, block: b})
						}
					}
				}
			}
		}

		if len(changed) == 0 {
			return &bowrainconn.PushResult{FilesScanned: len(hashMap)}, nil
		}
	}

	// Generate per-item editor metadata (BlockIndex + PreviewHTML) for changed items.
	itemMeta := c.buildItemMeta(ctx, changed)

	// Group changed blocks by item for the push API.
	blocksByItem := map[string][]*model.Block{}
	for _, ib := range changed {
		blocksByItem[ib.itemName] = append(blocksByItem[ib.itemName], ib.block)
	}

	// Push via init → diff → chunk → commit flow.
	resp, err := c.client.Push(ctx, blocksByItem, itemMeta)
	if err != nil {
		return nil, fmt.Errorf("push: %w", err)
	}
	totalStored := len(changed)
	var lastCursor int64
	pushID := ""
	if resp != nil {
		lastCursor = resp.NewCursor
		pushID = resp.PushID
	}

	// Fetch and cache server metadata (best-effort).
	c.fetchAndCacheMetadata(ctx)

	// Push assets (Bowrain AD-007): upload changed media to blob storage.
	assetsPushed := 0
	if c.client != nil && len(mediaMap) > 0 {
		assetsPushed = c.pushAssets(ctx, mediaHashMap, mediaMap, opts.Force)
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
	// Update cache with per-file asset hashes.
	for itemName, assetHashes := range mediaHashMap {
		fc, ok := c.cache.Files[itemName]
		if !ok {
			fc = &FileCache{Blocks: map[string]string{}, Assets: map[string]string{}}
			c.cache.Files[itemName] = fc
		}
		if fc.Assets == nil {
			fc.Assets = map[string]string{}
		}
		for sourceID, blobKey := range assetHashes {
			fc.Assets[sourceID] = blobKey
		}
	}
	c.cache.SetStreamCursor(c.stream, lastCursor)
	c.cache.LastSync = time.Now().UTC()
	c.cache.ServerURL = c.project.Recipe.Server.ServerURL()
	c.cache.ProjectID = c.project.Recipe.Server.ProjectID()

	if err := c.cache.Save(c.project.Layout); err != nil {
		return nil, fmt.Errorf("save sync cache: %w", err)
	}

	return &bowrainconn.PushResult{
		BlocksPushed: totalStored,
		AssetsPushed: assetsPushed,
		FilesScanned: len(hashMap),
		WordCount:    pushWords,
		PushID:       pushID,
	}, nil
}

// pushAssets uploads changed media assets to the server.
// For each asset: checks cache → requests upload URL → uploads blob → registers metadata.
func (c *BowrainSourceConnector) pushAssets(
	ctx context.Context,
	mediaHashMap map[string]map[string]string,
	mediaMap map[string][]*model.Media,
	force bool,
) int {
	pushed := 0
	for itemName, mediaList := range mediaMap {
		cachedAssets := map[string]string{}
		if fc, ok := c.cache.Files[itemName]; ok && fc.Assets != nil {
			cachedAssets = fc.Assets
		}

		for _, m := range mediaList {
			// Skip unchanged assets.
			if !force {
				if cachedKey, ok := cachedAssets[m.ID]; ok && cachedKey == m.BlobKey {
					continue
				}
			}

			// Request upload URL (dedup check).
			urlResp, err := c.client.GetAssetUploadURL(ctx, m.BlobKey, m.MimeType, m.Size)
			if err != nil {
				continue // best-effort
			}

			// If blob doesn't exist yet and we have a SAS URL, upload directly.
			// For local backend (no SAS URL), the server proxies the upload
			// via the PushAsset metadata call — the blob was already uploaded
			// inline if the server supports it, or the upload-url response
			// indicates the blob exists (dedup).
			if !urlResp.Exists && urlResp.UploadURL != "" {
				// Direct upload to blob storage via SAS URL.
				if err := uploadToSASURL(ctx, urlResp.UploadURL, m.Data, m.MimeType); err != nil {
					continue // best-effort
				}
			}

			// Register asset metadata on the server.
			_, err = c.client.PushAsset(ctx, apiclient.AssetInput{
				BlobKey:    m.BlobKey,
				ItemName:   itemName,
				SourceID:   m.ID,
				MimeType:   m.MimeType,
				Filename:   m.Filename,
				SizeBytes:  m.Size,
				AltText:    m.AltText,
				Properties: m.Properties,
			})
			if err != nil {
				continue // best-effort
			}
			pushed++
		}
	}
	return pushed
}

// uploadToSASURL uploads data directly to a pre-signed Azure SAS URL.
func uploadToSASURL(ctx context.Context, sasURL string, data []byte, contentType string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, sasURL, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", contentType)
	req.Header.Set("x-ms-blob-type", "BlockBlob")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("SAS upload failed: HTTP %d", resp.StatusCode)
	}
	return nil
}

// Pull retrieves translated content from Bowrain.
func (c *BowrainSourceConnector) Pull(ctx context.Context, opts bowrainconn.PullOptions) (*bowrainconn.PullResult, error) {
	// Refresh server metadata so we have up-to-date target locales.
	c.fetchAndCacheMetadata(ctx)

	// Resolve target locales: CLI args > config > server cache.
	pullLocales := opts.Locales
	if len(pullLocales) == 0 {
		pullLocales = c.project.Recipe.Defaults.TargetLanguages
	}
	if len(pullLocales) == 0 {
		pullLocales = c.ServerTargetLocales()
	}
	locales := make([]string, len(pullLocales))
	for i, l := range pullLocales {
		locales[i] = string(l)
	}

	cursor := c.cache.GetStreamCursor(c.stream)
	if opts.Force {
		cursor = 0
	}

	// Pull blocks directly from the server using the rich pull response.
	// Each response includes full SyncBlock records with structured segments.
	totalPulled := 0

	// Collect all blocks across paginated responses.
	var allBlocks []apiclient.SyncBlock

	for {
		resp, err := c.client.Pull(ctx, cursor, locales, 1000)
		if err != nil {
			return nil, fmt.Errorf("pull: %w", err)
		}

		totalPulled += len(resp.Blocks)
		allBlocks = append(allBlocks, resp.Blocks...)
		cursor = resp.Cursor

		if !resp.HasMore {
			break
		}
	}

	if opts.DryRun {
		return &bowrainconn.PullResult{
			BlocksPulled: totalPulled,
			LocalesCount: len(locales),
		}, nil
	}

	// Group pulled blocks by item name.
	filesWritten := 0

	if len(allBlocks) > 0 && len(locales) > 0 {
		blocksByItem := map[string][]apiclient.SyncBlock{}
		for _, b := range allBlocks {
			blocksByItem[b.ItemName] = append(blocksByItem[b.ItemName], b)
		}

		for itemName, blocks := range blocksByItem {
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
				// Build target map for this locale from structured segments.
				// Keyed by a stable match key (not the server block ID, which is
				// not preserved across push/pull) so writeTranslatedFile can match
				// the freshly re-parsed source blocks.
				targetMap := map[string]string{} // matchKey → translated text
				for _, b := range blocks {
					if segs, ok := b.Targets[loc]; ok {
						// Extract plain text from segments (flatten
						// TextRuns only — inline codes and structured
						// runs contribute nothing at export time).
						var textSb strings.Builder
						for _, seg := range segs {
							for _, r := range seg.Runs {
								if r.Text != nil {
									textSb.WriteString(r.Text.Text)
								}
							}
						}
						if text := textSb.String(); text != "" {
							targetMap[targetMatchKey(b.Name, b.SourceText)] = text
						}
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

				// Fetch locale-variant media for this item (Bowrain AD-007).
				var mediaRepl []MediaReplacement
				if c.project.Recipe.AssetsEnabled() {
					mediaRepl = c.fetchMediaReplacements(ctx, itemName, loc)
				}

				if err := c.writeTranslatedFile(ctx, absSource, absOut, formatName, loc, targetMap, mediaRepl...); err != nil {
					continue
				}
				filesWritten++
			}
		}
	}

	// Update cursor.
	c.cache.SetStreamCursor(c.stream, cursor)
	c.cache.LastSync = time.Now().UTC()
	if err := c.cache.Save(c.project.Layout); err != nil {
		return nil, fmt.Errorf("save sync cache: %w", err)
	}

	return &bowrainconn.PullResult{
		BlocksPulled: totalPulled,
		LocalesCount: len(locales),
		FilesWritten: filesWritten,
	}, nil
}

// scanLocalBlocks walks local source files, reads them with format readers,
// and extracts blocks grouped by item (file path relative to project root).
// Returns itemName→(blockID→hash) and itemName→blocks.
func (c *BowrainSourceConnector) scanLocalBlocks(ctx context.Context, paths []string) (map[string]map[string]string, map[string][]*model.Block, error) {
	hashMap, blockMap, _, _, err := c.scanLocalBlocksAndMedia(ctx, paths)
	return hashMap, blockMap, err
}

// scanLocalBlocksAndMedia extracts both blocks and media from local files.
// Returns block hashes, blocks, media hashes (sourceID→blobKey), and media grouped by item.
func (c *BowrainSourceConnector) scanLocalBlocksAndMedia(ctx context.Context, paths []string) (
	map[string]map[string]string, map[string][]*model.Block,
	map[string]map[string]string, map[string][]*model.Media, error,
) {
	hashMap := map[string]map[string]string{}
	blockMap := map[string][]*model.Block{}
	mediaHashMap := map[string]map[string]string{}
	mediaMap := map[string][]*model.Media{}

	recipe := c.project.Recipe
	assetsEnabled := recipe.AssetsEnabled()

	// If no specific paths, use content entries to discover files.
	if len(paths) == 0 {
		for _, it := range recipe.IterateContent() {
			lang := string(it.Item.ResolvedSourceLanguage(it.Collection, recipe.Defaults))
			pattern := coreproj.ResolvePathPattern(it.Item.Path, lang)
			relPaths, err := coreproj.ExpandGlob(c.project.Root, pattern, recipe.Defaults.Exclude...)
			if err != nil {
				continue
			}
			for _, rp := range relPaths {
				paths = append(paths, filepath.Join(c.project.Root, rp))
			}
		}
	}

	if len(paths) == 0 {
		return hashMap, blockMap, mediaHashMap, mediaMap, nil
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

		relPath, _ := c.project.RelativePath(absPath)

		// Extract blocks and optionally media.
		if assetsEnabled {
			blocks, media, err := c.readBlocksAndMedia(ctx, absPath, formatName)
			if err != nil {
				continue
			}

			fileHashes := map[string]string{}
			for _, b := range blocks {
				identity := model.ComputeIdentity(b)
				fileHashes[b.ID] = identity.ContentHash
			}
			hashMap[relPath] = fileHashes
			blockMap[relPath] = blocks

			if len(media) > 0 {
				assetHashes := map[string]string{}
				for _, m := range media {
					assetHashes[m.ID] = m.BlobKey
				}
				mediaHashMap[relPath] = assetHashes
				mediaMap[relPath] = media
			}
		} else {
			blocks, err := c.readBlocks(ctx, absPath, formatName)
			if err != nil {
				continue
			}

			fileHashes := map[string]string{}
			for _, b := range blocks {
				identity := model.ComputeIdentity(b)
				fileHashes[b.ID] = identity.ContentHash
			}
			hashMap[relPath] = fileHashes
			blockMap[relPath] = blocks
		}
	}

	return hashMap, blockMap, mediaHashMap, mediaMap, nil
}

// detectFormat determines the format for a file using mappings or the registry.
func (c *BowrainSourceConnector) detectFormat(absPath string) string {
	relPath, err := c.project.RelativePath(absPath)
	if err != nil {
		relPath = filepath.Base(absPath)
	}

	recipe := c.project.Recipe

	// Check content entries first.
	for _, it := range recipe.IterateContent() {
		lang := string(it.Item.ResolvedSourceLanguage(it.Collection, recipe.Defaults))
		pattern := coreproj.ResolvePathPattern(it.Item.Path, lang)
		formatName := coreproj.ResolveFormat(formatNameOf(it.Item.Format))
		matched, err := doublestar.Match(pattern, relPath)
		if err == nil && matched && formatName != "" {
			return formatName
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

// buildItemMeta generates editor metadata (BlockIndex + PreviewHTML) for each
// unique item that has changed blocks. It re-parses the source files using
// editor.ParseItem to build the full Part stream needed for metadata generation.
func (c *BowrainSourceConnector) buildItemMeta(ctx context.Context, changed []itemBlock) []apiclient.ItemMeta {
	// Collect unique item names that have changes.
	seen := map[string]bool{}
	var itemNames []string
	for _, ib := range changed {
		if ib.itemName != "" && !seen[ib.itemName] {
			seen[ib.itemName] = true
			itemNames = append(itemNames, ib.itemName)
		}
	}

	sourceLocale := string(c.project.Recipe.Defaults.SourceLanguage)
	var meta []apiclient.ItemMeta

	for _, itemName := range itemNames {
		absPath := c.project.ResolvePath(filepath.Join(c.project.Root, itemName))
		formatName := c.detectFormat(absPath)
		if formatName == "" {
			continue
		}

		reader, err := c.formatReg.NewReader(registry.FormatID(formatName))
		if err != nil {
			continue
		}

		f, err := os.Open(absPath)
		if err != nil {
			continue
		}

		doc := &model.RawDocument{
			URI:      absPath,
			FormatID: formatName,
			Reader:   f,
		}

		result, err := editor.ParseItem(ctx, reader, doc, sourceLocale, formatName, itemName)
		if err != nil {
			continue
		}

		meta = append(meta, apiclient.ItemMeta{
			Name:        itemName,
			Format:      formatName,
			BlockIndex:  result.BlockIndexJSON,
			PreviewHTML: result.PreviewHTML,
		})
	}

	return meta
}

// readBlocks reads a file and extracts blocks using the format reader.
func (c *BowrainSourceConnector) readBlocks(ctx context.Context, filePath, formatName string) ([]*model.Block, error) {
	blocks, _, err := c.readBlocksAndMedia(ctx, filePath, formatName)
	return blocks, err
}

// readBlocksAndMedia reads a file and extracts both blocks and media using the format reader.
func (c *BowrainSourceConnector) readBlocksAndMedia(ctx context.Context, filePath, formatName string) ([]*model.Block, []*model.Media, error) {
	reader, err := c.formatReg.NewReader(registry.FormatID(formatName))
	if err != nil {
		return nil, nil, fmt.Errorf("create reader for %s: %w", formatName, err)
	}

	// Enable media extraction if the format supports it.
	if cfg := reader.Config(); cfg != nil {
		_ = cfg.ApplyMap(map[string]any{"extractMedia": true})
	}

	f, err := os.Open(filePath)
	if err != nil {
		return nil, nil, fmt.Errorf("open file %s: %w", filePath, err)
	}

	doc := &model.RawDocument{
		URI:      filePath,
		FormatID: formatName,
		Reader:   f,
	}

	if err := reader.Open(ctx, doc); err != nil {
		f.Close()
		return nil, nil, fmt.Errorf("open document %s: %w", filePath, err)
	}

	var blocks []*model.Block
	var media []*model.Media
	ch := reader.Read(ctx)
	for pr := range ch {
		if pr.Error != nil {
			continue
		}
		switch pr.Part.Type {
		case model.PartBlock:
			if b, ok := pr.Part.Resource.(*model.Block); ok {
				blocks = append(blocks, b)
			}
		case model.PartMedia:
			if m, ok := pr.Part.Resource.(*model.Media); ok {
				media = append(media, m)
			}
		}
	}

	return blocks, media, nil
}

// resolveTargetPath determines the output path for a translated file.
// It checks content entries for a target pattern (which may contain {lang} or
// {locale}/{path}/{filename} placeholders), falls back to replacing the source
// locale in the path, or appends the locale as a suffix.
func (c *BowrainSourceConnector) resolveTargetPath(itemName, locale string) string {
	relPath := itemName
	recipe := c.project.Recipe
	srcLang := string(recipe.Defaults.SourceLanguage)

	// Check if any content entry has a target pattern.
	for _, it := range recipe.IterateContent() {
		entryLang := string(it.Item.ResolvedSourceLanguage(it.Collection, recipe.Defaults))
		pattern := coreproj.ResolvePathPattern(it.Item.Path, entryLang)
		matched, err := doublestar.Match(pattern, relPath)
		if err != nil || !matched {
			continue
		}

		dest := it.Item.Target
		if dest == "" {
			break // No target — fall through to default behavior.
		}

		// If dest contains {lang}, expand it with the target locale.
		if strings.Contains(dest, "{lang}") {
			// Derive the relative portion by comparing against the source pattern.
			// e.g. path: src/{lang}/**/*.json, relPath: src/en/foo/bar.json
			// We need to reconstruct: src/{locale}/foo/bar.json
			srcPattern := coreproj.ResolvePathPattern(it.Item.Path, srcLang)
			prefix := globFixedPrefix(srcPattern)
			relative := strings.TrimPrefix(relPath, prefix)
			destPrefix := coreproj.ResolvePathPattern(globFixedPrefix(it.Item.Target), locale)
			result := destPrefix + relative
			return result
		}

		// Legacy-style dest with {locale}/{path}/{filename} placeholders.
		result := dest
		result = strings.ReplaceAll(result, "{locale}", locale)

		prefix := globFixedPrefix(pattern)
		relative := strings.TrimPrefix(relPath, prefix)
		dir := filepath.Dir(relative)
		if dir == "." {
			dir = ""
		}
		file := filepath.Base(relative)
		result = strings.ReplaceAll(result, "{filename}", file)
		result = strings.ReplaceAll(result, "{path}", dir)
		for strings.Contains(result, "//") {
			result = strings.ReplaceAll(result, "//", "/")
		}
		return result
	}

	// Default: replace the source locale in the path with the target locale.
	if srcLang != "" && strings.Contains(relPath, srcLang) {
		return strings.Replace(relPath, srcLang, locale, 1)
	}

	// If we cannot determine the target path, put it next to the source with a locale suffix.
	ext := filepath.Ext(relPath)
	base := strings.TrimSuffix(relPath, ext)
	return base + "." + locale + ext
}

// writeTranslatedFile reads a source file, injects target translations into blocks,
// and writes the translated output file using the appropriate format writer.
// MediaReplacement describes a locale-variant media file to substitute in the output.
type MediaReplacement struct {
	ZipPath string // original ZIP entry path (e.g., "word/media/image1.png")
	Data    []byte // locale-variant binary content
}

// MediaReplacementSetter is implemented by writers that support locale-variant media substitution.
type MediaReplacementSetter interface {
	SetMediaReplacement(zipPath string, data []byte)
}

// targetMatchKey computes a stable key for matching a pulled translation to a
// locally re-parsed source block. Server-assigned block IDs are not preserved
// across push/pull, so matching on b.ID fails (translations would never land).
// Keyed catalogs (JSON, YAML, .strings, …) carry a stable block name; nameless
// prose blocks fall back to source-text identity.
func targetMatchKey(name, sourceText string) string {
	if name != "" {
		return "n:" + name
	}
	return "s:" + sourceText
}

func (c *BowrainSourceConnector) writeTranslatedFile(ctx context.Context, sourcePath, outputPath, formatName, locale string, targets map[string]string, mediaReplacements ...MediaReplacement) error {
	// Read source.
	reader, err := c.formatReg.NewReader(registry.FormatID(formatName))
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
				if t, exists := targets[targetMatchKey(b.Name, b.SourceText())]; exists {
					b.SetTargetText(model.LocaleID(locale), t)
				}
			}
		}
		parts = append(parts, p)
	}

	// Write output.
	writer, err := c.formatReg.NewWriter(registry.FormatID(formatName))
	if err != nil {
		return fmt.Errorf("create writer for %s: %w", formatName, err)
	}

	if consumer, ok := writer.(format.SkeletonStoreConsumer); ok {
		consumer.SetSkeletonStore(skelStore)
	}

	// For formats that need the original file content for reconstruction
	// (openxml, idml, odf, epub — ZIP-based formats that rebuild from original).
	if sps, ok := writer.(format.SourcePathSetter); ok {
		sps.SetSourcePath(sourcePath)
	} else if ocs, ok := writer.(format.OriginalContentSetter); ok {
		srcBytes, err := os.ReadFile(sourcePath)
		if err != nil {
			return fmt.Errorf("read source for writer: %w", err)
		}
		ocs.SetOriginalContent(srcBytes)
	}

	// Apply locale-variant media replacements (Bowrain AD-007).
	if mrs, ok := writer.(MediaReplacementSetter); ok {
		for _, mr := range mediaReplacements {
			mrs.SetMediaReplacement(mr.ZipPath, mr.Data)
		}
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

// fetchMediaReplacements downloads approved locale-variant media files for a given
// item and locale, returning them as MediaReplacement entries for the writer.
func (c *BowrainSourceConnector) fetchMediaReplacements(ctx context.Context, itemName, locale string) []MediaReplacement {
	if c.client == nil {
		return nil
	}

	// Fetch assets for this item.
	assets, err := c.client.ListAssets(ctx, itemName)
	if err != nil || len(assets) == 0 {
		return nil
	}

	var replacements []MediaReplacement
	for _, asset := range assets {
		// Fetch variants for this asset.
		variants, err := c.client.ListAssetVariants(ctx, asset.ID)
		if err != nil {
			continue
		}

		for _, v := range variants {
			if v.Locale != locale || v.Status != "approved" {
				continue
			}
			if v.DownloadURL == "" {
				continue
			}

			// Download the variant binary.
			data, err := c.client.DownloadBlob(ctx, v.DownloadURL)
			if err != nil {
				continue
			}

			// Use the asset's zipPath property if available, otherwise
			// fall back to sourceID which contains the ZIP path.
			zipPath := strings.TrimPrefix(asset.SourceID, "media:")

			replacements = append(replacements, MediaReplacement{
				ZipPath: zipPath,
				Data:    data,
			})
		}
	}

	return replacements
}

// fetchAndCacheMetadata fetches project metadata from the server and caches it.
// Errors are non-fatal — metadata is best-effort.
func (c *BowrainSourceConnector) fetchAndCacheMetadata(ctx context.Context) {
	if c.client == nil {
		return
	}

	meta, err := c.client.GetProjectMetadata(ctx)
	if err != nil {
		return
	}

	c.cache.ServerMeta = &CachedProjectMeta{
		TargetLanguages: meta.TargetLanguages,
		FetchedAt:       time.Now().UTC(),
	}
}

// ServerTargetLocales returns target locales from the server cache.
// Returns nil if no cached metadata is available.
func (c *BowrainSourceConnector) ServerTargetLocales() []model.LocaleID {
	if c.cache.ServerMeta == nil || len(c.cache.ServerMeta.TargetLanguages) == 0 {
		return nil
	}
	locales := make([]model.LocaleID, len(c.cache.ServerMeta.TargetLanguages))
	for i, l := range c.cache.ServerMeta.TargetLanguages {
		locales[i] = model.LocaleID(l)
	}
	return locales
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
func (c *BowrainSourceConnector) lookupCachedHashForItem(itemName, blockID string) (string, bool) {
	fc, ok := c.cache.Files[itemName]
	if !ok {
		return "", false
	}
	hash, found := fc.Blocks[blockID]
	return hash, found
}

// Stream returns the resolved stream name for this connector.
func (c *BowrainSourceConnector) Stream() string {
	return c.stream
}

// SetStream overrides the resolved stream (e.g. from --stream flag).
func (c *BowrainSourceConnector) SetStream(stream string) {
	c.stream = schema.NormalizeStreamName(stream)
	if c.client != nil {
		c.client.SetStream(c.stream)
	}
}

// Client returns the underlying BowrainClient for direct API calls (e.g. PushStatus).
func (c *BowrainSourceConnector) Client() *apiclient.BowrainClient {
	return c.client
}

// Ensure BowrainSourceConnector implements SourceConnector at compile time.
var _ bowrainconn.SourceConnector = (*BowrainSourceConnector)(nil)
