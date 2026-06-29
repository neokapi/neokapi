package cli

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/storage"
)

// hashKey returns a short stable hex digest used to name content-addressed cache
// files for a (config, source-hash) pair.
func hashKey(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:24]
}

// docCache is the project's streaming document cache: a parse-once source/sink
// in front of the file collections. It holds NO whole document in memory.
//
//   - The parsed Part stream is recorded to a per-document append log (one
//     length-delimited JSON record per part) and replayed one part at a time.
//   - The reconstruction skeleton is a content-addressed FILE, written as the
//     document parses and opened lazily — ONLY by a writer that reconstructs
//     output. A process-only flow (translate → overlays) never opens it, and the
//     skeleton never flows through the pipeline as Parts.
//   - A tiny SQLite index records per-file staleness (content hash, mtime, size)
//     and the document's log + skeleton refs.
//
// Invariant: a pure optimization over the files. The index key includes the
// content hash + config, so a stale entry is never served; delete `.kapi/cache`
// and a re-read reconstructs identical results.
type docCache struct {
	db  *storage.DB
	dir string // directory holding the per-document logs + skeleton files
}

var docCacheMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "streaming document index (parts log + skeleton file refs)",
		SQL: `CREATE TABLE documents (
			path         TEXT NOT NULL,
			config_key   TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			mtime        INTEGER NOT NULL,
			size         INTEGER NOT NULL,
			parts_ref    TEXT NOT NULL,
			skeleton_ref TEXT NOT NULL,
			format       TEXT NOT NULL,
			PRIMARY KEY (path, config_key)
		);`,
	},
}

// openDocCache opens (creating + migrating) the streaming document cache under
// cacheDir/docs.
func openDocCache(cacheDir string) (*docCache, error) {
	dir := filepath.Join(cacheDir, "docs")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	db, err := storage.Open(filepath.Join(dir, "index.db"))
	if err != nil {
		return nil, err
	}
	if err := storage.Migrate(db, "doc_cache_migrations", docCacheMigrations); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &docCache{db: db, dir: dir}, nil
}

func (c *docCache) close() {
	if c != nil && c.db != nil {
		_ = c.db.Close()
	}
}

// docRow is the index entry for one (path, config).
type docRow struct {
	contentHash string
	partsRef    string
	skeletonRef string
	format      string
}

// lookup returns the fresh index row for (path, configKey), or ok=false when the
// entry is missing or stale relative to the file on disk.
func (c *docCache) lookup(path, configKey string, st os.FileInfo) (docRow, bool) {
	var row docRow
	var mtime, size int64
	q := c.db.QueryRow(
		`SELECT content_hash, mtime, size, parts_ref, skeleton_ref, format
		   FROM documents WHERE path = ? AND config_key = ?`, path, configKey)
	if err := q.Scan(&row.contentHash, &mtime, &size, &row.partsRef, &row.skeletonRef, &row.format); err != nil {
		return docRow{}, false
	}
	if mtime == st.ModTime().UnixNano() && size == st.Size() {
		return row, true
	}
	// Stat changed — re-hash to confirm the bytes actually changed.
	h, err := project.HashFile(path)
	if err != nil || h != row.contentHash {
		return docRow{}, false
	}
	_, _ = c.db.Exec(`UPDATE documents SET mtime = ?, size = ? WHERE path = ? AND config_key = ?`,
		st.ModTime().UnixNano(), st.Size(), path, configKey)
	return row, true
}

// OpenDocument returns a streaming reader for a fresh cached document, or nil on
// a miss. Implements flow.PartCache.
func (c *docCache) OpenDocument(path, configKey string) flow.CachedDocument {
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	row, found := c.lookup(path, configKey, st)
	if !found {
		return nil
	}
	partsPath := filepath.Join(c.dir, row.partsRef)
	if _, err := os.Stat(partsPath); err != nil {
		return nil // log file vanished — treat as a miss, re-parse
	}
	skelPath := ""
	if row.skeletonRef != "" {
		skelPath = filepath.Join(c.dir, row.skeletonRef)
	}
	return &cachedDocument{partsPath: partsPath, skeletonPath: skelPath}
}

// RecordDocument returns a sink that persists a freshly-parsed document as it
// streams. Implements flow.PartCache. Returns a nil interface (not a typed nil)
// when a recorder can't be created, so the runner's nil check works.
func (c *docCache) RecordDocument(path, configKey, formatName string) flow.DocumentRecorder {
	rec := c.newRecorder(path, configKey, formatName)
	if rec == nil {
		return nil
	}
	return rec
}

// newRecorder builds a docRecorder for a fresh parse, or nil on setup failure.
func (c *docCache) newRecorder(path, configKey, formatName string) *docRecorder {
	st, err := os.Stat(path)
	if err != nil {
		return nil
	}
	hash, err := project.HashFile(path)
	if err != nil {
		return nil
	}
	// Content-addressed names keyed by the source hash + config, so identical
	// re-parses reuse the same files and concurrent writers don't collide on a
	// shared name (each writes its own temp, then renames into place).
	base := hashKey(configKey + "\x00" + hash)
	partsRef := "parts-" + base + ".log"
	skelRef := "skel-" + base + ".bin"
	partsTmp := filepath.Join(c.dir, partsRef+".tmp")
	pf, err := os.Create(partsTmp)
	if err != nil {
		return nil
	}
	skelTmp := filepath.Join(c.dir, skelRef+".tmp")
	skel, err := format.NewSkeletonStoreAt(skelTmp)
	if err != nil {
		_ = pf.Close()
		_ = os.Remove(partsTmp)
		return nil
	}
	skel.SetOriginFormat(formatName)
	return &docRecorder{
		c: c, path: path, configKey: configKey, contentHash: hash, format: formatName,
		st: st, partsRef: partsRef, skelRef: skelRef,
		partsTmp: partsTmp, skelTmp: skelTmp,
		pf: pf, pw: bufio.NewWriter(pf), skel: skel,
	}
}

