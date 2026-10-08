// Package projector is the one writer of a project's context stores.
//
// Every change to a project's context is an operation in the workspace's log
// (core/workspace), and the log carries what the change wrote. The terms
// store, the content memory, the voice profiles, the rules widened to the
// whole workspace, the decision ledger, the document adoptions and the block
// history are projections of that log: what retrieval, checks and review
// read, written by the projector alone, and rebuildable from the log at any
// time.
//
// A write goes through here in two steps under one lock. The projector records
// an operation carrying the rows the write puts or removes, and then applies
// every operation the store has not yet seen, in the order the log received
// them. A write made by another process, or merged in from another machine's
// log, is therefore applied by the next write here, and a store never holds a
// row the log does not explain.
//
// # What an operation carries
//
// One operation per subsystem a write touches, of kind [KindTerms],
// [KindMemory], [KindVoice] or [KindRules]. Its payload is the list of steps
// the write made, in order: concepts put and deleted, content-memory entries
// put and deleted, profiles created and updated, rules widened and narrowed.
// A decision is one [KindDecision] operation and a document adoption one
// [KindAdopt], each carrying its entry as a step and addressed by its content.
// An applied edit is one [KindEdit] operation per document, whose payload is
// the [Edit] itself rather than steps.
// A payload larger than [BlobThreshold] goes into a content-addressed blob the
// operation names, which is where an imported bundle or a convergence run's
// batch of approved wording ends up.
//
// Each step carries the rows as they are written, with every timestamp the
// store would otherwise take from the clock filled in from the operation's
// own. Replaying the log therefore writes the same rows the first
// application wrote, which is what [Projector.Rebuild] relies on.
//
// # Writers
//
// Callers do not build operations. [Projector.Terms], [Projector.Memory] and
// [Projector.Voice] hand out the store a caller already knows, with reads
// answered by the projection and writes recorded and applied here,
// [Projector.Rules] is the rule store core/contextop widens into,
// [Projector.Decisions] is the journal the decision ledger records through,
// and [Projector.RecordEdit] records an applied edit. A
// [Projector.Batch] collects the writes of one pass (an import, a convergence
// run's absorbed record) into one operation per subsystem.
//
// # Without a log
//
// A store opened in the embedded layout, with no workspace behind it, or under
// a workspace opened read-only, has no log to record into. The projector then
// applies each write directly, which keeps a test working and gives it nothing
// to rebuild from.
package projector

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/core/workhome"
	"github.com/neokapi/neokapi/core/workspace"
	"github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/terms"
	"github.com/neokapi/neokapi/voice"
)

// Operation kinds the projector records and applies.
const (
	// KindTerms puts and deletes concepts and relations in the terms store.
	KindTerms = "terms.write"
	// KindMemory puts and deletes content-memory entries and import sessions.
	KindMemory = "memory.write"
	// KindVoice creates, updates and deletes voice profiles.
	KindVoice = "voice.write"
	// KindRules widens rules to the whole workspace and narrows them again.
	KindRules = "rules.write"
	// KindDecision records one entry in the decision ledger (core/state). Its
	// content address is the entry's, so a decision recorded twice, here or on
	// another machine, is one operation.
	KindDecision = "decision.record"
	// KindAdopt records the key a document has, where it was read and what it
	// held there (state.Adoption), so every checkout of the project resolves
	// one key per document.
	KindAdopt = "document.adopt"
	// KindEdit records one applied change to one document's content: who made
	// it, through which surface, and each edition it changed (Edit). The
	// projector writes it into the block history (core/history).
	KindEdit = "content.edit"
)

// Kinds is every kind the projector applies.
var Kinds = []string{KindTerms, KindMemory, KindVoice, KindRules, KindDecision, KindAdopt, KindEdit}

// projects reports whether an operation kind is one the projector applies.
func projects(kind string) bool {
	switch kind {
	case KindTerms, KindMemory, KindVoice, KindRules, KindDecision, KindAdopt, KindEdit:
		return true
	}
	return false
}

// BlobThreshold is the payload size above which an operation's steps go into
// a blob rather than into the operation itself.
const BlobThreshold = 32 << 10

