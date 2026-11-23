package host

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/dataroot"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/rootpath"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/version"
)

// recordingBase names the parts and skeleton files a recording of one source
// writes under one key. The hash reads the parts as a JSON array, so two
// different sets of parts never share a name.
func recordingBase(key docKey, sourceHash string) string {
	b, _ := json.Marshal([]string{key.build, key.partsFormat, key.config, sourceHash})
	return hashKey(string(b))
}

// hashKey returns a short stable hex digest that names the content-addressed
// cache files for a (config, source-hash) pair.
func hashKey(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:24]
}

// docCache is the project's streaming document cache: a parse-once source/sink
// in front of the file collections. It holds NO whole document in memory.
//
//   - The parsed Part stream is recorded to a per-document append log (one
//     JSON record per part) and replayed one part at a time.
//   - The reconstruction skeleton is a content-addressed file, written as the
//     document parses. Only a writer that reconstructs output opens it.
//   - A small SQLite index records per-file staleness (content hash, mtime,
//     size) and the document's log and skeleton refs.
//
// The cache is a pure optimization. The index key includes the content hash and
// config, so a stale entry is never served.
//
// The key also carries the build that recorded the entry, because a reader's
// code decides what it emits for a file. An upgraded kapi therefore parses a
// file again rather than replaying the previous release's parts.
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

	// checkout names the checkout this cache was opened for (its resolved
	// root). A directory that several checkouts share answers a lookup by its
	// size and modification time only for the checkout that recorded it;
	// another checkout's file is compared by its content.
	checkout string

	// producer names what reads a format, for the formats an entry records
	// it handed embedded content to (App.formatProducer). An entry is served
	// only while every one of them still names the same producer. Nil names
	// none.
	producer func(format string) (identity string, cacheable bool)
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
	{
		Version:     2,
		Description: "key entries by build, record shape and config in separate columns",
		// A cache rebuilds what it drops, so the old entries are discarded
		// rather than split.
		SQL: `DROP TABLE documents;
		CREATE TABLE documents (
			path         TEXT NOT NULL,
			build        TEXT NOT NULL,
			parts_format TEXT NOT NULL,
			config_key   TEXT NOT NULL,
			content_hash TEXT NOT NULL,
			mtime        INTEGER NOT NULL,
			size         INTEGER NOT NULL,
			parts_ref    TEXT NOT NULL,
			skeleton_ref TEXT NOT NULL,
			format       TEXT NOT NULL,
			PRIMARY KEY (path, build, parts_format, config_key)
		);`,
	},
	{
		Version:     3,
		Description: "key entries by the path the project names, and record the checkout and the digests of the recorded files",
		// A cache rebuilds what it drops, so the old entries are discarded.
		SQL: `DROP TABLE documents;
		CREATE TABLE documents (
			path            TEXT NOT NULL,
			build           TEXT NOT NULL,
			parts_format    TEXT NOT NULL,
			config_key      TEXT NOT NULL,
			content_hash    TEXT NOT NULL,
			checkout        TEXT NOT NULL,
			mtime           INTEGER NOT NULL,
			size            INTEGER NOT NULL,
			parts_ref       TEXT NOT NULL,
			parts_digest    TEXT NOT NULL,
			skeleton_ref    TEXT NOT NULL,
			skeleton_digest TEXT NOT NULL,
			format          TEXT NOT NULL,
			PRIMARY KEY (path, build, parts_format, config_key)
		);`,
	},
	{
		Version:     4,
		Description: "record the formats a parse handed embedded content to, with what read each",
		// A cache rebuilds what it drops, so the old entries are discarded.
		SQL: `DELETE FROM documents;
		ALTER TABLE documents ADD COLUMN subformats TEXT NOT NULL DEFAULT '';`,
	},
}

// OpenDocCache opens (creating + migrating) the streaming document cache for
// cacheDir. A cacheDir that is a checkout's cache directory (work/cache in the
// checkout's state directory under the data root) binds the cache to that
// checkout: its layout is checked first, the cache lives where ParseCacheDir
// says, and sources are read through the root. Any other cacheDir holds the
// cache beneath it.
func OpenDocCache(cacheDir string) (*docCache, error) {
	root := projectOfCacheDir(cacheDir)
	if root == "" {
		return openDocCacheAt("", parseCacheDirUnder(cacheDir))
	}
	layout := project.LayoutAt(root)
	if err := layout.CheckState(); err != nil {
		return nil, err
	}
	dir, err := ParseCacheDir(layout)
	if err != nil {
		return nil, err
	}
	return openDocCacheAt(root, dir)
}

