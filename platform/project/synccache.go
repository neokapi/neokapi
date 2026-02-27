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
// It is stored in .brain/.sync-cache and is gitignored.
type SyncCache struct {
	ServerURL  string                `json:"server_url"`
	ProjectID  string                `json:"project_id"`
	SyncCursor int64                 `json:"sync_cursor"`
	LastSync   time.Time             `json:"last_sync"`
	Files      map[string]*FileCache `json:"files,omitempty"`
}

// FileCache tracks the last known hashes for blocks in a file.
type FileCache struct {
	Mtime  time.Time         `json:"mtime"`
	Size   int64             `json:"size"`
	Blocks map[string]string `json:"blocks"` // blockID → contentHash
}

// LoadSyncCache loads the sync cache from .brain/.sync-cache.
// Returns an empty cache if the file doesn't exist or is corrupt.
func LoadSyncCache(configDir string) *SyncCache {
	data, err := os.ReadFile(filepath.Join(configDir, syncCacheFile))
	if err != nil {
		return &SyncCache{Files: map[string]*FileCache{}}
	}

	var cache SyncCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return &SyncCache{Files: map[string]*FileCache{}}
	}
	if cache.Files == nil {
		cache.Files = map[string]*FileCache{}
	}
	return &cache
}

// Save persists the sync cache to .brain/.sync-cache.
func (c *SyncCache) Save(configDir string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal sync cache: %w", err)
	}
	return os.WriteFile(filepath.Join(configDir, syncCacheFile), data, 0644)
}