// Log is the workspace the projector records into and replays from.
// *workspace.Workspace satisfies it.
type Log interface {
	Record(ctx context.Context, ops ...workspace.Op) ([]workspace.Op, error)
	RecordIf(ctx context.Context, expect []workspace.Expect, ops ...workspace.Op) ([]workspace.Op, error)
	SubjectHead(ctx context.Context, project workspace.ProjectKey, subject string) (int64, error)
	Select(ctx context.Context, q workspace.OpQuery) ([]workspace.Op, error)
	PutBlob(ctx context.Context, data []byte) (string, error)
	Blob(ctx context.Context, digest string) ([]byte, error)

	// The rules widened to the whole workspace are a projection too, kept in
	// the workspace database rather than a project's.
	WidenRule(ctx context.Context, rule workspace.Rule) error
	NarrowRule(ctx context.Context, id string) error
	WidenedRules(ctx context.Context, kind string) ([]workspace.Rule, error)
}

// Origin says where a write came from, for a reader of the log.
type Origin struct {
	// By names what wrote it: "apply", "import", "keep", "merge", "absorb".
	By string `json:"by,omitempty"`
	// Cause is the context operation the write carries out, when there is
	// one: the keep that established a rule, the import that read a file.
	Cause string `json:"cause,omitempty"`
	// Source names what was read, for a write that read something: a file's
	// project-relative path.
	Source string `json:"source,omitempty"`
}

// Projector writes one project's context stores.
//
// It is safe for concurrent use. Two projectors over the same context store
// share one lock, so a process never applies two writes to one store at once.
type Projector struct {
	log    Log
	key    workspace.ProjectKey
	st     Stores
	origin Origin
	lock   *sync.Mutex
}

// Stores are the projections one projector writes, bound to one context
// database. A subsystem this build has no store for is nil, and writes to it
// report projectdb.ErrNoStore.
type Stores struct {
	// Raw is the context database the stores live in. The projector keeps its
	// position in the log there, and a rebuild empties the projection tables
	// in it.
	Raw     *storage.DB
	Terms   *terms.SQLiteStore
	Memory  *memory.SQLiteStore
	Voice   *voice.SQLiteStore
	History *history.Store
	// Heads is the workspace home's projection: the editions it keeps and
	// the head of each (core/workhome).
	Heads *workhome.Store
}

// ProjectStores are the projections a project store holds.
func ProjectStores(db *projectdb.DB) Stores {
	return Stores{Raw: db.Raw(), Terms: db.Terms(), Memory: db.Memory(), Voice: db.Voice(), History: db.History(), Heads: db.Heads()}
}

// ContextStores binds the projections to a project's context database straight
// out of the workspace, for a caller with no checkout of the project to open a
// project store in: a whole-workspace restore, a desktop view of a project
// that is not open.
func ContextStores(raw *storage.DB) (Stores, error) {
	tb, err := terms.NewSQLiteStoreFromDB(raw)
	if err != nil {
		return Stores{}, fmt.Errorf("projector: bind the terms store: %w", err)
	}
	tm, err := memory.NewSQLiteStoreFromDB(raw)
	if err != nil {
		return Stores{}, fmt.Errorf("projector: bind the content memory: %w", err)
	}
	vc, err := voice.NewSQLiteStore(raw)
	if err != nil {
		return Stores{}, fmt.Errorf("projector: bind the voice store: %w", err)
	}
	hs, err := history.Open(raw)
	if err != nil {
		return Stores{}, fmt.Errorf("projector: bind the block history: %w", err)
	}
	heads, err := workhome.Open(raw)
	if err != nil {
		return Stores{}, fmt.Errorf("projector: bind the workspace home: %w", err)
	}
	return Stores{Raw: raw, Terms: tb, Memory: tm, Voice: vc, History: hs, Heads: heads}, nil
}

// locks holds one mutex per context store, keyed by the store's pool, so every
// projector over one store in this process takes turns.
var locks sync.Map

// cursorMigrations is the projector's own table in the context database: the
// local position of the last operation applied there.
var cursorMigrations = []storage.Migration{{
	Version:     1,
	Description: "the position of the last operation applied",
	SQL: `
CREATE TABLE IF NOT EXISTS projector_cursor (
    id  INTEGER PRIMARY KEY CHECK (id = 1),
    seq INTEGER NOT NULL
);`,
}}

// New binds a projector to a project's stores and the log they project. A nil
// log is the embedded layout: writes apply directly and nothing is recorded.
func New(log Log, key workspace.ProjectKey, st Stores) (*Projector, error) {
	if log != nil && key == "" {
		return nil, workspace.ErrNoProjectKey
	}
	if log != nil && st.Raw == nil {
		return nil, errors.New("projector: a logged projector needs the context database")
	}
	if log != nil {
		if err := storage.Migrate(st.Raw, "projector_migrations", cursorMigrations); err != nil {
			return nil, fmt.Errorf("projector: migrate: %w", err)
		}
	}
	mu := &sync.Mutex{}
	if st.Raw != nil {
		held, _ := locks.LoadOrStore(st.Raw, mu)
		mu = held.(*sync.Mutex)
	}
	return &Projector{log: log, key: key, st: st, lock: mu}, nil
}

