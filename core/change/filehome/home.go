// Package filehome is the file home of the change service: each document is a
// file in a working tree, and a change commits by renaming a staged file onto
// it under an advisory lock.
//
// A stage reads the document through its format's reader, with the writer's
// skeleton store wired, hands every block to the service's editor, and writes
// the result through the same format's writer into a temporary file beside
// the document, with the document's mode. Where the reader and the writer
// both stream, the read and the write run together and the document is never
// held whole.
//
// A commit takes the document's lock, hashes the file again, and renames the
// staged file onto it when the file is still what the stage read. When the
// file moved (another kapi process committed first, or a person saved), the
// home reads it again under the lock, applies the editor once more, and
// renames that result; an operation whose precondition the new content breaks
// is refused stale. Two processes editing different blocks of one file both
// land. The lock file sits under .kapi/work/locks inside a project and under
// the temporary directory outside one.
package filehome

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"sync"

	"github.com/neokapi/neokapi/core/atomicfile"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/storage/filelock"
)

// Home is the file home.
type Home struct {
	layout       Layout
	lockDir      string
	prepareLocks func() error
	prepared     sync.Once
	prepareErr   error
	beforeSettle func(doc string)
	backup       string
	writeNothing bool
	writeUnder   string
	materialize  bool
}

// Options configures a Home.
type Options struct {
	// LockDir holds the lock files. Empty is DefaultLockDir.
	LockDir string
	// PrepareLocks, when set, runs once, before the home first opens a lock
	// file, which a commit does and a read never does, and a failure stops
	// that commit before anything is written. The kapi host writes a
	// project's state directory there, with the ignore rule that keeps the
	// lock files out of a commit, and opens the project's recorder, so a read
	// leaves the project as it found it.
	PrepareLocks func() error
	// BeforeSettle, when set, is called after a document is staged and
	// before its commit lock is taken. A test sets it to force two writers to
	// stage against the same content before either commits.
	BeforeSettle func(doc string)
	// BackupSuffix, when set, keeps a copy of each file a commit replaces,
	// beside it with the suffix appended. The copy is written under the
	// commit lock from the bytes the change was applied to.
	BackupSuffix string
	// WriteNothing makes a home that commits no produced document: Produce
	// runs the producer into a digest and stages no file, and Commit writes
	// nothing. A flow run that prints the change set it would apply
	// (--print-ops) commits through such a home.
	WriteNothing bool
	// WriteUnder, with WriteNothing, names a directory whose files the home
	// still writes as any home writes them: the private tree a printing
	// convergence pass drafts into, which its delivery gate reads.
	WriteUnder string
	// Materialize writes every edition file a change writes from the
	// document's skeleton, the way kapi merge and kapi pull write a
	// translation: each block of the document carries the edition the change
	// gave it, or else the one the file held, or else the document's own
	// content. The file then follows the document's structure, so a change
	// to a block the file does not hold yet lands too, and whatever only the
	// file held is gone. Without it an edition file that exists is edited
	// through its own skeleton, and a change to a block it does not hold is
	// refused.
	Materialize bool
}

// DefaultLockDir is where lock files go when the caller names no directory: a
// directory under the temporary directory, one per user, created with mode
// 0700. The kapi host names one in the project's .kapi directory, or in its
// data directory outside a project.
func DefaultLockDir() string {
	return filepath.Join(os.TempDir(), "kapi-locks-"+strconv.Itoa(os.Getuid()))
}

// New returns a file home over layout. A home that only commits the
// documents producers write (Produce) needs no layout and may be given nil;
// Open on it fails.
func New(layout Layout, opts Options) *Home {
	dir := opts.LockDir
	if dir == "" {
		dir = DefaultLockDir()
	}
	return &Home{layout: layout, lockDir: dir, prepareLocks: opts.PrepareLocks, beforeSettle: opts.BeforeSettle, backup: opts.BackupSuffix,
		writeNothing: opts.WriteNothing, writeUnder: opts.WriteUnder, materialize: opts.Materialize}
}

