package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
)

// The decision ledger — the server side of core/state. A decision is a FACT
// (who, when, which rung, the hash of the translation it blesses); the
// unit_decisions table folds the event log to latest-per-(item, unit, variant),
// block_history keeps the events, and target_json.status is a written
// PROJECTION of the ledger so every existing status reader keeps working.
// Freshness is derived at the point of use, never stored: a decision whose
// TargetHash no longer matches the current translation is stale, and a source
// edit demotes the projection (storeBlocks, "decision.stale").

// deciderRole classifies a decision identity for the audit trail: "" (plain
// human), "ai/<model>" (autonomous), "agent/<client>" (acting for a person).
func deciderRole(by string) string {
	switch {
	case strings.HasPrefix(by, state.AIIdentityPrefix):
		return "ai"
	case strings.HasPrefix(by, "agent/"):
		return "agent"
	default:
		return "human"
	}
}

// DecisionUnchanged reports whether an incoming decision carries nothing the
// stored row does not already say — the idempotency test for re-pushes. Shared
// with the SQLite store so the two backends cannot disagree on what counts as
// a change.
func DecisionUnchanged(old, next platstore.UnitDecision) bool {
	return old.Status == next.Status &&
		old.TargetHash == next.TargetHash &&
		old.ReviewState == next.ReviewState &&
		old.DecidedBy == next.DecidedBy &&
		old.DecidedAt == next.DecidedAt &&
		old.Note == next.Note &&
		old.Parked == next.Parked &&
		old.Assignee == next.Assignee
}

