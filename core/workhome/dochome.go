package workhome

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	pathpkg "path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/change/filehome"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/storage/filelock"
	"github.com/neokapi/neokapi/core/workspace"
)

// DocLog is where the workspace home records the writes to the documents it
// keeps whole, and what their projection catches up from: the projector of
// the log.
type DocLog interface {
	// CommitDocument appends the record of a write to a whole document,
	// with the document's bytes after it in a blob, only while the
	// document's head in the log is still at c.Expect, and folds it into the
	// projection. A head that moved is workspace.ErrHeadMoved, and nothing
	// is appended.
	CommitDocument(ctx context.Context, c DocCommit) (string, error)
	// CatchUp folds into the projection what the log holds that it has not
	// yet folded.
	CatchUp(ctx context.Context) error
	// DocumentSubjectHead is the local position of the latest operation the
	// log holds on one document, zero for none.
	DocumentSubjectHead(ctx context.Context, key string) (int64, error)
	// Blob reads a blob from the log.
	Blob(ctx context.Context, address string) ([]byte, error)
}

// DocCommit is one write to a document the workspace home keeps whole, as
// the log records it: a content.edit operation naming the blob of the
// document's bytes after the write.
type DocCommit struct {
	// Key is the document's key and Path the reference it was written
	// through.
	Key  string
	Path string
	// Expect is the document's head in the log (DocLog.DocumentSubjectHead)
	// the write was staged on, and Base the operation the folded head is at.
	Expect int64
	Base   string
	// Format reads and writes Data, the document's bytes after the write.
	// Before and After are its revisions around the write (DocumentRevision);
	// Before is empty for the write that opens a document.
	Format string
	Data   []byte
	Before string
	After  string

	Actor       change.Actor
	Origin      string
	Fingerprint string
	Note        string
	Set         *change.Set
	Overridden  []change.Finding
	// Blocks are the editions the write changed.
	Blocks []DocTransition
}

// DocTransition is one edition a write to a whole document changed.
type DocTransition struct {
	Block string
	Key   string
	// Edition is the edition in its text form.
	Edition string
	// Before and After are the edition's revisions around the write, and
	// BeforeRuns and AfterRuns its runs, kept for a person's or an agent's
	// write.
	Before      string
	After       string
	BeforeRuns  []model.Run
	AfterRuns   []model.Run
	Basis       string
	ContentHash string
	ContextHash string
	Ops         []change.Kind
}

// Seeded is a document as a host first hands it to the workspace home: what
// opening it for editing records.
type Seeded struct {
	Data   []byte
	Format string
}

// Documents is the workspace home over documents it keeps whole: each is a
// native document whose bytes live in a blob of the log and whose head is a
// row of document_head. A change reads the document at its head through its
// format, applies the change service's editor, writes the result through the
// same format, and commits by recording the new bytes, only while the
// document's head is still the one the change read. A head that moved
// between the stage and the commit is read again and the change applied to
// it once, as the file home does with a file another writer saved.
//
// Documents serves a KPZ opened for editing: each source document the KPZ
// carries is a document of the home, named by the KPZ's reference and the
// document's name (work.kpz!guide.md).
type Documents struct {
	Store *Store
	Log   DocLog
	// Formats opens each document's reader and writer.
	Formats *registry.FormatRegistry
	// SourceLocale is the language the documents are written in, and
	// TargetLocale the language of the translation a bilingual document
	// holds, where its reader has to be told.
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
	// Prefix is how a reference names the documents of the home: the
	// reference of a document keyed key is Prefix+key. Empty names each by
	// its key.
	Prefix string
	// WorkDir holds the working copies a change reads and writes, and the
	// commit locks every process writing the home shares.
	WorkDir string
	// Seed, when set, is asked for a document the home does not keep yet;
	// what it returns is recorded as the document opened for editing.
	Seed func(ctx context.Context, key string) (Seeded, bool, error)
	// BeforeSettle, when set, is called once a document is staged and before
	// its commit lock is taken.
	BeforeSettle func(doc string)
}

var _ change.Home = (*Documents)(nil)

// Name is the home as a change result reports it.
func (d *Documents) Name() string { return Name }

