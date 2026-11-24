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
// land. The lock file of a file sits under locks in the user's data root,
// named for the file (LockFile), whichever home commits it.
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
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/atomicfile"
	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/dataroot"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/rootpath"
	"github.com/neokapi/neokapi/core/storage/filelock"
	"golang.org/x/text/cases"
	"golang.org/x/text/unicode/norm"
)

// Home is the file home.
type Home struct {
	layout       Layout
	prepareLocks func() error
	prepared     sync.Once
	prepareErr   error
	beforeSettle func(doc string)
	backup       string
	writeNothing bool
	writeUnder   string
	materialize  bool
	root         string

	// mu guards permitted, the destinations outside root a person named.
	mu        sync.Mutex
	permitted map[string]bool
}

// Options configures a Home.
type Options struct {
	// PrepareLocks, when set, runs once, before the home first opens a lock
	// file, which a commit does and a read never does, and a failure stops
	// that commit before anything is written. The kapi host writes a
	// project's state directory there and opens the project's recorder, so
	// a read leaves the project as it found it.
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
	// WriteUnder names the private tree a convergence pass drafts into, which
	// its delivery gate reads. The home writes beneath it as it writes beneath
	// Root, through a root on it, and with WriteNothing it writes there alone.
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
	// Root is the directory the home's documents live under, a project's root
	// or the directory a layout serves. A file the home writes beneath it
	// must resolve beneath it too: a path that a symbolic link leads out of
	// Root is refused before anything is staged, and again under the commit
	// lock (rootpath.Contain). A path outside Root as written, one a person
	// named, is written as named. Empty is the DirLayout's Root, if the
	// layout is one, and otherwise no check.
	Root string
}

// LockDir is the directory every lock file lives in: locks in the user's data
// root (dataroot.Dir), created with mode 0700. No caller chooses another, so
// two writers of one file find one lock whatever route they take to it.
func LockDir() string {
	return filepath.Join(dataroot.Dir(), "locks")
}

// LockFile is the lock file that orders the writers of the file at target:
// one per file, in LockDir, named for the file's canonical path (lockName).
// Every commit takes the lock of each file it writes or reads from this
// function and from nothing else. It names the file and creates nothing.
func LockFile(target string) (string, error) {
	resolved, _, _, err := atomicfile.Resolve(target)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256([]byte(lockName(dataroot.NormalizePath(resolved))))
	return filepath.Join(LockDir(), hex.EncodeToString(sum[:16])+".lock"), nil
}

// lockName is the name a lock file is keyed by for the absolute path abs,
// with its links resolved: abs case-folded and in Unicode normal form C. A
// case-insensitive file system (APFS and HFS+ as macOS formats them, NTFS)
// reaches one file by Foo.txt and foo.txt, and a normalization-insensitive
// one by either form of an accented name, so every spelling of one file has
// to give one name, whether the file exists yet or not. On a file system
// that tells the spellings apart, two such files share a lock, and their
// writers wait for each other without need. That costs a wait and never
// lets two writers of one file past each other, so the rule is the same on
// every platform, and no file system is asked how it compares names.
func lockName(abs string) string {
	return norm.NFC.String(cases.Fold().String(abs))
}

// New returns a file home over layout. A home that only commits the
// documents producers write (Produce) needs no layout and may be given nil;
// Open on it fails.
func New(layout Layout, opts Options) *Home {
	root := opts.Root
	if l, ok := layout.(DirLayout); ok && root == "" {
		root = l.Root
	}
	return &Home{layout: layout, prepareLocks: opts.PrepareLocks, beforeSettle: opts.BeforeSettle, backup: opts.BackupSuffix,
		writeNothing: opts.WriteNothing, writeUnder: opts.WriteUnder, materialize: opts.Materialize, root: root}
}

// PermitOutside names a destination a person chose outside the home's root:
// a command-line -o or --out argument, or a file chosen in the desktop. The
// home writes such a path as named. A host never passes a path computed from
// recipe content. Every other path outside the root is refused.
func (h *Home) PermitOutside(path string) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.permitted == nil {
		h.permitted = map[string]bool{}
	}
	h.permitted[filepath.Clean(abs)] = true
}

