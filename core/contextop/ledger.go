package contextop

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/axis"
	"github.com/neokapi/neokapi/core/workspace"
)

// Log is the workspace operation log this package writes to and reads back.
// *workspace.Workspace satisfies it.
type Log interface {
	// Record appends operations and returns them with the ids the backend
	// assigned.
	Record(ctx context.Context, ops ...workspace.Op) ([]workspace.Op, error)
	// Select returns the operations a query names, in the order this log
	// received them. The ledger reads the context operations alone, so the
	// unit decisions and memory entries sharing the log cost it nothing.
	Select(ctx context.Context, q workspace.OpQuery) ([]workspace.Op, error)
}

// Ledger reads and writes a workspace's context operations under one policy.
//
// It holds no state of its own: every read folds the log from the beginning,
// which is what makes a status the log's answer rather than a cached one. A
// workspace holds one operation per context change, so the log a person
// accumulates stays small enough to read whole.
type Ledger struct {
	log    Log
	policy Policy
	// axes types each project's coordinate axes for the fold, nil when the
	// reader knows none.
	axes func(workspace.ProjectKey) axis.Space
}

// WithAxes tells the fold how each project types its coordinate axes, so two
// rules at a region and at a country inside it are read as meeting, and a
// correction seen in Norway counts for a suggestion scoped to the Nordics.
// Without it, coordinates are compared exactly. Every reader of one workspace
// should pass the same function, or two readers fold the log differently.
func (l *Ledger) WithAxes(axes func(workspace.ProjectKey) axis.Space) *Ledger {
	l.axes = axes
	return l
}

// NewLedger binds a log to a policy. A nil policy means [PersonDecides], which
// is what every surface in kapi uses.
func NewLedger(log Log, policy Policy) *Ledger {
	if policy == nil {
		policy = PersonDecides
	}
	return &Ledger{log: log, policy: policy}
}

// ErrNotFound reports an operation id, or the start of one, that the log does
// not hold. A prefix that starts more than one id is reported as a
// *workspace.AmbiguousOpIDError listing them.
var ErrNotFound = workspace.ErrNoOperation

// Append records one operation, after the policy has allowed it.
//
// The caller fills the actor, the kind, the subject, the evidence, the basis
// and the scope; Append stamps the moment from Go's clock, resolves the target
// an acting operation names, puts the transition to the policy, and returns the
// record as the log now holds it, with the id it was given.
func (l *Ledger) Append(ctx context.Context, r Record) (Record, error) {
	if !r.Kind.Valid() {
		return Record{}, fmt.Errorf("contextop: %q is not an operation kind", r.Kind)
	}
	if r.Actor.Kind == "" {
		r.Actor.Kind = ActorPerson
	}
	if r.Scope.Level == "" {
		r.Scope.Level = LevelProject
	}
	if r.At.IsZero() {
		r.At = time.Now().UTC()
	} else {
		r.At = r.At.UTC()
	}

	held, err := l.Records(ctx, Filter{})
	if err != nil {
		return Record{}, err
	}

	transition := Transition{
		Actor:    r.Actor,
		Kind:     r.Kind,
		Subject:  r.Subject.Kind,
		Widening: r.Kind == KindWiden || (r.Kind == KindKeep && (r.Scope.Level == LevelWorkspace || r.Scope.AllProfiles)),
		Editing:  r.Kind == KindKeep && r.Subject.Kind != SubjectNone,
	}
	if r.Target != "" {
		target, err := find(held, r.Target)
		if err != nil {
			return Record{}, err
		}
		r.Target = target.ID
		transition.Target, transition.Targeted = target, true
	}
	if err := l.policy(transition); err != nil {
		return Record{}, err
	}

	op, err := encode(r)
	if err != nil {
		return Record{}, err
	}
	written, err := l.log.Record(ctx, op)
	if err != nil {
		return Record{}, err
	}
	if len(written) != 1 {
		return Record{}, fmt.Errorf("contextop: the log accepted %d operations for one", len(written))
	}
	r.ID = written[0].ID
	r.Short = workspace.ShortOpID(r.ID)
	r.At = written[0].At.UTC()
	r.Status = statusAtBirth(r.Kind)
	r.Established = r.Kind.Bears() && r.Status == StatusEstablished
	return r, nil
}