// UpsertUnitDecisions implements platstore.DecisionStore: idempotent
// last-writer-wins upsert keyed by (item, unit, variant). A record older than
// the stored one (by Updated) is skipped — both ends send full sets, so a
// stale replay must not roll a newer decision back. Each record that actually
// changes appends a block_history event and re-projects the target status,
// when the unit resolves to a stored block and the decision still blesses the
// current translation.
func (s *PostgresStore) UpsertUnitDecisions(ctx context.Context, projectID, stream string, decisions []platstore.UnitDecision) (int, error) {
	if len(decisions) == 0 {
		return 0, nil
	}
	stream = storeutil.DefaultStream(stream)

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin decisions tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	changed := 0
	now := time.Now().UTC()
	for _, d := range decisions {
		if d.Unit == "" || d.Variant == "" {
			continue
		}

		var old platstore.UnitDecision
		var haveOld bool
		row := tx.QueryRowContext(ctx,
			`SELECT status, target_hash, review_state, decided_by, decided_at, note, parked, assignee, updated
			 FROM unit_decisions
			 WHERE project_id=$1 AND stream=$2 AND item_name=$3 AND unit=$4 AND variant=$5`,
			projectID, stream, d.ItemName, d.Unit, d.Variant)
		switch err := row.Scan(&old.Status, &old.TargetHash, &old.ReviewState, &old.DecidedBy,
			&old.DecidedAt, &old.Note, &old.Parked, &old.Assignee, &old.Updated); {
		case err == nil:
			haveOld = true
		case err == sql.ErrNoRows:
		default:
			return changed, fmt.Errorf("read decision %s/%s: %w", d.Unit, d.Variant, err)
		}
		if haveOld {
			if DecisionUnchanged(old, d) {
				continue
			}
			// Last-writer-wins by record time: both directions send full sets,
			// so a replay of an older record must not undo a newer decision.
			if old.Updated != "" && d.Updated != "" && d.Updated < old.Updated {
				continue
			}
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO unit_decisions
				(project_id, stream, item_name, unit, variant, status, target_hash, review_state,
				 decided_by, decided_at, note, parked, assignee, updated, updated_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
			 ON CONFLICT (project_id, stream, item_name, unit, variant) DO UPDATE SET
				status=EXCLUDED.status, target_hash=EXCLUDED.target_hash,
				review_state=EXCLUDED.review_state, decided_by=EXCLUDED.decided_by,
				decided_at=EXCLUDED.decided_at, note=EXCLUDED.note, parked=EXCLUDED.parked,
				assignee=EXCLUDED.assignee, updated=EXCLUDED.updated, updated_at=EXCLUDED.updated_at`,
			projectID, stream, d.ItemName, d.Unit, d.Variant, d.Status, d.TargetHash, d.ReviewState,
			d.DecidedBy, d.DecidedAt, d.Note, d.Parked, d.Assignee, d.Updated, now); err != nil {
			return changed, fmt.Errorf("upsert decision %s/%s: %w", d.Unit, d.Variant, err)
		}
		changed++

		// Resolve the stored block the unit names. A decision for content this
		// store has never held stays ledger-only and heals when the content
		// arrives.
		var blockID string
		if d.ItemName != "" {
			err := tx.QueryRowContext(ctx,
				`SELECT id FROM blocks WHERE project_id=$1 AND item_name=$2 AND source_id=$3`,
				projectID, d.ItemName, d.Unit).Scan(&blockID)
			if err != nil && err != sql.ErrNoRows {
				return changed, fmt.Errorf("resolve decision block %s/%s: %w", d.ItemName, d.Unit, err)
			}
		}
		if blockID == "" {
			continue
		}

		// The event, in the log the audit trail already reads.
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO block_history
				(project_id, block_id, locale, change_type, text, origin, author, actor_role, edit_reason, stream, created_at)
			 VALUES ($1,$2,$3,'decision','',$4,$5,$6,$7,$8,$9)`,
			projectID, blockID, d.Variant, d.ReviewState, d.DecidedBy, deciderRole(d.DecidedBy),
			d.Note, stream, now); err != nil {
			return changed, fmt.Errorf("log decision %s/%s: %w", d.Unit, d.Variant, err)
		}

		// Project the status — only when the decision still blesses the
		// translation the row currently holds. A stale-on-arrival decision is
		// recorded but moves nothing.
		if d.Status == "" {
			continue
		}
		var targetJSON string
		err := tx.QueryRowContext(ctx,
			`SELECT target_json FROM translations WHERE project_id=$1 AND stream=$2 AND block_id=$3 AND locale=$4`,
			projectID, stream, blockID, d.Variant).Scan(&targetJSON)
		if err == sql.ErrNoRows {
			continue
		}
		if err != nil {
			return changed, fmt.Errorf("read target for decision %s/%s: %w", d.Unit, d.Variant, err)
		}
		var tgt model.Target
		if uerr := json.Unmarshal([]byte(targetJSON), &tgt); uerr != nil {
			continue // an unreadable target is not this write's to repair
		}
		if d.TargetHash != "" && state.TargetHash(model.RunsText(tgt.Runs)) != d.TargetHash {
			continue // decision blesses a different translation — stale on arrival
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE translations SET target_json = jsonb_set(target_json, '{status}', to_jsonb($5::text)), updated_at=$6
			 WHERE project_id=$1 AND stream=$2 AND block_id=$3 AND locale=$4`,
			projectID, stream, blockID, d.Variant, d.Status, now); err != nil {
			return changed, fmt.Errorf("project decision status %s/%s: %w", d.Unit, d.Variant, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return changed, fmt.Errorf("commit decisions: %w", err)
	}
	return changed, nil
}

// ListUnitDecisions implements platstore.DecisionStore.
func (s *PostgresStore) ListUnitDecisions(ctx context.Context, projectID, stream string) ([]platstore.UnitDecision, error) {
	stream = storeutil.DefaultStream(stream)
	rows, err := s.db.QueryContext(ctx,
		`SELECT item_name, unit, variant, status, target_hash, review_state,
			decided_by, decided_at, note, parked, assignee, updated
		 FROM unit_decisions WHERE project_id=$1 AND stream=$2
		 ORDER BY item_name, unit, variant`,
		projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("list decisions: %w", err)
	}
	defer rows.Close()

	var out []platstore.UnitDecision
	for rows.Next() {
		d := platstore.UnitDecision{ProjectID: projectID, Stream: stream}
		if err := rows.Scan(&d.ItemName, &d.Unit, &d.Variant, &d.Status, &d.TargetHash,
			&d.ReviewState, &d.DecidedBy, &d.DecidedAt, &d.Note, &d.Parked, &d.Assignee, &d.Updated); err != nil {
			return nil, fmt.Errorf("scan decision: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// demoteStaleApprovalsPg drops reviewed/signed-off projected statuses back to
// the presence baseline for every target of a block whose SOURCE content just
// changed — an approval is about a specific pairing of source and translation,
// and half of that pairing is gone. Runs inside the storeBlocks transaction.
// The block_history event ("decision.stale", author "system") is what makes
// the demotion auditable rather than a silent status flip.
func demoteStaleApprovalsPg(ctx context.Context, tx *sql.Tx, projectID, blockID string) error {
	rows, err := tx.QueryContext(ctx,
		`UPDATE translations
		 SET target_json = jsonb_set(target_json, '{status}', to_jsonb('translated'::text)), updated_at = NOW()
		 WHERE project_id=$1 AND block_id=$2
		   AND COALESCE(target_json->>'status','') IN ('reviewed','signed-off')
		 RETURNING stream, locale`,
		projectID, blockID)
	if err != nil {
		return fmt.Errorf("demote stale approvals for block %s: %w", blockID, err)
	}
	type demoted struct{ stream, locale string }
	var hits []demoted
	for rows.Next() {
		var d demoted
		if err := rows.Scan(&d.stream, &d.locale); err != nil {
			rows.Close()
			return fmt.Errorf("scan demoted approval: %w", err)
		}
		hits = append(hits, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, h := range hits {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO block_history
				(project_id, block_id, locale, change_type, origin, author, actor_role, edit_reason, stream, created_at)
			 VALUES ($1,$2,$3,'decision.stale','','system','system','source content changed',$4,$5)`,
			projectID, blockID, h.locale, h.stream, now); err != nil {
			return fmt.Errorf("log stale approval for block %s: %w", blockID, err)
		}
	}
	return nil
}
