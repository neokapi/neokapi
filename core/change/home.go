package change

import (
	"context"
	"errors"

	"github.com/neokapi/neokapi/core/model"
)

// A home holds the text of documents and decides when two writers of one
// document conflict. The service reads and writes every document through its
// home and knows nothing else about where the text lives:
//
//   - the file home (package filehome) keeps a document as a file in a
//     working tree and commits by renaming a staged file under an advisory
//     lock;
//   - a workspace home keeps content that has no file in the workspace log;
//   - a server keeps a stream's documents in rows and commits in a
//     transaction.
//
// A home is opened once per document and change set (Session). A session reads
// the document at one head, stages a change against that head, and commits it
// only while the head is still the one it read, or after it has applied the
// change again to the head it finds.

// Home holds documents.
type Home interface {
	// Name is the home as a result reports it: file, workspace or stream:<id>.
	Name() string
	// Open starts work on the document doc names: a project-relative path,
	// container!entry for an archive member, a stream item path or a workspace
	// document key. A document that does not exist is an *Error with
	// CodeNotFound.
	Open(ctx context.Context, doc string) (Session, error)
}

// Homes picks the home of each document a change set names.
type Homes interface {
	For(doc string) (Home, error)
}

// OneHome is Homes that keeps every document in h.
func OneHome(h Home) Homes { return oneHome{h} }

type oneHome struct{ h Home }

func (o oneHome) For(string) (Home, error) { return o.h, nil }

// Editions says how a format holds a document's editions.
type Editions string

const (
	// EditionsInFile: one file holds every edition, as a bilingual
	// interchange file (PO, XLIFF, TMX, xcstrings) does.
	EditionsInFile Editions = "in-file"
	// EditionsPerFile: a file holds one edition, and each other edition of
	// the document lives in a file of its own, as a project's target files
	// do.
	EditionsPerFile Editions = "one-per-file"
)

// DocInfo describes a document open in its home.
type DocInfo struct {
	// Doc is the document's canonical reference. A reference sent in another
	// spelling (./docs/guide.md, or the file of one of its editions) resolves
	// to it, and results echo it.
	Doc string
	// Format is the name of the document's format.
	Format string
	// SourceLocale is the language the document is written in.
	SourceLocale model.LocaleID
	// Editions says how the format holds editions.
	Editions Editions
	// Edition is set when the reference as sent named a file that holds one
	// edition of Doc, such as the German file of an English page: it is that
	// edition, and an operation sent to that file with no edition addresses
	// it.
	Edition *model.EditionKey
	// Derived lists the editions of the document that live in files of their
	// own and exist, so a change to the document's own edition can name them
	// among the editions it leaves on an older basis.
	Derived []model.EditionKey
}

// Place is where an edition of a document lives.
type Place struct {
	// Kind says whether the edition lives in the document, in a file of its
	// own, or nowhere.
	Kind PlaceKind
	// File is the edition's own file, as a document reference, for
	// PlaceOwnFile.
	File string
	// Why says, for PlaceNone, why the edition has no home, as a refusal
	// reports it.
	Why string
}

// PlaceKind says where an edition lives.
type PlaceKind string

const (
	// PlaceInDocument: the document holds the edition: its own edition, or
	// one a bilingual file holds.
	PlaceInDocument PlaceKind = "in-document"
	// PlaceOwnFile: the edition is a file of its own.
	PlaceOwnFile PlaceKind = "own-file"
	// PlaceNone: the edition has no home. A monolingual document outside a
	// project holds one edition and nowhere to keep another.
	PlaceNone PlaceKind = "none"
)

// Session is one document open in its home.
type Session interface {
	// Info describes the document.
	Info() DocInfo
	// Place says where edition k of the document lives.
	Place(k model.EditionKey) Place
	// Read calls fn with each block of the document at its head, in document
	// order, with the editions want names joined in from their own files. It
	// returns the head's digest. fn returning ErrStop ends the read early with
	// no error.
	Read(ctx context.Context, want Want, fn func(*model.Block) error) (head string, err error)
	// Stage applies e to the document and holds the result ready to commit.
	// Nothing a reader of the home sees changes. When e.End reports a
	// refusal, Stage returns that error and holds nothing.
	Stage(ctx context.Context, want Want, e Editor) (Staged, error)
	// Close releases what the session holds. A staged result must be
	// released first.
	Close() error
}

// ErrStop, returned by a Read callback, ends the read early.
var ErrStop = errors.New("change: stop reading")

// ErrRefused is what an Editor's End returns when an operation of the pass
// was refused. The results say which and why.
var ErrRefused = errors.New("change: an operation was refused")

// Want says what a read or a stage needs from a document.
type Want struct {
	// Blocks are the keys of the blocks the caller needs; empty is every
	// block. A home that reads a document whole passes every block anyway,
	// and one that reads by key looks up only these.
	Blocks []string
	// Editions are the editions living in files of their own to join into
	// the blocks.
	Editions []model.EditionKey
	// Own says the document's own file is written: an operation changes its
	// own edition, or an edition it holds in-file. Without it a stage only
	// reads the document.
	Own bool
}

// Editor applies a change set's operations to the blocks of one document. A
// home calls Begin before each pass over the document, Edit with every block
// in document order, and End after the last. A home that finds at commit that
// the document moved under it makes a second pass over the document as it now
// stands; the editor then starts afresh.
type Editor interface {
	Begin()
	// Edit applies the operations addressed to b, in place, and returns the
	// keys of the editions it changed (b.EditionKeyOf of each).
	Edit(b *model.Block) (changed []model.EditionKey, err error)
	// End finishes a pass. It returns ErrRefused when an operation of the
	// pass was refused.
	End() error
}

// StagedFile is one file a staged change writes, or reads and leaves alone.
type StagedFile struct {
	// File is the file as a document reference.
	File string
	// Edition is set for a file that holds one edition of the document.
	Edition *model.EditionKey
	// Before and After are the file's digests around the change. Before is
	// empty for a file the change creates. After equals Before for a file the
	// change leaves as it was.
	Before string
	After  string
	// Written says Commit put the file's new content in place. A commit an
	// I/O error interrupted leaves some files written and others not.
	Written bool
}

// Staged is a change held ready to commit.
type Staged interface {
	// Files lists the files the change touches.
	Files() []StagedFile
	// Diff renders what the change writes, for a person reading a preview,
	// as a unified diff; empty when the home renders none.
	Diff() string
	// LockKeys names the commit locks the change needs, sorted: for the file
	// home, one per file it reads or writes. The service takes the locks of
	// every staged document of a change set in the order of their keys, so
	// two change sets take the locks they share in one order and cannot
	// deadlock, and it refuses a change set two of whose documents need one
	// lock, which is one file named twice.
	LockKeys() []string
	// Lock takes the commit lock key names, blocking until it holds it or
	// ctx ends.
	Lock(ctx context.Context, key string) error
	// Settle makes sure the staged change still applies to the head, with
	// every lock LockKeys names held; it takes any the caller has not. When
	// the head moved since the stage, the home applies the editor to the head
	// as it now stands, once. A refused operation in that pass returns
	// ErrRefused; a head that keeps moving returns an *Error with
	// CodeDocChanged.
	Settle(ctx context.Context) error
	// Commit makes the staged change the head. Settle must have succeeded.
	// After an error, Files reports which files were written.
	Commit(ctx context.Context) error
	// Release drops the commit locks and any staged data. It is safe after
	// Commit, without Settle, and more than once.
	Release() error
}