// Filter narrows a reading of the log. A zero Filter asks for everything.
type Filter struct {
	// Project narrows to one project's operations.
	Project workspace.ProjectKey
	// Session narrows to the operations one agent session recorded.
	Session string
	// Status narrows to the operations that ended up at one status.
	Status Status
	// Actor narrows to one actor, matched against the actor's name and, failing
	// that, its kind.
	Actor string
	// Since drops everything recorded before an instant.
	Since time.Time
	// Kinds narrows to particular operation kinds. Empty asks for every kind.
	Kinds []Kind
	// Subjects narrows to the operations that carry a subject of their own,
	// which is what a surface listing what the project has learned wants.
	Subjects bool
	// Limit keeps the most recent N operations after every other narrowing. A
	// limit of zero or less keeps them all.
	Limit int
}

// Records reads the log, folds each subject-bearing operation's status from the
// operations that named it, and returns what the filter keeps, newest first.
func (l *Ledger) Records(ctx context.Context, f Filter) ([]Record, error) {
	all, err := l.fold(ctx)
	if err != nil {
		return nil, err
	}
	return filterNewestFirst(all, f), nil
}

// filterNewestFirst keeps the folded records a filter matches, newest first,
// and applies its limit.
func filterNewestFirst(all []Record, f Filter) []Record {
	kept := make([]Record, 0, len(all))
	for _, r := range all {
		if f.matches(r) {
			kept = append(kept, r)
		}
	}
	// Newest first, the order a person reading a history wants.
	for i, j := 0, len(kept)-1; i < j; i, j = i+1, j-1 {
		kept[i], kept[j] = kept[j], kept[i]
	}
	if f.Limit > 0 && len(kept) > f.Limit {
		kept = kept[:f.Limit]
	}
	return kept
}

// matches reports whether a folded record survives the filter.
func (f Filter) matches(r Record) bool {
	if f.Project != "" && r.Project != f.Project {
		return false
	}
	if f.Session != "" && r.Actor.Session != f.Session {
		return false
	}
	if f.Status != "" && r.Status != f.Status {
		return false
	}
	if f.Actor != "" && !strings.EqualFold(r.Actor.Name, f.Actor) && !strings.EqualFold(string(r.Actor.Kind), f.Actor) {
		return false
	}
	if !f.Since.IsZero() && r.At.Before(f.Since) {
		return false
	}
	if f.Subjects && !r.Kind.Bears() {
		return false
	}
	if len(f.Kinds) > 0 && !slices.Contains(f.Kinds, r.Kind) {
		return false
	}
	return true
}

// sinceRemoval drops the operations recorded before their project's latest
// removal from the workspace (workspace.Forget). Those built a context the
// removal deleted, and the projector replays none of them, so the log reports
// none of them either: a project registered again begins with an empty record.
// The position compared is the log's own (Op.Seq), as the projector compares
// it, so an operation merged in after the removal counts whatever its id.
func (l *Ledger) sinceRemoval(ctx context.Context, ops []workspace.Op) ([]workspace.Op, error) {
	removals, err := l.log.Select(ctx, workspace.OpQuery{KindPrefix: workspace.OpForgetProject})
	if err != nil {
		return nil, err
	}
	if len(removals) == 0 {
		return ops, nil
	}
	removedAt := map[workspace.ProjectKey]int64{}
	for _, op := range removals {
		if op.Kind == workspace.OpForgetProject && op.Project != "" {
			removedAt[op.Project] = max(removedAt[op.Project], op.Seq)
		}
	}
	kept := ops[:0]
	for _, op := range ops {
		if at, ok := removedAt[op.Project]; ok && op.Seq < at {
			continue
		}
		kept = append(kept, op)
	}
	return kept, nil
}

// Get returns one operation by id.
func (l *Ledger) Get(ctx context.Context, id string) (Record, error) {
	all, err := l.fold(ctx)
	if err != nil {
		return Record{}, err
	}
	return find(all, id)
}

// Subject resolves an id to the subject-bearing operation behind it: the
// operation itself when it carries a subject, and otherwise the one it acts on.
// Keeping an established rule by naming the keep rather than the suggestion
// therefore reaches the same rule.
func (l *Ledger) Subject(ctx context.Context, id string) (Record, error) {
	all, err := l.fold(ctx)
	if err != nil {
		return Record{}, err
	}
	r, err := find(all, id)
	if err != nil {
		return Record{}, err
	}
	resolved, ok := bearer(index(all), r)
	if !ok {
		return Record{}, fmt.Errorf("contextop: operation %s acts on nothing that carries a subject", id)
	}
	return resolved, nil
}

// fold reads the whole log and reports each operation with the status the
// operations that named it left it at.
func (l *Ledger) fold(ctx context.Context) ([]Record, error) {
	ops, err := l.read(ctx)
	if err != nil {
		return nil, err
	}
	return foldOps(ops, l.axes)
}

