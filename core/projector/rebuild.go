package projector

import (
	"context"
	"errors"
	"fmt"
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
}

// Total is the number of operations the rebuild applied.
func (r RebuildReport) Total() int {
	n := 0
	for _, c := range r.Operations {
		n += c
	}
	return n
}

// projectionTables says which tables of the context store the projector
// writes: every table of the terms store and of the content memory, and the
// voice profiles with their archived versions. The voice store's scores,
// corrections and tags are kept by the checks that write them and are left in
// place.
func projectionTable(name string) bool {
	switch {
	case strings.HasPrefix(name, "tm_"), strings.HasPrefix(name, "tb_"):
		return !strings.HasSuffix(name, "_migrations")
	case name == "voice_profiles", name == "voice_profile_versions":
		return true
	}
	return false
}

// Rebuild empties the project's projections and replays the log into them: the
// terms store, the content memory, the voice profiles and the rules this
// project widened to the whole workspace. The operations are replayed in id
// order, which is the order every machine whose log has been merged agrees on.
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
	workspace.SortOps(ops)

	if err := p.reset(ctx); err != nil {
		return report, err
	}
	for _, op := range ops {
		if !projects(op.Kind) {
			continue
		}
		steps, _, derr := p.decode(ctx, op)
		if derr == nil {
			derr = p.applySteps(ctx, op.Kind, steps)
		}
		if derr != nil {
			if ctx.Err() != nil {
				return report, ctx.Err()
			}
			report.Failed = append(report.Failed, fmt.Sprintf("%s %s: %v", op.Kind, workspace.ShortOpID(op.ID), derr))
		}
		report.Operations[op.Kind]++
	}
	if err := p.rebuildIndexes(ctx); err != nil {
		return report, err
	}
	return report, p.setCursor(ctx, head)
}

// reset empties every projection table in the context store, and takes back
// out of the workspace every rule this project widened.
func (p *Projector) reset(ctx context.Context) error {
	raw := p.db.Raw()
	if raw == nil {
		return errNoSubsystem
	}
	rows, err := raw.QueryContext(ctx, `SELECT name, type, COALESCE(sql, '') FROM sqlite_master WHERE type = 'table'`)
	if err != nil {
		return fmt.Errorf("projector: list the context store's tables: %w", err)
	}
	var (
		names   []string
		virtual []string
	)
	for rows.Next() {
		var name, kind, sql string
		if err := rows.Scan(&name, &kind, &sql); err != nil {
			_ = rows.Close()
			return fmt.Errorf("projector: list the context store's tables: %w", err)
		}
		if !projectionTable(name) {
			continue
		}
		names = append(names, name)
		if strings.HasPrefix(strings.ToUpper(sql), "CREATE VIRTUAL TABLE") {
			virtual = append(virtual, name)
		}
	}
	_ = rows.Close()
	if err := rows.Err(); err != nil {
		return fmt.Errorf("projector: list the context store's tables: %w", err)
	}

	tx, err := raw.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("projector: empty the projections: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, name := range names {
		if shadowOf(name, virtual) {
			// A full-text index keeps its own shadow tables, which emptying
			// the index itself empties.
			continue
		}
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

// shadowOf reports whether a table is one a full-text index keeps for itself.
func shadowOf(name string, virtual []string) bool {
	for _, v := range virtual {
		if name != v && strings.HasPrefix(name, v+"_") {
			return true
		}
	}
	return false
}