// ForProject binds a projector to a project store.
func ForProject(log Log, key workspace.ProjectKey, db *projectdb.DB) (*Projector, error) {
	if db == nil {
		return nil, errors.New("projector: no project store")
	}
	return New(log, key, ProjectStores(db))
}

// With returns a projector that stamps the writes it makes with an origin.
func (p *Projector) With(origin Origin) *Projector {
	out := *p
	out.origin = origin
	return &out
}

// Key is the project whose stores this projector writes.
func (p *Projector) Key() workspace.ProjectKey { return p.key }

// Logged reports whether writes are recorded in a log, which is what makes the
// stores rebuildable.
func (p *Projector) Logged() bool { return p.log != nil }

// payload is what an operation of one of the projector's kinds carries.
type payload struct {
	Origin Origin `json:"origin,omitzero"`
	Steps  []step `json:"steps,omitempty"`
	// Blob names the blob holding the steps, for a payload too large to carry
	// them itself.
	Blob string `json:"blob,omitempty"`
}

// pending is one operation a write is about to record: its kind, its steps
// (or, for a content.edit, its edit), and the content address it is recorded
// under, if any.
type pending struct {
	kind    string
	steps   []step
	edit    *Edit
	address string
}

// commit records a write's operations and applies everything the store has not
// yet seen, its own operations included. It returns the first error applying
// the write's own steps met; an error applying another writer's operation
// stops nothing, because that write already answered its own caller.
func (p *Projector) commit(ctx context.Context, writes []pending) error {
	var todo []pending
	for _, w := range writes {
		if len(w.steps) > 0 {
			todo = append(todo, w)
		}
	}
	if len(todo) == 0 {
		return nil
	}
	p.lock.Lock()
	defer p.lock.Unlock()

	now := time.Now().UTC()
	for i := range todo {
		for j := range todo[i].steps {
			todo[i].steps[j].stamp(now)
		}
	}
	if p.log == nil {
		for _, w := range todo {
			replayed, err := p.applySteps(ctx, w.kind, w.steps)
			if err != nil {
				return err
			}
			if replayed {
				if err := p.rebuildIndexes(ctx); err != nil {
					return err
				}
			}
		}
		return nil
	}

	var (
		ops   []workspace.Op
		parts []pending
	)
	for _, w := range todo {
		for _, part := range split(w) {
			op, err := p.encode(ctx, part, now)
			if err != nil {
				return err
			}
			ops = append(ops, op)
			parts = append(parts, part)
		}
	}
	written, err := p.log.Record(ctx, ops...)
	if err != nil {
		return err
	}
	mine := make(map[string]pending, len(written))
	for i, op := range written {
		mine[op.ID] = parts[i]
	}
	return p.catchUpLocked(ctx, mine)
}

// maxEntriesPerStep bounds the content-memory entries one step carries, and
// maxStepsPerOp the steps one operation carries, so every blob stays well
// inside workspace.MaxBlobSize: a convergence run's batch of approved wording
// becomes several operations rather than one blob too large to store.
const (
	maxEntriesPerStep = 2000
	maxStepsPerOp     = 64
)

// split cuts a pending write into operations small enough to record: a step
// putting many entries or concepts becomes several steps, and a write of many
// steps becomes several operations. The pieces keep their order, so applying
// them in sequence writes what the whole would have.
func split(w pending) []pending {
	if w.address != "" {
		// An addressed operation is one statement, recorded whole.
		return []pending{w}
	}
	var steps []step
	for _, s := range w.steps {
		for len(s.PutEntries) > maxEntriesPerStep {
			head := step{Stream: s.Stream, Bulk: s.Bulk, PutEntries: s.PutEntries[:maxEntriesPerStep]}
			steps = append(steps, head)
			s.PutEntries = s.PutEntries[maxEntriesPerStep:]
		}
		for len(s.PutConcepts) > maxEntriesPerStep {
			head := step{Stream: s.Stream, PutConcepts: s.PutConcepts[:maxEntriesPerStep]}
			steps = append(steps, head)
			s.PutConcepts = s.PutConcepts[maxEntriesPerStep:]
		}
		steps = append(steps, s)
	}
	var out []pending
	for len(steps) > maxStepsPerOp {
		out = append(out, pending{kind: w.kind, steps: steps[:maxStepsPerOp]})
		steps = steps[maxStepsPerOp:]
	}
	return append(out, pending{kind: w.kind, steps: steps})
}

