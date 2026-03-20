package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const syncCacheFile = ".sync-cache"

// SyncCache tracks the last known server state for incremental sync.
// It is stored in .bowrain/.sync-cache and is gitignored.
type SyncCache struct {
	ServerURL  string                `json:"server_url"`
	ProjectID  string                `json:"project_id"`
	SyncCursor int64                 `json:"sync_cursor"` // deprecated: use StreamCursors
	LastSync   time.Time             `json:"last_sync"`
	Files      map[string]*FileCache `json:"files,omitempty"`

	// StreamCursors tracks per-stream sync cursors.
	// Key is stream name (e.g., "main", "feature/new-ui").
	StreamCursors map[string]int64 `json:"stream_cursors,omitempty"`

	// ActiveStream is the last active stream name used for sync.
	ActiveStream string `json:"active_stream,omitempty"`

	// ClaimToken stores the claim token for anonymous projects.
	// Stored here (gitignored) rather than in config.yaml to avoid
	// accidentally committing credentials to version control.
	ClaimToken string `json:"claim_token,omitempty"`

	// ServerMeta caches project metadata fetched from the server.
	// Updated on each push/pull to keep target languages in sync.
	ServerMeta *CachedProjectMeta `json:"server_meta,omitempty"`
}

// CachedProjectMeta caches server-side project metadata locally.
type CachedProjectMeta struct {
	TargetLanguages []string  `json:"target_languages,omitempty"`
	FetchedAt       time.Time `json:"fetched_at"`
}

// FileCache tracks the last known hashes for blocks and assets in a file.
type FileCache struct {
	Mtime  time.Time         `json:"mtime"`
	Size   int64             `json:"size"`
	Blocks map[string]string `json:"blocks"`           // blockID → contentHash
	Assets map[string]string `json:"assets,omitempty"` // sourceID → blobKey (SHA-256)
}

// GetStreamCursor returns the sync cursor for a specific stream.
// Falls back to the legacy SyncCursor field for backwards compatibility.
func (c *SyncCache) GetStreamCursor(stream string) int64 {
	if stream == "" {
		stream = "main"
	}
	if c.StreamCursors != nil {
		if cursor, ok := c.StreamCursors[stream]; ok {
			return cursor
		}
	}
	// Fall back to legacy field for "main" stream.
	if stream == "main" {
		return c.SyncCursor
	}
	return 0
}

// SetStreamCursor updates the sync cursor for a specific stream.
func (c *SyncCache) SetStreamCursor(stream string, cursor int64) {
	if stream == "" {
		stream = "main"
	}
	if c.StreamCursors == nil {
		c.StreamCursors = map[string]int64{}
	}
	c.StreamCursors[stream] = cursor
	// Also update legacy field for backwards compatibility.
	if stream == "main" {
		c.SyncCursor = cursor
	}
}

// LoadSyncCache loads the sync cache from .bowrain/.sync-cache.
// Returns an empty cache if the file doesn't exist or is corrupt.
func LoadSyncCache(configDir string) *SyncCache {
	data, err := os.ReadFile(filepath.Join(configDir, syncCacheFile))
	if err != nil {
		return &SyncCache{Files: map[string]*FileCache{}, StreamCursors: map[string]int64{}}
	}

	var cache SyncCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return &SyncCache{Files: map[string]*FileCache{}, StreamCursors: map[string]int64{}}
	}
	if cache.Files == nil {
		cache.Files = map[string]*FileCache{}
	}
	if cache.StreamCursors == nil {
		cache.StreamCursors = map[string]int64{}
	}
	// Migrate legacy SyncCursor to StreamCursors.
	if cache.SyncCursor > 0 {
		if _, ok := cache.StreamCursors["main"]; !ok {
			cache.StreamCursors["main"] = cache.SyncCursor
		}
	}
	return &cache
}

// Save persists the sync cache to .bowrain/.sync-cache.
func (c *SyncCache) Save(configDir string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sync cache: %w", err)
	}
	return os.WriteFile(filepath.Join(configDir, syncCacheFile), data, 0644)
}
