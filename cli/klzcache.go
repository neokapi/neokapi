package cli

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/blockstore/exporter"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/klz"
)

// A .klz workspace is worked on through a persistent shadow cache — its
// runtime — keeping the .klz itself a stable transport artifact written only
// by `pack` (AD-025 §5, the git-bundle model). The cache lives under
// $XDG_CACHE_HOME/kapi/klz/<key>, keyed by the .klz's absolute path, so the
// working directory stays a single file. Transforms hit the cache's
// persistent per-source block stores directly (incremental, no re-zip); a
// dirty workspace is one whose cache has diverged from the packed .klz.

// klzCacheRoot returns the root directory for all workspace caches.
func klzCacheRoot() string {
	if v := os.Getenv("KAPI_KLZ_CACHE"); v != "" {
		return v
	}
	xdg := os.Getenv("XDG_CACHE_HOME")
	if xdg == "" {
		if home, err := os.UserHomeDir(); err == nil {
			xdg = filepath.Join(home, ".cache")
		} else {
			xdg = filepath.Join(os.TempDir(), "kapi-cache")
		}
	}
	return filepath.Join(xdg, "kapi", "klz")
}

// klzCacheDir returns the cache directory for a given .klz, keyed by its
// absolute path so two like-named files in different folders don't collide.
func klzCacheDir(klzPath string) string {
	abs, err := filepath.Abs(klzPath)
	if err != nil {
		abs = klzPath
	}
	sum := sha256.Sum256([]byte(abs))
	return filepath.Join(klzCacheRoot(), hex.EncodeToString(sum[:])[:16])
}

// klzCacheMeta is the cache's metadata: the workspace recipe, its sources,
// and the rootHash of the .klz the cache was last in sync with (for dirty
// detection).
type klzCacheMeta struct {
	KlzPath        string           `json:"klzPath"`
	PackedRootHash string           `json:"packedRootHash"`
	Recipe         *klz.Recipe      `json:"recipe,omitempty"`
	Sources        []klzCacheSource `json:"sources"`
}

type klzCacheSource struct {
	Path string `json:"path"` // archive path, e.g. "source/app.json"
	Name string `json:"name"` // base name, e.g. "app.json"
	Key  string `json:"key"`  // per-source store key
}

// klzCache is an open workspace cache.
type klzCache struct {
	dir  string
	meta klzCacheMeta
}

func (c *klzCache) metaPath() string              { return filepath.Join(c.dir, "meta.json") }
func (c *klzCache) sourcePath(name string) string { return filepath.Join(c.dir, "sources", name) }
func (c *klzCache) storePath(key string) string   { return filepath.Join(c.dir, "overlays", key+".db") }

