package host

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/rootpath"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/version"
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
// content hash + config, so a stale entry is never served; delete `.kapi/work/cache`
// and a re-read reconstructs identical results.
//
// The key also carries the BUILD that recorded the entry. What a reader emits
// for a given file and config is decided by the reader's code, not by the file
// alone: a release that names blocks differently, emits an annotation it did not
// emit before, or classifies one more span as content produces a different part
// stream from the same bytes. Without the build in the key an upgraded kapi
// replays the previous release's parse until the file itself changes, which is
// the shape of bug the cache exists to be incapable of — a measurement taken
// over content the current reader would not produce.
type docCache struct {
	DB  *storage.DB
	dir string // directory holding the per-document logs + skeleton files

	// root is the project the cache belongs to. A source is read through it,
	// so a file reached through a link that leads out of the project is
	// neither recorded nor served. Empty for a cache outside a project, whose
	// sources are read as named.
	root string

	// inflight guards against two concurrent recordings of the same document
	// (same content + config): converge passes fan locales out concurrently,
	// and every worker that misses on the same source would otherwise record
	// it at once. Only the first recorder wins; the rest get nil and fall
	// back to the live (uncached) read path.
	mu       sync.Mutex
	inflight map[string]bool
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

// OpenDocCache opens (creating + migrating) the streaming document cache under
// cacheDir/docs. A cacheDir that is a project's cache directory
// (<root>/.kapi/work/cache) binds the cache to that project: the state
// directory is checked first, and sources are read through the root.
func OpenDocCache(cacheDir string) (*docCache, error) {
	root := projectOfCacheDir(cacheDir)
	if root != "" {
		if err := project.LayoutAt(root).CheckState(); err != nil {
			return nil, err
		}
	}
	return openDocCacheAt(root, cacheDir)
}

// projectOfCacheDir returns the project root whose cache directory cacheDir
// is, or "" when cacheDir is not one.
func projectOfCacheDir(cacheDir string) string {
	work := filepath.Dir(cacheDir)
	state := filepath.Dir(work)
	if filepath.Base(cacheDir) != "cache" || filepath.Base(work) != project.WorkDirName || filepath.Base(state) != project.StateDirName {
		return ""
	}
	return filepath.Dir(state)
}

// openDocCacheAt opens the cache files beneath cacheDir: a project's cache
// directory that passed Layout.CheckState, or a directory outside any project.
func openDocCacheAt(root, cacheDir string) (*docCache, error) {
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
	return &docCache{DB: db, dir: dir, root: root}, nil
}

func (c *docCache) Close() {
	if c != nil && c.DB != nil {
		_ = c.DB.Close()
	}
}

// docRow is the index entry for one (path, config).
type docRow struct {
	contentHash string
	partsRef    string
	skeletonRef string
	format      string
}

// partsFormat names the shape of the part records the cache writes: the JSON
// form of a block, whose content is its peer editions (Block.Editions and
// Block.Native), with the translation filed under no language and its overlays
// beside it. Every development build shares the version "dev", so the format
// is part of the key too: an entry a build with another record shape wrote
// reads as a miss under any version, rather than decoding into a block that
// holds none of its content or files an overlay on the wrong edition.
const partsFormat = "editions/2"

// buildKey namespaces a caller's config key by the build that recorded the
// entry and the shape of its records, so an entry another build produced reads
// as a miss and is re-parsed.
func buildKey(configKey string) string {
	return version.Version + "\x00" + partsFormat + "\x00" + configKey
}

// openSource opens the source file path. In a project the file must be a
// regular file reached through the root; outside one it is opened as named.
func (c *docCache) openSource(path string) (*os.File, os.FileInfo, error) {
	var f *os.File
	var err error
	if c.root == "" {
		f, err = rootpath.Open("", path)
	} else {
		f, err = rootpath.OpenRegular(c.root, path)
	}
	if err != nil {
		return nil, nil, err
	}
	st, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, nil, err
	}
	return f, st, nil
}

// hashOpen returns the digest of the open file f, read from its start, in the
// form project.HashFile gives.
func hashOpen(f *os.File) (string, error) {
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}

// lookup returns the fresh index row for (path, configKey), or ok=false when the
// entry is missing or stale relative to the open source f.
func (c *docCache) lookup(path, configKey string, f *os.File, st os.FileInfo) (docRow, bool) {
	configKey = buildKey(configKey)
	var row docRow
	var mtime, size int64
	q := c.DB.QueryRow(
		`SELECT content_hash, mtime, size, parts_ref, skeleton_ref, format
		   FROM documents WHERE path = ? AND config_key = ?`, path, configKey)
	if err := q.Scan(&row.contentHash, &mtime, &size, &row.partsRef, &row.skeletonRef, &row.format); err != nil {
		return docRow{}, false
	}
	if mtime == st.ModTime().UnixNano() && size == st.Size() {
		return row, true
	}
	// Stat changed — re-hash to Confirm the bytes actually changed.
	h, err := hashOpen(f)
	if err != nil || h != row.contentHash {
		return docRow{}, false
	}
	_, _ = c.DB.Exec(`UPDATE documents SET mtime = ?, size = ? WHERE path = ? AND config_key = ?`,
		st.ModTime().UnixNano(), st.Size(), path, configKey)
	return row, true
}

// OpenDocument returns a streaming reader for a fresh cached document, or nil on
// a miss. Implements flow.PartCache.
func (c *docCache) OpenDocument(path, configKey string) flow.CachedDocument {
	f, st, err := c.openSource(path)
	if err != nil {
		return nil
	}
	row, found := c.lookup(path, configKey, f, st)
	_ = f.Close()
	if !found {
		return nil
	}
	return c.recorded(row)
}

