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
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/klz"
	"gopkg.in/yaml.v3"
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
	Sources        []klzCacheSource `json:"sources"`

	// Recipe is the full project recipe (AD-025 §6). It is held in-memory as
	// a *project.KapiProject but persisted to the cache meta.json via
	// RecipeYAML, because KapiProject's Extras (which holds the klz output
	// layout) are yaml-inline and JSON-excluded — JSON-encoding the struct
	// directly would silently drop them. RecipeYAML is the YAML encoding,
	// kept in sync by save()/load.
	Recipe     *project.KapiProject `json:"-"`
	RecipeYAML string               `json:"recipeYaml,omitempty"`
}

type klzCacheSource struct {
	Path string `json:"path"` // archive path, e.g. "source/app.json"
	Name string `json:"name"` // base name, e.g. "app.json"
	Key  string `json:"key"`  // per-source store key

	// Skeleton, FormatID, and ContentHash carry this source's identity +
	// round-trip skeleton (AD-025 §6). Skeleton is the cache-local skeleton
	// filename (under the cache's skeletons/ dir), empty when none was
	// captured.
	Skeleton    string `json:"skeleton,omitempty"`
	FormatID    string `json:"formatId,omitempty"`
	ContentHash string `json:"contentHash,omitempty"`

	// RawPresent records that the raw source bytes are in the cache locally
	// (the runtime needs them for transform/merge), distinct from whether they
	// should be PACKED into the .klz. HasRawSource is the pack-retention flag:
	// the raw bytes ride in the .klz only when it is set (extract/pack
	// --with-source).
	RawPresent   bool `json:"rawPresent,omitempty"`
	HasRawSource bool `json:"hasRawSource,omitempty"`
}

// klzCache is an open workspace cache.
type klzCache struct {
	dir  string
	meta klzCacheMeta
}

func (c *klzCache) metaPath() string                { return filepath.Join(c.dir, "meta.json") }
func (c *klzCache) sourcePath(name string) string   { return filepath.Join(c.dir, "sources", name) }
func (c *klzCache) storePath(key string) string     { return filepath.Join(c.dir, "overlays", key+".db") }
func (c *klzCache) skeletonPath(name string) string { return filepath.Join(c.dir, "skeletons", name) }

// hasSource reports whether the cache holds this source's raw bytes on disk
// (the runtime needs them for transform/merge), regardless of pack retention.
func (c *klzCache) hasSource(src klzCacheSource) bool {
	if !src.RawPresent {
		return false
	}
	_, err := os.Stat(c.sourcePath(src.Name))
	return err == nil
}

func (c *klzCache) save() error {
	// Persist the recipe as YAML so its Extras (the klz output layout) survive
	// the JSON cache file — KapiProject Extras are yaml-inline/json-excluded.
	if c.meta.Recipe != nil {
		yml, err := yaml.Marshal(c.meta.Recipe)
		if err != nil {
			return fmt.Errorf("klz cache: marshal recipe: %w", err)
		}
		c.meta.RecipeYAML = string(yml)
	} else {
		c.meta.RecipeYAML = ""
	}
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
	return buildKlzCacheFromPackage(ctx, klzPath, pkg)
}

