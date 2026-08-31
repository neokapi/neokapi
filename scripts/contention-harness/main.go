// Command contention-harness measures SQLite write contention for the
// project store, to answer one question with numbers: can the four
// per-project databases — the content memory, the terms store, the block
// cache and the working store — live in ONE file without the single-writer
// lock that SQLite enforces per file turning into starvation?
//
// Today those four files partition the writers. A converge run writing
// memory entries, an extraction holding a long purge-and-refill transaction
// over the block cache, and a status poller reading unit state never contend
// with one another, because they hold write locks on different files. Merged
// into `.kapi/work/store.db` they all queue behind one lock, and `busy_timeout`
// (5s, set by core/storage) is the only thing between that queue and an
// error.
//
// The harness runs the real stores — core/blockstore/sqlitestore,
// memory.SQLiteStore, terms.SQLiteStore, core/state.WorkStore — against a
// synthetic project at dogfood scale, in six topologies. The first four vary
// only WHICH FILES the subsystems share, keeping today's write discipline —
// a connection pool per subsystem and deferred transactions:
//
//	-mode=split       the four files of today (the baseline)
//	-mode=merged      every subsystem opened against one file
//	-mode=hybrid      everything but the block cache merged
//	-mode=work-apart  everything but the working store merged
//
// The last two vary the write discipline as well, and are what a merged store
// would actually ship with — one shared handle per process, in place of a pool
// per subsystem:
//
//	-mode=merged-immediate  one shared handle, BEGIN IMMEDIATE on every write tx
//	-mode=merged-gated      the above, plus an in-process write gate
//
// The distinction matters because a deferred transaction that reads before it
// writes has to upgrade its lock, and SQLite refuses that upgrade IMMEDIATELY
// rather than consulting busy_timeout — so the failure mode the first four
// modes measure is partly an artefact of the write discipline, not of merging.
//
// The seventh mode measures what was actually built:
//
//	-mode=projectdb   core/projectdb.Open — the shipped store, its shipped stores
//
// It is the acceptance run. Where merged-gated hand-rolls the discipline in this
// file (a hand-built DSN, shim writers, an explicit permit around each library
// call), projectdb opens the real handle through storage.ProjectOptions() and
// drives the real memory, terms, block and working stores over it, with the
// permit taken inside core/storage where no call site can forget it. If the two
// modes disagree, the design was measured and the implementation is not it.
//
// For each topology it reports, per workload, op counts, errors split into
// lock contention versus everything else, and the latency distribution.
//
// Run it as:
//
//	go run -tags fts5 ./scripts/contention-harness
//
// The fts5 tag must be spelled in one comma-separated -tags flag together
// with any other tag: `go` does not union repeated -tags flags, and the
// content memory's FTS5 rebuild — one of the workloads measured here — is
// exactly what notices when it is dropped.
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
)

// ─── Configuration ──────────────────────────────────────────────

// config is the shape of one measurement run. The defaults describe the
// dogfood project this decision is being taken for: roughly 75k blocks, a
// content memory that has accumulated 20k entries, a terms store of a couple
// of thousand concepts, and a decision recorded for every block.
type config struct {
	mode        topology
	dir         string
	duration    time.Duration
	blocks      int
	entries     int
	concepts    int
	units       int
	convergeJ   int
	overlayN    int
	memoryPutN  int
	memoryOneAt int // of memoryPutN, how many go one-transaction-per-entry
	extractN    int
	extractTick time.Duration
	decisionHz  int
	commitTick  time.Duration
	statusHz    int
	keepData    bool
}

// topology names which databases share a file.
type topology string

const (
	// topoSplit is today's layout: four files, four independent write locks.
	topoSplit topology = "split"
	// topoMerged puts every subsystem in `.kapi/work/store.db` — the proposal.
	topoMerged topology = "merged"
	// topoHybrid merges everything except the block cache, on the assumption
	// that the block cache is the hottest writer. It is the first fallback
	// the decision needs quantified once merged fails.
	topoHybrid topology = "hybrid"
	// topoWorkApart merges everything except the working store. It is the
	// second fallback, and it exists because the first one does not help:
	// the writer that saturates the lock is the content memory, not the
	// block cache, and the workload that starves is the drip of small
	// unit-state writes. This topology keeps that drip on a file of its own.
	topoWorkApart topology = "work-apart"
	// topoMergedImmediate is the merged store as it would actually be built:
	// one shared handle for every subsystem in a process, and every write
	// transaction begun IMMEDIATE. A deferred transaction takes a read lock
	// first and must upgrade it to write; SQLite refuses a contended upgrade
	// without consulting busy_timeout, because waiting could deadlock two
	// transactions that each hold a read lock. IMMEDIATE takes the write lock
	// up front, where the busy handler applies and the wait is legal.
	topoMergedImmediate topology = "merged-immediate"
	// topoMergedGated adds the in-process write gate: one permit shared by
	// every writer holding the same handle, taken for the length of a write
	// transaction. Writers in one process then queue in Go, in FIFO order,
	// instead of racing into SQLite's busy handler, whose backoff is not fair
	// and which is what lets a steady stream of large writes shut out a drip
	// of small ones.
	topoMergedGated topology = "merged-gated"
	// topoProjectDB is the shipped store: core/projectdb.Open, whose handle
	// comes from storage.OpenWith(path, storage.ProjectOptions()) — the same
	// two properties merged-gated models by hand, but taken inside
	// core/storage, where every write a subsystem issues passes through them
	// whether or not the subsystem knows they exist. The subsystem stores are
	// the real ones, not this file's shims.
	topoProjectDB topology = "projectdb"
)

// shared reports whether the topology opens ONE handle per process for every
// subsystem, rather than today's pool per subsystem. It implies the merged
// file layout and IMMEDIATE transactions.
func (m topology) shared() bool {
	return m == topoMergedImmediate || m == topoMergedGated || m == topoProjectDB
}

// gated reports whether writers in one process queue behind a single permit.
func (m topology) gated() bool { return m == topoMergedGated || m == topoProjectDB }

// real reports whether the topology drives core/projectdb rather than this
// file's model of it.
func (m topology) real() bool { return m == topoProjectDB }

// ─── Store paths ────────────────────────────────────────────────

// storePaths resolves where each subsystem's database lives under a project
// root, for one topology. Two subsystems sharing a path is the whole point:
// the constructors accept it (the migration ledgers are separately named —
// `cache_migrations`, `sievepen_migrations`, `termbase_migrations`, `state` —
// and the table namespaces do not collide), so merging is a matter of
// pointing them at the same file.
type storePaths struct {
	memory string
	terms  string
	blocks string
	work   string
	units  string
}

func newStorePaths(root string, m topology) storePaths {
	kapi := filepath.Join(root, ".kapi")
	merged := filepath.Join(kapi, "store.db")
	p := storePaths{units: filepath.Join(kapi, "units")}
	switch m {
	case topoMerged:
		p.memory, p.terms, p.blocks, p.work = merged, merged, merged, merged
	case topoHybrid:
		p.memory, p.terms, p.work = merged, merged, merged
		p.blocks = filepath.Join(kapi, "cache", "blocks.db")
	case topoWorkApart:
		p.memory, p.terms, p.blocks = merged, merged, merged
		p.work = filepath.Join(kapi, "work", "state.db")
	case topoMergedImmediate, topoMergedGated, topoProjectDB:
		p.memory, p.terms, p.blocks, p.work = merged, merged, merged, merged
	default:
		p.memory = filepath.Join(kapi, "memory.db")
		p.terms = filepath.Join(kapi, "terms.db")
		p.blocks = filepath.Join(kapi, "cache", "blocks.db")
		p.work = filepath.Join(kapi, "work", "state.db")
	}
	return p
}