// read selects the context operations of the log, less those a project's
// removal from the workspace left behind, in the order the log received them.
func (l *Ledger) read(ctx context.Context) ([]workspace.Op, error) {
	ops, err := l.log.Select(ctx, workspace.OpQuery{KindPrefix: OpKindPrefix})
	if err != nil {
		return nil, err
	}
	return l.sinceRemoval(ctx, ops)
}

// RecordsAsOf folds only the operations the log accepted at or before an
// instant (Record.At), and returns what the filter keeps, newest first. It is
// how the status a subject held at a past instant is known: a keep recorded
// after it has not happened yet, and a reset after it sets nothing aside. The
// fold itself is the one Records runs, over that prefix of the log.
func (l *Ledger) RecordsAsOf(ctx context.Context, at time.Time, f Filter) ([]Record, error) {
	ops, err := l.read(ctx)
	if err != nil {
		return nil, err
	}
	all, err := foldOps(acceptedBy(ops, at), l.axes)
	if err != nil {
		return nil, err
	}
	return filterNewestFirst(all, f), nil
}

// acceptedBy keeps the operations accepted at or before an instant.
func acceptedBy(ops []workspace.Op, at time.Time) []workspace.Op {
	out := make([]workspace.Op, 0, len(ops))
	for _, op := range ops {
		if !op.At.After(at) {
			out = append(out, op)
		}
	}
	return out
}

// StatusAt is the status a subject-bearing operation held from an instant on.
type StatusAt struct {
	At     time.Time `json:"at"`
	Status Status    `json:"status"`
}

// HistoryLimit is how many instants Histories folds the log at before it
// reports each subject's first and current status alone.
const HistoryLimit = 500

// Histories reports, for each subject-bearing operation the filter keeps, the
// status it held after each operation that moved it, oldest first: the status
// it was born with at its own instant, then every change at the instant of the
// operation that made it. The log is read once and folded as of each distinct
// instant an operation of the filtered project was accepted at (RecordsAsOf),
// so drops, keeps, contests, settling and resets all show at the moment they
// happened.
//
// The cost grows with the square of the log, so it is bounded: past limit
// instants (HistoryLimit when limit is zero or less) each subject reports its
// first status at its birth and its current status at the latest operation,
// and complete reports false.
func (l *Ledger) Histories(ctx context.Context, f Filter, limit int) (histories map[string][]StatusAt, complete bool, err error) {
	if limit <= 0 {
		limit = HistoryLimit
	}
	ops, err := l.read(ctx)
	if err != nil {
		return nil, false, err
	}
	decoded, err := decodeOps(ops)
	if err != nil {
		return nil, false, err
	}
	current := foldDecoded(decoded, l.axes)
	keep := map[string]bool{}
	for _, r := range current {
		if r.Kind.Bears() && f.matches(r) {
			keep[r.ID] = true
		}
	}
	instants := make([]time.Time, 0, len(ops))
	seen := map[time.Time]bool{}
	for _, op := range ops {
		if f.Project != "" && op.Project != f.Project {
			continue
		}
		at := op.At.UTC()
		if !seen[at] {
			seen[at] = true
			instants = append(instants, at)
		}
	}
	slices.SortFunc(instants, func(a, b time.Time) int { return a.Compare(b) })

	histories = make(map[string][]StatusAt, len(keep))
	if len(instants) > limit {
		latest := instants[len(instants)-1]
		for _, r := range current {
			if !keep[r.ID] {
				continue
			}
			h := []StatusAt{{At: r.At, Status: statusAtBirth(r.Kind)}}
			if r.Status != h[0].Status {
				h = append(h, StatusAt{At: latest, Status: r.Status})
			}
			histories[r.ID] = h
		}
		return histories, false, nil
	}
	for _, at := range instants {
		for _, r := range foldDecoded(acceptedByDecoded(decoded, at), l.axes) {
			if !keep[r.ID] {
				continue
			}
			h := histories[r.ID]
			if len(h) > 0 && h[len(h)-1].Status == r.Status {
				continue
			}
			histories[r.ID] = append(h, StatusAt{At: at, Status: r.Status})
		}
	}
	return histories, true, nil
}

// decodedOp is one operation of the log with its record decoded, so a reader
// that folds the log many times (Histories) decodes each payload once.
type decodedOp struct {
	op  workspace.Op
	rec Record
	ok  bool
}