// outsidePermitted reports whether path is a destination PermitOutside named.
func (h *Home) outsidePermitted(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		return false
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.permitted[filepath.Clean(abs)]
}

// contain refuses a path beneath the home's root that resolves outside it,
// and a path outside the root that no person named (PermitOutside).
func (h *Home) contain(path string) error {
	if h.root == "" {
		return nil
	}
	absRoot, ok := h.under(path)
	if !ok {
		if h.outsidePermitted(path) {
			return nil
		}
		return &change.Error{Code: change.CodeInvalid, Field: "at/doc",
			Message: path + " lies outside the project. Move the recipe or the target so the target lies inside the project"}
	}
	if _, err := rootpath.Contain(absRoot, path); err != nil {
		return &change.Error{Code: change.CodeInvalid, Field: "at/doc", Message: err.Error()}
	}
	return nil
}

// beforeCommit runs between the last containment check and the rename. A test
// sets it to put a link in place there.
var beforeCommit func(path string)

// stage stages what write produces as the new content of the file at path. A
// path beneath the home's root is staged through an os.Root opened on the
// root (atomicfile.StageWithin). The temporary file is created there, and the
// commit renames there. A link that leads out of the root, even one put in
// place after contain, is refused, and no byte is written outside it. A path
// outside the root, one a person named, is staged as named.
func (h *Home) stage(path string, write func(io.Writer) error) (*atomicfile.Staged, error) {
	if root, ok := h.under(path); ok {
		return atomicfile.StageWithin(root, path, write)
	}
	if err := h.contain(path); err != nil {
		return nil, err
	}
	return atomicfile.StageWithParents(path, write)
}

// commit puts a staged file at path in place, and names the link when the
// commit was refused because a link now leads out of the root.
func (h *Home) commit(path string, tmp *atomicfile.Staged) error {
	if beforeCommit != nil {
		beforeCommit(path)
	}
	if err := tmp.Commit(); err != nil {
		if cerr := h.contain(path); cerr != nil {
			return cerr
		}
		return err
	}
	return nil
}

// under reports whether path lies beneath the home's root, and returns the
// root as an absolute path. It is decided from the file system, not from the
// spelling: path is beneath the root when the root is the same directory as
// one of the directories above path, by name, through a link, or under
// another case on a case-insensitive volume. A path beneath the root is then
// written through the root, which refuses it when it resolves outside.
func (h *Home) under(path string) (string, bool) {
	if h == nil {
		return "", false
	}
	if root, ok := beneath(h.root, path); ok {
		return root, true
	}
	// The draft tree a convergence pass writes into is kapi's own, kept in
	// the checkout's state directory outside the checkout, and is written
	// through a root on it as the project is. A home with no root of its own
	// writes that tree as named.
	if h.root == "" {
		return "", false
	}
	return beneath(h.writeUnder, path)
}

// openRead opens the file at path for reading. A file beneath the home's
// root is opened through rootpath.OpenRegular: a link that leads out of the
// root, or anything that is not a regular file, is refused at the open, and a
// FIFO is refused without blocking.
func (h *Home) openRead(path string) (*os.File, error) {
	if h == nil {
		return rootpath.Open("", path)
	}
	return openBeneath(h.root, path)
}

// Digest is the digest of the file at path in the form Produce takes as
// before, or "" when there is none, read as openRead reads it.
func (h *Home) Digest(path string) (string, error) { return h.hashFile(path) }

// hashFile is the digest of the file at path, or "" when there is none.
func (h *Home) hashFile(path string) (string, error) {
	f, err := h.openRead(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer f.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, f); err != nil {
		return "", err
	}
	return "sha256:" + hex.EncodeToString(sum.Sum(nil)), nil
}

// openBeneath opens path through rootpath.OpenRegular when it lies beneath
// root, and as named otherwise (rootpath.Open). A path beneath the root is
// never opened by path.
func openBeneath(root, path string) (*os.File, error) {
	return rootpath.Open(root, path)
}