// mkdirs creates the directories every path needs. storage.Open creates the
// file but not its parent.
func (p storePaths) mkdirs() error {
	for _, dir := range []string{
		filepath.Dir(p.memory), filepath.Dir(p.terms),
		filepath.Dir(p.blocks), filepath.Dir(p.work), p.units,
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create %s: %w", dir, err)
		}
	}
	return nil
}

// ─── The shared handle the merged store would ship with ─────────

// pragmas are what core/storage applies on open. They are repeated here
// because the immediate-mode handle is built by hand: core/storage owns no
// _txlock knob, and the instruction is to measure the redesign without
// changing core/storage to do it.
var pragmas = []string{
	"PRAGMA busy_timeout=5000",
	"PRAGMA journal_mode=WAL",
	"PRAGMA synchronous=NORMAL",
	"PRAGMA foreign_keys=ON",
	"PRAGMA cache_size=-131072",
	"PRAGMA wal_autocheckpoint=10000",
	"PRAGMA temp_store=MEMORY",
}

// openImmediate opens path exactly as storage.Open does, plus
// _txlock=immediate, so every transaction database/sql begins on this handle
// is a BEGIN IMMEDIATE. mattn applies underscore-prefixed DSN parameters to
// every pooled connection as it is established, which is the only way a
// per-connection setting reaches all of them.
//
// The driver is registered by core/storage's blank import; the harness does
// not import it directly, because which driver core/storage registers is a
// build-tag decision that belongs to core/storage. The harness is a cgo tool
// (it is built with fts5), so it requires the cgo driver and says so rather
// than guessing at another driver's DSN vocabulary.
func openImmediate(path string) (*storage.DB, error) {
	const driver = "sqlite3"
	if !slices.Contains(sql.Drivers(), driver) {
		return nil, fmt.Errorf("immediate-mode handle needs the %q driver (build with cgo); registered: %v",
			driver, sql.Drivers())
	}
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	dsn := path + sep + strings.Join([]string{
		"_foreign_keys=1",
		"_busy_timeout=5000",
		"_synchronous=NORMAL",
		"_cache_size=-131072",
		"_txlock=immediate",
	}, "&")
	raw, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	raw.SetMaxOpenConns(25)
	raw.SetMaxIdleConns(5)
	raw.SetConnMaxLifetime(30 * time.Minute)
	for _, p := range pragmas {
		if _, err := raw.Exec(p); err != nil { //nolint:noctx // startup pragmas
			raw.Close()
			return nil, fmt.Errorf("apply %s: %w", p, err)
		}
	}
	return &storage.DB{DB: raw}, nil
}

// projectDB is one process's handle on the project store: a single pool every
// subsystem shares, and — in the gated topology — a single permit every write
// transaction in the process passes through.
//
// The permit is a buffered channel rather than a mutex because Go's channel
// wait queue is FIFO. That is the property being tested: SQLite's busy handler
// backs off with no memory of who has been waiting longest, so a writer that
// wants the lock for two milliseconds can lose to a writer that takes it for
// two seconds, over and over. A FIFO permit gives the short writer its turn.
type projectDB struct {
	db   *storage.DB
	gate chan struct{}
}

// openProjectDB opens the shared handle. Gated topologies get the permit.
func openProjectDB(path string, m topology) (*projectDB, error) {
	db, err := openImmediate(path)
	if err != nil {
		return nil, err
	}
	p := &projectDB{db: db}
	if m.gated() {
		p.gate = make(chan struct{}, 1)
	}
	return p, nil
}

// write runs fn holding the process's write permit, if there is one. Every
// write transaction on a shared handle goes through here, including the ones
// begun inside the store libraries: the permit is taken around the library
// call, which is exactly the extent of its transaction.
func (p *projectDB) write(fn func() error) error {
	if p.gate != nil {
		p.gate <- struct{}{}
		defer func() { <-p.gate }()
	}
	return fn()
}

func (p *projectDB) Close() error { return p.db.Close() }

// ─── Store shims over the shared handle ─────────────────────────

// The block store and the working store are constructed from a PATH today
// (sqlitestore.New, state.OpenWork), so neither can be handed an existing
// pool the way memory.NewSQLiteStoreFromDB and terms.NewSQLiteStoreFromDB can.
// These two shims issue the same SQL those packages issue, against the shared
// handle, so the shared-handle topologies measure the same work. They are the
// harness's model of the constructor the merged store would grow; if the
// numbers say merge, that constructor is the change to make.

// overlayWriter writes a batch of overlays in ONE transaction — a converge
// worker's unit of block-store work.
type overlayWriter interface {
	writeBatch(ctx context.Context, overlays []blockstore.Overlay) error
	Close() error
}

// storeOverlayWriter is today's path: an autocommit session on a block store
// with its own pool.
type storeOverlayWriter struct{ store blockstore.Store }

func (w *storeOverlayWriter) writeBatch(ctx context.Context, overlays []blockstore.Overlay) error {
	sess, err := w.store.Begin(ctx)
	if err != nil {
		return err
	}
	defer sess.Close()
	for _, o := range overlays {
		if err := sess.PutOverlay(o); err != nil {
			return err
		}
	}
	return sess.Commit()
}

func (w *storeOverlayWriter) Close() error { return w.store.Close() }

// sharedOverlayWriter is the redesign: one IMMEDIATE transaction on the
// process's shared handle, under the write permit.
type sharedOverlayWriter struct{ p *projectDB }

func (w *sharedOverlayWriter) writeBatch(ctx context.Context, overlays []blockstore.Overlay) error {
	return w.p.write(func() error {
		tx, err := w.p.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer func() { _ = tx.Rollback() }()
		for _, o := range overlays {
			if o.UpdatedAt == 0 {
				o.UpdatedAt = time.Now().Unix()
			}
			if _, err := tx.ExecContext(ctx, `
				INSERT INTO overlays (kind, block_hash, payload, updated_at)
				VALUES (?, ?, ?, ?)
				ON CONFLICT(kind, block_hash) DO UPDATE SET
					payload=excluded.payload,
					updated_at=excluded.updated_at
			`, o.Kind, o.BlockHash, o.Payload, o.UpdatedAt); err != nil {
				return err
			}
		}
		return tx.Commit()
	})
}

func (w *sharedOverlayWriter) Close() error { return nil } // the handle is shared

// workWriter is the decision loop's view of the working store.
type workWriter interface {
	Put(ctx context.Context, u state.UnitState) error
	Commit(ctx context.Context) error
	Close() error
}

// sharedWork issues state.WorkStore's statements against the shared handle.
// The serialization it commits to is the package's own (state.WriteCommitted),
// so the once-per-run write costs what it really costs.
type sharedWork struct {
	p         *projectDB
	committed string
}

func (w *sharedWork) Put(ctx context.Context, u state.UnitState) error {
	variant, _ := u.Variant.MarshalText()
	payload, err := json.Marshal(u)
	if err != nil {
		return err
	}
	return w.p.write(func() error {
		_, err := w.p.db.ExecContext(ctx, `
INSERT INTO unit_state (unit, variant, scope, content_hash, context_hash, payload, staged)
VALUES (?, ?, ?, ?, ?, ?, 1)
ON CONFLICT(unit, variant) DO UPDATE SET
    scope = excluded.scope, content_hash = excluded.content_hash,
    context_hash = excluded.context_hash, payload = excluded.payload,
    staged = MAX(unit_state.staged, excluded.staged)`,
			u.Unit, string(variant), u.Scope, u.ContentHash, u.ContextHash, string(payload))
		return err
	})
}