// Key is the key of the document a reference names, and false for a
// reference outside the home.
func (d *Documents) Key(doc string) (string, bool) {
	key, ok := strings.CutPrefix(doc, d.Prefix)
	if !ok {
		return "", false
	}
	key = strings.TrimPrefix(pathpkg.Clean("/"+strings.TrimPrefix(key, "./")), "/")
	return key, key != "" && key != "."
}

// opener is the actor that records a document opened for editing.
var opener = change.Actor{Kind: change.ActorTool, Name: "open"}

// Put records data as document key's head, written by actor through origin,
// only while the head is still the one the call read: a host opens a document
// for editing with it, and a delivery that rewrites one whole. It returns the
// id of the operation, empty when the head already holds data.
func (d *Documents) Put(ctx context.Context, key, format string, data []byte, actor change.Actor, origin string) (string, error) {
	seq, err := d.Log.DocumentSubjectHead(ctx, key)
	if err != nil {
		return "", err
	}
	if err := d.Log.CatchUp(ctx); err != nil {
		return "", err
	}
	head, _, err := d.Store.Document(ctx, key)
	if err != nil {
		return "", err
	}
	rev := DocumentRevision(data)
	if head.Rev == rev && head.Format == format {
		return "", nil
	}
	return d.Log.CommitDocument(ctx, DocCommit{Key: key, Path: d.Prefix + key, Expect: seq, Base: head.Op,
		Format: format, Data: data, Before: head.Rev, After: rev, Actor: actor, Origin: origin})
}

// Head reads the head of document key and its bytes, with the projection
// caught up first. found is false for a document the home does not keep.
func (d *Documents) Head(ctx context.Context, key string) (DocHead, []byte, bool, error) {
	if err := d.Log.CatchUp(ctx); err != nil {
		return DocHead{}, nil, false, err
	}
	head, found, err := d.Store.Document(ctx, key)
	if err != nil || !found {
		return DocHead{}, nil, false, err
	}
	data, err := d.Log.Blob(ctx, head.Blob)
	if err != nil {
		return DocHead{}, nil, false, fmt.Errorf("workhome: read %s: %w", key, err)
	}
	return head, data, true, nil
}

// read reads document key at its head: the log's position on it, the folded
// head, and the bytes. A document the home does not keep yet is opened from
// Seed when it has one.
func (d *Documents) read(ctx context.Context, key string) (int64, DocHead, []byte, error) {
	for attempt := 0; ; attempt++ {
		seq, err := d.Log.DocumentSubjectHead(ctx, key)
		if err != nil {
			return 0, DocHead{}, nil, err
		}
		head, data, found, err := d.Head(ctx, key)
		if err != nil {
			return 0, DocHead{}, nil, err
		}
		if found {
			return seq, head, data, nil
		}
		if d.Seed == nil || attempt > 0 {
			return 0, DocHead{}, nil, &change.Error{Code: change.CodeNotFound, Field: "at/doc", Message: "no document " + d.Prefix + key}
		}
		seed, ok, err := d.Seed(ctx, key)
		if err != nil {
			return 0, DocHead{}, nil, err
		}
		if !ok {
			return 0, DocHead{}, nil, &change.Error{Code: change.CodeNotFound, Field: "at/doc", Message: "no document " + d.Prefix + key}
		}
		if _, err := d.Put(ctx, key, seed.Format, seed.Data, opener, "open"); err != nil && !errors.Is(err, workspace.ErrHeadMoved) {
			return 0, DocHead{}, nil, err
		}
	}
}

