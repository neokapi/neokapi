package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
)

// The decision ledger, SQLite flavor — the same contract the Postgres store
// implements (bowrain/store/decisions.go): unit_decisions folds the event log
// to latest-per-(item, unit, variant), block_history keeps the events, and
// target_json.status is a written projection. One contract, two backends.

// UpsertUnitDecisions implements platstore.DecisionStore.
func (s *SQLiteStore) UpsertUnitDecisions(ctx context.Context, projectID, stream string, decisions []platstore.UnitDecision) (int, error) {
	if len(decisions) == 0 {
		return 0, nil
	}
	stream = storeutil.DefaultStream(stream)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin decisions tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit; the commit error is what matters

	changed := 0
	now := time.Now().UTC().Format(time.RFC3339)
	for _, d := range decisions {
		if d.Unit == "" || d.Variant == "" {
			continue
		}

		var old platstore.UnitDecision
		var haveOld bool
		var parked int
		row := tx.QueryRowContext(ctx,
			`SELECT status, target_hash, content_hash, review_state, decided_by, decided_at, note, parked, assignee, updated
			 FROM unit_decisions
			 WHERE project_id=? AND stream=? AND item_name=? AND unit=? AND variant=?`,
			projectID, stream, d.ItemName, d.Unit, d.Variant)
		switch err := row.Scan(&old.Status, &old.TargetHash, &old.ContentHash, &old.ReviewState, &old.DecidedBy,
			&old.DecidedAt, &old.Note, &parked, &old.Assignee, &old.Updated); {
		case err == nil:
			old.Parked = parked != 0
			haveOld = true
		case err == sql.ErrNoRows:
		default:
			return changed, fmt.Errorf("read decision %s/%s: %w", d.Unit, d.Variant, err)
		}
		if haveOld {
			if bstore.DecisionUnchanged(old, d) {
				continue
			}
			if old.Updated != "" && d.Updated != "" && d.Updated < old.Updated {
				continue // last-writer-wins: never roll a newer decision back
			}
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO unit_decisions
				(project_id, stream, item_name, unit, variant, status, target_hash, content_hash, review_state,
				 decided_by, decided_at, note, parked, assignee, updated, updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			 ON CONFLICT (project_id, stream, item_name, unit, variant) DO UPDATE SET
				status=excluded.status, target_hash=excluded.target_hash,
				content_hash=excluded.content_hash,
				review_state=excluded.review_state, decided_by=excluded.decided_by,
				decided_at=excluded.decided_at, note=excluded.note, parked=excluded.parked,
				assignee=excluded.assignee, updated=excluded.updated, updated_at=excluded.updated_at`,
			projectID, stream, d.ItemName, d.Unit, d.Variant, d.Status, d.TargetHash, d.ContentHash, d.ReviewState,
			d.DecidedBy, d.DecidedAt, d.Note, boolInt(d.Parked), d.Assignee, d.Updated, now); err != nil {
			return changed, fmt.Errorf("upsert decision %s/%s: %w", d.Unit, d.Variant, err)
		}
		changed++

		var blockID string
		if d.ItemName != "" {
			err := tx.QueryRowContext(ctx,
				`SELECT id FROM blocks WHERE project_id=? AND item_name=? AND source_id=?`,
				projectID, d.ItemName, d.Unit).Scan(&blockID)
			if err != nil && err != sql.ErrNoRows {
				return changed, fmt.Errorf("resolve decision block %s/%s: %w", d.ItemName, d.Unit, err)
			}
		}
		if blockID == "" {
			continue
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO block_history
				(project_id, block_id, locale, change_type, text, origin, author, stream, created_at)
			 VALUES (?,?,?,'decision','',?,?,?,?)`,
			projectID, blockID, d.Variant, d.ReviewState, d.DecidedBy, stream, now); err != nil {
			return changed, fmt.Errorf("log decision %s/%s: %w", d.Unit, d.Variant, err)
		}

		if d.Status == "" {
			continue
		}
		var targetJSON string
		err := tx.QueryRowContext(ctx,
			`SELECT target_json FROM translations WHERE project_id=? AND stream=? AND block_id=? AND locale=?`,
			projectID, stream, blockID, d.Variant).Scan(&targetJSON)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return changed, fmt.Errorf("read target for decision %s/%s: %w", d.Unit, d.Variant, err)
		}
		var tgt model.Target
		if uerr := json.Unmarshal([]byte(targetJSON), &tgt); uerr != nil {
			continue
		}
		if d.TargetHash != "" && state.TargetHash(model.RunsText(tgt.Runs)) != d.TargetHash {
			continue // decision blesses a different translation — stale on arrival
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE translations SET target_json = json_set(target_json, '$.status', ?), updated_at=?
			 WHERE project_id=? AND stream=? AND block_id=? AND locale=?`,
			d.Status, now, projectID, stream, blockID, d.Variant); err != nil {
			return changed, fmt.Errorf("project decision status %s/%s: %w", d.Unit, d.Variant, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return changed, fmt.Errorf("commit decisions: %w", err)
	}
	return changed, nil
}

// ListUnitDecisions implements platstore.DecisionStore.
func (s *SQLiteStore) ListUnitDecisions(ctx context.Context, projectID, stream string) ([]platstore.UnitDecision, error) {
	stream = storeutil.DefaultStream(stream)
	rows, err := s.db.QueryContext(ctx,
		`SELECT item_name, unit, variant, status, target_hash, content_hash, review_state,
			decided_by, decided_at, note, parked, assignee, updated
		 FROM unit_decisions WHERE project_id=? AND stream=?
		 ORDER BY item_name, unit, variant`,
		projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("list decisions: %w", err)
	}
	defer rows.Close()

	var out []platstore.UnitDecision
	for rows.Next() {
		d := platstore.UnitDecision{ProjectID: projectID, Stream: stream}
		var parked int
		if err := rows.Scan(&d.ItemName, &d.Unit, &d.Variant, &d.Status, &d.TargetHash, &d.ContentHash,
			&d.ReviewState, &d.DecidedBy, &d.DecidedAt, &d.Note, &parked, &d.Assignee, &d.Updated); err != nil {
			return nil, fmt.Errorf("scan decision: %w", err)
		}
		d.Parked = parked != 0
		out = append(out, d)
	}
	return out, rows.Err()
}

// demoteStaleApprovals drops reviewed/signed-off projected statuses to the
// presence baseline for every target of a block whose SOURCE content just
// changed — the SQLite mirror of demoteStaleApprovalsPg.
func demoteStaleApprovals(ctx context.Context, tx *sql.Tx, projectID, blockID string) error {
	rows, err := tx.QueryContext(ctx,
		`SELECT stream, locale FROM translations
		 WHERE project_id=? AND block_id=?
		   AND COALESCE(json_extract(target_json, '$.status'),'') IN ('reviewed','signed-off')`,
		projectID, blockID)
	if err != nil {
		return fmt.Errorf("find stale approvals for block %s: %w", blockID, err)
	}
	type demoted struct{ stream, locale string }
	var hits []demoted
	for rows.Next() {
		var d demoted
		if err := rows.Scan(&d.stream, &d.locale); err != nil {
			rows.Close()
			return fmt.Errorf("scan stale approval: %w", err)
		}
		hits = append(hits, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339)
	for _, h := range hits {
		if _, err := tx.ExecContext(ctx,
			`UPDATE translations SET target_json = json_set(target_json, '$.status', 'translated'), updated_at=?
			 WHERE project_id=? AND stream=? AND block_id=? AND locale=?`,
			now, projectID, h.stream, blockID, h.locale); err != nil {
			return fmt.Errorf("demote stale approval for block %s: %w", blockID, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO block_history
				(project_id, block_id, locale, change_type, origin, author, stream, created_at)
			 VALUES (?,?,?,'decision.stale','','system',?,?)`,
			projectID, blockID, h.locale, h.stream, now); err != nil {
			return fmt.Errorf("log stale approval for block %s: %w", blockID, err)
		}
	}
	return nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