// encode renders a pending write as the operation that records it, moving the
// steps into a blob when they are too large to carry.
func (p *Projector) encode(ctx context.Context, w pending, at time.Time) (workspace.Op, error) {
	body, err := json.Marshal(payload{Origin: p.origin, Steps: w.steps})
	if err != nil {
		return workspace.Op{}, fmt.Errorf("projector: encode %s: %w", w.kind, err)
	}
	if len(body) > BlobThreshold {
		steps, err := json.Marshal(w.steps)
		if err != nil {
			return workspace.Op{}, fmt.Errorf("projector: encode %s: %w", w.kind, err)
		}
		address, err := p.log.PutBlob(ctx, steps)
		if err != nil {
			return workspace.Op{}, err
		}
		if body, err = json.Marshal(payload{Origin: p.origin, Blob: address}); err != nil {
			return workspace.Op{}, fmt.Errorf("projector: encode %s: %w", w.kind, err)
		}
	}
	return workspace.Op{Project: p.key, Kind: w.kind, Payload: body, At: at, Address: w.address}, nil
}

// decode reads the steps an operation carries, from its payload or its blob.
func (p *Projector) decode(ctx context.Context, op workspace.Op) ([]step, Origin, error) {
	var body payload
	if len(op.Payload) > 0 {
		if err := json.Unmarshal(op.Payload, &body); err != nil {
			return nil, Origin{}, fmt.Errorf("projector: read %s %s: %w", op.Kind, workspace.ShortOpID(op.ID), err)
		}
	}
	if body.Blob == "" {
		return body.Steps, body.Origin, nil
	}
	data, err := p.log.Blob(ctx, body.Blob)
	if err != nil {
		return nil, body.Origin, fmt.Errorf("projector: read %s %s: %w", op.Kind, workspace.ShortOpID(op.ID), err)
	}
	var steps []step
	if err := json.Unmarshal(data, &steps); err != nil {
		return nil, body.Origin, fmt.Errorf("projector: read %s %s: %w", op.Kind, workspace.ShortOpID(op.ID), err)
	}
	return steps, body.Origin, nil
}

