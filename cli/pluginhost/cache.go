package pluginhost

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/neokapi/neokapi/core/plugin/manifest"
)

// cacheVersion bumps when the cache file shape changes incompatibly.
// On version mismatch, kapi discards the cache and rescans.
const cacheVersion = "1"

// Cache is the on-disk dispatch cache. It is built as a side effect of
// install/update/remove and consumed at startup when no discovery root
// has been touched since the last write.
type Cache struct {
	Version string         `json:"cache_version"`
	GOOS    string         `json:"goos"`
	GOARCH  string         `json:"goarch"`
	Roots   []CachedRoot   `json:"roots"`
	Plugins []CachedPlugin `json:"plugins"`
}

// CachedRoot records one discovery root and its mtime at the time of
// the last cache write. Startup compares each root's current mtime
// against this; any newer mtime means the cache is stale.
type CachedRoot struct {
	Order         int    `json:"order"`
	Label         string `json:"label"`
	Path          string `json:"path"`
	MtimeUnixNano int64  `json:"mtime_unix_nano"`
}

// CachedPlugin is a verbatim copy of one discovered plugin's manifest
// plus its install dir.
type CachedPlugin struct {
	Dir        string             `json:"dir"`
	Source     Source             `json:"source"`
	Manifest   *manifest.Manifest `json:"manifest"`
	BinaryPath string             `json:"binary_path"`
}

// xdgCacheDir returns the effective XDG cache base directory, falling back to
// ~/.cache or a tmp dir when neither XDG_CACHE_HOME nor $HOME is available.
// Extracted so every cache/registry/install path resolves via one code path.
func xdgCacheDir() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return dir
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".cache")
	}
	return filepath.Join(os.TempDir(), "kapi-cache")
}

// CacheLocation returns the path to the dispatch cache file.
func CacheLocation() string {
	if v := os.Getenv("KAPI_PLUGIN_CACHE"); v != "" {
		return v
	}
	return filepath.Join(xdgCacheDir(), "kapi", "plugins-cache.json")
}

// LoadCache reads the cache file. Returns nil and a nil error when the
// cache does not exist; returns a non-nil error only on read/parse
// failure.
func LoadCache(path string) (*Cache, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("plugin cache: read: %w", err)
	}
	var c Cache
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("plugin cache: parse: %w", err)
	}
	return &c, nil
}

// SaveCache atomically writes the cache to path. Creates parent dirs.
func SaveCache(path string, cache *Cache) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("plugin cache: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("plugin cache: marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return fmt.Errorf("plugin cache: write: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("plugin cache: rename: %w", err)
	}
	return nil
}

// BuildCache materializes a Cache from a discovery result + the roots
// that were scanned. Captures each root's current dir mtime so startup
// can detect changes.
func BuildCache(opts DiscoverOptions, plugins []*Plugin) *Cache {
	roots := assembleRoots(opts)
	cachedRoots := make([]CachedRoot, 0, len(roots))
	for _, r := range roots {
		mtime := dirMtime(r.Path)
		cachedRoots = append(cachedRoots, CachedRoot{
			Order:         r.Order,
			Label:         r.Label,
			Path:          r.Path,
			MtimeUnixNano: mtime,
		})
	}
	cachedPlugins := make([]CachedPlugin, 0, len(plugins))
	for _, p := range plugins {
		cachedPlugins = append(cachedPlugins, CachedPlugin{
			Dir:        p.Dir,
			Source:     p.Source,
			Manifest:   p.Manifest,
			BinaryPath: p.BinaryPath,
		})
	}
	return &Cache{
		Version: cacheVersion,
		GOOS:    runtime.GOOS,
		GOARCH:  runtime.GOARCH,
		Roots:   cachedRoots,
		Plugins: cachedPlugins,
	}
}

// IsFresh reports whether cache is still valid for the given options.
// True when:
//   - cache version matches current binary
//   - GOOS/GOARCH match
//   - the set of roots has not changed
//   - every root's current mtime ≤ the recorded mtime
//
// Any miss flips IsFresh to false; the caller falls back to a full
// rescan + cache rebuild.
func IsFresh(cache *Cache, opts DiscoverOptions) bool {
	if cache == nil {
		return false
	}
	if cache.Version != cacheVersion {
		return false
	}
	if cache.GOOS != runtime.GOOS || cache.GOARCH != runtime.GOARCH {
		return false
	}
	currentRoots := assembleRoots(opts)
	if len(currentRoots) != len(cache.Roots) {
		return false
	}
	cachedByPath := make(map[string]CachedRoot, len(cache.Roots))
	for _, r := range cache.Roots {
		cachedByPath[r.Path] = r
	}
	for _, r := range currentRoots {
		cr, ok := cachedByPath[r.Path]
		if !ok {
			return false
		}
		if dirMtime(r.Path) > cr.MtimeUnixNano {
			return false
		}
	}
	return true
}

// PluginsFromCache rehydrates discovered plugins from the cache.
func PluginsFromCache(cache *Cache) []*Plugin {
	if cache == nil {
		return nil
	}
	out := make([]*Plugin, 0, len(cache.Plugins))
	for _, c := range cache.Plugins {
		out = append(out, &Plugin{
			Dir:        c.Dir,
			Source:     c.Source,
			Manifest:   c.Manifest,
			BinaryPath: c.BinaryPath,
		})
	}
	return out
}

// dirMtime returns the directory's mtime in UnixNano. Missing dirs
// return 0 (treated as "always older than the cache").
func dirMtime(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.ModTime().UnixNano()
}