// projectOfCacheDir returns the project root whose cache directory cacheDir
// is, or "" when cacheDir is not one.
func projectOfCacheDir(cacheDir string) string {
	work := filepath.Dir(cacheDir)
	if filepath.Base(cacheDir) != project.CacheDirName || filepath.Base(work) != project.WorkDirName {
		return ""
	}
	root, _ := dataroot.CheckoutRootOf(filepath.Dir(work))
	return root
}

// openProjectDocCache opens the document cache of the project at layout, in
// its checkout's state directory, bound to the project's root: sources are
// read through the root, so a link out of the project or a FIFO is refused.
func openProjectDocCache(layout project.Layout) (*docCache, error) {
	if err := project.EnsureLayout(layout); err != nil {
		return nil, err
	}
	dir, err := ParseCacheDir(layout)
	if err != nil {
		return nil, err
	}
	return openDocCacheAt(layout.Root, dir)
}

// openDocCacheAt opens the cache files in dir, the directory ParseCacheDir
// named for a project, or one beneath a cache directory outside any project.
// The index is opened only as a regular file, never through a link.
func openDocCacheAt(root, dir string) (*docCache, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	index := filepath.Join(dir, "index.db")
	if st, err := os.Lstat(index); err == nil && !st.Mode().IsRegular() {
		return nil, fmt.Errorf("parse cache index %s is not a regular file", index)
	}
	db, err := storage.Open(index)
	if err != nil {
		return nil, err
	}
	if err := storage.Migrate(db, "doc_cache_migrations", docCacheMigrations); err != nil {
		_ = db.Close()
		return nil, err
	}
	c := &docCache{DB: db, dir: dir, root: root}
	if root != "" {
		c.checkout = dataroot.NormalizePath(root)
	}
	return c, nil
}