func (w *sharedWork) Commit(ctx context.Context) error {
	var pending int
	if err := w.p.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM unit_state WHERE staged = 1`).Scan(&pending); err != nil {
		return err
	}
	if pending == 0 {
		return nil
	}
	rows, err := w.p.db.QueryContext(ctx, `SELECT payload FROM unit_state ORDER BY unit, variant`)
	if err != nil {
		return err
	}
	var units []state.UnitState
	for rows.Next() {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			rows.Close()
			return err
		}
		var u state.UnitState
		if err := json.Unmarshal([]byte(payload), &u); err != nil {
			rows.Close()
			return err
		}
		units = append(units, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if err := state.WriteCommitted(w.committed, units); err != nil {
		return err
	}
	return w.p.write(func() error {
		_, err := w.p.db.ExecContext(ctx, `UPDATE unit_state SET staged = 0 WHERE staged = 1`)
		return err
	})
}

func (w *sharedWork) Close() error { return nil } // the handle is shared

// ─── Measurement ────────────────────────────────────────────────

// reservoirSize bounds what one stream retains. Quantiles are computed from a
// uniform sample of the run rather than every observation, so a workload that
// completes a million operations costs the same memory as one that completes a
// thousand. The exact count, the exact error split and the exact maximum are
// tracked outside the reservoir and are never approximated.
const reservoirSize = 50_000

// stream accumulates the latency and error record of one kind of operation.
type stream struct {
	mu        sync.Mutex
	name      string
	ops       int64
	busy      int64
	other     int64
	cancelled int64
	max       time.Duration
	total     time.Duration
	samples   []time.Duration
	seen      int64
	rnd       *rand.Rand
}

func newStream(name string) *stream {
	return &stream{
		name:    name,
		samples: make([]time.Duration, 0, 1024),
		rnd:     rand.New(rand.NewPCG(0x5eed, uint64(len(name)))),
	}
}

// record files one completed operation. Operations that failed because the
// run's deadline passed are counted separately and kept out of the latency
// distribution: a cancelled write says nothing about how long the lock was
// held.
func (s *stream) record(d time.Duration, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)) {
		s.cancelled++
		return
	}
	s.ops++
	s.total += d
	if err != nil {
		if isLockContention(err) {
			s.busy++
		} else {
			s.other++
		}
	}
	if d > s.max {
		s.max = d
	}
	s.seen++
	if len(s.samples) < reservoirSize {
		s.samples = append(s.samples, d)
		return
	}
	// Reservoir sampling (Vitter's algorithm R): every observation has an
	// equal chance of being retained, so the quantiles describe the whole run
	// and not just its first fifty thousand operations.
	if j := s.rnd.Int64N(s.seen); j < reservoirSize {
		s.samples[j] = d
	}
}

// isLockContention reports whether err is SQLite refusing because another
// connection holds the write lock. core/storage classifies the same way
// (storage.isBusyErr, unexported); neither the cgo nor the pure-Go driver
// surfaces a portable sentinel through database/sql, so the phrasing is what
// there is to match on.
func isLockContention(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "database is locked") ||
		strings.Contains(s, "SQLITE_BUSY") ||
		strings.Contains(s, "database table is locked") ||
		strings.Contains(s, "table is locked")
}

// streamStats is the reportable summary of a stream. Durations are
// milliseconds so the JSON that crosses the process boundary carries no
// unit ambiguity.
type streamStats struct {
	Name      string  `json:"name"`
	Ops       int64   `json:"ops"`
	Busy      int64   `json:"busy"`
	Other     int64   `json:"other"`
	Cancelled int64   `json:"cancelled"`
	P50       float64 `json:"p50Ms"`
	P95       float64 `json:"p95Ms"`
	P99       float64 `json:"p99Ms"`
	Max       float64 `json:"maxMs"`
	Mean      float64 `json:"meanMs"`
}

func (s *stream) stats() streamStats {
	s.mu.Lock()
	defer s.mu.Unlock()
	sorted := make([]time.Duration, len(s.samples))
	copy(sorted, s.samples)
	slices.Sort(sorted)
	st := streamStats{
		Name: s.name, Ops: s.ops, Busy: s.busy, Other: s.other, Cancelled: s.cancelled,
		P50: ms(quantile(sorted, 0.50)),
		P95: ms(quantile(sorted, 0.95)),
		P99: ms(quantile(sorted, 0.99)),
		Max: ms(s.max),
	}
	if s.ops > 0 {
		st.Mean = ms(s.total / time.Duration(s.ops))
	}
	return st
}

func quantile(sorted []time.Duration, q float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	i := max(int(math.Ceil(q*float64(len(sorted))))-1, 0)
	if i >= len(sorted) {
		i = len(sorted) - 1
	}
	return sorted[i]
}

func ms(d time.Duration) float64 { return float64(d.Nanoseconds()) / 1e6 }

// recorder owns the streams of one process, in the order they were created so
// the report reads top to bottom in workload order.
type recorder struct {
	mu    sync.Mutex
	order []*stream
	byKey map[string]*stream
}

func newRecorder() *recorder { return &recorder{byKey: map[string]*stream{}} }

func (r *recorder) stream(name string) *stream {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.byKey[name]; ok {
		return s
	}
	s := newStream(name)
	r.byKey[name] = s
	r.order = append(r.order, s)
	return s
}

// observe times fn and files the result under name.
func (r *recorder) observe(name string, fn func() error) error {
	s := r.stream(name)
	start := time.Now()
	err := fn()
	s.record(time.Since(start), err)
	return err
}

func (r *recorder) all() []streamStats {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]streamStats, 0, len(r.order))
	for _, s := range r.order {
		out = append(out, s.stats())
	}
	return out
}

// ─── Synthetic project data ─────────────────────────────────────

// collections is how many documents the synthetic block corpus is spread
// over. It matters because the extraction workload re-extracts a slice of
// documents, and that slice is addressed by collection.
const collections = 150

func collectionOf(i int) string { return fmt.Sprintf("docs/%03d", i%collections) }

func blockHash(i int) string { return fmt.Sprintf("k1_%040x", i) }

// sourceText builds a few hundred bytes of plausible source content, so the
// serialized block payload is representative rather than a token.
func sourceText(i int) string {
	return fmt.Sprintf(
		"Unit %d. The project store keeps every block it has read, together with "+
			"the overlays each tool has written over it, so a later run can tell what "+
			"changed rather than re-reading the world. This sentence exists to give "+
			"the payload a realistic size; block %d belongs to collection %s.",
		i, i, collectionOf(i))
}

func makeBlock(i int) *blockstore.Block {
	return &blockstore.Block{
		ID:           fmt.Sprintf("u%d", i),
		Hash:         blockHash(i),
		Translatable: true,
		Source:       []model.Run{model.TextR(sourceText(i))},
		Properties: model.BlockProperties{
			File:      collectionOf(i) + ".md",
			Line:      i % 400,
			Component: fmt.Sprintf("Section%d", i%37),
			Element:   "p",
		},
	}
}

func makeEntry(i int) memory.Entry {
	return memory.Entry{
		ID: fmt.Sprintf("e%08d", i),
		Variants: map[model.LocaleID][]model.Run{
			"en": {model.TextR(fmt.Sprintf("The store records decision %d against the unit it was made for.", i))},
			"nb": {model.TextR(fmt.Sprintf("Lageret registrerer avgjørelse %d mot enheten den ble tatt for.", i))},
		},
		HintSrcLang: "en",
	}
}

func makeConcept(i int) terms.Concept {
	return terms.Concept{
		ID:         fmt.Sprintf("c%06d", i),
		Domain:     "software",
		Definition: fmt.Sprintf("Concept %d of the synthetic terms store.", i),
		Terms: []terms.Term{
			{Text: fmt.Sprintf("content memory %d", i), Locale: "en", Status: model.TermPreferred},
			{Text: fmt.Sprintf("innholdsminne %d", i), Locale: "nb", Status: model.TermApproved},
			{Text: fmt.Sprintf("legacy term %d", i), Locale: "en", Status: model.TermDeprecated},
		},
	}
}

func makeUnitState(i int) state.UnitState {
	return state.UnitState{
		Unit:        blockHash(i),
		Variant:     model.Variant("nb"),
		Status:      model.TargetStatusTranslated,
		Origin:      model.Origin{Kind: "memory", Reference: fmt.Sprintf("e%08d", i%20000)},
		TargetHash:  fmt.Sprintf("t1_%040x", i),
		Scope:       collectionOf(i),
		ContentHash: blockHash(i),
		ContextHash: fmt.Sprintf("x1_%016x", i),
		Updated:     time.Now().UTC().Format(time.RFC3339),
	}
}

// ─── Seeding ────────────────────────────────────────────────────

// ─── The shipped store ──────────────────────────────────────────

// projectLayout describes the synthetic project to core/projectdb. The paths
// are the same ones newStorePaths computes for the merged topologies —
// `.kapi/work/store.db` and `.kapi/state/` — because project.Layout computes them the
// same way; the assertion in openRealProject keeps that true.
func projectLayout(root string) project.Layout {
	return project.Layout{
		Root:       root,
		RecipePath: filepath.Join(root, project.RecipeFileName),
		StateDir:   filepath.Join(root, ".kapi"),
	}
}

// openRealProject opens the shipped project store and checks that it landed on
// the file this harness measures. A mismatch would silently measure two
// different databases against each other.
func openRealProject(ctx context.Context, root string, p storePaths) (*projectdb.DB, error) {
	layout := projectLayout(root)
	if got := layout.StorePath(); got != p.memory {
		return nil, fmt.Errorf("project store at %s, harness measuring %s", got, p.memory)
	}
	db, err := projectdb.Open(ctx, layout)
	if err != nil {
		return nil, fmt.Errorf("open project store: %w", err)
	}
	if db.Memory() == nil {
		_ = db.Close()
		return nil, errors.New("project store opened without a content memory (no SQLite driver?)")
	}
	return db, nil
}

// seed builds the synthetic project. It runs strictly sequentially: every
// migration ledger is created here, once, so the measured run never pays for
// a first open and never measures two openers racing to switch a fresh
// database into WAL mode (a race core/storage already absorbs, and not the
// question being asked).
func seed(ctx context.Context, root string, p storePaths, cfg config) error {
	if err := p.mkdirs(); err != nil {
		return err
	}
	if cfg.mode.real() {
		return seedProject(ctx, root, p, cfg)
	}
	if err := seedBlocks(ctx, p, cfg); err != nil {
		return err
	}
	if err := seedMemoryAndTerms(ctx, p, cfg); err != nil {
		return err
	}
	return seedState(ctx, p, cfg)
}

// seedProject builds the same corpus through the shipped store, so the measured
// run starts from a database core/projectdb created and migrated.
func seedProject(ctx context.Context, root string, p storePaths, cfg config) error {
	db, err := openRealProject(ctx, root, p)
	if err != nil {
		return fmt.Errorf("seed: %w", err)
	}
	defer db.Close()

	const blockChunk = 5000
	for start := 0; start < cfg.blocks; start += blockChunk {
		sess, err := db.Blocks().Begin(ctx)
		if err != nil {
			return fmt.Errorf("seed: begin block session: %w", err)
		}
		end := min(start+blockChunk, cfg.blocks)
		for i := start; i < end; i++ {
			if err := sess.PutBlock(collectionOf(i), makeBlock(i)); err != nil {
				_ = sess.Rollback()
				return fmt.Errorf("seed: put block %d: %w", i, err)
			}
		}
		if err := sess.Commit(); err != nil {
			return fmt.Errorf("seed: commit blocks: %w", err)
		}
	}

	mem := db.Memory()
	const entryChunk = 2000
	for start := 0; start < cfg.entries; start += entryChunk {
		end := min(start+entryChunk, cfg.entries)
		batch := make([]memory.Entry, 0, end-start)
		for i := start; i < end; i++ {
			batch = append(batch, makeEntry(i))
		}
		if err := mem.BulkAddWithStream(ctx, batch, ""); err != nil {
			return fmt.Errorf("seed: bulk add entries: %w", err)
		}
	}
	if err := mem.RebuildFuzzyIndex(ctx); err != nil {
		return fmt.Errorf("seed: rebuild fuzzy index: %w", err)
	}
	if err := mem.RebuildSearchIndex(ctx); err != nil {
		return fmt.Errorf("seed: rebuild search index: %w", err)
	}

	tb := db.Terms()
	for i := range cfg.concepts {
		if err := tb.AddConcept(ctx, makeConcept(i)); err != nil {
			return fmt.Errorf("seed: add concept %d: %w", i, err)
		}
	}

	work := db.Work()
	for i := range cfg.units {
		if err := work.Put(ctx, makeUnitState(i)); err != nil {
			return fmt.Errorf("seed: put unit %d: %w", i, err)
		}
	}
	return work.Commit(ctx)
}

func seedBlocks(ctx context.Context, p storePaths, cfg config) error {
	store, err := sqlitestore.New(p.blocks)
	if err != nil {
		return fmt.Errorf("seed: open block store: %w", err)
	}
	defer store.Close()

	// Chunked so the seeding transaction never holds the whole corpus in one
	// journal; the measured extraction workload exercises the long-transaction
	// case deliberately, seeding need not.
	const chunk = 5000
	for start := 0; start < cfg.blocks; start += chunk {
		sess, err := store.Begin(ctx)
		if err != nil {
			return fmt.Errorf("seed: begin block session: %w", err)
		}
		end := min(start+chunk, cfg.blocks)
		for i := start; i < end; i++ {
			if err := sess.PutBlock(collectionOf(i), makeBlock(i)); err != nil {
				_ = sess.Rollback()
				return fmt.Errorf("seed: put block %d: %w", i, err)
			}
		}
		if err := sess.Commit(); err != nil {
			return fmt.Errorf("seed: commit blocks: %w", err)
		}
	}
	return nil
}

func seedMemoryAndTerms(ctx context.Context, p storePaths, cfg config) error {
	db, err := storage.Open(p.memory)
	if err != nil {
		return fmt.Errorf("seed: open memory: %w", err)
	}
	defer db.Close()
	mem, err := memory.NewSQLiteStoreFromDB(db)
	if err != nil {
		return fmt.Errorf("seed: migrate memory: %w", err)
	}

	const chunk = 2000
	for start := 0; start < cfg.entries; start += chunk {
		end := min(start+chunk, cfg.entries)
		batch := make([]memory.Entry, 0, end-start)
		for i := start; i < end; i++ {
			batch = append(batch, makeEntry(i))
		}
		if err := mem.BulkAddWithStream(ctx, batch, ""); err != nil {
			return fmt.Errorf("seed: bulk add entries: %w", err)
		}
	}
	// The bulk path deliberately skips the FTS5 tables; populating them is
	// the same set-based rebuild the measured workload re-runs mid-flight.
	if err := mem.RebuildFuzzyIndex(ctx); err != nil {
		return fmt.Errorf("seed: rebuild fuzzy index: %w", err)
	}
	if err := mem.RebuildSearchIndex(ctx); err != nil {
		return fmt.Errorf("seed: rebuild search index: %w", err)
	}

	// The terms store shares the memory pool when the topology merges them —
	// the case the decision is about — and opens its own otherwise.
	termsDB := db
	if p.terms != p.memory {
		termsDB, err = storage.Open(p.terms)
		if err != nil {
			return fmt.Errorf("seed: open terms: %w", err)
		}
		defer termsDB.Close()
	}
	tb, err := terms.NewSQLiteStoreFromDB(termsDB)
	if err != nil {
		return fmt.Errorf("seed: migrate terms: %w", err)
	}
	for i := range cfg.concepts {
		if err := tb.AddConcept(ctx, makeConcept(i)); err != nil {
			return fmt.Errorf("seed: add concept %d: %w", i, err)
		}
	}
	return nil
}

func seedState(ctx context.Context, p storePaths, cfg config) error {
	w, err := state.OpenWork(ctx, p.work, p.units)
	if err != nil {
		return fmt.Errorf("seed: open work store: %w", err)
	}
	defer w.Close()
	for i := range cfg.units {
		if err := w.Put(ctx, makeUnitState(i)); err != nil {
			return fmt.Errorf("seed: put unit %d: %w", i, err)
		}
	}
	return w.Commit(ctx)
}

// ─── Workloads ──────────────────────────────────────────────────

// Stream names. They are the rows of the report, and the child process
// reports under the same names as the parent so the tables line up.
const (
	nW1Overlay   = "W1 converge  overlay batch"
	nW1Memory    = "W1 converge  memory put"
	nW1MemBatch  = "W1 converge  memory put batched"
	nW2Extract   = "W2 extract   purge+refill tx (child proc)"
	nW3Fuzzy     = "W3 rebuild   fuzzy index"
	nW3Search    = "W3 rebuild   search index"
	nW4Put       = "W4 decisions unit put"
	nW4Commit    = "W4 decisions commit"
	nW5Open      = "W5 status    open (child proc)"
	nW5Poll      = "W5 status    poll (child proc)"
	nW5CountBlks = "W5 status    block count (child proc)"
)

// ─── The simulated processes ────────────────────────────────────

// The workloads are grouped into three OS processes, the same way in every
// topology so the comparison is like for like:
//
//	process A (this one)  W1 converge, W3 the content memory's FTS rebuild,
//	                      W4 the decision loop — the stages of one kapi run,
//	                      sharing one handle where the topology shares handles
//	process B (child)     W2 re-extraction, holding the long transaction
//	process C (child)     W5 the status poller, read only
//
// W2 is a genuinely separate process on purpose. An in-process write gate can
// only order the writers that hold the same handle; if the long transaction
// were inside process A the gate would order that too, and the measurement
// would flatter the design by removing the one writer it cannot control.

// parentStores is process A's access to the project store.
type parentStores struct {
	overlays overlayWriter
	mem      *memory.SQLiteStore
	// write runs a write transaction under the process's write permit. It is
	// the identity function in topologies that have no gate, and it is what
	// wraps the transactions the store libraries begin internally
	// (memory.Add, the FTS rebuilds), which take no handle from the caller.
	write   func(func() error) error
	work    workWriter
	closers []func() error
}

func (ps *parentStores) Close() {
	for _, v := range slices.Backward(ps.closers) {
		_ = v()
	}
}

func openParentStores(ctx context.Context, root string, p storePaths, cfg config) (*parentStores, error) {
	ps := &parentStores{write: func(fn func() error) error { return fn() }}
	fail := func(err error) (*parentStores, error) {
		ps.Close()
		return nil, err
	}

	// The shipped store needs no `write` wrapper at all: the permit is taken
	// inside core/storage, by the same BeginTx and ExecContext every subsystem
	// already calls. That absence is the point of this mode — in merged-gated
	// the wrapper is what makes the numbers, and here nothing in the harness
	// is doing anything.
	if cfg.mode.real() {
		db, err := openRealProject(ctx, root, p)
		if err != nil {
			return fail(fmt.Errorf("process A: %w", err))
		}
		ps.closers = append(ps.closers, db.Close)
		ps.overlays = &storeOverlayWriter{store: db.Blocks()}
		ps.mem = db.Memory()
		ps.work = db.Work()
		return ps, nil
	}

	if cfg.mode.shared() {
		pdb, err := openProjectDB(p.memory, cfg.mode)
		if err != nil {
			return fail(fmt.Errorf("process A: open project store: %w", err))
		}
		ps.closers = append(ps.closers, pdb.Close)
		mem, err := memory.NewSQLiteStoreFromDB(pdb.db)
		if err != nil {
			return fail(fmt.Errorf("process A: content memory: %w", err))
		}
		ps.overlays = &sharedOverlayWriter{p: pdb}
		ps.mem = mem
		ps.write = pdb.write
		ps.work = &sharedWork{p: pdb, committed: p.units}
		return ps, nil
	}

	store, err := sqlitestore.NewAutocommit(p.blocks)
	if err != nil {
		return fail(fmt.Errorf("process A: open block store: %w", err))
	}
	ps.overlays = &storeOverlayWriter{store: store}
	ps.closers = append(ps.closers, ps.overlays.Close)

	memDB, err := storage.Open(p.memory)
	if err != nil {
		return fail(fmt.Errorf("process A: open content memory: %w", err))
	}
	ps.closers = append(ps.closers, memDB.Close)
	if ps.mem, err = memory.NewSQLiteStoreFromDB(memDB); err != nil {
		return fail(fmt.Errorf("process A: content memory: %w", err))
	}

	work, err := state.OpenWork(ctx, p.work, p.units)
	if err != nil {
		return fail(fmt.Errorf("process A: open work store: %w", err))
	}
	ps.work = work
	ps.closers = append(ps.closers, work.Close)
	return ps, nil
}

// runWorkloads drives every simulated process against a seeded project for
// cfg.duration and returns what each observed.
func runWorkloads(ctx context.Context, root string, p storePaths, cfg config) ([]streamStats, error) {
	rec := newRecorder()
	// Create every stream up front so a workload that never completed an
	// operation still appears in the report as a zero row rather than
	// vanishing.
	for _, n := range []string{nW1Overlay, nW1Memory, nW1MemBatch, nW3Fuzzy, nW3Search, nW4Put, nW4Commit} {
		rec.stream(n)
	}

	ps, err := openParentStores(ctx, root, p, cfg)
	if err != nil {
		return nil, err
	}
	defer ps.Close()

	runCtx, cancel := context.WithTimeout(ctx, cfg.duration)
	defer cancel()

	children := make([]*childHandle, 0, 2)
	for _, role := range []string{roleExtract, roleStatus} {
		c, err := startChild(ctx, role, cfg)
		if err != nil {
			return nil, err
		}
		children = append(children, c)
	}

	var wg sync.WaitGroup
	errs := make(chan error, 3)
	for _, w := range []func(context.Context, *parentStores, config, *recorder) error{
		convergeSim, memoryRebuildSim, decisionsSim,
	} {
		wg.Go(func() {
			if err := w(runCtx, ps, cfg, rec); err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	close(errs)

	stats := rec.all()
	for _, c := range children {
		childStats, err := c.wait()
		if err != nil {
			return nil, err
		}
		stats = append(stats, childStats...)
	}

	for err := range errs {
		// A workload that could not even open its stores invalidates the
		// measurement; contention errors inside the loop are data, not faults,
		// and never reach here.
		return stats, err
	}
	return stats, nil
}

// convergeSim is the converge run: J locale workers writing target overlays in
// batched sessions and content-memory entries as they go.
//
// The memory writes are issued two ways on purpose. Half go through Add, one
// transaction per entry, which is what the executor does today and what costs
// 14 ms apiece: every Add maintains the FTS5 tables row by row, and FTS5
// tokenization is the expensive part. The other half go through
// BulkAddWithStream, one transaction for the batch, which is what a converge
// worker can trivially do instead — it already has the entries in hand. The
// two lines in the report are directly comparable: same locale, same entries,
// same run, half the rows each.
func convergeSim(ctx context.Context, ps *parentStores, cfg config, rec *recorder) error {
	locales := []string{"nb", "de", "fr", "ja", "es", "it", "nl", "pt"}
	half := min(max(cfg.memoryOneAt, 0), cfg.memoryPutN)

	var wg sync.WaitGroup
	for j := range cfg.convergeJ {
		wg.Go(func() {
			locale := locales[j%len(locales)]
			kind := "targets/" + locale
			n := 0
			for ctx.Err() == nil {
				overlays := make([]blockstore.Overlay, 0, cfg.overlayN)
				for k := range cfg.overlayN {
					idx := (j*1_000_003 + n*cfg.overlayN + k) % cfg.blocks
					payload, err := json.Marshal(map[string]any{
						"locale": locale,
						"target": sourceText(idx),
						"status": "translated",
					})
					if err != nil {
						return
					}
					overlays = append(overlays, blockstore.Overlay{
						Kind: kind, BlockHash: blockHash(idx), Payload: payload,
					})
				}
				_ = rec.observe(nW1Overlay, func() error {
					return ps.overlays.writeBatch(ctx, overlays)
				})

				entry := func(k int) memory.Entry {
					idx := (j*7919 + n*cfg.memoryPutN + k) % cfg.entries
					e := makeEntry(idx)
					e.Variants[model.LocaleID(locale)] = []model.Run{
						model.TextR(fmt.Sprintf("converge %s pass %d for entry %d", locale, n, idx)),
					}
					return e
				}
				for k := range half {
					e := entry(k)
					_ = rec.observe(nW1Memory, func() error {
						return ps.write(func() error { return ps.mem.Add(ctx, e) })
					})
				}
				batch := make([]memory.Entry, 0, cfg.memoryPutN-half)
				for k := half; k < cfg.memoryPutN; k++ {
					batch = append(batch, entry(k))
				}
				if len(batch) > 0 {
					_ = rec.observe(nW1MemBatch, func() error {
						return ps.write(func() error { return ps.mem.BulkAddWithStream(ctx, batch, "") })
					})
				}
				n++
			}
		})
	}
	wg.Wait()
	return nil
}

// memoryRebuildSim is the content memory's FTS5 rebuild: DELETE followed by
// INSERT … SELECT over whole tables, twice, once mid-run. It is the largest
// single write the content memory ever makes, and under a merged store it
// holds the same lock every other subsystem needs.
//
// It is also where this harness found a bug rather than a number: both rebuilds
// used to issue their statements on context.Background(), so a write measured in
// seconds could not be stopped by the caller that asked for it. They take a ctx
// now, and it is the run's — which is why a rebuild still in flight at the end
// of the window is cancelled rather than holding the store past it.
func memoryRebuildSim(ctx context.Context, ps *parentStores, cfg config, rec *recorder) error {
	select {
	case <-ctx.Done():
		return nil
	case <-time.After(cfg.duration / 2):
	}
	_ = rec.observe(nW3Fuzzy, func() error {
		return ps.write(func() error { return ps.mem.RebuildFuzzyIndex(ctx) })
	})
	_ = rec.observe(nW3Search, func() error {
		return ps.write(func() error { return ps.mem.RebuildSearchIndex(ctx) })
	})
	return nil
}

// decisionsSim is the review loop recording decisions: a steady drip of unit
// state writes, and the once-per-run serialization of the working set. It is
// the workload most exposed to starvation, because each write is small and
// short and has nothing to amortize a five-second wait against.
func decisionsSim(ctx context.Context, ps *parentStores, cfg config, rec *recorder) error {
	puts := time.NewTicker(time.Second / time.Duration(cfg.decisionHz))
	defer puts.Stop()
	commits := time.NewTicker(cfg.commitTick)
	defer commits.Stop()

	i := 0
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-puts.C:
			u := makeUnitState(i % cfg.units)
			u.Status = model.TargetStatusReviewed
			i++
			_ = rec.observe(nW4Put, func() error { return ps.work.Put(ctx, u) })
		case <-commits.C:
			_ = rec.observe(nW4Commit, func() error { return ps.work.Commit(ctx) })
		}
	}
}

// ─── Re-extraction, out of process ──────────────────────────────

// runExtractChild is process B: re-extraction, one long transaction that
// purges the blocks of the documents being re-read and refills them, all or
// nothing. That is the case sqlitestore.New (as opposed to NewAutocommit)
// exists to serve, and the one that holds the file's write lock for as long as
// the refill takes.
//
// It issues the purge and the refill as the same SQL sqlitestore does, in one
// transaction, because the purge here is SCOPED to the collections being
// re-extracted. blockstore.BlockPurger.DeleteBlocks drops every block in the
// store — correct for a whole-project re-extraction, but it would leave the
// other workloads reading an empty corpus for the rest of the run, measuring
// contention against a store that no longer resembles the one the decision is
// about.
func runExtractChild(ctx context.Context, root string, p storePaths, cfg config) error {
	rec := newRecorder()
	rec.stream(nW2Extract)

	var db *storage.DB
	var write func(func() error) error
	switch {
	case cfg.mode.real():
		// The shipped store's own handle, and no wrapper: BeginTx below takes
		// the permit itself and holds it for the whole transaction. The SQL is
		// the same as every other mode's, so the row stays comparable — the
		// blockstore.Session interface can only purge the WHOLE cache, which
		// would leave the other workloads reading an empty corpus.
		pdb, err := openRealProject(ctx, root, p)
		if err != nil {
			return fmt.Errorf("extract child: %w", err)
		}
		defer pdb.Close()
		db, write = pdb.Raw(), func(fn func() error) error { return fn() }
	case cfg.mode.shared():
		pdb, err := openProjectDB(p.blocks, cfg.mode)
		if err != nil {
			return fmt.Errorf("extract child: open project store: %w", err)
		}
		defer pdb.Close()
		db, write = pdb.db, pdb.write
	default:
		plain, err := storage.Open(p.blocks)
		if err != nil {
			return fmt.Errorf("extract child: open block store: %w", err)
		}
		defer plain.Close()
		db, write = plain, func(fn func() error) error { return fn() }
	}

	runCtx, cancel := context.WithTimeout(ctx, cfg.duration)
	defer cancel()
	ticker := time.NewTicker(cfg.extractTick)
	defer ticker.Stop()
	pass := 0
	for {
		select {
		case <-runCtx.Done():
			return emitReport(rec.all())
		case <-ticker.C:
		}
		start := pass * cfg.extractN
		pass++
		_ = rec.observe(nW2Extract, func() error {
			return write(func() error {
				tx, err := db.BeginTx(runCtx, nil)
				if err != nil {
					return err
				}
				defer func() { _ = tx.Rollback() }()

				// The collections holding the blocks about to be re-read.
				seen := map[string]bool{}
				for i := start; i < start+cfg.extractN; i++ {
					seen[collectionOf(i%cfg.blocks)] = true
				}
				for c := range seen {
					if _, err := tx.ExecContext(runCtx, `DELETE FROM blocks WHERE collection = ?`, c); err != nil {
						return err
					}
				}
				for i := start; i < start+cfg.extractN; i++ {
					idx := i % cfg.blocks
					b := makeBlock(idx)
					payload, err := json.Marshal(b)
					if err != nil {
						return err
					}
					if _, err := tx.ExecContext(runCtx, `
						INSERT INTO blocks (hash, collection, translatable, payload)
						VALUES (?, ?, ?, ?)
						ON CONFLICT(hash) DO UPDATE SET
							collection=excluded.collection,
							translatable=excluded.translatable,
							payload=excluded.payload
					`, b.Hash, collectionOf(idx), 1, payload); err != nil {
						return err
					}
				}
				return tx.Commit()
			})
		})
	}
}

// ─── The status poller, out of process ──────────────────────────

// The two out-of-process roles this binary can re-exec itself into.
const (
	roleStatus  = "status"
	roleExtract = "extract"
)

// childHandle is the parent's side of one out-of-process workload.
type childHandle struct {
	role string
	cmd  *exec.Cmd
	out  *strings.Builder
}

// startChild re-execs this binary in the given role. Goroutines sharing the
// parent's pools would exercise SQLite's IN-process locking, which is not what
// a `kapi status` beside a converge run does, and — more importantly for the
// gated topology — an in-process write gate can only order writers that hold
// the same handle. Running the status poller and the long extraction
// transaction as real processes keeps two writers the gate cannot reach, so
// the file-level lock is still genuinely contended.
func startChild(ctx context.Context, role string, cfg config) (*childHandle, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("locate self for %s child: %w", role, err)
	}
	cmd := exec.CommandContext(ctx, exe,
		"-child-role="+role,
		"-mode="+string(cfg.mode),
		"-dir="+cfg.dir,
		"-duration="+cfg.duration.String(),
		fmt.Sprintf("-status-hz=%d", cfg.statusHz),
		fmt.Sprintf("-blocks=%d", cfg.blocks),
		fmt.Sprintf("-extract-blocks=%d", cfg.extractN),
		"-extract-every="+cfg.extractTick.String(),
	)
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("start %s child: %w", role, err)
	}
	return &childHandle{role: role, cmd: cmd, out: &out}, nil
}

func (c *childHandle) wait() ([]streamStats, error) {
	if err := c.cmd.Wait(); err != nil {
		return nil, fmt.Errorf("%s child: %w (output %q)", c.role, err, c.out.String())
	}
	var stats []streamStats
	if err := json.Unmarshal([]byte(c.out.String()), &stats); err != nil {
		return nil, fmt.Errorf("%s child: parse report: %w (output %q)", c.role, err, c.out.String())
	}
	return stats, nil
}

// runStatusChild is the child's whole life: open the databases a status read
// touches, poll them, and report on stdout. It never migrates and never
// writes — the only lock it takes beyond the shared read lock is whatever
// storage.Open's journal-mode pragma needs.
func runStatusChild(ctx context.Context, p storePaths, cfg config) error {
	rec := newRecorder()
	for _, n := range []string{nW5Open, nW5Poll, nW5CountBlks} {
		rec.stream(n)
	}

	var workDB, blocksDB *storage.DB
	err := rec.observe(nW5Open, func() error {
		var err error
		workDB, err = storage.Open(p.work)
		return err
	})
	if err != nil {
		return fmt.Errorf("status child: open work store: %w", err)
	}
	defer workDB.Close()

	blocksDB = workDB
	if p.blocks != p.work {
		if err := rec.observe(nW5Open, func() error {
			var err error
			blocksDB, err = storage.Open(p.blocks)
			return err
		}); err != nil {
			return fmt.Errorf("status child: open block store: %w", err)
		}
		defer blocksDB.Close()
	}

	runCtx, cancel := context.WithTimeout(ctx, cfg.duration)
	defer cancel()
	tick := time.NewTicker(time.Second / time.Duration(cfg.statusHz))
	defer tick.Stop()
	i := 0
	for {
		select {
		case <-runCtx.Done():
			return emitReport(rec.all())
		case <-tick.C:
		}
		unit := blockHash(i % cfg.blocks)
		i++
		_ = rec.observe(nW5Poll, func() error {
			var pending int
			if err := workDB.QueryRowContext(runCtx,
				`SELECT COUNT(*) FROM unit_state WHERE staged = 1`).Scan(&pending); err != nil {
				return err
			}
			var payload string
			err := workDB.QueryRowContext(runCtx,
				`SELECT payload FROM unit_state WHERE unit = ? AND variant = ?`, unit, "nb").Scan(&payload)
			if errors.Is(err, sql.ErrNoRows) {
				return nil
			}
			return err
		})
		_ = rec.observe(nW5CountBlks, func() error {
			var n int
			return blocksDB.QueryRowContext(runCtx, `SELECT COUNT(*) FROM blocks`).Scan(&n)
		})
	}
}

func emitReport(stats []streamStats) error {
	enc := json.NewEncoder(os.Stdout)
	return enc.Encode(stats)
}

// ─── Reporting ──────────────────────────────────────────────────

// result is one topology's measurement.
type result struct {
	mode     topology
	seed     time.Duration
	stats    []streamStats
	dbBytes  map[string]int64
	totalErr int64
}

func printModeTable(r result) {
	fmt.Printf("\n── %s ─────────────────────────────────────────────────────────────────\n", strings.ToUpper(string(r.mode)))
	fmt.Printf("seed %.1fs\n", r.seed.Seconds())
	fmt.Printf("%-40s %9s %8s %8s %9s %9s %9s %9s\n",
		"workload", "ops", "busy", "other", "p50 ms", "p95 ms", "p99 ms", "max ms")
	fmt.Println(strings.Repeat("─", 104))
	for _, s := range r.stats {
		fmt.Printf("%-40s %9d %8d %8d %9.2f %9.2f %9.2f %9.2f\n",
			s.Name, s.Ops, s.Busy, s.Other, s.P50, s.P95, s.P99, s.Max)
	}
	files := make([]string, 0, len(r.dbBytes))
	for f := range r.dbBytes {
		files = append(files, f)
	}
	sort.Strings(files)
	parts := make([]string, 0, len(files))
	for _, f := range files {
		parts = append(parts, fmt.Sprintf("%s %.1f MiB", f, float64(r.dbBytes[f])/(1<<20)))
	}
	fmt.Printf("files: %s\n", strings.Join(parts, ", "))
}

// printComparison puts every measured topology side by side on the numbers the
// decision turns on: how often a write was refused for the lock, and how far
// the tail moved.
func printComparison(results []result) {
	if len(results) < 2 {
		return
	}
	base := results[0]
	byName := map[string]map[topology]streamStats{}
	var order []string
	for _, r := range results {
		for _, s := range r.stats {
			if _, ok := byName[s.Name]; !ok {
				byName[s.Name] = map[topology]streamStats{}
				order = append(order, s.Name)
			}
			byName[s.Name][r.mode] = s
		}
	}

	fmt.Printf("\n══ COMPARISON (baseline: %s) ═══════════════════════════════════════════\n", base.mode)
	header := fmt.Sprintf("%-40s", "workload")
	var headerSb1534 strings.Builder
	for _, r := range results {
		headerSb1534.WriteString(fmt.Sprintf(" %11s %9s", string(r.mode)+" p95", string(r.mode)+" busy"))
	}
	header += headerSb1534.String()
	header += "     p95 x"
	fmt.Println(header)
	fmt.Println(strings.Repeat("─", len(header)+4))
	for _, name := range order {
		row := fmt.Sprintf("%-40s", name)
		var rowSb1542 strings.Builder
		for _, r := range results {
			s := byName[name][r.mode]
			rowSb1542.WriteString(fmt.Sprintf(" %11.2f %9d", s.P95, s.Busy))
		}
		row += rowSb1542.String()
		b := byName[name][base.mode]
		last := byName[name][results[len(results)-1].mode]
		ratio := "     n/a"
		if b.P95 > 0 {
			ratio = fmt.Sprintf("%8.2f", last.P95/b.P95)
		}
		fmt.Println(row + " " + ratio)
	}

	fmt.Println("\nthroughput (ops completed over the measured window)")
	fmt.Printf("%-40s", "workload")
	for _, r := range results {
		fmt.Printf(" %14s", string(r.mode))
	}
	fmt.Println()
	for _, name := range order {
		fmt.Printf("%-40s", name)
		for _, r := range results {
			fmt.Printf(" %14d", byName[name][r.mode].Ops)
		}
		fmt.Println()
	}
}

func dbSizes(p storePaths) map[string]int64 {
	out := map[string]int64{}
	for _, path := range []string{p.memory, p.terms, p.blocks, p.work} {
		for _, f := range []string{path, path + "-wal"} {
			if fi, err := os.Stat(f); err == nil {
				out[filepath.Base(f)] = fi.Size()
			}
		}
	}
	return out
}

// ─── Entry point ────────────────────────────────────────────────

func main() {
	var (
		cfg       config
		modeFlag  string
		childRole string
	)
	flag.StringVar(&modeFlag, "mode", "split,merged,merged-immediate,merged-gated",
		"topologies to measure, comma separated: split, merged, hybrid, work-apart, merged-immediate, merged-gated")
	flag.StringVar(&cfg.dir, "dir", "",
		"directory holding the synthetic projects (default: .contention-harness beside this source tree)")
	flag.DurationVar(&cfg.duration, "duration", 60*time.Second, "measured window per topology")
	flag.IntVar(&cfg.blocks, "blocks", 75_000, "blocks in the synthetic corpus")
	flag.IntVar(&cfg.entries, "entries", 20_000, "content-memory entries seeded")
	flag.IntVar(&cfg.concepts, "concepts", 2_000, "terms concepts seeded")
	flag.IntVar(&cfg.units, "units", 75_000, "unit-state rows seeded")
	flag.IntVar(&cfg.convergeJ, "workers", 4, "concurrent converge locale workers (J)")
	flag.IntVar(&cfg.overlayN, "overlay-batch", 25, "overlays written per converge session")
	flag.IntVar(&cfg.memoryPutN, "memory-batch", 10, "memory entries per converge iteration")
	flag.IntVar(&cfg.memoryOneAt, "memory-one-at-a-time", -1,
		"of -memory-batch, how many entries go one transaction per entry (the rest go in one batched "+
			"transaction); -1 means half and half, 0 means batch everything")
	flag.IntVar(&cfg.extractN, "extract-blocks", 5_000, "blocks purged and refilled per extraction")
	flag.DurationVar(&cfg.extractTick, "extract-every", 10*time.Second, "interval between extractions")
	flag.IntVar(&cfg.decisionHz, "decision-hz", 50, "unit-state writes per second")
	flag.DurationVar(&cfg.commitTick, "commit-every", 5*time.Second, "interval between working-set commits")
	flag.IntVar(&cfg.statusHz, "status-hz", 5, "status polls per second (child process)")
	flag.BoolVar(&cfg.keepData, "keep", false, "keep the synthetic projects instead of deleting them")
	flag.StringVar(&childRole, "child-role", "",
		"internal: run as an out-of-process workload (status or extract)")
	flag.Parse()

	if cfg.memoryOneAt < 0 {
		cfg.memoryOneAt = cfg.memoryPutN / 2
	}

	if cfg.dir == "" {
		wd, err := os.Getwd()
		if err != nil {
			fail(err)
		}
		cfg.dir = filepath.Join(wd, ".contention-harness")
	}

	ctx := context.Background()

	if childRole != "" {
		cfg.mode = topology(modeFlag)
		root := filepath.Join(cfg.dir, string(cfg.mode))
		p := newStorePaths(root, cfg.mode)
		var err error
		switch childRole {
		case roleStatus:
			err = runStatusChild(ctx, p, cfg)
		case roleExtract:
			err = runExtractChild(ctx, root, p, cfg)
		default:
			err = fmt.Errorf("unknown child role %q", childRole)
		}
		if err != nil {
			fail(err)
		}
		return
	}

	modes, err := parseModes(modeFlag)
	if err != nil {
		fail(err)
	}
	if !cfg.keepData {
		defer func() {
			if err := os.RemoveAll(cfg.dir); err != nil {
				fmt.Fprintf(os.Stderr, "cleanup %s: %v\n", cfg.dir, err)
			}
		}()
	}

	fmt.Printf("contention harness: %d blocks, %d memory entries, %d concepts, %d units\n",
		cfg.blocks, cfg.entries, cfg.concepts, cfg.units)
	fmt.Printf("window %s per topology, J=%d converge workers, extraction every %s of %d blocks\n",
		cfg.duration, cfg.convergeJ, cfg.extractTick, cfg.extractN)
	fmt.Printf("data under %s\n", cfg.dir)

	var results []result
	for _, m := range modes {
		r, err := measure(ctx, m, cfg)
		if err != nil {
			fail(fmt.Errorf("%s: %w", m, err))
		}
		printModeTable(r)
		results = append(results, r)
	}
	printComparison(results)
}

func measure(ctx context.Context, m topology, cfg config) (result, error) {
	cfg.mode = m
	root := filepath.Join(cfg.dir, string(m))
	if err := os.RemoveAll(root); err != nil {
		return result{}, fmt.Errorf("clear %s: %w", root, err)
	}
	p := newStorePaths(root, m)

	fmt.Printf("\nseeding %s …\n", m)
	seedStart := time.Now()
	if err := seed(ctx, root, p, cfg); err != nil {
		return result{}, err
	}
	seedTook := time.Since(seedStart)
	fmt.Printf("seeded in %.1fs; measuring for %s …\n", seedTook.Seconds(), cfg.duration)

	stats, err := runWorkloads(ctx, root, p, cfg)
	if err != nil {
		return result{}, err
	}
	r := result{mode: m, seed: seedTook, stats: stats, dbBytes: dbSizes(p)}
	for _, s := range stats {
		r.totalErr += s.Busy + s.Other
	}
	return r, nil
}

func parseModes(s string) ([]topology, error) {
	var out []topology
	for part := range strings.SplitSeq(s, ",") {
		switch m := topology(strings.TrimSpace(part)); m {
		case topoSplit, topoMerged, topoHybrid, topoWorkApart,
			topoMergedImmediate, topoMergedGated, topoProjectDB:
			out = append(out, m)
		case "":
			continue
		default:
			return nil, fmt.Errorf("unknown mode %q (want split, merged, hybrid, work-apart, "+
				"merged-immediate, merged-gated or projectdb)", part)
		}
	}
	if len(out) == 0 {
		return nil, errors.New("no modes selected")
	}
	return out, nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "contention-harness:", err)
	os.Exit(1)
}
