package projector

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/history"
	"github.com/neokapi/neokapi/core/workhome"
	"github.com/neokapi/neokapi/core/workspace"
)

// RebuildReport says what a rebuild replayed.
type RebuildReport struct {
	// Operations counts the operations applied, by kind.
	Operations map[string]int `json:"operations"`
	// Failed lists the operations that could not be read or whose steps the
	// store refused. A step a live write had refused is refused again, so the
	// store ends where the live writes left it.
	Failed []string `json:"failed,omitempty"`
	// Checkpoint is the last operation of the checkpoint the rebuild started
	// from, empty when it replayed the whole log.
	Checkpoint string `json:"checkpoint,omitempty"`
	// Retired counts, by kind, the operations of a kind the projector no
	// longer applies (RetiredKinds), which the rebuild left out.
	Retired map[string]int `json:"retired,omitempty"`
}

// RetiredKinds are the operation kinds the projector once applied and no
// longer does, each with the kind that took its place. The standing decision
// is to reset data rather than migrate it, so a log written before a kind was
// retired still holds its operations; a rebuild leaves them out and reports
// them, so a store that lost rows to the reset says so.
var RetiredKinds = map[string]string{
	// The decision ledger's entries, recorded as decision.record since.
	"unit.record": KindDecision,
}

// Total is the number of operations the rebuild applied.
func (r RebuildReport) Total() int {
	n := 0
	for _, c := range r.Operations {
		n += c
	}
	return n
}

// projectionTable says which tables of the context store the projector
// writes: every table of the terms store and of the content memory, the voice
// profiles with their archived versions, the decision ledger, the document
// adoptions and the block history. The voice store's scores, corrections and
// tags are kept by the checks that write them, and each checkout's view of the
// ledger and of its documents is kept by the checkout; both are left in place.
func projectionTable(name string) bool {
	switch {
	case strings.HasPrefix(name, "tm_"), strings.HasPrefix(name, "tb_"):
		return !strings.HasSuffix(name, "_migrations")
	case name == "voice_profiles", name == "voice_profile_versions", name == "unit_decision",
		name == "document_adoption", name == "block_history", name == "block_history_op":
		return true
	case slices.Contains(workhome.Tables, name):
		return true
	}
	return false
}

// Rebuild empties the project's projections and replays the log into them: the
// terms store, the content memory, the voice profiles, the decision ledger, the
// document adoptions, the block history and the rules this project widened to
// the whole workspace. It starts from the
// latest checkpoint that still stands and replays the operations after it, or
// from nothing when there is none. The operations are replayed in id order,
// which is the order every machine whose log has been merged agrees on.
//
// On a log one machine wrote, the rebuilt stores equal the ones the writes
// left behind, because each operation carries the rows its write put and every
// timestamp it stamped.
func (p *Projector) Rebuild(ctx context.Context) (RebuildReport, error) {
	report := RebuildReport{Operations: map[string]int{}}
	if p.log == nil {
		return report, errors.New("projector: this store has no log to rebuild from")
	}
	p.lock.Lock()
	defer p.lock.Unlock()
	return p.rebuildLocked(ctx)
}