// decodeOps decodes every operation of a run of the log.
func decodeOps(ops []workspace.Op) ([]decodedOp, error) {
	out := make([]decodedOp, len(ops))
	for i, op := range ops {
		r, ok, err := decode(op)
		if err != nil {
			return nil, err
		}
		out[i] = decodedOp{op: op, rec: r, ok: ok}
	}
	return out, nil
}

// cloneRecord copies the parts of a decoded record the fold appends to, so two
// folds over one decoded run never share them. The fold replaces the scope,
// the subject and the standing whole rather than editing them.
func cloneRecord(r Record) Record {
	r.ContestedBy = slices.Clone(r.ContestedBy)
	r.Because = slices.Clone(r.Because)
	return r
}

// acceptedByDecoded keeps the decoded operations accepted at or before at.
func acceptedByDecoded(ops []decodedOp, at time.Time) []decodedOp {
	out := make([]decodedOp, 0, len(ops))
	for _, d := range ops {
		if !d.op.At.After(at) {
			out = append(out, d)
		}
	}
	return out
}

// foldOps folds a run of the log's operations into records, each with the
// status the operations that named it left it at.
func foldOps(ops []workspace.Op, axes func(workspace.ProjectKey) axis.Space) ([]Record, error) {
	decoded, err := decodeOps(ops)
	if err != nil {
		return nil, err
	}
	return foldDecoded(decoded, axes), nil
}