func (c *klzCache) save() error {
	data, err := json.MarshalIndent(c.meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(c.dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(c.metaPath(), append(data, '\n'), 0o644)
}

// sourceStoreKey derives a stable per-source store key from its archive path.
func sourceStoreKey(archivePath string) string {
	sum := sha256.Sum256([]byte(archivePath))
	return hex.EncodeToString(sum[:])[:12]
}

// buildKlzCache (re)builds a workspace cache from a .klz, overwriting any
// existing cache at that location.
func buildKlzCache(ctx context.Context, klzPath string) (*klzCache, error) {
	pkg, err := loadWorkspace(klzPath)
	if err != nil {
		return nil, err
	}
	rootHash, err := pkg.RootHash()
	if err != nil {
		return nil, err
	}
	dir := klzCacheDir(klzPath)
	if err := os.RemoveAll(dir); err != nil {
		return nil, fmt.Errorf("klz cache: clear: %w", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "sources"), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(dir, "overlays"), 0o755); err != nil {
		return nil, err
	}
	abs, _ := filepath.Abs(klzPath)
	c := &klzCache{dir: dir, meta: klzCacheMeta{KlzPath: abs, PackedRootHash: rootHash, Recipe: pkg.Recipe}}

	for _, src := range pkg.Source {
		name := filepath.Base(src.Path)
		key := sourceStoreKey(src.Path)
		if err := os.WriteFile(c.sourcePath(name), src.Data, 0o644); err != nil {
			return nil, fmt.Errorf("klz cache: write source: %w", err)
		}
		store, err := sqlitestore.New(c.storePath(key))
		if err != nil {
			return nil, err
		}
		err = exporter.LoadOverlays(ctx, store, klzToStoreOverlays(overlaysForSource(pkg.Overlays, src.Path)))
		_ = store.Close()
		if err != nil {
			return nil, fmt.Errorf("klz cache: load overlays: %w", err)
		}
		c.meta.Sources = append(c.meta.Sources, klzCacheSource{Path: src.Path, Name: name, Key: key})
	}
	if err := c.save(); err != nil {
		return nil, err
	}
	return c, nil
}

// openKlzCache reads an existing cache. Returns (nil, false, nil) when none
// exists yet.
func openKlzCache(klzPath string) (*klzCache, bool, error) {
	dir := klzCacheDir(klzPath)
	data, err := os.ReadFile(filepath.Join(dir, "meta.json"))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var meta klzCacheMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, false, err
	}
	return &klzCache{dir: dir, meta: meta}, true, nil
}

// ensureKlzCache returns the workspace cache for a .klz, building it from the
// file when no cache exists, or when the .klz changed on disk while the cache
// is clean (the file was replaced — re-sync to it). A dirty cache is kept
// even if the .klz changed, with a warning, so unpacked work is never lost.
func (a *App) ensureKlzCache(ctx context.Context, klzPath string) (*klzCache, error) {
	c, ok, err := openKlzCache(klzPath)
	if err != nil {
		return nil, err
	}
	if !ok {
		return buildKlzCache(ctx, klzPath)
	}
	fileRoot, err := klzFileRootHash(klzPath)
	if err != nil {
		return nil, err
	}
	if fileRoot != c.meta.PackedRootHash {
		dirty, derr := c.dirty(ctx)
		if derr != nil {
			return nil, derr
		}
		if dirty {
			fmt.Fprintf(os.Stderr, "Warning: %s changed on disk but the workspace cache has unpacked work — keeping the cache (run `kapi unpack %s` to discard it)\n", filepath.Base(klzPath), filepath.Base(klzPath))
			return c, nil
		}
		return buildKlzCache(ctx, klzPath)
	}
	return c, nil
}

// klzFileRootHash reads a .klz and returns its content rootHash.
func klzFileRootHash(klzPath string) (string, error) {
	pkg, err := loadWorkspace(klzPath)
	if err != nil {
		return "", err
	}
	return pkg.RootHash()
}

// toPackage assembles the cache's current state into a klz.Package.
func (c *klzCache) toPackage(ctx context.Context) (*klz.Package, error) {
	pkg := &klz.Package{Generator: &klz.GeneratorInfo{ID: "kapi"}, Recipe: c.meta.Recipe}
	for _, src := range c.meta.Sources {
		data, err := os.ReadFile(c.sourcePath(src.Name))
		if err != nil {
			return nil, fmt.Errorf("klz cache: read source: %w", err)
		}
		pkg.Source = append(pkg.Source, klz.SourceDoc{Path: src.Path, Data: data})

		store, err := sqlitestore.New(c.storePath(src.Key))
		if err != nil {
			return nil, err
		}
		snap, err := exporter.Export(ctx, store)
		_ = store.Close()
		if err != nil {
			return nil, fmt.Errorf("klz cache: export overlays: %w", err)
		}
		for _, o := range storeToKlzOverlays(snap.Overlays) {
			o.Source = src.Path
			pkg.Overlays = append(pkg.Overlays, o)
		}
	}
	return pkg, nil
}

// dirty reports whether the cache has diverged from the packed .klz.
func (c *klzCache) dirty(ctx context.Context) (bool, error) {
	pkg, err := c.toPackage(ctx)
	if err != nil {
		return false, err
	}
	cur, err := pkg.RootHash()
	if err != nil {
		return false, err
	}
	return cur != c.meta.PackedRootHash, nil
}

// pack writes the cache's current state into its .klz and marks it clean.
func (c *klzCache) pack(ctx context.Context) error {
	pkg, err := c.toPackage(ctx)
	if err != nil {
		return err
	}
	if err := saveWorkspace(pkg, c.meta.KlzPath); err != nil {
		return err
	}
	root, err := pkg.RootHash()
	if err != nil {
		return err
	}
	c.meta.PackedRootHash = root
	return c.save()
}