// CatchUp applies every operation the store has not yet seen: writes another
// process recorded, and operations merged in from another log. A projector
// opened over a log that moved while it was closed calls it once.
func (p *Projector) CatchUp(ctx context.Context) error {
	if p.log == nil {
		return nil
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	return p.catchUpLocked(ctx, nil)
}

// catchUpLocked applies the operations after the store's cursor, in the order
// the log received them, and moves the cursor past them. mine holds the steps
// (or the edit) of operations this call recorded, which are applied from
// memory rather than read back.
func (p *Projector) catchUpLocked(ctx context.Context, mine map[string]pending) error {
	cursor, err := p.cursor(ctx)
	if err != nil {
		return err
	}
	// A store that has applied nothing starts after the project's latest
	// removal from the workspace, if there was one.
	start := cursor
	if cursor == 0 {
		if start, err = p.forgottenAt(ctx); err != nil {
			return err
		}
	}
	ops, err := p.log.Select(ctx, workspace.OpQuery{After: start, Project: p.key})
	if err != nil {
		return err
	}
	// A reset changes what the operations before it apply, and an operation
	// merged in from another machine may fall inside what a reset set aside.
	// Either is answered by rebuilding the stores from the log.
	if rebuild, rerr := p.resetReached(ctx, ops); rerr != nil {
		return rerr
	} else if rebuild {
		_, rerr := p.rebuildLocked(ctx)
		return rerr
	}
	var first error
	last := start
	foreignBulk, stale := false, false
	// Ledger entries arrive one operation each, and an import brings thousands,
	// so a run of them is applied in one transaction. A run of edits is written
	// into the block history the same way.
	var (
		units     []step
		unitsMine bool
		edits     []history.Row
		editsMine bool
		works     []workhome.Write
		docs      []workhome.DocWrite
	)
	flush := func() {
		if len(units) > 0 {
			if _, aerr := p.applySteps(ctx, KindDecision, units); aerr != nil && unitsMine && first == nil {
				first = aerr
			}
			units, unitsMine = nil, false
		}
		if len(edits) > 0 || len(docs) > 0 {
			aerr := p.applyEditRows(ctx, edits)
			if aerr == nil {
				aerr = p.applyWorkWrites(ctx, works)
			}
			if aerr == nil {
				aerr = p.applyDocWrites(ctx, docs)
			}
			if aerr != nil && editsMine && first == nil {
				first = aerr
			}
			edits, editsMine, works, docs = nil, false, nil, nil
		}
	}
	for _, op := range ops {
		last = op.Seq
		if !projects(op.Kind) {
			continue
		}
		own, isMine := mine[op.ID]
		if op.Kind == KindEdit {
			var e Edit
			if isMine && own.edit != nil {
				e = *own.edit
			} else {
				var derr error
				if e, derr = p.decodeEdit(ctx, op); derr != nil {
					continue
				}
			}
			if len(units) > 0 {
				flush()
			}
			if e.kept() {
				w, werr := p.workWrite(ctx, op, e)
				if werr != nil {
					if isMine && first == nil {
						first = werr
					}
					continue
				}
				works = append(works, w)
			}
			if e.whole() {
				docs = append(docs, docWrite(op, e))
			}
			edits = append(edits, editRows(op.ID, opAddress(op), op.At, e)...)
			editsMine = editsMine || isMine
			continue
		}
		steps := own.steps
		if !isMine {
			var derr error
			if steps, _, derr = p.decode(ctx, op); derr != nil {
				// An operation that cannot be read is skipped rather than
				// holding every later one back; a rebuild reports it.
				continue
			}
			foreignBulk = foreignBulk || bulkMemory(op.Kind, steps)
		}
		if op.Kind == KindDecision {
			if len(edits) > 0 || len(docs) > 0 {
				flush()
			}
			units = append(units, steps...)
			unitsMine = unitsMine || isMine
			continue
		}
		flush()
		replayed, aerr := p.applySteps(ctx, op.Kind, steps)
		if aerr != nil && isMine && first == nil {
			first = aerr
		}
		stale = stale || replayed
	}
	flush()
	// A writer that asked for a bulk write rebuilds the indexes itself; one
	// that another process made is this catch-up's to finish.
	if foreignBulk || stale {
		if err := p.rebuildIndexes(ctx); err != nil && first == nil {
			first = err
		}
	}
	if last != cursor {
		if err := p.setCursor(ctx, last); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// resetReached reports whether the operations a catch-up is about to apply
// need a rebuild instead: one of them is a reset, or one of them sorts before
// a reset the log already holds and may therefore be one it sets aside.
func (p *Projector) resetReached(ctx context.Context, ops []workspace.Op) (bool, error) {
	if len(ops) == 0 {
		return false, nil
	}
	for _, op := range ops {
		if op.Kind == workspace.OpContextReset {
			return true, nil
		}
	}
	resets, err := p.log.Select(ctx, workspace.OpQuery{Project: p.key, KindPrefix: workspace.OpContextReset})
	if err != nil || len(resets) == 0 {
		return false, err
	}
	var latest string
	for _, r := range resets {
		latest = max(latest, r.ID)
	}
	for _, op := range ops {
		if workspace.ResetKind(op.Kind) && op.ID < latest {
			return true, nil
		}
	}
	return false, nil
}

// forgottenAt returns the local position of the project's latest removal from
// the workspace (workspace.Forget), or 0 when it was never removed. The
// operations before it built the context the removal deleted, so a store that
// starts from nothing starts after it, and the project registered again begins
// with an empty context.
func (p *Projector) forgottenAt(ctx context.Context) (int64, error) {
	ops, err := p.log.Select(ctx, workspace.OpQuery{Project: p.key, KindPrefix: workspace.OpForgetProject})
	if err != nil {
		return 0, err
	}
	return lastForget(ops), nil
}

// lastForget returns the position of the latest removal among ops, 0 when
// there is none.
func lastForget(ops []workspace.Op) int64 {
	var at int64
	for _, op := range ops {
		if op.Kind == workspace.OpForgetProject {
			at = max(at, op.Seq)
		}
	}
	return at
}

// cursor reads the local position of the last operation the store applied.
func (p *Projector) cursor(ctx context.Context) (int64, error) {
	var seq int64
	err := p.st.Raw.QueryRowContext(ctx, `SELECT seq FROM projector_cursor WHERE id = 1`).Scan(&seq)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("projector: read the applied position: %w", err)
	}
	return seq, nil
}

// setCursor records the local position of the last operation the store applied.
func (p *Projector) setCursor(ctx context.Context, seq int64) error {
	if _, err := p.st.Raw.ExecContext(ctx, `
INSERT INTO projector_cursor (id, seq) VALUES (1, ?)
ON CONFLICT(id) DO UPDATE SET seq = excluded.seq`, seq); err != nil {
		return fmt.Errorf("projector: record the applied position: %w", err)
	}
	return nil
}