// recorded returns the recorded files of row, or nil when its log file has
// gone, or is not a regular file inside the cache directory, which reads as a
// miss.
func (c *docCache) recorded(row docRow) flow.CachedDocument {
	partsPath := filepath.Join(c.dir, row.partsRef)
	if _, err := rootpath.CheckRegular(c.dir, partsPath); err != nil {
		return nil
	}
	skelPath := ""
	if row.skeletonRef != "" {
		skelPath = filepath.Join(c.dir, row.skeletonRef)
	}
	return &cachedDocument{dir: c.dir, partsPath: partsPath, skeletonPath: skelPath}
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
	configKey = buildKey(configKey)
	f, st, err := c.openSource(path)
	if err != nil {
		return nil
	}
	hash, err := hashOpen(f)
	_ = f.Close()
	if err != nil {
		return nil
	}
	// Content-addressed names keyed by the source hash + config, so identical
	// re-parses reuse the same final files. Only one recording per document
	// may be in flight in this process (converge workers share the cache and
	// race on shared sources); losers fall back to the live read path.
	base := hashKey(configKey + "\x00" + hash)
	c.mu.Lock()
	if c.inflight == nil {
		c.inflight = make(map[string]bool)
	}
	if c.inflight[base] {
		c.mu.Unlock()
		return nil
	}
	c.inflight[base] = true
	c.mu.Unlock()
	release := func() {
		c.mu.Lock()
		delete(c.inflight, base)
		c.mu.Unlock()
	}
	partsRef := "parts-" + base + ".log"
	skelRef := "skel-" + base + ".bin"
	// Unique temp names (CreateTemp), not derived ones: another PROCESS (the
	// desktop app beside a CLI run) may be recording the same document at the
	// same time, and a shared ".tmp" name would truncate its in-progress file.
	pf, err := os.CreateTemp(c.dir, partsRef+".*.tmp")
	if err != nil {
		release()
		return nil
	}
	// The skeleton store is created lazily, on the first SkeletonStore() call, so a
	// lean (non-reconstructing) recording writes no skeleton file at all.
	return &docRecorder{
		c: c, path: path, configKey: configKey, contentHash: hash, format: formatName,
		st: st, partsRef: partsRef, skelRef: skelRef,
		partsTmp: pf.Name(), skelTmp: pf.Name() + ".skel",
		pf: pf, pw: bufio.NewWriter(pf),
		release: release,
	}
}

// cachedDocument streams a previously-recorded document from disk.
type cachedDocument struct {
	// dir is the cache directory that holds both files.
	dir          string
	partsPath    string
	skeletonPath string
}

// Feed streams the document's parts to inCh one at a time, reading the append
// log line by line so the whole document is never held in memory. It closes inCh.
func (d *cachedDocument) Feed(ctx context.Context, inCh chan<- *model.Part) error {
	defer close(inCh)
	f, err := rootpath.OpenRegular(d.dir, d.partsPath)
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
// file. (nil, nil) means the document has no skeleton (e.g. a generative format)
// — the writer then reconstructs from the content model alone. Caller closes it.
//
// A recorded-but-unreadable skeleton is an ERROR, not "no skeleton": the two used
// to be conflated, so a missing or unopenable cache file made the run
// re-serialize the document — losing the source's exact formatting — and report
// success. Faithful write-back is the contract; the cache is rebuildable, so a
// fault here is worth failing over.
func (d *cachedDocument) OpenSkeleton() (*format.SkeletonStore, error) {
	if d.skeletonPath == "" {
		return nil, nil
	}
	if _, err := os.Stat(d.skeletonPath); err != nil {
		return nil, fmt.Errorf("recorded skeleton %s is not readable: %w", d.skeletonPath, err)
	}
	s, err := format.OpenSkeletonStore(d.skeletonPath)
	if err != nil {
		return nil, err
	}
	return s, nil
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
	// release frees the cache's in-flight slot for this document; called
	// exactly once, from Commit or Abort.
	release func()
}

// done releases the in-flight slot (idempotent).
func (r *docRecorder) done() {
	if r.release != nil {
		r.release()
		r.release = nil
	}
}

// SkeletonStore returns the recorder's skeleton store, creating its file lazily on
// first use — so a lean recording (a process-only/read run that never calls this)
// writes no skeleton file. nil if the file can't be created.
func (r *docRecorder) SkeletonStore() *format.SkeletonStore {
	if r.skel == nil {
		s, err := format.NewSkeletonStoreAt(r.skelTmp)
		if err != nil {
			return nil
		}
		s.SetOriginFormat(r.format)
		r.skel = s
	}
	return r.skel
}

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
	defer r.done()
	if err := r.pw.Flush(); err != nil {
		r.Abort()
		return err
	}
	if err := r.pf.Close(); err != nil {
		r.Abort()
		return err
	}
	skelRef := r.skelRef
	hadSkeleton := r.skel != nil && r.skel.EntriesWritten() > 0
	if r.skel != nil {
		if err := r.skel.Close(); err != nil { // persistent: flushes + closes, keeps file
			r.Abort()
			return err
		}
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
	_, err := r.c.DB.Exec(
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
	defer r.done()
	_ = r.pw.Flush()
	_ = r.pf.Close()
	if r.skel != nil {
		_ = r.skel.Close()
	}
	_ = os.Remove(r.partsTmp)
	_ = os.Remove(r.skelTmp)
}

// compile-time checks that docCache satisfies the streaming flow seam.
var (
	_ flow.PartCache        = (*docCache)(nil)
	_ flow.CachedDocument   = (*cachedDocument)(nil)
	_ flow.DocumentRecorder = (*docRecorder)(nil)
)