// rebuildLocked is Rebuild for a caller already holding the store's lock.
func (p *Projector) rebuildLocked(ctx context.Context) (RebuildReport, error) {
	report := RebuildReport{Operations: map[string]int{}}
	ops, err := p.log.Select(ctx, workspace.OpQuery{Project: p.key})
	if err != nil {
		return report, err
	}
	// What the log holds from before the project's latest removal from the
	// workspace built the context the removal deleted.
	if forgot := lastForget(ops); forgot > 0 {
		later := ops[:0:0]
		for _, op := range ops {
			if op.Seq > forgot {
				later = append(later, op)
			}
		}
		ops = later
	}
	var head int64
	for _, op := range ops {
		head = max(head, op.Seq)
	}
	cp, pkg, fromCheckpoint := p.latestCheckpoint(ctx, ops)
	// What a reset set aside stays in the log and is not replayed, so the
	// stores stand as they did before the point the reset names.
	if aside := workspace.SetAside(ops); len(aside) > 0 {
		kept := ops[:0:0]
		for _, op := range ops {
			if !aside[op.ID] {
				kept = append(kept, op)
			}
		}
		ops = kept
	}

	if err := p.reset(ctx); err != nil {
		return report, err
	}
	if fromCheckpoint {
		if err := p.loadTables(ctx, pkg); err != nil {
			// A checkpoint that cannot be loaded, a part of it missing or
			// unreadable, costs a replay of the whole log and nothing more.
			report.Failed = append(report.Failed, fmt.Sprintf("%s through %s: %v",
				KindCheckpoint, workspace.ShortOpID(cp.Through), err))
			if err := p.reset(ctx); err != nil {
				return report, err
			}
			fromCheckpoint = false
		}
	}
	if fromCheckpoint {
		report.Checkpoint = cp.Through
		later := ops[:0:0]
		for _, op := range ops {
			if op.Seq > cp.Seq {
				later = append(later, op)
			}
		}
		ops = later
	}
	workspace.SortOps(ops)
	// Consecutive operations that each put content-memory entries one at a
	// time are replayed together, which is what keeps a log of thousands of
	// single writes to seconds. Anything else in between ends the run.
	// Consecutive ledger entries are applied in one transaction the same way,
	// and so are the block-history rows of consecutive edits.
	var (
		run     []step
		runOps  []workspace.Op
		runKind string
		edits   []history.Row
		editOps []workspace.Op
		works   []workhome.Write
		docs    []workhome.DocWrite
	)
	fail := func(op workspace.Op, err error) {
		report.Failed = append(report.Failed, fmt.Sprintf("%s %s: %v", op.Kind, workspace.ShortOpID(op.ID), err))
	}
	flush := func() {
		if len(run) == 0 {
			return
		}
		if _, err := p.applySteps(ctx, runKind, run); err != nil {
			fail(runOps[len(runOps)-1], err)
		}
		run, runOps, runKind = nil, nil, ""
	}
	flushEdits := func() {
		if len(edits) == 0 && len(docs) == 0 {
			return
		}
		if err := p.applyEditRows(ctx, edits); err != nil {
			fail(editOps[len(editOps)-1], err)
		}
		if err := p.applyWorkWrites(ctx, works); err != nil {
			fail(editOps[len(editOps)-1], err)
		}
		if err := p.applyDocWrites(ctx, docs); err != nil {
			fail(editOps[len(editOps)-1], err)
		}
		edits, editOps, works, docs = nil, nil, nil, nil
	}
	for _, op := range ops {
		if !projects(op.Kind) {
			if _, retired := RetiredKinds[op.Kind]; retired {
				if report.Retired == nil {
					report.Retired = map[string]int{}
				}
				report.Retired[op.Kind]++
			}
			continue
		}
		report.Operations[op.Kind]++
		if op.Kind == KindEdit {
			flush()
			e, derr := p.decodeEdit(ctx, op)
			if derr != nil {
				fail(op, derr)
				continue
			}
			if e.kept() {
				w, werr := p.workWrite(ctx, op, e)
				if werr != nil {
					fail(op, werr)
					continue
				}
				works = append(works, w)
			}
			if e.whole() {
				docs = append(docs, docWrite(op, e))
			}
			edits, editOps = append(edits, editRows(op.ID, opAddress(op), op.At, e)...), append(editOps, op)
			continue
		}
		flushEdits()
		steps, _, derr := p.decode(ctx, op)
		if derr != nil {
			fail(op, derr)
			continue
		}
		joins := (op.Kind == KindDecision && (len(run) == 0 || runKind == KindDecision)) ||
			(op.Kind == KindMemory && allReplayable(steps) &&
				(len(run) == 0 || (runKind == KindMemory && steps[0].Stream == run[0].Stream)))
		if !joins && len(run) > 0 {
			flush()
			joins = op.Kind == KindDecision || (op.Kind == KindMemory && allReplayable(steps))
		}
		if joins {
			run, runOps, runKind = append(run, steps...), append(runOps, op), op.Kind
			continue
		}
		flush()
		if _, err := p.applySteps(ctx, op.Kind, steps); err != nil {
			if ctx.Err() != nil {
				return report, ctx.Err()
			}
			fail(op, err)
		}
	}
	flush()
	flushEdits()
	if ctx.Err() != nil {
		return report, ctx.Err()
	}
	if err := p.rebuildIndexes(ctx); err != nil {
		return report, err
	}
	return report, p.setCursor(ctx, head)
}

