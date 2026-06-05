package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// CacheTTL is the maximum age of a cached registry index before
// non-mutating reads (auto-install prompt, plugin info) refresh.
const CacheTTL = time.Hour

// CachedIndex pairs the index with its fetch time.
type CachedIndex struct {
	URL       string  `json:"url"`
	FetchedAt int64   `json:"fetched_at_unix_nano"`
	Index     IndexV2 `json:"index"`
}

// xdgCacheDir returns the effective XDG cache base directory.
// Mirrors the helper in the pluginhost package; duplicated here to avoid an
// import cycle (registry is a sub-package of pluginhost).
func xdgCacheDir() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".cache")
	}
	return filepath.Join(os.TempDir(), "kapi-cache")
}

// CacheLocation returns the on-disk path of the registry index cache.
// Defaults to $XDG_CACHE_HOME/kapi/registry-index.json.
func CacheLocation() string {
	if v := os.Getenv("KAPI_REGISTRY_CACHE"); v != "" {
		return v
	}
	return filepath.Join(xdgCacheDir(), "kapi", "registry-index.json")
}

// LoadCache returns the cached entry for url, or (nil, nil) when none
// is present.
func LoadCache(url string) (*CachedIndex, error) {
	data, err := os.ReadFile(CacheLocation())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var c CachedIndex
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	if c.URL != url {
		return nil, nil // cache is for a different registry
	}
	return &c, nil
}

// SaveCache atomically persists the cached index.
func SaveCache(c *CachedIndex) error {
	path := CacheLocation()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("registry cache: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Age reports how long ago the cache was fetched.
func (c *CachedIndex) Age() time.Duration {
	return time.Since(time.Unix(0, c.FetchedAt))
}

// FetchOrCached returns the index, using the on-disk cache when it's
// younger than CacheTTL and `force` is false. The fetched index is
// always written back to disk.
func FetchOrCached(ctx context.Context, url string, force bool) (*IndexV2, error) {
	if !force {
		if c, err := LoadCache(url); err == nil && c != nil && c.Age() < CacheTTL {
			out := c.Index
			return &out, nil
		}
	}
	idx, err := FetchIndex(ctx, url)
	if err != nil {
		return nil, err
	}
	_ = SaveCache(&CachedIndex{
		URL:       url,
		FetchedAt: time.Now().UnixNano(),
		Index:     *idx,
	})
	return idx, nil
}
