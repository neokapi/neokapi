// Package projector is the one writer of a project's context stores.
//
// Every change to a project's context is an operation in the workspace's log
// (core/workspace), and the log carries what the change wrote. The terms
// store, the content memory, the voice profiles and the rules widened to the
// whole workspace are projections of that log: what retrieval and checks read,
// written by the projector alone, and rebuildable from the log at any time.
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
// answered by the projection and writes recorded and applied here, and
// [Projector.Rules] is the rule store core/contextop widens into. A
// [Projector.Batch] collects the writes of one pass (an import, a convergence
// run's absorbed record) into one operation per subsystem.
//
// # Without a log
//
// A store opened in the embedded layout, with no workspace behind it, has no
// log to record into. The projector then applies each write directly, which
// keeps a test and the browser build working and gives them nothing to
// rebuild from.
package projector

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/projectdb"
	"github.com/neokapi/neokapi/core/workspace"
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
)

// Kinds is every kind the projector applies.
var Kinds = []string{KindTerms, KindMemory, KindVoice, KindRules}

// projects reports whether an operation kind is one the projector applies.
func projects(kind string) bool {
	switch kind {
	case KindTerms, KindMemory, KindVoice, KindRules:
		return true
	}
	return false
}

// BlobThreshold is the payload size above which an operation's steps go into
// a blob rather than into the operation itself.
const BlobThreshold = 32 << 10

// cursorKey is the context-store metadata key holding the local position of
// the last operation the projection has applied.
const cursorKey = "projector.applied"

// Log is the workspace the projector records into and replays from.
// *workspace.Workspace satisfies it.
type Log interface {
	Record(ctx context.Context, ops ...workspace.Op) ([]workspace.Op, error)
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
	db     *projectdb.DB
	origin Origin
	lock   *sync.Mutex
}

// locks holds one mutex per context store, keyed by the store's pool, so every
// projector over one store in this process takes turns.
var locks sync.Map

// New binds a projector to a project's stores and the log they project. A nil
// log is the embedded layout: writes apply directly and nothing is recorded.
func New(log Log, key workspace.ProjectKey, db *projectdb.DB) (*Projector, error) {
	if db == nil {
		return nil, errors.New("projector: no project store")
	}
	if log != nil && key == "" {
		return nil, workspace.ErrNoProjectKey
	}
	var lockKey any = db
	if raw := db.Raw(); raw != nil {
		lockKey = raw
	}
	mu, _ := locks.LoadOrStore(lockKey, &sync.Mutex{})
	return &Projector{log: log, key: key, db: db, lock: mu.(*sync.Mutex)}, nil
}

// With returns a projector that stamps the writes it makes with an origin.
func (p *Projector) With(origin Origin) *Projector {
	out := *p
	out.origin = origin
	return &out
}

// Key is the project whose stores this projector writes.
func (p *Projector) Key() workspace.ProjectKey { return p.key }

// Store is the project store this projector writes.
func (p *Projector) Store() *projectdb.DB { return p.db }

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

// pending is one operation a write is about to record: its kind and its steps.
type pending struct {
	kind  string
	steps []step
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
			if err := p.applySteps(ctx, w.kind, w.steps); err != nil {
				return err
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
	return workspace.Op{Project: p.key, Kind: w.kind, Payload: body, At: at}, nil
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
// of operations this call recorded, which are applied from memory rather than
// read back.
func (p *Projector) catchUpLocked(ctx context.Context, mine map[string]pending) error {
	cursor, err := p.cursor(ctx)
	if err != nil {
		return err
	}
	ops, err := p.log.Select(ctx, workspace.OpQuery{After: cursor, Project: p.key})
	if err != nil {
		return err
	}
	var first error
	last := cursor
	foreignBulk := false
	for _, op := range ops {
		last = op.Seq
		if !projects(op.Kind) {
			continue
		}
		own, isMine := mine[op.ID]
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
		if aerr := p.applySteps(ctx, op.Kind, steps); aerr != nil && isMine && first == nil {
			first = aerr
		}
	}
	// A writer that asked for a bulk write rebuilds the indexes itself; one
	// that another process made is this catch-up's to finish.
	if foreignBulk {
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

// cursor reads the local position of the last operation the store applied.
func (p *Projector) cursor(ctx context.Context) (int64, error) {
	value, ok, err := p.db.ContextMeta(ctx, cursorKey)
	if err != nil || !ok {
		if errors.Is(err, projectdb.ErrNoStore) {
			err = nil
		}
		return 0, err
	}
	n, perr := strconv.ParseInt(value, 10, 64)
	if perr != nil {
		return 0, nil
	}
	return n, nil
}

// setCursor records the local position of the last operation the store applied.
func (p *Projector) setCursor(ctx context.Context, seq int64) error {
	err := p.db.PutContextMeta(ctx, cursorKey, strconv.FormatInt(seq, 10))
	if errors.Is(err, projectdb.ErrNoStore) {
		return nil
	}
	return err
}
