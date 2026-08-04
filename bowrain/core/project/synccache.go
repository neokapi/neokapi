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
	// pull last wrote into the project's bound terms, so a later concept push
	// can diff local terms edits against what was pulled (ordinary edits go
	// up directly, governed edits become a reviewed change-set). It is
	// regenerable — every pull refreshes it.
	ConceptBaseline *ConceptBaseline `json:"concept_baseline,omitempty"`

	// ContextHash is the context the last push carried — the fold over every
	// declared collection's entry (bowrain/core/sync.ContextHashOf). It is the
	// local half of the context fast path: an unedited recipe is not worth a
	// round trip, and a recipe that gained a collection or rebound a voice must
	// reach the server even when no content moved. Regenerable — it is derived
	// from the recipe, and losing it costs one redundant reconcile.
	ContextHash string `json:"context_hash,omitempty"`

	// DecisionsHash is the committed decision record the last push carried
	// (sha256 over the wire serialization). A push whose content and context
	// are unchanged but whose decisions differ must still commit — this is
	// what notices. Same cache discipline as block hashes: keyed to this
	// destination, rebuilt by re-sending (the server upserts idempotently).
	DecisionsHash string `json:"decisions_hash,omitempty"`

	// ServerContext records the collections the last pull observed on the
	// server, keyed by collection name. It is an OBSERVATION, never an
	// instruction: nothing derived from a recipe-owned entry is applied to
	// local governance, because kapi.yaml is the authority for those and a pull
	// that could rewrite them would make the same content resolve differently
	// depending on where it was last synced. What it buys is the ability to say
	// so — `kapi status` reports a recipe-owned collection the server governs
	// differently instead of letting the two diverge unremarked.
	ServerContext map[string]ServerCollection `json:"server_context,omitempty"`
}

// ServerCollection is one collection as the server holds it, recorded by a pull.
type ServerCollection struct {
	// Coordinates is the point the server has the collection at.
	Coordinates map[string]string `json:"coordinates,omitempty"`
	// Channel is the channel bound to it server-side.
	Channel string `json:"channel,omitempty"`
	// VoiceProfile is the name of the voice profile bound to it server-side.
	VoiceProfile string `json:"voice_profile,omitempty"`
	// Owner is "recipe" or "workspace" — which side is authoritative. Every
	// decision about what a pull may act on is a lookup of this field.
	Owner string `json:"owner,omitempty"`
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

// NewEmptySyncCache returns a cache describing nothing yet synced. Callers use
// it to discard a cache that belongs to a different server or project: what it
// records — confirmed block hashes, issued stream cursors, the context the
// server holds — is true of that destination and of no other.
func NewEmptySyncCache() *SyncCache { return newEmptySyncCache() }

func newEmptySyncCache() *SyncCache {
	return &SyncCache{
		Files:         map[string]*FileCache{},
		StreamCursors: map[string]int64{},
	}
}
