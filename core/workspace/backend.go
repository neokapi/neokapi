// Package workspace holds the context a machine account has accumulated across
// every project it works on: the terms, the voice profiles, the content memory,
// the unit-state working set and the context graph.
//
// A project keeps two pools. The PROJECTION lives in the checkout, at
// `.kapi/work/store.db`, and holds what a parse produced: blocks, overlays and
// extraction stamps. The CONTEXT store lives here, outside every checkout, and
// holds what a person authored or decided. Two checkouts of one project (a
// second clone, a git worktree) each keep a projection of their own and share
// one context store, so a decision recorded on one branch is in force on the
// other and a `git switch` moves no authored context.
//
// # Where the authoritative copy lives
//
// A workspace is reached through a Backend. The local backend is the only one
// shipped: it keeps the workspace as a directory of SQLite files under the
// user's data root. The interface is drawn so an adapter that keeps the
// authoritative copy elsewhere can sit behind it without the callers changing:
// Registry and Project hand back database handles the adapter has materialized
// locally, and Record and Since carry the operations a synchronizing adapter
// would exchange with its authority.
//
// # The shape on disk
//
//	<root>/workspace.db          the project registry, the context graph and the
//	                             operation log
//	<root>/projects/<key>.db     one context store per project
//
// One database per project rather than one file with a project column on every
// table. The terms, memory and voice schemas are then unchanged by moving out
// of the checkout, write contention and corruption are bounded to the project
// that caused them, exporting a project is copying a file, and what was learned
// in a project stays scoped to it. Questions that span projects are answered
// from the workspace graph, whose node ids already carry the project dimension,
// with a fan-out over the project databases for anything the graph does not
// hold.
package workspace

import (
	"context"
	"errors"
	"time"

	"github.com/neokapi/neokapi/core/storage"
)

// ProjectKey identifies a project inside a workspace. It is
// project.KapiProject.Identity: the recipe's stable `id:`, or its `name:` where
// the recipe carries no id.
type ProjectKey string

// Kind names a backend implementation, for a message that has to say where a
// workspace is being read from.
type Kind string

// KindLocal is the backend that keeps the workspace as a directory of SQLite
// files on this machine.
const KindLocal Kind = "local"

// Descriptor is what a backend says about itself.
type Descriptor struct {
	// Kind names the implementation.
	Kind Kind
	// Location is where the workspace is kept, in the spelling a person would
	// recognise: a directory for the local backend.
	Location string
	// ReadOnly reports that this backend can answer reads and refuses writes.
	// The local backend reports it when the workspace directory cannot be
	// written, which is what a check running in a write-restricted sandbox
	// meets.
	ReadOnly bool
}

// Op is one recorded change to a workspace.
//
// The log is what a synchronizing backend exchanges with its authority, and
// what a background synchronizer would read to learn what this machine did
// while it was offline. Two logs merge by union: an operation is the same
// operation in every log that holds its ID, so recording one a log already
// holds changes nothing, and the merged log is ordered by ID whichever side
// was read first.
type Op struct {
	// ID names the operation in every log that holds it (see NewOpID). Record
	// assigns one to an operation that arrives without; an operation that
	// arrives with one, because it was read from another log, keeps it.
	ID string
	// Seq is the position at which this log received the operation. It is
	// local to one log: it is what a watcher polling Head and reading Since
	// uses as its cursor, and the order across logs is the ID. It is assigned
	// by Record and ignored on input.
	Seq int64
	// Address, when set, is the content address of what the operation says.
	// Two operations with one address say the same thing, so a log holds the
	// first and recording the second changes nothing: recording one decision
	// twice, on one machine or on two, is one operation.
	Address string
	// Project names the project the operation concerns. Empty for an operation
	// about the workspace itself.
	Project ProjectKey
	// Kind says what happened, in a vocabulary the workspace owns (see the
	// Op* constants).
	Kind string
	// Payload carries the operation's detail as JSON. It may be nil.
	Payload []byte
	// At is when the operation was accepted, in UTC.
	At time.Time
}