// foldDecoded is foldOps over operations already decoded. Each fold works on
// its own copy of every record, so folding one decoded run many times is safe.
func foldDecoded(decoded []decodedOp, axes func(workspace.ProjectKey) axis.Space) []Record {
	spaces := map[workspace.ProjectKey]axis.Space{}
	spaceOf := func(key workspace.ProjectKey) axis.Space {
		if axes == nil {
			return nil
		}
		s, ok := spaces[key]
		if !ok {
			s = axes(key)
			spaces[key] = s
		}
		return s
	}
	ops := make([]workspace.Op, len(decoded))
	for i, d := range decoded {
		ops[i] = d.op
	}
	// What a reset set aside stays in the log and takes no part in the fold:
	// it is reported at StatusReset, beside the operations still in force.
	aside := workspace.SetAside(ops)
	records := make([]Record, 0, len(ops))
	var setAside []Record
	for _, d := range decoded {
		op, r, ok := d.op, cloneRecord(d.rec), d.ok
		if !ok {
			continue
		}
		r.space = spaceOf(r.Project)
		if aside[op.ID] {
			r.Status = StatusReset
			setAside = append(setAside, r)
			continue
		}
		r.Status = statusAtBirth(r.Kind)
		r.Established = r.Kind.Bears() && r.Status == StatusEstablished
		records = append(records, r)
	}
	// Id order, which is the order the operations were accepted in on every
	// machine whose log has been merged into this one. The position in that
	// order is what the fold compares.
	sort.SliceStable(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	for i := range records {
		records[i].Seq = int64(i + 1)
	}

	at := make(map[string]int, len(records))
	for i, r := range records {
		at[r.ID] = i
	}
	byID := index(records)
	// establishedAt is the position of the act that last established each
	// subject, so a correction can be told apart from one the person already
	// answered by keeping the rule again.
	establishedAt := make([]int64, len(records))
	facts := newSettleFacts(len(records))
	for i, r := range records {
		if r.Kind.Bears() && r.Status == StatusEstablished {
			establishedAt[i] = r.Seq
			facts.kept[i] = true
		}
	}

	for _, act := range records {
		if act.Kind.Bears() {
			continue
		}
		target, ok := bearer(byID, act)
		if !ok {
			continue
		}
		i, ok := at[target.ID]
		if !ok {
			continue
		}
		switch act.Kind {
		case KindKeep:
			records[i].Status = StatusEstablished
			establishedAt[i] = act.Seq
			records[i].Established = true
			facts.kept[i] = true
			if act.Subject.Kind != SubjectNone {
				records[i].Subject = act.Subject
			}
			if act.Scope.Level != "" || len(act.Scope.Coordinates) > 0 || act.Scope.AllProfiles || act.Scope.Windowed() {
				records[i].Scope = act.Scope
			}
		case KindDrop:
			records[i].Status = StatusDropped
			records[i].Established = false
			facts.droppedAt[i] = act.Seq
		case KindWithdraw:
			records[i].Status = StatusWithdrawn
			records[i].Established = false
			facts.withdrawnBy[i] = act.ID
		case KindEstablish:
			// Settling decides below whether the evidence still holds.
			facts.establishedBy[i] = append(facts.establishedBy[i], act.ID)
			if establishedAt[i] == 0 {
				establishedAt[i] = act.Seq
			}
		case KindSignal:
			facts.signals[i] = append(facts.signals[i], at[act.ID])
		case KindWiden:
			records[i].Scope = act.Scope
		}
	}
	settle(records, facts)
	contest(records, establishedAt)
	if len(setAside) == 0 {
		return records
	}
	records = append(records, setAside...)
	sort.SliceStable(records, func(i, j int) bool { return records[i].ID < records[j].ID })
	return records
}

// SessionSummary is what one agent session did: how many operations of each
// kind it recorded, and what became of them.
type SessionSummary struct {
	// Session is the id the agent recorded under.
	Session string `json:"session"`
	// Project is the project the session worked in, empty when it touched more
	// than one.
	Project workspace.ProjectKey `json:"project,omitempty"`
	// Actor is who the session belonged to.
	Actor Actor `json:"actor"`
	// Operations is how many operations the session recorded.
	Operations int `json:"operations"`
	// First and Last bound the session in time.
	First time.Time `json:"first,omitzero"`
	Last  time.Time `json:"last,omitzero"`
	// ByKind counts the session's operations by what they did.
	ByKind map[Kind]int `json:"by_kind,omitempty"`
	// ByStatus counts the session's subject-bearing operations by what became
	// of them.
	ByStatus map[Status]int `json:"by_status,omitempty"`
}

// Session summarizes one session: counts by kind and by status, with the span
// it covered. A session the log does not hold summarizes as zero operations
// rather than as an error, because "that session did nothing here" is an
// answer.
func (l *Ledger) Session(ctx context.Context, session string) (SessionSummary, error) {
	if strings.TrimSpace(session) == "" {
		return SessionSummary{}, errors.New("contextop: no session named")
	}
	all, err := l.fold(ctx)
	if err != nil {
		return SessionSummary{}, err
	}
	out := SessionSummary{
		Session:  session,
		ByKind:   map[Kind]int{},
		ByStatus: map[Status]int{},
	}
	projects := map[workspace.ProjectKey]bool{}
	for _, r := range all {
		if r.Actor.Session != session {
			continue
		}
		out.Operations++
		out.ByKind[r.Kind]++
		if r.Kind.Bears() {
			out.ByStatus[r.Status]++
		}
		projects[r.Project] = true
		out.Actor = r.Actor
		if out.First.IsZero() || r.At.Before(out.First) {
			out.First = r.At
		}
		if r.At.After(out.Last) {
			out.Last = r.At
		}
	}
	if len(projects) == 1 {
		for key := range projects {
			out.Project = key
		}
	}
	return out, nil
}

// statusAtBirth is the status an operation holds before anything acts on it.
// What somebody observed or corrected is a suggestion; what a person imported
// or wrote directly is established; an operation that acts on another stands
// as done.
func statusAtBirth(k Kind) Status {
	if k == KindObserve || k == KindCorrect {
		return StatusSuggested
	}
	return StatusEstablished
}

// index maps records by id.
func index(records []Record) map[string]Record {
	out := make(map[string]Record, len(records))
	for _, r := range records {
		out[r.ID] = r
	}
	return out
}

// find looks one record up by its id or an unambiguous prefix of it.
func find(records []Record, typed string) (Record, error) {
	ids := make([]string, len(records))
	for i, r := range records {
		ids[i] = r.ID
	}
	id, err := workspace.ResolveOpID(typed, ids)
	if err != nil {
		if ambiguous, ok := errors.AsType[*workspace.AmbiguousOpIDError](err); ok {
			ambiguous.Candidates = ShortIDs(ambiguous.Candidates)
		}
		return Record{}, fmt.Errorf("contextop: %w", err)
	}
	for _, r := range records {
		if r.ID == id {
			return r, nil
		}
	}
	return Record{}, fmt.Errorf("contextop: %w: %s", ErrNotFound, typed)
}

// bearer walks from an acting operation to the subject-bearing operation it
// ultimately acts on: dropping a keep reaches the suggestion the keep
// established. The walk is bounded by the number of records, so a chain that
// loops (which only a hand-built log could hold) ends rather than spinning.
func bearer(byID map[string]Record, r Record) (Record, bool) {
	for range len(byID) + 1 {
		if r.Kind.Bears() {
			return r, true
		}
		if r.Target == "" {
			return Record{}, false
		}
		next, ok := byID[r.Target]
		if !ok {
			return Record{}, false
		}
		r = next
	}
	return Record{}, false
}
