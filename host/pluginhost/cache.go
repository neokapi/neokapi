package pluginhost

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"

	"github.com/neokapi/neokapi/core/dataroot"

	"github.com/neokapi/neokapi/core/plugin/manifest"
)

// cacheVersion bumps when the cache file shape changes incompatibly, or when a
// scanner fix makes previously-written contents wrong. On version mismatch,
// kapi discards the cache and rescans.
//
// 3: the cache records the plugins the safety rule refused and the facts it
// read (mode, owner, change time) for every path it checked, and no longer
// records a plugin's source order or binary path, which kapi works out from
// the current roots instead.
const cacheVersion = "3"

// Cache is the on-disk dispatch cache. It is built as a side effect of
// install/update/remove and consumed at startup when nothing kapi checked
// has changed since the last write.
type Cache struct {
	Version string         `json:"cache_version"`
	GOOS    string         `json:"goos"`
	GOARCH  string         `json:"goarch"`
	Roots   []CachedRoot   `json:"roots"`
	Plugins []CachedPlugin `json:"plugins"`
	// Refused are the plugins found and not loaded, with why, so a listing
	// served from the cache still shows them.
	Refused []Refusal `json:"refused,omitempty"`
	// Checked is every path the safety rule read (roots, plugin directories,
	// binaries) and what it read there. A change to any of them, such as the
	// chmod a refusal asks for, makes the cache stale.
	Checked []CheckedPath `json:"checked,omitempty"`
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

// CachedPlugin is a verbatim copy of one discovered plugin's manifest plus its
// install dir. Its source and binary path are worked out from the current
// roots and the manifest when the cache is read, never taken from the file.
type CachedPlugin struct {
	Dir      string             `json:"dir"`
	Manifest *manifest.Manifest `json:"manifest"`
}

// CheckedPath is one path the safety rule read and its fingerprint then.
type CheckedPath struct {
	Path        string `json:"path"`
	Fingerprint string `json:"fingerprint"`
}

// xdgCacheDir returns the effective XDG cache base directory, falling back to
// ~/.cache. With neither XDG_CACHE_HOME nor a home directory it returns "":
// the cache names plugin binaries kapi runs, so it is never kept in a shared
// directory another user could write to.
func xdgCacheDir() string {
	if dir := os.Getenv("XDG_CACHE_HOME"); dir != "" {
		return dir
	}
	if home, err := dataroot.UserHomeDir(); err == nil && home != "" {
		return filepath.Join(home, ".cache")
	}
	return ""
}

// CacheLocation returns the path to the dispatch cache file.
func CacheLocation() string {
	if v := os.Getenv("KAPI_PLUGIN_CACHE"); v != "" {
		return v
	}
	dir := xdgCacheDir()
	if dir == "" {
		return ""
	}
	return filepath.Join(dir, "kapi", "plugins-cache.json")
}

// LoadCache reads the cache file. Returns nil and a nil error when the
// cache does not exist; returns a non-nil error only on read/parse
// failure.
//
// The cache names binaries kapi runs, so it is read only when no other user
// can have written it: the file and its directory pass the same rule as a
// plugin path (safety.go). A cache that does not is refused with an error,
// and the caller scans the roots instead.
func LoadCache(path string) (*Cache, error) {
	if path == "" {
		return nil, nil
	}
	for _, p := range []string{path, filepath.Dir(path)} {
		if err := checkPath(p); err != nil {
			return nil, fmt.Errorf("plugin cache: not read: %w", err)
		}
	}
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
	if path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("plugin cache: mkdir: %w", err)
	}
	data, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return fmt.Errorf("plugin cache: marshal: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("plugin cache: write: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("plugin cache: rename: %w", err)
	}
	return nil
}

// BuildCache materializes a Cache from a discovery result (the plugins loaded
// and the ones refused) and the roots that were scanned. It captures each
// root's mtime, and the fingerprint of every path the safety rule read, so
// startup can detect changes.
func BuildCache(opts DiscoverOptions, plugins []*Plugin, refused []Refusal) *Cache {
	roots := assembleRoots(opts)
	cachedRoots := make([]CachedRoot, 0, len(roots))
	var checked []string
	for _, r := range roots {
		cachedRoots = append(cachedRoots, CachedRoot{
			Order:         r.Order,
			Label:         r.Label,
			Path:          r.Path,
			MtimeUnixNano: dirMtime(r.Path),
		})
		checked = append(checked, r.Path)
	}
	cachedPlugins := make([]CachedPlugin, 0, len(plugins))
	for _, p := range plugins {
		cachedPlugins = append(cachedPlugins, CachedPlugin{Dir: p.Dir, Manifest: p.Manifest})
		checked = append(checked, p.Dir, p.BinaryPath)
	}
	for _, r := range refused {
		checked = append(checked, r.Dir)
		if r.Binary != "" {
			checked = append(checked, r.Binary)
		}
	}
	cache := &Cache{
		Version: cacheVersion,
		GOOS:    runtime.GOOS,
		GOARCH:  runtime.GOARCH,
		Roots:   cachedRoots,
		Plugins: cachedPlugins,
		Refused: refused,
	}
	for _, path := range checked {
		cache.Checked = append(cache.Checked, CheckedPath{Path: path, Fingerprint: pathFingerprint(path)})
	}
	return cache
}

// IsFresh reports whether cache is still valid for the given options.
// True when:
//   - cache version matches current binary
//   - GOOS/GOARCH match
//   - the set of roots has not changed
//   - every root's current mtime ≤ the recorded mtime
//   - every path the safety rule read has the fingerprint it had
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
	for _, c := range cache.Checked {
		if pathFingerprint(c.Path) != c.Fingerprint {
			return false
		}
	}
	return true
}

// PluginsFromCache rehydrates discovered plugins from the cache. Each plugin
// takes its source from the current roots: a cached directory that is not an
// entry of one of them is dropped, whatever the file says. Its binary path
// is the manifest's binary inside that directory, and must stay inside it.
func PluginsFromCache(cache *Cache, opts DiscoverOptions) []*Plugin {
	if cache == nil {
		return nil
	}
	roots := assembleRoots(opts)
	out := make([]*Plugin, 0, len(cache.Plugins))
	for _, c := range cache.Plugins {
		if c.Manifest == nil || c.Dir == "" {
			continue
		}
		src, ok := rootHolding(roots, c.Dir)
		if !ok {
			continue
		}
		bin := filepath.Join(c.Dir, c.Manifest.Binary)
		if rel, err := filepath.Rel(c.Dir, bin); err != nil || !filepath.IsLocal(rel) {
			continue
		}
		out = append(out, &Plugin{Dir: c.Dir, Source: src, Manifest: c.Manifest, BinaryPath: bin})
	}
	return out
}

// rootHolding is the current root whose direct entry dir is.
func rootHolding(roots []Source, dir string) (Source, bool) {
	parent := filepath.Dir(filepath.Clean(dir))
	for _, r := range roots {
		if filepath.Clean(r.Path) == parent {
			return r, true
		}
	}
	return Source{}, false
}

// pathFingerprint is what the safety rule reads about path (its mode, owner
// and group) plus its modification and change times, or "missing". chmod and
// chown change the change time without touching the modification time, so
// both are recorded.
func pathFingerprint(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return "missing"
	}
	uid, gid, _ := fileOwner(info)
	return fmt.Sprintf("%o:%d:%d:%d:%d", info.Mode(), uid, gid, info.ModTime().UnixNano(), changeTime(info))
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
