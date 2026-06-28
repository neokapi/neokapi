package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/storage"
)

// parseCache is a content-hash-keyed cache of a file's fully-parsed document —
// the complete ordered Part stream (blocks AND the surrounding structure:
// layers, groups, data, media) plus the reconstruction skeleton — persisted in
// the project's `.kapi/cache/` (gitignored, rebuildable). It is the file→document
// layer that makes repeated reads cheap and incremental: an unchanged file is
// served from the cache without re-parsing; a changed file re-parses; only
// changed files re-parse.
//
// It is the project's one internal document model: every read consumer plugs in
// through it — status/coverage/verify (which need the translatable blocks) and
// the flow runner (which needs the full Part stream to drive tools) — so no
// operation re-parses a file another already parsed under the same config.
//
// Invariant: the cache is a pure optimization over the files. Its key includes
// every input that determines the parse — the file's content hash and a config
// key (the format and any reader config the parse used) — so a stale entry can
// never be served. Delete the cache and a re-read reconstructs the identical
// document.
type parseCache struct {
	db *storage.DB
}

var parseCacheMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "file → parsed translatable blocks (superseded by v2)",
		SQL: `CREATE TABLE file_blocks (
			path         TEXT NOT NULL,
			config_key   TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			mtime        INTEGER NOT NULL,
			size         INTEGER NOT NULL,
			blocks       BLOB NOT NULL,
			PRIMARY KEY (path, config_key)
		);`,
	},
	{
		Version:     2,
		Description: "file → full parsed document (parts + skeleton); replaces the slim block cache",
		// The slim block table is a strict subset of the document table — drop it
		// (it is a rebuildable cache, so no data is lost) and key the full document.
		SQL: `DROP TABLE IF EXISTS file_blocks;
		CREATE TABLE file_documents (
			path         TEXT NOT NULL,
			config_key   TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			mtime        INTEGER NOT NULL,
			size         INTEGER NOT NULL,
			doc          BLOB NOT NULL,
			PRIMARY KEY (path, config_key)
		);`,
	},
}

// openParseCache opens (creating + migrating) the parse cache under cacheDir.
func openParseCache(cacheDir string) (*parseCache, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return nil, err
	}
	db, err := storage.Open(filepath.Join(cacheDir, "parse.db"))
	if err != nil {
		return nil, err
	}
	if err := storage.Migrate(db, "parse_cache_migrations", parseCacheMigrations); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &parseCache{db: db}, nil
}

func (c *parseCache) close() {
	if c != nil && c.db != nil {
		_ = c.db.Close()
	}
}

// cachedDoc is the faithful, serializable record of a fully-parsed document: the
// ordered Part stream plus the reconstruction skeleton bytes and the format that
// produced them. Layers bracket via start/end parts, so the flat ordered slice
// preserves the document structure exactly as the reader emitted it.
type cachedDoc struct {
	Parts        []cachedPart `json:"parts"`
	Skeleton     []byte       `json:"skel,omitempty"`
	OriginFormat string       `json:"fmt,omitempty"`
}

// cachedPart is one Part with its concrete Resource hoisted into a typed field —
// Part.Resource is an interface, so it is serialized as a tagged union keyed by
// the PartType. Exactly one payload field is set per part.
type cachedPart struct {
	Type   model.PartType    `json:"t"`
	Block  *model.Block      `json:"b,omitempty"`
	Layer  *model.Layer      `json:"l,omitempty"`
	Data   *model.Data       `json:"d,omitempty"`
	Media  *model.Media      `json:"m,omitempty"`
	GStart *model.GroupStart `json:"gs,omitempty"`
	GEnd   *model.GroupEnd   `json:"ge,omitempty"`
}