// beneath reports whether path lies beneath root, decided from the file
// system rather than from the spelling (rootpath.IsBeneath), and returns root
// as an absolute path.
func beneath(root, path string) (string, bool) {
	if root == "" {
		return "", false
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", false
	}
	return absRoot, rootpath.IsBeneath(absRoot, path)
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
	// kept holds, while a stage runs, what each kept edition's keeper held
	// when the pass joined it.
	kept map[model.EditionKey]*Kept
}

var _ change.ContestedSession = (*session)(nil)

// Contested reports whether a write that did not land still names edition k
// of the block keyed block, as the keeper of the edition held it when the
// stage in progress joined it.
func (s *session) Contested(k model.EditionKey, block string) bool {
	kept := s.kept[k.Canonical()]
	return kept != nil && kept.Contested[block]
}

func (s *session) Info() change.DocInfo {
	return change.DocInfo{
		Doc:           s.doc.Ref,
		Format:        s.doc.Format.Name,
		SourceLocale:  s.doc.SourceLocale,
		Editions:      s.doc.Editions,
		Edition:       s.doc.Edition,
		Derived:       s.doc.Derived,
		LanguageNamed: s.doc.LanguageNamed,
		Capabilities:  s.caps,
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
	src.root = s.h.root
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
	if src.root == "" {
		src.root = s.h.root
	}
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
	head, err := s.h.hashFile(s.doc.Path)
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
		if errors.Is(err, change.ErrRefused) {
			// A refusal reports the files the stage read, each as it was.
			files := st.Files()
			for i := range files {
				files[i].After, files[i].Written = files[i].Before, false
			}
			err = &change.StageRefusal{Files: files}
		}
		_ = st.Release()
		return nil, err
	}
	return st, nil
}

func (s *session) Close() error { return nil }

// takeLock takes the lock on the lock file at path, creating the lock
// directory, which belongs to this user alone, when it is not there, and
// sweeping it at most once a day.
func (h *Home) takeLock(ctx context.Context, path string) (*filelock.Lock, error) {
	if h.prepareLocks != nil {
		h.prepared.Do(func() { h.prepareErr = h.prepareLocks() })
		if h.prepareErr != nil {
			return nil, fmt.Errorf("prepare the lock directory: %w", h.prepareErr)
		}
	}
	dir := filepath.Dir(path)
	if err := ensureLockDir(dir); err != nil {
		return nil, err
	}
	filelock.SweepDue(dir, sweepEvery, lockFileMaxAge, sweepLimit)
	return filelock.Acquire(ctx, path)
}

// The lock directory holds one lock file for every file a commit ever took a
// lock on. A sweep removes the ones untouched for lockFileMaxAge, at most once
// every sweepEvery and looking at no more than sweepLimit of them. The
// directory then stays bounded by what was written lately. filelock.Sweep
// removes a file only while it holds the lock on it, and filelock.Acquire
// takes the lock again on a file a sweep removed while it waited.
const (
	lockFileMaxAge = 30 * 24 * time.Hour
	sweepEvery     = 24 * time.Hour
	sweepLimit     = 1000
)

// ensureLockDir creates dir with mode 0700. It refuses a lock directory this
// user does not hold alone: a link, something other than a directory, a
// directory another user owns, or one group or other users may write. The
// owner check is dataroot.CheckOwnedDir, the one the data root passes. A
// directory that cannot be created is reported with the variable that moves
// the data root.
func ensureLockDir(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create the lock directory %s in kapi's data root: %w; make the data root writable, or set %s to a directory you can write", dir, err, dataroot.EnvDir)
	}
	if err := dataroot.CheckOwnedDir(dir); err != nil {
		return fmt.Errorf("refused the lock directory %w; remove it, or name a lock directory of your own", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return fmt.Errorf("create the lock directory: %w", err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o022 != 0 {
		return fmt.Errorf("the lock directory %s can be written by other users; remove it, or name a lock directory of your own", dir)
	}
	return nil
}
