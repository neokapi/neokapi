package cli

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/storage"
)

// parseCache is a content-hash-keyed cache of a file's parsed translatable
// blocks, persisted in the project's `.kapi/cache/` (gitignored, rebuildable).
// It is the file→blocks layer that makes repeated reads cheap and incremental:
// an unchanged file is served from the cache without re-parsing; a changed file
// re-parses; only changed files re-parse.
//
// Invariant: the cache is a pure optimization over the files. Its key includes
// every input that determines the parsed blocks — the file's content hash and a
// config key (the format and any reader config the parse used) — so a stale
// entry can never be served. Delete the cache and a re-read reconstructs the
// identical blocks.
type parseCache struct {
	db *storage.DB
}

var parseCacheMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "file → parsed translatable blocks",
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

// cachedBlock is the slim, faithful record of a parsed block the read/coverage
// consumers need: the pairing key (Name/ID), the source content, and the
// classification. It deliberately omits the skeleton, overlays, and annotations
// — read consumers (coverage, terminology, QA) never need them, and the
// file-writing round-trip re-reads the file fresh.
type cachedBlock struct {
	ID           string      `json:"id,omitempty"`
	Name         string      `json:"name,omitempty"`
	Type         string      `json:"type,omitempty"`
	MimeType     string      `json:"mime,omitempty"`
	Translatable bool        `json:"t"`
	Source       []model.Run `json:"src"`
}

func toCached(blocks []*model.Block) []cachedBlock {
	out := make([]cachedBlock, len(blocks))
	for i, b := range blocks {
		out[i] = cachedBlock{
			ID:           b.ID,
			Name:         b.Name,
			Type:         b.Type,
			MimeType:     b.MimeType,
			Translatable: b.Translatable,
			Source:       b.Source,
		}
	}
	return out
}

func fromCached(cbs []cachedBlock) []*model.Block {
	out := make([]*model.Block, len(cbs))
	for i := range cbs {
		cb := cbs[i]
		out[i] = &model.Block{
			ID:           cb.ID,
			Name:         cb.Name,
			Type:         cb.Type,
			MimeType:     cb.MimeType,
			Translatable: cb.Translatable,
			Source:       cb.Source,
		}
	}
	return out
}

// get returns the cached blocks for (path, configKey) when the entry is fresh —
// the stat (mtime+size) matches, or, after a cheap re-hash, the content hash
// matches (handling `touch`/clock skew without re-parsing). Returns ok=false on
// any miss or staleness, so the caller re-parses.
func (c *parseCache) get(path, configKey string, st os.FileInfo) ([]*model.Block, bool) {
	var contentHash string
	var mtime, size int64
	var blob []byte
	row := c.db.QueryRow(
		`SELECT content_hash, mtime, size, blocks FROM file_blocks WHERE path = ? AND config_key = ?`,
		path, configKey)
	if err := row.Scan(&contentHash, &mtime, &size, &blob); err != nil {
		return nil, false
	}

	fresh := mtime == st.ModTime().UnixNano() && size == st.Size()
	if !fresh {
		// Stat changed — re-hash to see whether the bytes actually changed.
		h, herr := project.HashFile(path)
		if herr != nil || h != contentHash {
			return nil, false
		}
		// Same bytes, new stat (touch / checkout): refresh the fast-path keys.
		_, _ = c.db.Exec(
			`UPDATE file_blocks SET mtime = ?, size = ? WHERE path = ? AND config_key = ?`,
			st.ModTime().UnixNano(), st.Size(), path, configKey)
	}

	var cbs []cachedBlock
	if err := json.Unmarshal(blob, &cbs); err != nil {
		return nil, false
	}
	return fromCached(cbs), true
}

// put records the freshly-parsed blocks for (path, configKey).
func (c *parseCache) put(path, configKey string, st os.FileInfo, blocks []*model.Block) {
	h, err := project.HashFile(path)
	if err != nil {
		return
	}
	blob, err := json.Marshal(toCached(blocks))
	if err != nil {
		return
	}
	_, _ = c.db.Exec(
		`INSERT INTO file_blocks (path, config_key, content_hash, mtime, size, blocks)
		 VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT(path, config_key) DO UPDATE SET
		   content_hash = excluded.content_hash,
		   mtime        = excluded.mtime,
		   size         = excluded.size,
		   blocks       = excluded.blocks`,
		path, configKey, h, st.ModTime().UnixNano(), st.Size(), blob)
}

// withParseCache opens the project's parse cache for the duration of fn, so the
// read paths underneath (readBlocks) serve unchanged files from it. A failure to
// open the cache is non-fatal — fn still runs, just without caching. No-op when
// root is empty (ad-hoc, no project).
func (a *App) withParseCache(root string, fn func() error) error {
	if root == "" || a.parseCache != nil {
		return fn()
	}
	layout, err := project.ResolveLayout(root)
	if err != nil {
		return fn() // not inside a project layout — parse directly
	}
	c, err := openParseCache(layout.CacheDir())
	if err != nil {
		return fn() // cache unavailable — degrade to direct parsing
	}
	a.parseCache = c
	defer func() {
		c.close()
		a.parseCache = nil
	}()
	return fn()
}
