package projector

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

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
// profiles with their archived versions, and the unit decision ledger. The
// voice store's scores, corrections and tags are kept by the checks that write
// them, and each checkout's view of the ledger is kept by the checkout; both
// are left in place.
func projectionTable(name string) bool {
	switch {
	case strings.HasPrefix(name, "tm_"), strings.HasPrefix(name, "tb_"):
		return !strings.HasSuffix(name, "_migrations")
	case name == "voice_profiles", name == "voice_profile_versions", name == "unit_decision":
		return true
	}
	return false
}

// Rebuild empties the project's projections and replays the log into them: the
// terms store, the content memory, the voice profiles, the unit decision ledger
// and the rules this project widened to the whole workspace. It starts from the
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

	ops, err := p.log.Select(ctx, workspace.OpQuery{Project: p.key})
	if err != nil {
		return report, err
	}
	var head int64
	for _, op := range ops {
		head = max(head, op.Seq)
	}
	cp, pkg, fromCheckpoint := p.latestCheckpoint(ctx, ops)

	if err := p.reset(ctx); err != nil {
		return report, err
	}
	if fromCheckpoint {
		if err := p.loadTables(ctx, pkg); err != nil {
			return report, err
		}
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
	// Consecutive ledger entries are applied in one transaction the same way.
	var (
		run     []step
		runOps  []workspace.Op
		runKind string
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
	for _, op := range ops {
		if !projects(op.Kind) {
			continue
		}
		report.Operations[op.Kind]++
		steps, _, derr := p.decode(ctx, op)
		if derr != nil {
			fail(op, derr)
			continue
		}
		joins := (op.Kind == KindUnit && (len(run) == 0 || runKind == KindUnit)) ||
			(op.Kind == KindMemory && allReplayable(steps) &&
				(len(run) == 0 || (runKind == KindMemory && steps[0].Stream == run[0].Stream)))
		if !joins && len(run) > 0 {
			flush()
			joins = op.Kind == KindUnit || (op.Kind == KindMemory && allReplayable(steps))
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