func toCachedParts(parts []*model.Part) []cachedPart {
	out := make([]cachedPart, 0, len(parts))
	for _, p := range parts {
		if p == nil {
			continue
		}
		cp := cachedPart{Type: p.Type}
		switch r := p.Resource.(type) {
		case *model.Block:
			cp.Block = r
		case *model.Layer:
			cp.Layer = r
		case *model.Data:
			cp.Data = r
		case *model.Media:
			cp.Media = r
		case *model.GroupStart:
			cp.GStart = r
		case *model.GroupEnd:
			cp.GEnd = r
		default:
			// A Part type we don't model in the cache (RawDocument/Custom) — skip
			// it. Readers never emit these into the processing stream, so a parsed
			// document never relies on round-tripping them.
			continue
		}
		out = append(out, cp)
	}
	return out
}

func fromCachedParts(cps []cachedPart) []*model.Part {
	out := make([]*model.Part, 0, len(cps))
	for i := range cps {
		cp := cps[i]
		var res model.Resource
		switch cp.Type {
		case model.PartBlock:
			res = cp.Block
		case model.PartLayerStart, model.PartLayerEnd:
			res = cp.Layer
		case model.PartData:
			res = cp.Data
		case model.PartMedia:
			res = cp.Media
		case model.PartGroupStart:
			res = cp.GStart
		case model.PartGroupEnd:
			res = cp.GEnd
		default:
			continue
		}
		if res == nil {
			continue
		}
		out = append(out, &model.Part{Type: cp.Type, Resource: res})
	}
	return out
}

// getDoc returns the cached full document for (path, configKey) when the entry is
// fresh — the stat (mtime+size) matches, or, after a cheap re-hash, the content
// hash matches (handling `touch`/clock skew without re-parsing). Returns ok=false
// on any miss or staleness, so the caller re-parses.
func (c *parseCache) getDoc(path, configKey string, st os.FileInfo) (parts []*model.Part, skeleton []byte, originFormat string, ok bool) {
	var contentHash string
	var mtime, size int64
	var blob []byte
	row := c.db.QueryRow(
		`SELECT content_hash, mtime, size, doc FROM file_documents WHERE path = ? AND config_key = ?`,
		path, configKey)
	if err := row.Scan(&contentHash, &mtime, &size, &blob); err != nil {
		return nil, nil, "", false
	}

	fresh := mtime == st.ModTime().UnixNano() && size == st.Size()
	if !fresh {
		// Stat changed — re-hash to see whether the bytes actually changed.
		h, herr := project.HashFile(path)
		if herr != nil || h != contentHash {
			return nil, nil, "", false
		}
		// Same bytes, new stat (touch / checkout): refresh the fast-path keys.
		_, _ = c.db.Exec(
			`UPDATE file_documents SET mtime = ?, size = ? WHERE path = ? AND config_key = ?`,
			st.ModTime().UnixNano(), st.Size(), path, configKey)
	}

	var doc cachedDoc
	if err := json.Unmarshal(blob, &doc); err != nil {
		return nil, nil, "", false
	}
	return fromCachedParts(doc.Parts), doc.Skeleton, doc.OriginFormat, true
}

// putDoc records the freshly-parsed full document for (path, configKey).
func (c *parseCache) putDoc(path, configKey string, st os.FileInfo, parts []*model.Part, skeleton []byte, originFormat string) {
	h, err := project.HashFile(path)
	if err != nil {
		return
	}
	blob, err := json.Marshal(cachedDoc{
		Parts:        toCachedParts(parts),
		Skeleton:     skeleton,
		OriginFormat: originFormat,
	})
	if err != nil {
		return
	}
	_, _ = c.db.Exec(
		`INSERT INTO file_documents (path, config_key, content_hash, mtime, size, doc)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(path, config_key) DO UPDATE SET
		   content_hash = excluded.content_hash,
		   mtime        = excluded.mtime,
		   size         = excluded.size,
		   doc          = excluded.doc`,
		path, configKey, h, st.ModTime().UnixNano(), st.Size(), blob)
}