// reset empties every projection table in the context store, and takes back
// out of the workspace every rule this project widened.
func (p *Projector) reset(ctx context.Context) error {
	raw := p.st.Raw
	if raw == nil {
		return errNoSubsystem
	}
	names, virtual, err := p.projectionTables(ctx)
	if err != nil {
		return err
	}
	tx, err := raw.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("projector: empty the projections: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	// A full-text index is emptied through the virtual table, which empties
	// the shadow tables it keeps for itself.
	for _, name := range append(names, virtual...) {
		if _, err := tx.ExecContext(ctx, `DELETE FROM "`+name+`"`); err != nil {
			return fmt.Errorf("projector: empty %s: %w", name, err)
		}
	}
	// A table numbering its rows with AUTOINCREMENT starts again from one, so
	// the replay numbers them as the first writes did.
	var sequences int
	if err := tx.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'sqlite_sequence'`).Scan(&sequences); err != nil {
		return fmt.Errorf("projector: empty the projections: %w", err)
	}
	if sequences > 0 {
		for _, name := range names {
			if _, err := tx.ExecContext(ctx, `DELETE FROM sqlite_sequence WHERE name = ?`, name); err != nil {
				return fmt.Errorf("projector: restart the numbering of %s: %w", name, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("projector: empty the projections: %w", err)
	}

	held, err := p.log.WidenedRules(ctx, "")
	if err != nil {
		return err
	}
	for _, rule := range held {
		if rule.Origin == p.key {
			if err := p.log.NarrowRule(ctx, rule.ID); err != nil {
				return err
			}
		}
	}
	return nil
}

// allReplayable reports whether every step of an operation only puts entries
// one at a time, on one stream.
func allReplayable(steps []step) bool {
	if len(steps) == 0 {
		return false
	}
	for _, s := range steps {
		if !replayable(s) || s.Stream != steps[0].Stream {
			return false
		}
	}
	return true
}

// projectionTables lists the projection tables of the context store: names
// are the ordinary tables, and virtual the full-text indexes, whose own shadow
// tables are left out of both.
func (p *Projector) projectionTables(ctx context.Context) (names, virtual []string, err error) {
	rows, err := p.st.Raw.QueryContext(ctx, `SELECT name, COALESCE(sql, '') FROM sqlite_master WHERE type = 'table' ORDER BY name`)
	if err != nil {
		return nil, nil, fmt.Errorf("projector: list the context store's tables: %w", err)
	}
	var all []string
	for rows.Next() {
		var name, ddl string
		if err := rows.Scan(&name, &ddl); err != nil {
			_ = rows.Close()
			return nil, nil, fmt.Errorf("projector: list the context store's tables: %w", err)
		}
		if !projectionTable(name) {
			continue
		}
		all = append(all, name)
		if strings.HasPrefix(strings.ToUpper(ddl), "CREATE VIRTUAL TABLE") {
			virtual = append(virtual, name)
		}
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("projector: list the context store's tables: %w", err)
	}
	for _, name := range all {
		if !slices.Contains(virtual, name) && !shadowOf(name, virtual) {
			names = append(names, name)
		}
	}
	return names, virtual, nil
}

// shadowOf reports whether a table is one a full-text index keeps for itself.
func shadowOf(name string, virtual []string) bool {
	for _, v := range virtual {
		if name != v && strings.HasPrefix(name, v+"_") {
			return true
		}
	}
	return false
}