// writes reports whether the home writes a document it produces at path.
func (h *Home) writes(path string) bool {
	if !h.writeNothing {
		return true
	}
	if h.writeUnder == "" {
		return false
	}
	under, err := filepath.Abs(h.writeUnder)
	if err != nil {
		return false
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(under, abs)
	return err == nil && filepath.IsLocal(rel)
}

// Name is "file".
func (h *Home) Name() string { return "file" }

// Open locates doc through the layout.
func (h *Home) Open(ctx context.Context, doc string) (change.Session, error) {
	if h.layout == nil {
		return nil, &change.Error{Code: change.CodeUnreachable, Message: "this file home locates no documents"}
	}
	d, err := h.layout.Locate(ctx, doc)
	if err != nil {
		return nil, err
	}
	return &session{h: h, doc: d, caps: d.Format.Capabilities()}, nil
}

// session is one document open in the file home.
type session struct {
	h        *Home
	doc      Doc
	caps     change.Capabilities
	keyJoins map[model.EditionKey]keyJoin
	// overlay holds, while a stage runs, the files a structural edit
	// rewrote, by path: every pass of the stage reads them from here.
	overlay map[string][]byte
}

func (s *session) Info() change.DocInfo {
	return change.DocInfo{
		Doc:          s.doc.Ref,
		Format:       s.doc.Format.Name,
		SourceLocale: s.doc.SourceLocale,
		Editions:     s.doc.Editions,
		Edition:      s.doc.Edition,
		Derived:      s.doc.Derived,
		Capabilities: s.caps,
	}
}

// own reports whether k names the document's own edition.
func (s *session) own(k model.EditionKey) bool {
	if k.IsZero() {
		return true
	}
	return k.Tone == "" && k.Channel == "" && s.doc.SourceLocale != "" &&
		model.NormalizeLocale(k.Locale) == model.NormalizeLocale(s.doc.SourceLocale)
}

func (s *session) Place(k model.EditionKey) change.Place {
	if s.own(k) {
		return change.Place{Kind: change.PlaceInDocument}
	}
	if s.doc.Editions == change.EditionsInFile {
		// A bilingual file keys its translations by language alone.
		if k.Tone != "" || k.Channel != "" {
			return change.Place{Kind: change.PlaceNone,
				Why: fmt.Sprintf("the %s format keeps one translation per language, and an edition with a tone or a channel has no place in %s", s.doc.Format.Name, s.doc.Ref)}
		}
		return change.Place{Kind: change.PlaceInDocument}
	}
	if f, ok := s.editionFile(k); ok {
		if f.Kept != nil {
			return change.Place{Kind: change.PlaceOwnFile, File: f.Ref, Home: f.Kept.Name()}
		}
		return change.Place{Kind: change.PlaceOwnFile, File: f.Ref}
	}
	return change.Place{Kind: change.PlaceNone, Why: s.doc.NoEditionFile}
}

// editionFile is the file of edition k, with its format filled in.
func (s *session) editionFile(k model.EditionKey) (EditionFile, bool) {
	if s.doc.EditionFile == nil || s.own(k) {
		return EditionFile{}, false
	}
	f, ok := s.doc.EditionFile(k.Canonical())
	if !ok {
		return EditionFile{}, false
	}
	if f.Format.NewReader == nil {
		f.Format = s.doc.Format
	}
	return f, true
}

func (s *session) ownSource() source {
	return s.fileSource(source{path: s.doc.Path, entry: s.doc.Entry})
}

// fileSource is src as the stage reads it: from the overlay when a
// structural edit rewrote it.
func (s *session) fileSource(src source) source {
	if data, ok := s.overlay[overlayKey(src)]; ok {
		return src.with(data)
	}
	return src
}

// overlayKey names a file, or an archive member, in the overlay.
func overlayKey(src source) string {
	if src.entry != "" {
		return src.path + "!" + src.entry
	}
	return src.path
}

// Structural lists the structural operations the document's writer writes.
func (s *session) Structural() []change.Kind {
	if s.doc.Format.NewWriter == nil {
		return nil
	}
	w, err := s.doc.Format.NewWriter()
	if err != nil {
		return nil
	}
	var out []change.Kind
	for _, op := range format.StructuralOps(w) {
		out = append(out, change.Kind(op))
	}
	return out
}

// readPass is a read of src in format f, as this document is read.
func (s *session) readPass(src source, f Binding, fn func(*model.Block) error) pass {
	return pass{src: src, format: f, locale: s.doc.SourceLocale, encoding: s.doc.Encoding, fn: fn}
}

// ownPass is a pass over the document's own file: a bilingual file is read
// with the language of the translation it holds, where the layout knows it,
// so the reader models that translation and the writer writes it.
func (s *session) ownPass(fn func(*model.Block) error) pass {
	p := s.readPass(s.ownSource(), s.doc.Format, fn)
	if s.doc.Editions == change.EditionsInFile {
		p.target = s.doc.TargetLocale
		p.writeLocale = s.doc.TargetLocale
	}
	return p
}

func (s *session) Read(ctx context.Context, want change.Want, fn func(*model.Block) error) (string, error) {
	head, err := hashFile(s.doc.Path)
	if err != nil {
		return "", err
	}
	if head == "" {
		return "", &change.Error{Code: change.CodeNotFound, Field: "at/doc", Message: "no document " + s.doc.Ref}
	}
	if len(want.Editions) == 0 {
		return head, s.ownPass(fn).run(ctx)
	}
	// One pass over the document: its blocks are held while the files of the
	// editions it joins are read and paired with them, which needs every
	// block's key before it pairs any.
	var blocks []*model.Block
	ix := &blockIndex{}
	if err := s.ownPass(func(b *model.Block) error {
		blocks = append(blocks, b)
		ix.add(b)
		return nil
	}).run(ctx); err != nil {
		return "", err
	}
	editions, err := s.joinIndexed(ctx, want.Editions, ix)
	if err != nil {
		return "", err
	}
	for i, b := range blocks {
		join(editions, i, b)
		if err := fn(b); err != nil {
			if errors.Is(err, change.ErrStop) {
				return head, nil
			}
			return head, err
		}
	}
	return head, nil
}

// DocumentBlockKey translates the key of a block in edition k's own file into
// the key of the document block it holds the edition of.
// The join is read once per edition and kept for the session.
func (s *session) DocumentBlockKey(ctx context.Context, k model.EditionKey, key string) (string, bool, error) {
	j, ok := s.keyJoins[k.Canonical()]
	if !ok {
		editions, ix, err := s.joinEditions(ctx, []model.EditionKey{k})
		if err != nil {
			return "", false, err
		}
		j = keyJoin{ix: ix}
		if len(editions) > 0 {
			j.je = editions[0]
		}
		if s.keyJoins == nil {
			s.keyJoins = map[model.EditionKey]keyJoin{}
		}
		s.keyJoins[k.Canonical()] = j
	}
	if j.je == nil {
		return "", false, nil
	}
	got, found := j.je.documentKey(j.ix, key)
	return got, found, nil
}

// keyJoin is an edition's file joined to the document, for translating the
// keys that file reads with.
type keyJoin struct {
	je *joinedEdition
	ix *blockIndex
}

func (s *session) Stage(ctx context.Context, want change.Want, e change.Editor) (change.Staged, error) {
	st := &staged{s: s, want: want, e: e}
	if err := st.run(ctx); err != nil {
		_ = st.Release()
		return nil, err
	}
	return st, nil
}

func (s *session) Close() error { return nil }

// lockPath is the lock file that orders the writers of the file at path: one
// per file, named for the file it guards with any symlink resolved, so two
// links to one file share it. It names the file and creates nothing.
func (h *Home) lockPath(path string) (string, error) {
	target, _, _, err := atomicfile.Resolve(path)
	if err != nil {
		return "", err
	}
	abs, err := filepath.Abs(target)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(abs))
	return filepath.Join(h.lockDir, hex.EncodeToString(sum[:16])+".lock"), nil
}

// openLock opens the lock file at path, creating the lock directory, which
// belongs to this user alone, when it is not there.
func (h *Home) openLock(path string) (*filelock.Lock, error) {
	if h.prepareLocks != nil {
		h.prepared.Do(func() { h.prepareErr = h.prepareLocks() })
		if h.prepareErr != nil {
			return nil, fmt.Errorf("prepare the lock directory: %w", h.prepareErr)
		}
	}
	if err := ensureLockDir(h.lockDir); err != nil {
		return nil, err
	}
	return filelock.Open(path)
}

// ensureLockDir creates dir with mode 0700 and refuses one this user cannot
// hold alone: a link, or a directory others may write. A lock directory
// under a temporary directory other users share could otherwise be made in
// advance by one of them, and the lock files opened there redirected.
func ensureLockDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create the lock directory: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("create the lock directory: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("the lock directory %s is not a directory", dir)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("the lock directory %s can be written by other users; remove it, or name a lock directory of your own", dir)
	}
	return nil
}

// hashFile is the digest of the file at path, or "" when there is none.
func hashFile(path string) (string, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(h.Sum(nil)), nil
}