// buildKlzCacheFromPackage builds a workspace cache from an in-memory package,
// overwriting any existing cache for klzPath. Used both by buildKlzCache (after
// loading from disk) and by extract (which holds the full package in memory,
// raw source included, before the packed .klz drops it).
func buildKlzCacheFromPackage(ctx context.Context, klzPath string, pkg *klz.Package) (*klzCache, error) {
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
	if err := os.MkdirAll(filepath.Join(dir, "skeletons"), 0o755); err != nil {
		return nil, err
	}
	abs, _ := filepath.Abs(klzPath)
	c := &klzCache{dir: dir, meta: klzCacheMeta{KlzPath: abs, PackedRootHash: rootHash, Recipe: pkg.Recipe}}

	// Index raw source bytes + skeletons by archive/source path so each
	// source's cache entry can be built whether it carries raw bytes, just a
	// skeleton (the default source-retention), or both (AD-025 §6).
	rawByPath := make(map[string]klz.SourceDoc, len(pkg.Source))
	for _, s := range pkg.Source {
		rawByPath[s.Path] = s
	}
	skelBySource := make(map[string]klz.SkeletonDoc, len(pkg.Skeletons))
	for _, s := range pkg.Skeletons {
		skelBySource[s.SourcePath] = s
	}

	// Build the per-source list. Prefer the manifest's Sources identities (the
	// canonical inventory); fall back to raw Source docs for older packages
	// that predate the identity list.
	type srcEntry struct {
		archivePath  string // "source/<name>" — overlay scoping key
		sourcePath   string // logical source path
		formatID     string
		contentHash  string
		hasRawSource bool
	}
	var entries []srcEntry
	if len(pkg.Sources) > 0 {
		for _, si := range pkg.Sources {
			entries = append(entries, srcEntry{
				archivePath:  "source/" + filepath.Base(si.SourcePath),
				sourcePath:   si.SourcePath,
				formatID:     si.FormatID,
				contentHash:  si.ContentHash,
				hasRawSource: si.HasRawSource,
			})
		}
	} else {
		for _, s := range pkg.Source {
			entries = append(entries, srcEntry{archivePath: s.Path, sourcePath: s.Path, hasRawSource: true})
		}
	}

	for _, e := range entries {
		name := filepath.Base(e.sourcePath)
		key := sourceStoreKey(e.archivePath)
		cs := klzCacheSource{
			Path: e.archivePath, Name: name, Key: key,
			FormatID: e.formatID, ContentHash: e.contentHash,
		}

		// Raw source bytes: stream them into the cache when present (the runtime
		// needs them). Pack retention is tracked separately by hasRawSource.
		if raw, ok := rawByPath[e.archivePath]; ok {
			if err := copyContentToFile(raw.Content, c.sourcePath(name)); err != nil {
				return nil, fmt.Errorf("klz cache: write source: %w", err)
			}
			cs.RawPresent = true
		}
		cs.HasRawSource = e.hasRawSource

		// Skeleton (default source-retention): stream it into the cache.
		if skel, ok := skelBySource[e.sourcePath]; ok {
			skelName := name + ".skel"
			if err := copyContentToFile(skel.Content, c.skeletonPath(skelName)); err != nil {
				return nil, fmt.Errorf("klz cache: write skeleton: %w", err)
			}
			cs.Skeleton = skelName
			if cs.FormatID == "" {
				cs.FormatID = skel.FormatID
			}
			if cs.ContentHash == "" {
				cs.ContentHash = skel.ContentHash
			}
		}

		store, err := sqlitestore.New(c.storePath(key))
		if err != nil {
			return nil, err
		}
		err = exporter.LoadOverlays(ctx, store, klzToStoreOverlays(overlaysForSource(pkg.Overlays, e.archivePath)))
		_ = store.Close()
		if err != nil {
			return nil, fmt.Errorf("klz cache: load overlays: %w", err)
		}
		c.meta.Sources = append(c.meta.Sources, cs)
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
	if meta.RecipeYAML != "" {
		var r project.KapiProject
		if err := yaml.Unmarshal([]byte(meta.RecipeYAML), &r); err != nil {
			return nil, false, fmt.Errorf("klz cache: parse recipe: %w", err)
		}
		meta.Recipe = &r
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

// toPackage assembles the cache's current state into a klz.Package. Source is
// retained as identity + skeleton by default; the raw bytes ride only when the
// cache holds them (the .klz was packed --with-source) (AD-025 §6).
func (c *klzCache) toPackage(ctx context.Context) (*klz.Package, error) {
	pkg := &klz.Package{Generator: &klz.GeneratorInfo{ID: "kapi"}, Recipe: c.meta.Recipe}
	for _, src := range c.meta.Sources {
		si := klz.SourceIdentity{
			SourcePath:  src.Name,
			FormatID:    src.FormatID,
			ContentHash: src.ContentHash,
		}

		// Skeleton (default source-retention). The cache file is streamed into
		// the package on demand (hash/pack), never read into memory here.
		if src.Skeleton != "" {
			skelPath := klz.SkeletonDir + filepath.Base(src.Name)
			pkg.Skeletons = append(pkg.Skeletons, klz.SkeletonDoc{
				Path:        skelPath,
				SourcePath:  src.Name,
				FormatID:    src.FormatID,
				ContentHash: src.ContentHash,
				Content:     klz.FileContent(c.skeletonPath(src.Skeleton)),
			})
			si.SkeletonPath = skelPath
		}

		// Raw source bytes (opt-in retention): pack them only when the source
		// is retained (--with-source) and the cache holds them.
		if src.HasRawSource && c.hasSource(src) {
			pkg.Source = append(pkg.Source, klz.SourceDoc{Path: src.Path, Content: klz.FileContent(c.sourcePath(src.Name))})
			si.HasRawSource = true
		}

		pkg.Sources = append(pkg.Sources, si)

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