// getBlocks is the translatable-block projection of the cached document — what
// the read/coverage/verify consumers need. It filters the cached Part stream so
// those callers share the one document cache with the flow runner.
func (c *parseCache) getBlocks(path, configKey string, st os.FileInfo) ([]*model.Block, bool) {
	parts, _, _, ok := c.getDoc(path, configKey, st)
	if !ok {
		return nil, false
	}
	return translatableBlocks(parts), true
}

// translatableBlocks extracts the translatable blocks from a Part stream in
// order — the projection the read path returns.
func translatableBlocks(parts []*model.Part) []*model.Block {
	var blocks []*model.Block
	for _, p := range parts {
		if p == nil || p.Type != model.PartBlock {
			continue
		}
		if b, ok := p.Resource.(*model.Block); ok && b.Translatable {
			blocks = append(blocks, b)
		}
	}
	return blocks
}

// withParseCache opens the project's parse cache for the duration of fn, so the
// read paths underneath (readBlocks) and the flow runner serve unchanged files
// from it. A failure to open the cache is non-fatal — fn still runs, just without
// caching. No-op when root is empty (ad-hoc, no project).
func (a *App) withParseCache(root string, fn func() error) error {
	closeCache := a.openParseCacheDefer(root)
	defer closeCache()
	return fn()
}

// openParseCacheDefer opens the project's parse cache and returns a closer to
// defer — the non-closure form of withParseCache, for call sites (the flow
// runner) whose body isn't naturally a single fn. Returns a no-op closer when
// there's no project layout or the cache can't open (parse directly), or when a
// cache is already open. The returned closer is always safe to call.
func (a *App) openParseCacheDefer(root string) func() {
	if root == "" || a.parseCache != nil {
		return func() {}
	}
	layout, err := project.ResolveLayout(root)
	if err != nil {
		return func() {}
	}
	c, err := openParseCache(layout.CacheDir())
	if err != nil {
		return func() {}
	}
	a.parseCache = c
	return func() {
		c.close()
		a.parseCache = nil
	}
}

// appPartCache adapts the App's open parse cache to the flow runner's PartCache
// seam: it serves a file's full Part stream from a prior parse under the same
// config key. It reads a.parseCache at call time, so it is safe whether or not a
// cache is currently open (a nil cache simply misses).
type appPartCache struct{ a *App }

func (p appPartCache) GetDocument(path, configKey string) ([]*model.Part, bool) {
	if p.a.parseCache == nil {
		return nil, false
	}
	st, err := os.Stat(path)
	if err != nil {
		return nil, false
	}
	parts, _, _, ok := p.a.parseCache.getDoc(path, configKey, st)
	return parts, ok
}

func (p appPartCache) PutDocument(path, configKey string, parts []*model.Part) {
	if p.a.parseCache == nil {
		return
	}
	st, err := os.Stat(path)
	if err != nil {
		return
	}
	// The process-only runner path needs no skeleton (it writes no file); the
	// origin format is carried in the runner's config key, so store neither here.
	p.a.parseCache.putDoc(path, configKey, st, parts, nil, "")
}

// runnerPartCache returns the runner document-cache seam and its config-key
// fingerprint for the current project, or (nil, "") when not in a project. The
// fingerprint folds in the source locale, any preset/format config the caller
// merged, and the project recipe hash — every non-byte input that shapes the
// parse — so a recipe or config change re-parses.
func (a *App) runnerPartCache(root string, mergedConfig map[string]any) (flow.PartCache, string) {
	if a.projectContext == nil {
		return nil, ""
	}
	h := sha256.New()
	fmt.Fprintf(h, "%s\x00", a.SourceLang)
	if len(mergedConfig) > 0 {
		if b, err := json.Marshal(mergedConfig); err == nil {
			h.Write(b)
		}
	}
	if layout, err := project.ResolveLayout(root); err == nil {
		if rb, rerr := os.ReadFile(layout.RecipePath); rerr == nil {
			h.Write(rb)
		}
	}
	return appPartCache{a}, hex.EncodeToString(h.Sum(nil))[:16]
}