// keyPath is the path an entry is filed under: in a project, the source's
// path relative to the project root, so a clone of the project in another
// folder finds the entries another clone recorded; outside one, the path as
// named.
func (c *docCache) keyPath(path string) string {
	if c.root == "" {
		return path
	}
	rel, err := filepath.Rel(c.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return filepath.ToSlash(rel)
}

func (c *docCache) Close() {
	if c != nil && c.DB != nil {
		_ = c.DB.Close()
	}
}

// docRow is the index entry for one (path, config).
type docRow struct {
	subformats     string
	key            docKey
	path           string
	contentHash    string
	partsRef       string
	partsDigest    string
	skeletonRef    string
	skeletonDigest string
	format         string
}

// partsFormat names the shape of the part records the cache writes: the JSON
// form of a block, whose content is its peer editions (Block.Editions and
// Block.Native), with the translation filed under no language and its overlays
// beside it. Every development build shares the version "dev", so the format
// is part of the key too. An entry written with another record shape reads as
// a miss under any version. It never decodes into a block that holds none of
// its content or files an overlay on the wrong edition.
const partsFormat = "editions/2"

// docKey is what the cache files an entry under, beside its path: the build
// that recorded it, the shape of its records and the caller's config key.
type docKey struct {
	build, partsFormat, config string
}

// buildKey namespaces a caller's config key by the build that recorded the
// entry and the shape of its records, so an entry another build produced reads
// as a miss and is re-parsed.
func buildKey(configKey string) docKey {
	return docKey{build: version.Version, partsFormat: partsFormat, config: configKey}
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
	key := buildKey(configKey)
	row := docRow{key: key, path: c.keyPath(path)}
	var checkout string
	var mtime, size int64
	q := c.DB.QueryRow(
		`SELECT content_hash, checkout, mtime, size, parts_ref, parts_digest, skeleton_ref, skeleton_digest, format, subformats
		   FROM documents WHERE path = ? AND build = ? AND parts_format = ? AND config_key = ?`,
		row.path, key.build, key.partsFormat, key.config)
	if err := q.Scan(&row.contentHash, &checkout, &mtime, &size, &row.partsRef, &row.partsDigest,
		&row.skeletonRef, &row.skeletonDigest, &row.format, &row.subformats); err != nil {
		return docRow{}, false
	}
	// The size and modification time stand for the content only in the
	// checkout that recorded them.
	if checkout == c.checkout && mtime == st.ModTime().UnixNano() && size == st.Size() {
		return row, true
	}
	h, err := hashOpen(f)
	if err != nil || h != row.contentHash {
		return docRow{}, false
	}
	_, _ = c.DB.Exec(`UPDATE documents SET checkout = ?, mtime = ?, size = ? WHERE path = ? AND build = ? AND parts_format = ? AND config_key = ?`,
		c.checkout, st.ModTime().UnixNano(), st.Size(), row.path, key.build, key.partsFormat, key.config)
	return row, true
}

// discard removes the index entry for row, so the document is parsed again.
func (c *docCache) discard(row docRow) {
	_, _ = c.DB.Exec(`DELETE FROM documents WHERE path = ? AND build = ? AND parts_format = ? AND config_key = ?`,
		row.path, row.key.build, row.key.partsFormat, row.key.config)
}

// checkCacheEntry reports why p is not a regular file inside dir reached
// through no link.
func checkCacheEntry(dir, p string) error {
	f, err := openCacheEntry(dir, p)
	if err != nil {
		return err
	}
	return f.Close()
}

// openCacheEntry opens a file of the cache: a regular file inside dir,
// reached through no link.
func openCacheEntry(dir, p string) (*os.File, error) {
	return rootpath.OpenRegularNoLink(dir, p)
}

// subformatProducers names, for each format a parse handed embedded content
// to, what reads it now: "format=identity" entries joined by newlines. ok is
// false when one of them cannot be named, and such a parse is not recorded.
func (c *docCache) subformatProducers(formats []string) (string, bool) {
	if len(formats) == 0 || c.producer == nil {
		return "", true
	}
	parts := make([]string, 0, len(formats))
	for _, f := range formats {
		id, ok := c.producer(f)
		if !ok {
			return "", false
		}
		parts = append(parts, f+"="+id)
	}
	return strings.Join(parts, "\n"), true
}

// subformatsHold reports whether every format an entry recorded still names
// the producer it named when the entry was recorded.
func (c *docCache) subformatsHold(recorded string) bool {
	if recorded == "" {
		return true
	}
	if c.producer == nil {
		return false
	}
	for line := range strings.SplitSeq(recorded, "\n") {
		f, want, ok := strings.Cut(line, "=")
		if !ok {
			return false
		}
		if id, cacheable := c.producer(f); !cacheable || id != want {
			return false
		}
	}
	return true
}

// validLog reports whether the regular file p inside dir is a part log whose
// every line decodes as one part and whose bytes digest to want. It reads the
// log once, line by line, as Feed does.
func validLog(dir, p, want string) bool {
	f, err := openCacheEntry(dir, p)
	if err != nil {
		return false
	}
	defer f.Close()
	h := sha256.New()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 16*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		h.Write(line)
		h.Write([]byte{'\n'})
		var cp cachedPart
		if err := json.Unmarshal(line, &cp); err != nil {
			return false
		}
		if len(fromCachedParts([]cachedPart{cp})) != 1 {
			return false
		}
	}
	return sc.Err() == nil && hex.EncodeToString(h.Sum(nil)) == want
}

