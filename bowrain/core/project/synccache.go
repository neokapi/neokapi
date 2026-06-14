package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	coreproj "github.com/neokapi/neokapi/core/project"
)

// SyncCacheFilename is the file written under <state-dir>/cache/ that tracks
// the last known server state for incremental bowrain sync.
const SyncCacheFilename = "sync-cache.json"

// SyncCache tracks the last known server state for incremental sync. It
// lives at <state-dir>/cache/sync-cache.json (always gitignored — the file
// holds claim tokens and is regenerable from server state).
type SyncCache struct {
	ServerURL string                `json:"server_url"`
	ProjectID string                `json:"project_id"`
	LastSync  time.Time             `json:"last_sync"`
	Files     map[string]*FileCache `json:"files,omitempty"`

	// StreamCursors tracks per-stream sync cursors keyed by stream name.
	StreamCursors map[string]int64 `json:"stream_cursors,omitempty"`

	// ActiveStream is the last stream name used for sync.
	ActiveStream string `json:"active_stream,omitempty"`

	// ClaimToken stores the claim token for anonymous projects. Kept in the
	// cache (not the recipe) to avoid committing credentials to git.
	ClaimToken string `json:"claim_token,omitempty"`

	// ServerMeta caches project metadata fetched from the server.
	ServerMeta *CachedProjectMeta `json:"server_meta,omitempty"`

	// ConceptBaseline snapshots the governed concepts and relations a concept
	// pull last wrote into the project's bound termbase, so a later concept push
	// can diff local termbase edits against what was pulled (ordinary edits go
	// up directly, governed edits become a reviewed change-set). It is
	// regenerable — every pull refreshes it.
	ConceptBaseline *ConceptBaseline `json:"concept_baseline,omitempty"`
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

// GetStreamCursor returns the cursor for a specific stream (defaulting to
// "main" when stream is empty), or 0 when the stream has not been synced.
func (c *SyncCache) GetStreamCursor(stream string) int64 {
	if stream == "" {
		stream = StreamMain
	}
	if c.StreamCursors != nil {
		if cursor, ok := c.StreamCursors[stream]; ok {
			return cursor
		}
	}
	return 0
}

// SetStreamCursor updates the cursor for a specific stream.
func (c *SyncCache) SetStreamCursor(stream string, cursor int64) {
	if stream == "" {
		stream = StreamMain
	}
	if c.StreamCursors == nil {
		c.StreamCursors = map[string]int64{}
	}
	c.StreamCursors[stream] = cursor
}

// SyncCachePathFor returns the on-disk path of the bowrain sync cache for
// the given Layout. Bowrain owns this path; the framework Layout has no
// notion of a sync cache.
func SyncCachePathFor(layout coreproj.Layout) string {
	return filepath.Join(layout.CacheDir(), SyncCacheFilename)
}

// LoadSyncCache loads the sync cache for the given project layout. Returns
// an empty (but non-nil) cache when the file is missing or corrupt — push
// and pull are responsible for repopulating it.
func LoadSyncCache(layout coreproj.Layout) *SyncCache {
	data, err := os.ReadFile(SyncCachePathFor(layout))
	if err != nil {
		return newEmptySyncCache()
	}
	var cache SyncCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return newEmptySyncCache()
	}
	if cache.Files == nil {
		cache.Files = map[string]*FileCache{}
	}
	if cache.StreamCursors == nil {
		cache.StreamCursors = map[string]int64{}
	}
	return &cache
}

// Save persists the sync cache to <state-dir>/cache/sync-cache.json. The
// cache directory is created if missing.
func (c *SyncCache) Save(layout coreproj.Layout) error {
	if err := os.MkdirAll(layout.CacheDir(), 0o755); err != nil {
		return fmt.Errorf("project: create cache dir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("project: marshal sync cache: %w", err)
	}
	return os.WriteFile(SyncCachePathFor(layout), data, 0o644)
}

func newEmptySyncCache() *SyncCache {
	return &SyncCache{
		Files:         map[string]*FileCache{},
		StreamCursors: map[string]int64{},
	}
}