// Open reads the document doc names at its head into a working copy, which
// the file home reads and writes through the document's format.
func (d *Documents) Open(ctx context.Context, doc string) (change.Session, error) {
	key, ok := d.Key(doc)
	if !ok {
		return nil, &change.Error{Code: change.CodeNotFound, Field: "at/doc", Message: "no document " + doc}
	}
	seq, head, data, err := d.read(ctx, key)
	if err != nil {
		return nil, err
	}
	if head.Format == "" {
		return nil, &change.Error{Code: change.CodeUnsupported, Capability: "format", Message: "no format reads " + d.Prefix + key}
	}
	if err := os.MkdirAll(d.WorkDir, 0o700); err != nil {
		return nil, fmt.Errorf("workhome: %w", err)
	}
	dir, err := os.MkdirTemp(d.WorkDir, "doc-")
	if err != nil {
		return nil, fmt.Errorf("workhome: %w", err)
	}
	copyPath := filepath.Join(dir, pathpkg.Base(key))
	if err := os.WriteFile(copyPath, data, 0o600); err != nil {
		_ = os.RemoveAll(dir)
		return nil, fmt.Errorf("workhome: %w", err)
	}
	ref := d.Prefix + key
	located := filehome.Doc{Ref: ref, Path: copyPath, Format: filehome.RegistryBinding(d.Formats, head.Format, ""),
		SourceLocale: d.SourceLocale, Editions: change.EditionsPerFile,
		NoEditionFile: "a document kept whole in the workspace holds its own edition and the translations its format holds in the document"}
	if info := d.Formats.FormatInfo(registry.FormatID(head.Format)); info != nil && info.Interchange {
		located.Editions, located.TargetLocale = change.EditionsInFile, d.TargetLocale
	}
	inner := filehome.New(oneDoc{located}, filehome.Options{LockDir: filepath.Join(dir, "locks")})
	sess, err := inner.Open(ctx, ref)
	if err != nil {
		_ = os.RemoveAll(dir)
		return nil, err
	}
	return &docSession{d: d, key: key, dir: dir, copy: copyPath, inner: sess, seq: seq, head: head}, nil
}

// oneDoc is the layout of one working copy.
type oneDoc struct{ doc filehome.Doc }

func (o oneDoc) Locate(_ context.Context, doc string) (filehome.Doc, error) {
	if doc != o.doc.Ref {
		return filehome.Doc{}, &change.Error{Code: change.CodeNotFound, Field: "at/doc", Message: "no document " + doc}
	}
	return o.doc, nil
}

// docSession is one document of the home open at one head.
type docSession struct {
	d     *Documents
	key   string
	dir   string
	copy  string
	inner change.Session
	seq   int64
	head  DocHead
}

var _ change.StructuralSession = (*docSession)(nil)

func (s *docSession) Info() change.DocInfo { return s.inner.Info() }

func (s *docSession) Place(k model.EditionKey) change.Place { return s.inner.Place(k) }

func (s *docSession) Read(ctx context.Context, want change.Want, fn func(*model.Block) error) (string, error) {
	return s.inner.Read(ctx, want, fn)
}

// Structural lists the structural operations the document's format writes.
func (s *docSession) Structural() []change.Kind {
	if ss, ok := s.inner.(change.StructuralSession); ok {
		return ss.Structural()
	}
	return nil
}

func (s *docSession) Stage(ctx context.Context, want change.Want, e change.Editor) (change.Staged, error) {
	st, err := s.inner.Stage(ctx, want, e)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256([]byte(s.key))
	lock := filepath.Join(s.d.WorkDir, "locks", "document-"+hex.EncodeToString(sum[:8])+".lock")
	return &docStaged{s: s, inner: st, lockKey: lock, seq: s.seq, head: s.head}, nil
}

func (s *docSession) Close() error {
	err := s.inner.Close()
	if rerr := os.RemoveAll(s.dir); err == nil {
		err = rerr
	}
	return err
}

// docStaged is a change to a whole document held ready to commit.
type docStaged struct {
	s       *docSession
	inner   change.Staged
	lockKey string
	lock    *filelock.Lock
	seq     int64
	head    DocHead
	rec     *change.Record
	id      string
	landed  bool
}

var _ change.RecordingStaged = (*docStaged)(nil)

// Files lists the document's working copy as the document: the commit
// records it, and it is written once the record lands.
func (st *docStaged) Files() []change.StagedFile {
	files := st.inner.Files()
	for i := range files {
		files[i].Recorded = true
		files[i].Written = files[i].Written && st.landed
	}
	return files
}

func (st *docStaged) Diff() string { return st.inner.Diff() }

// LockKeys names the document's commit lock, which every process writing the
// home takes in turn.
func (st *docStaged) LockKeys() []string { return []string{st.lockKey} }