// cachedDocument streams a previously-recorded document from disk.
type cachedDocument struct {
	partsPath    string
	skeletonPath string
}

// Feed streams the document's parts to inCh one at a time, reading the append
// log line by line so the whole document is never held in memory. It closes inCh.
func (d *cachedDocument) Feed(ctx context.Context, inCh chan<- *model.Part) error {
	defer close(inCh)
	f, err := os.Open(d.partsPath)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024) // allow large single-part lines
	for sc.Scan() {
		var cp cachedPart
		if err := json.Unmarshal(sc.Bytes(), &cp); err != nil {
			return err
		}
		parts := fromCachedParts([]cachedPart{cp})
		if len(parts) == 0 {
			continue
		}
		select {
		case inCh <- parts[0]:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return sc.Err()
}

// OpenSkeleton opens the reconstruction skeleton for a writer, streamed from its
// file. Returns nil when the document has no skeleton (e.g. a generative format)
// — the writer then reconstructs from the content model alone. Caller closes it.
func (d *cachedDocument) OpenSkeleton() *format.SkeletonStore {
	if d.skeletonPath == "" {
		return nil
	}
	if _, err := os.Stat(d.skeletonPath); err != nil {
		return nil
	}
	s, err := format.OpenSkeletonStore(d.skeletonPath)
	if err != nil {
		return nil
	}
	return s
}

func (d *cachedDocument) Close() error { return nil }

// docRecorder persists a document as it parses: each part appended to the log,
// the skeleton written to its file by the reader's emitter.
type docRecorder struct {
	c                                    *docCache
	path, configKey, contentHash, format string
	st                                   os.FileInfo
	partsRef, skelRef, partsTmp, skelTmp string
	pf                                   *os.File
	pw                                   *bufio.Writer
	skel                                 *format.SkeletonStore
	parts                                int
}

// SkeletonStore is wired to the reader's emitter so the skeleton is written to
// its file as the document parses.
func (r *docRecorder) SkeletonStore() *format.SkeletonStore { return r.skel }

// Add appends one parsed part to the log (streamed; not buffered into a slice).
func (r *docRecorder) Add(p *model.Part) error {
	cps := toCachedParts([]*model.Part{p})
	if len(cps) == 0 {
		return nil // a part type the cache doesn't model — skip (never replayed)
	}
	b, err := json.Marshal(cps[0])
	if err != nil {
		return err
	}
	if _, err := r.pw.Write(b); err != nil {
		return err
	}
	if err := r.pw.WriteByte('\n'); err != nil {
		return err
	}
	r.parts++
	return nil
}

// Commit finalizes the log + skeleton files and writes the index row. A skeleton
// with no entries is dropped (the format emitted none) so OpenSkeleton returns
// nil and the writer reconstructs from the content model.
func (r *docRecorder) Commit() error {
	if err := r.pw.Flush(); err != nil {
		r.Abort()
		return err
	}
	if err := r.pf.Close(); err != nil {
		r.Abort()
		return err
	}
	skelRef := r.skelRef
	hadSkeleton := r.skel.EntriesWritten() > 0
	if err := r.skel.Close(); err != nil { // persistent: flushes + closes, keeps file
		r.Abort()
		return err
	}
	if err := os.Rename(r.partsTmp, filepath.Join(r.c.dir, r.partsRef)); err != nil {
		r.Abort()
		return err
	}
	if hadSkeleton {
		if err := os.Rename(r.skelTmp, filepath.Join(r.c.dir, r.skelRef)); err != nil {
			r.Abort()
			return err
		}
	} else {
		_ = os.Remove(r.skelTmp)
		skelRef = ""
	}
	_, err := r.c.db.Exec(
		`INSERT INTO documents (path, config_key, content_hash, mtime, size, parts_ref, skeleton_ref, format)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(path, config_key) DO UPDATE SET
		   content_hash = excluded.content_hash, mtime = excluded.mtime, size = excluded.size,
		   parts_ref = excluded.parts_ref, skeleton_ref = excluded.skeleton_ref, format = excluded.format`,
		r.path, r.configKey, r.contentHash, r.st.ModTime().UnixNano(), r.st.Size(), r.partsRef, skelRef, r.format)
	return err
}

// Abort discards the in-progress files without recording an index entry.
func (r *docRecorder) Abort() {
	_ = r.pw.Flush()
	_ = r.pf.Close()
	_ = r.skel.Close()
	_ = os.Remove(r.partsTmp)
	_ = os.Remove(r.skelTmp)
}

// compile-time checks that docCache satisfies the streaming flow seam.
var (
	_ flow.PartCache        = (*docCache)(nil)
	_ flow.CachedDocument   = (*cachedDocument)(nil)
	_ flow.DocumentRecorder = (*docRecorder)(nil)
)
