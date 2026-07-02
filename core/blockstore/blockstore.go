// Package blockstore defines the substrate for kapi flows: a
// block-addressed, append-only overlay store with multiple providers
// (in-memory, local sqlite cache, remote). Tools operate against a
// Session opened on a Store; the executor wires the right provider
// based on the project's declared store.
//
// See Framework AD-008 for the design.
//
// Design summary:
//   - Blocks are content-addressed by hash. Once written, they are
//     immutable. Flows don't rewrite them; they append overlays.
//   - Overlays are append layers keyed by (kind, blockHash). "kind"
//     is a namespace string ("targets", "annotations/termbase", …).
//     Different tools write different kinds in parallel.
//   - Every provider supports forward-only streaming via
//     Session.Blocks. Random access (GetBlock, GetOverlay) is
//     optional and declared via Capabilities.RandomAccess.
//   - Transactional semantics: Begin → Put*/Get* → Commit or
//     Rollback. Providers that don't support rollback (memory)
//     commit-on-close.
package blockstore

import (
	"context"
	"errors"
	"iter"

	"github.com/neokapi/neokapi/core/klf"
)

// Block is the unit of translation tracking. Aliased from core/klf
// so the store speaks the same type extractors produce.
type Block = klf.Block

// Capabilities advertises what a provider supports. Tools that need
// more than bare streaming probe this at Session.Capabilities() and
// fail fast when missing.
type Capabilities struct {
	// RandomAccess: GetBlock / GetOverlay / ListOverlays are O(log n)
	// or better. memory and cache: yes. remote: server-dependent.
	RandomAccess bool
	// Concurrent: multiple Sessions can write different overlay kinds
	// in parallel without corrupting state. cache (SQLite WAL): yes.
	// memory: no (single goroutine). remote: depends on server-side
	// semantics.
	Concurrent bool
	// Remote: provider is network-backed. Hint for tools to prefer
	// batched reads/writes and avoid per-block RTTs.
	Remote bool
	// Writable: PutBlock / PutOverlay are allowed.
	Writable bool
	// Persistent: the store survives the process and reuses its state
	// across runs (SQLite cache, remote ContentStore). The default
	// in-memory store is NOT persistent — its overlay cache is discarded
	// when the process exits, so writing overlays during a one-shot run
	// is wasted work. Executors use this to route one-shot runs through
	// the plain streaming Tool.Process path instead of SessionTool's
	// overlay-caching SessionProcess. memory: no. cache/remote: yes.
	Persistent bool
}

// Store is the top-level provider handle. A Store is opened once per
// kapi process against a project's declared location, then each flow
// opens Sessions on it.
type Store interface {
	// Begin opens a Session. Sessions hold the transaction scope:
	// every Put* is buffered until Commit succeeds (or discarded
	// on Rollback/Close-without-Commit). For streaming read-only
	// work, Commit is a no-op.
	Begin(ctx context.Context) (Session, error)

	// Capabilities advertises what this provider supports. Safe to
	// call before Begin.
	Capabilities() Capabilities

	// Close releases any long-lived resources (DB handle, open
	// zip). Safe to call multiple times.
	Close() error
}

// Overlay is one append-layer entry for a block. Opaque JSON-serialisable
// payload; schema is owned by the tool kind ("targets", "annotations/qa",
// …).
type Overlay struct {
	// Kind namespaces the overlay. Conventions:
	//   "targets/<locale>"            → translated targets
	//   "annotations/<name>"          → term matches, TM fuzzies, QA
	//   "skeletons/<format>"          → round-trip skeletons
	// Dots and slashes in Kind are allowed; stores must not interpret
	// them beyond indexing.
	Kind string
	// BlockHash is the content-addressed key.
	BlockHash string
	// Payload is the tool-owned JSON body.
	Payload []byte
	// UpdatedAt in UTC. Zero value means the store picks (usually
	// time.Now()).
	UpdatedAt int64 // Unix seconds
}

// BlockFilter scopes a Blocks iteration. Providers that support
// random access can push the filter down; streaming providers iterate
// everything and apply it client-side.
type BlockFilter struct {
	// Collection restricts to one collection by name. Empty = all.
	Collection string
	// Translatable, if set, restricts to blocks where
	// Block.Translatable matches the flag value.
	Translatable *bool
	// Limit caps the number of blocks returned. 0 = no limit.
	Limit int
}

// Session is the transaction handle on a Store. One Session per flow
// run (or per logical unit of work). Close-without-Commit is treated
// as Rollback.
type Session interface {
	Capabilities() Capabilities

	// Blocks streams every block visible to this session, filtered.
	// Providers with RandomAccess may push the filter down; others
	// iterate everything. Order is stable within one session but
	// otherwise unspecified.
	Blocks(filter BlockFilter) iter.Seq2[*Block, error]

	// GetBlock returns a single block by hash. Requires RandomAccess.
	// Returns (nil, ErrNotFound) when the hash is unknown.
	GetBlock(hash string) (*Block, error)

	// PutBlock writes or replaces a block. Provider may coalesce;
	// visible after Commit.
	PutBlock(collection string, b *Block) error

	// GetOverlay returns one overlay by (kind, blockHash). Returns
	// ErrNotFound when absent. Requires RandomAccess.
	GetOverlay(kind, blockHash string) (Overlay, error)

	// PutOverlay appends or replaces an overlay. Idempotent per
	// (kind, blockHash).
	PutOverlay(s Overlay) error

	// ListOverlays streams every overlay of a given kind.
	ListOverlays(kind string) iter.Seq2[Overlay, error]

	// Commit makes buffered writes visible. After Commit the session
	// is closed; further Put* calls return ErrClosed.
	Commit() error

	// Rollback discards buffered writes and closes the session.
	Rollback() error

	// Close releases the session. Equivalent to Rollback if not
	// already committed. Safe to call multiple times.
	Close() error
}

// OverlayEnumerator is an optional Session capability: enumerate every
// overlay regardless of kind, in (kind, blockHash) order. Stores that can
// scan their full overlay space (memory, sqlite cache) implement it; it is
// what the block-store exporter uses to snapshot all in-progress work
// without knowing the kinds up front. Streaming/remote sessions that can't
// enumerate need not implement it — callers fall back to per-kind
// ListOverlays.
type OverlayEnumerator interface {
	// AllOverlays streams every overlay the session can see, ordered by
	// (kind, blockHash) for deterministic export.
	AllOverlays() iter.Seq2[Overlay, error]
}

// BlockPurger is an optional Session capability: drop every block in one call,
// leaving overlays intact. A full re-extraction rebuilds the block set from
// source, so it clears the previous set first rather than relying on per-key
// upserts (whose keys can change between kapi versions). Stores that can't
// bulk-delete need not implement it — callers treat its absence as a no-op.
type BlockPurger interface {
	DeleteBlocks() error
}

// Sentinel errors returned by Store/Session implementations.
var (
	// ErrNotFound indicates a block hash or overlay (kind,hash) not
	// present in the store.
	ErrNotFound = errors.New("blockstore: not found")
	// ErrClosed indicates an operation on a session that has already
	// been committed, rolled back, or closed.
	ErrClosed = errors.New("blockstore: session closed")
	// ErrReadOnly indicates a write attempt against a read-only store.
	ErrReadOnly = errors.New("blockstore: read-only")
	// ErrCapability indicates an operation requiring a capability the
	// provider doesn't advertise (e.g. GetBlock without RandomAccess).
	ErrCapability = errors.New("blockstore: capability not supported")
)