func (st *docStaged) Lock(ctx context.Context, key string) error {
	if key != st.lockKey {
		return fmt.Errorf("lock %s: the change to %s takes no such lock", key, st.s.d.Prefix+st.s.key)
	}
	if st.lock != nil {
		return nil
	}
	if st.s.d.BeforeSettle != nil {
		st.s.d.BeforeSettle(st.s.d.Prefix + st.s.key)
	}
	if err := os.MkdirAll(filepath.Dir(key), 0o700); err != nil {
		return fmt.Errorf("create the lock directory: %w", err)
	}
	l, err := filelock.Open(key)
	if err != nil {
		return err
	}
	if err := l.Lock(ctx); err != nil {
		_ = l.Close()
		return err
	}
	st.lock = l
	return nil
}

// Settle reads the document again when another writer moved its head since
// the stage, puts that head in the working copy, and lets the file home
// apply the change to it once.
func (st *docStaged) Settle(ctx context.Context) error {
	if err := st.Lock(ctx, st.lockKey); err != nil {
		return err
	}
	seq, err := st.s.d.Log.DocumentSubjectHead(ctx, st.s.key)
	if err != nil {
		return err
	}
	if seq != st.seq {
		head, data, found, err := st.s.d.Head(ctx, st.s.key)
		if err != nil {
			return err
		}
		if !found {
			return &change.Error{Code: change.CodeDocChanged, Message: st.s.d.Prefix + st.s.key + " left the workspace while the edit was committed"}
		}
		if head.Rev != st.head.Rev {
			if err := replaceCopy(st.s.copy, data); err != nil {
				return err
			}
		}
		st.seq, st.head = seq, head
	}
	return st.inner.Settle(ctx)
}

// replaceCopy puts data in the working copy at path, by rename, so the file
// home's next read of it finds the whole of one head.
func replaceCopy(path string, data []byte) error {
	tmp := path + ".head"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return fmt.Errorf("workhome: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("workhome: %w", err)
	}
	return nil
}

func (st *docStaged) Recording(rec change.Record) { st.rec = &rec }

func (st *docStaged) RecordID() string { return st.id }

// Commit writes the change into the working copy and records the copy's bytes
// as the document's new head, only while the head is the one Settle found.
func (st *docStaged) Commit(ctx context.Context) error {
	if err := st.inner.Commit(ctx); err != nil {
		return err
	}
	files := st.inner.Files()
	if !slices.ContainsFunc(files, func(f change.StagedFile) bool { return f.Written && f.After != f.Before }) {
		return nil
	}
	data, err := os.ReadFile(st.s.copy)
	if err != nil {
		return fmt.Errorf("workhome: %w", err)
	}
	c := DocCommit{Key: st.s.key, Path: st.s.d.Prefix + st.s.key, Expect: st.seq, Base: st.head.Op,
		Format: st.head.Format, Data: data, Before: st.head.Rev, After: DocumentRevision(data)}
	if rec := st.rec; rec != nil {
		c.Actor, c.Origin, c.Fingerprint, c.Set, c.Overridden = rec.Actor, rec.Origin, rec.Fingerprint, rec.Set, rec.Overridden
		if rec.Set != nil {
			c.Note = rec.Set.Note
		}
		keep := rec.Actor.Kind == change.ActorPerson || rec.Actor.Kind == change.ActorAgent
		for _, t := range rec.Transitions {
			dt := DocTransition{Block: t.Ref.Block, Key: t.Key, Edition: editionText(t.Ref.Edition), Before: t.BeforeRev, After: t.AfterRev,
				Basis: t.Basis, ContentHash: t.ContentHash, ContextHash: t.ContextHash, Ops: slices.Clone(t.Ops)}
			if keep {
				dt.BeforeRuns, dt.AfterRuns = t.Before, t.After
			}
			c.Blocks = append(c.Blocks, dt)
		}
	}
	id, err := st.s.d.Log.CommitDocument(ctx, c)
	if errors.Is(err, workspace.ErrHeadMoved) {
		return &change.Error{Code: change.CodeDocChanged,
			Message: c.Path + " changed in the workspace while the edit was committed; read it and send the change again"}
	}
	if err != nil {
		return err
	}
	st.id, st.landed = id, true
	return nil
}

func (st *docStaged) Release() error {
	err := st.inner.Release()
	if st.lock != nil {
		st.lock.Unlock()
		_ = st.lock.Close()
		st.lock = nil
	}
	return err
}