// fileDigestIn is the sha256 of the regular file p inside dir, in hex, or ""
// when it cannot be read as one.
func fileDigestIn(dir, p string) string {
	f, err := openCacheEntry(dir, p)
	if err != nil {
		return ""
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return ""
	}
	return hex.EncodeToString(h.Sum(nil))
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

// recorded returns the recorded files of row, or nil, which reads as a miss
// and drops the entry, when anything about it does not hold:
//
//   - the file names are the ones the inputs give (recordingBase over the key
//     computed for this lookup and the source's content hash), never names
//     taken from the entry;
//   - each file is a regular file inside the cache directory;
//   - every line of the log decodes as a part, and the log and the skeleton
//     hold the bytes the entry recorded.
//
// A cache restored from elsewhere is checked the same way, so an entry whose
// files were truncated, replaced or renamed is parsed again, never an error.
func (c *docCache) recorded(row docRow) flow.CachedDocument {
	if !c.subformatsHold(row.subformats) {
		c.discard(row)
		return nil
	}
	base := recordingBase(row.key, row.contentHash)
	if row.partsRef != "parts-"+base+".log" || (row.skeletonRef != "" && row.skeletonRef != "skel-"+base+".bin") {
		c.discard(row)
		return nil
	}
	partsPath := filepath.Join(c.dir, row.partsRef)
	if row.partsDigest == "" || !validLog(c.dir, partsPath, row.partsDigest) {
		c.discard(row)
		return nil
	}
	skelPath := ""
	if row.skeletonRef != "" {
		skelPath = filepath.Join(c.dir, row.skeletonRef)
		if fileDigestIn(c.dir, skelPath) != row.skeletonDigest {
			c.discard(row)
			return nil
		}
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
	key := buildKey(configKey)
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
	base := recordingBase(key, hash)
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
		c: c, path: path, key: key, contentHash: hash, format: formatName,
		st: st, partsRef: partsRef, skelRef: skelRef,
		partsTmp: pf.Name(), skelTmp: pf.Name() + ".skel",
		pf: pf, pw: bufio.NewWriter(pf), ph: sha256.New(),
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
	f, err := openCacheEntry(d.dir, d.partsPath)
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
// file. (nil, nil) means the document has no skeleton (e.g. a generative
// format), and the writer reconstructs from the content model alone. Caller
// closes it.
//
// A skeleton that was recorded and cannot be read is an error. Reading it as
// "no skeleton" would serialize the document from the model, lose the source's
// exact formatting, and report success. The cache can be rebuilt, so a fault
// here fails the run.
func (d *cachedDocument) OpenSkeleton() (*format.SkeletonStore, error) {
	if d.skeletonPath == "" {
		return nil, nil
	}
	if err := checkCacheEntry(d.dir, d.skeletonPath); err != nil {
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
	path, contentHash, format            string
	key                                  docKey
	st                                   os.FileInfo
	partsRef, skelRef, partsTmp, skelTmp string
	pf                                   *os.File
	pw                                   *bufio.Writer
	// ph digests the log as it is written, for the entry to record.
	ph    hash.Hash
	skel  *format.SkeletonStore
	parts int
	// release frees the cache's in-flight slot for this document; called
	// exactly once, from Commit or Abort.
	release func()
	// formats lists the formats the parse handed embedded content to, read at
	// Commit (flow.FormatsRecorder). Nil when the runner records none.
	formats func() []string
}

// done releases the in-flight slot (idempotent).
func (r *docRecorder) done() {
	if r.release != nil {
		r.release()
		r.release = nil
	}
}

// SkeletonStore returns the recorder's skeleton store, creating its file on first
// use, so a recording that never calls this (a process-only or read run) writes
// no skeleton file. It returns nil if the file can't be created.
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
		return nil // a part type the cache doesn't model is skipped and never replayed
	}
	b, err := json.Marshal(cps[0])
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if _, err := r.pw.Write(b); err != nil {
		return err
	}
	r.ph.Write(b)
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
	subformats := ""
	if r.formats != nil {
		var ok bool
		if subformats, ok = r.c.subformatProducers(r.formats()); !ok {
			// A reader behind part of the parse cannot be named, so the parse
			// is not kept; the next read parses again.
			r.discardFiles(skelRef)
			return nil
		}
	}
	skelDigest := ""
	if skelRef != "" {
		skelDigest = fileDigestIn(r.c.dir, filepath.Join(r.c.dir, skelRef))
	}
	_, err := r.c.DB.Exec(
		`INSERT INTO documents (path, build, parts_format, config_key, content_hash, checkout, mtime, size,
		                        parts_ref, parts_digest, skeleton_ref, skeleton_digest, format, subformats)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(path, build, parts_format, config_key) DO UPDATE SET
		   content_hash = excluded.content_hash, checkout = excluded.checkout, mtime = excluded.mtime,
		   size = excluded.size, parts_ref = excluded.parts_ref, parts_digest = excluded.parts_digest,
		   skeleton_ref = excluded.skeleton_ref, skeleton_digest = excluded.skeleton_digest, format = excluded.format,
		   subformats = excluded.subformats`,
		r.c.keyPath(r.path), r.key.build, r.key.partsFormat, r.key.config, r.contentHash, r.c.checkout,
		r.st.ModTime().UnixNano(), r.st.Size(), r.partsRef, hex.EncodeToString(r.ph.Sum(nil)), skelRef, skelDigest, r.format, subformats)
	return err
}

// RecordFormatsFrom implements flow.FormatsRecorder.
func (r *docRecorder) RecordFormatsFrom(formats func() []string) { r.formats = formats }

// discardFiles removes the files a committed recording renamed into place,
// for a recording that is then not kept.
func (r *docRecorder) discardFiles(skelRef string) {
	_ = rootpath.Remove(r.c.dir, filepath.Join(r.c.dir, r.partsRef))
	if skelRef != "" {
		_ = rootpath.Remove(r.c.dir, filepath.Join(r.c.dir, skelRef))
	}
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