// OpQuery narrows a reading of the operation log. A zero OpQuery asks for
// every operation.
type OpQuery struct {
	// After is the local position to read from, exclusive.
	After int64
	// KindPrefix keeps the operations whose kind starts with it: "context."
	// for the context operations, "context.keep" for one kind.
	KindPrefix string
	// Project keeps one project's operations.
	Project ProjectKey
	// Limit caps the number returned. Zero or less asks for every one.
	Limit int
}

// Operation kinds the workspace itself records.
const (
	// OpRegisterProject records a project registering, or re-registering with a
	// changed display name or a checkout path not seen before.
	OpRegisterProject = "project.register"
	// OpForgetProject records a project being removed from the workspace: its
	// registration and the context store behind it.
	OpForgetProject = "project.forget"
)

// Backend is where a workspace's authoritative copy lives.
//
// Every method is safe for concurrent use. Registry and Project return handles
// the BACKEND owns: a caller reads and writes through them and closes the
// backend, never a handle.
type Backend interface {
	// Describe reports the backend's kind, where it keeps the workspace, and
	// whether it can be written.
	Describe() Descriptor

	// Registry opens the workspace-wide database, creating it on first use. It
	// holds the project registry, the context graph and the operation log.
	Registry(ctx context.Context) (*storage.DB, error)

	// Project opens one project's context store, creating it on first use.
	Project(ctx context.Context, key ProjectKey) (*storage.DB, error)

	// Forget releases one project's context store and discards it. A key the
	// workspace holds no store for is not an error, so a caller cleaning up
	// after a partial write need not look first.
	Forget(ctx context.Context, key ProjectKey) error

	// Record appends operations to the log and returns them as the log holds
	// them, in the order given. An operation with no ID is given one that
	// sorts after every id the log holds. An operation whose ID or Address the
	// log already holds is not written again, and the operation already held
	// is returned in its place, which is what makes merging two logs by union
	// idempotent. Where two logs recorded one Address under different IDs, the
	// older ID stands: an arriving operation with an older ID replaces the one
	// held, so both logs end on the same operation.
	Record(ctx context.Context, ops ...Op) ([]Op, error)

	// Since returns up to limit operations this log received after the local
	// position given, in the order it received them. A limit of zero or less
	// asks for every operation. A caller that wants the order across machines
	// sorts the result by ID (SortOps).
	Since(ctx context.Context, after int64, limit int) ([]Op, error)

	// Select returns the operations a query names, in the order this log
	// received them. It is Since narrowed to kinds and a project, answered by
	// the backend rather than by reading every operation and discarding most
	// of them: a subsystem folding its own operations reads those alone.
	Select(ctx context.Context, q OpQuery) ([]Op, error)

	// Head returns the local position of the last operation this log
	// received, and zero for an empty log. It moves whenever an operation is
	// recorded here or merged in from another log.
	//
	// It is what a surface watching the workspace for change reads: one number
	// per poll, rather than the operations themselves. A process that has seen
	// Head can ask Since for what it missed. It is also the revision a
	// retrieval answer reports as the state it was read at. Both callers read
	// it repeatedly, so it costs one query rather than a walk of the log.
	Head(ctx context.Context) (int64, error)

	// PutBlob stores bytes an operation names and returns their address:
	// "sha256:" and the lower-case hex digest of the bytes. Storing bytes the
	// backend already holds writes nothing and returns the same address, so
	// two logs merged by union hold one copy of every blob.
	PutBlob(ctx context.Context, data []byte) (string, error)

	// Blob returns the bytes stored under an address, and ErrNoBlob for an
	// address the backend does not hold.
	Blob(ctx context.Context, digest string) ([]byte, error)

	// Close releases every handle the backend opened. It is idempotent.
	Close() error
}

// ErrNoBlob reports a blob address the workspace does not hold.
var ErrNoBlob = errors.New("workspace: no blob at that address")

// ErrReadOnly reports a write asked of a workspace whose backend cannot write.
// The local backend reports it when the workspace directory is not writable.
var ErrReadOnly = errors.New("workspace: this workspace is open for reading only")

// ErrNoProjectKey reports an operation that needs to name a project and was
// given no key.
var ErrNoProjectKey = errors.New("workspace: no project key")
