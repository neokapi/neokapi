package sqlitestore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
)

// The decision ledger, SQLite flavor — the same contract the Postgres store
// implements (bowrain/store/decisions.go): unit_decisions folds the event log
// to latest-per-(item, unit, variant), block_history keeps the events, and
// target_json.status is a written projection. One contract, two backends.

// UpsertUnitDecisions implements platstore.DecisionStore.
// resolveItemIDSQLite is the item a decision names, by identity rather than
// address. Mirrors resolveItemIDPg — see it for why an unknown name gets a row
// rather than an empty key.
func resolveItemIDSQLite(ctx context.Context, tx *sql.Tx, projectID, stream, itemName string) (string, error) {
	if itemName == "" {
		return "", nil
	}
	var itemID string
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM items WHERE project_id=? AND stream=? AND name=?`,
		projectID, stream, itemName).Scan(&itemID)
	if err == nil {
		return itemID, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("resolve item %q: %w", itemName, err)
	}
	itemID = id.New()
	if _, err := tx.ExecContext(ctx,
		`INSERT INTO items (id, project_id, stream, name, format, item_type)
		 VALUES (?,?,?,?,'','file')
		 ON CONFLICT (project_id, stream, name) DO NOTHING`,
		itemID, projectID, stream, itemName); err != nil {
		return "", fmt.Errorf("record item %q for its decisions: %w", itemName, err)
	}
	if err := tx.QueryRowContext(ctx,
		`SELECT id FROM items WHERE project_id=? AND stream=? AND name=?`,
		projectID, stream, itemName).Scan(&itemID); err != nil {
		return "", fmt.Errorf("resolve item %q after recording it: %w", itemName, err)
	}
	return itemID, nil
}

func (s *SQLiteStore) UpsertUnitDecisions(ctx context.Context, projectID, stream string, decisions []venue.UnitDecision) (int, error) {
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

		itemID, err := resolveItemIDSQLite(ctx, tx, projectID, stream, d.ItemName)
		if err != nil {
			return changed, err
		}

		var old venue.UnitDecision
		var haveOld bool
		var parked int
		row := tx.QueryRowContext(ctx,
			`SELECT status, target_hash, content_hash, review_state, decided_by, decided_at, note, parked, assignee,
				governing_fingerprint, updated
			 FROM unit_decisions
			 WHERE project_id=? AND stream=? AND item_id=? AND unit=? AND variant=?`,
			projectID, stream, itemID, d.Unit, d.Variant)
		switch err := row.Scan(&old.Status, &old.TargetHash, &old.ContentHash, &old.ReviewState, &old.DecidedBy,
			&old.DecidedAt, &old.Note, &parked, &old.Assignee, &old.GoverningFingerprint, &old.Updated); {
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
				(project_id, stream, item_id, item_name, unit, variant, status, target_hash, content_hash, review_state,
				 decided_by, decided_at, note, parked, assignee, governing_fingerprint, updated, updated_at)
			 VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
			 ON CONFLICT (project_id, stream, item_id, unit, variant) DO UPDATE SET
				item_name=excluded.item_name,
				status=excluded.status, target_hash=excluded.target_hash,
				content_hash=excluded.content_hash,
				review_state=excluded.review_state, decided_by=excluded.decided_by,
				decided_at=excluded.decided_at, note=excluded.note, parked=excluded.parked,
				assignee=excluded.assignee, governing_fingerprint=excluded.governing_fingerprint,
				updated=excluded.updated, updated_at=excluded.updated_at`,
			projectID, stream, itemID, d.ItemName, d.Unit, d.Variant, d.Status, d.TargetHash, d.ContentHash, d.ReviewState,
			d.DecidedBy, d.DecidedAt, d.Note, boolInt(d.Parked), d.Assignee, d.GoverningFingerprint, d.Updated, now); err != nil {
			return changed, fmt.Errorf("upsert decision %s/%s: %w", d.Unit, d.Variant, err)
		}
		changed++

		var blockID, blockHash string
		if d.ItemName != "" {
			err := tx.QueryRowContext(ctx,
				`SELECT id, content_hash FROM blocks WHERE project_id=? AND stream=? AND item_name=? AND source_id=?`,
				projectID, stream, d.ItemName, d.Unit).Scan(&blockID, &blockHash)
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

		// The projection holds only while both halves of the blessed pairing do:
		// the translation the row carries, and the source it was blessed for.
		// An empty basis is unknown, not stale, and projects as before.
		if d.Status == "" {
			continue
		}
		if d.ContentHash != "" && blockHash != "" && d.ContentHash != blockHash {
			continue
		}
		var targetJSON string
		err = tx.QueryRowContext(ctx,
			`SELECT target_json FROM translations WHERE project_id=? AND stream=? AND block_id=? AND locale=?`,
			projectID, stream, blockID, d.Variant).Scan(&targetJSON)
		if errors.Is(err, sql.ErrNoRows) {
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
func (s *SQLiteStore) ListUnitDecisions(ctx context.Context, projectID, stream string) ([]venue.UnitDecision, error) {
	stream = storeutil.DefaultStream(stream)
	rows, err := s.db.QueryContext(ctx,
		`SELECT item_name, unit, variant, status, target_hash, content_hash, review_state,
			decided_by, decided_at, note, parked, assignee, governing_fingerprint, updated
		 FROM unit_decisions WHERE project_id=? AND stream=?
		 ORDER BY item_name, unit, variant`,
		projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("list decisions: %w", err)
	}
	defer rows.Close()

	var out []venue.UnitDecision
	for rows.Next() {
		d := venue.UnitDecision{ProjectID: projectID, Stream: stream}
		var parked int
		if err := rows.Scan(&d.ItemName, &d.Unit, &d.Variant, &d.Status, &d.TargetHash, &d.ContentHash,
			&d.ReviewState, &d.DecidedBy, &d.DecidedAt, &d.Note, &parked, &d.Assignee, &d.GoverningFingerprint, &d.Updated); err != nil {
			return nil, fmt.Errorf("scan decision: %w", err)
		}
		d.Parked = parked != 0
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetUnitDecision implements platstore.UnitDecisionReader.
func (s *SQLiteStore) GetUnitDecision(ctx context.Context, projectID, stream, itemName, unit, variant string) (*venue.UnitDecision, error) {
	stream = storeutil.DefaultStream(stream)
	d := venue.UnitDecision{ProjectID: projectID, Stream: stream}
	var parked int
	err := s.db.QueryRowContext(ctx,
		`SELECT item_name, unit, variant, status, target_hash, content_hash, review_state,
			decided_by, decided_at, note, parked, assignee, governing_fingerprint, updated
		 FROM unit_decisions
		 WHERE project_id=? AND stream=? AND item_name=? AND unit=? AND variant=?`,
		projectID, stream, itemName, unit, variant).
		Scan(&d.ItemName, &d.Unit, &d.Variant, &d.Status, &d.TargetHash, &d.ContentHash,
			&d.ReviewState, &d.DecidedBy, &d.DecidedAt, &d.Note, &parked, &d.Assignee, &d.GoverningFingerprint, &d.Updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get decision: %w", err)
	}
	d.Parked = parked != 0
	return &d, nil
}

// TallyDecisionBasis implements platstore.DecisionStore — the SQLite mirror of
// the Postgres grading, joining each decision's recorded basis to the block's
// current source hash, and its draft basis beside it for the owed count.
func (s *SQLiteStore) TallyDecisionBasis(ctx context.Context, projectID, stream string) ([]platstore.DecisionBasisTally, error) {
	stream = storeutil.DefaultStream(stream)
	rows, err := s.db.QueryContext(ctx,
		`SELECT d.item_name, d.variant,
			COALESCE(SUM(CASE WHEN d.content_hash <> '' AND d.content_hash <> b.content_hash THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN d.content_hash = '' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN d.content_hash <> '' AND d.content_hash <> b.content_hash
				AND d.draft_basis <> b.content_hash
				AND EXISTS (SELECT 1 FROM translations t
					WHERE t.project_id = b.project_id AND t.stream = b.stream
					AND t.block_id = b.id AND t.locale = d.variant)
				THEN 1 ELSE 0 END), 0)
		 FROM unit_decisions d
		 JOIN blocks b ON b.project_id = d.project_id
			AND b.stream = d.stream
			AND b.item_id = d.item_id
			AND b.source_id = d.unit
		 WHERE d.project_id=? AND d.stream=? AND b.translatable
		 GROUP BY d.item_name, d.variant
		 ORDER BY d.item_name, d.variant`,
		projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("tally decision basis: %w", err)
	}
	defer rows.Close()

	var out []platstore.DecisionBasisTally
	for rows.Next() {
		var t platstore.DecisionBasisTally
		if err := rows.Scan(&t.ItemName, &t.Variant, &t.Stale, &t.BasisUnknown, &t.Owed); err != nil {
			return nil, fmt.Errorf("scan decision basis tally: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// RecordDraftBases implements platstore.DecisionStore, mirroring the Postgres
// stamp: one UPDATE per unit, keyed on the item by identity, and no row
// created for a unit the ledger does not hold.
func (s *SQLiteStore) RecordDraftBases(ctx context.Context, projectID, stream string, drafts []platstore.DraftBasis) error {
	if len(drafts) == 0 {
		return nil
	}
	stream = storeutil.DefaultStream(stream)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin draft basis tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit; the commit error is what matters

	now := time.Now().UTC().Format(time.RFC3339)
	for _, d := range drafts {
		if d.ItemName == "" || d.Unit == "" || d.Variant == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE unit_decisions SET draft_basis=?, updated_at=?
			 WHERE project_id=? AND stream=? AND unit=? AND variant=?
			   AND item_id = (SELECT id FROM items WHERE project_id=? AND stream=? AND name=?)`,
			d.SourceHash, now, projectID, stream, d.Unit, d.Variant, projectID, stream, d.ItemName); err != nil {
			return fmt.Errorf("record draft basis %s/%s: %w", d.Unit, d.Variant, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit draft bases: %w", err)
	}
	return nil
}

// ListDraftBases implements platstore.DecisionStore.
func (s *SQLiteStore) ListDraftBases(ctx context.Context, projectID, stream string) ([]platstore.DraftBasis, error) {
	stream = storeutil.DefaultStream(stream)
	rows, err := s.db.QueryContext(ctx,
		`SELECT item_name, unit, variant, draft_basis
		 FROM unit_decisions WHERE project_id=? AND stream=? AND draft_basis <> ''
		 ORDER BY item_name, unit, variant`,
		projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("list draft bases: %w", err)
	}
	defer rows.Close()

	var out []platstore.DraftBasis
	for rows.Next() {
		var d platstore.DraftBasis
		if err := rows.Scan(&d.ItemName, &d.Unit, &d.Variant, &d.SourceHash); err != nil {
			return nil, fmt.Errorf("scan draft basis: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// settleDecisionProjections re-derives the projected statuses of every target
// of a block whose SOURCE content just changed, against the decision ledger —
// the SQLite mirror of settleDecisionProjectionsPg, grading through the same
// shared bstore.SettleDecisionProjection.
func settleDecisionProjections(ctx context.Context, tx *sql.Tx, projectID, stream, blockID, contentHash string) error {
	// The ledger keys on the unit identity (item + source_id), not the block id.
	var itemID, unit string
	if err := tx.QueryRowContext(ctx,
		`SELECT item_id, source_id FROM blocks WHERE project_id=? AND stream=? AND id=?`,
		projectID, stream, blockID).Scan(&itemID, &unit); err != nil {
		return fmt.Errorf("look up unit for block %s: %w", blockID, err)
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT t.stream, t.locale, COALESCE(json_extract(t.target_json, '$.status'),''), t.target_json,
			COALESCE(d.status,''), COALESCE(d.content_hash,''), COALESCE(d.target_hash,'')
		 FROM translations t
		 LEFT JOIN unit_decisions d
			ON d.project_id=t.project_id AND d.stream=t.stream
			AND d.item_id=? AND d.unit=? AND d.variant=t.locale
		 WHERE t.project_id=? AND t.stream=? AND t.block_id=?`,
		itemID, unit, projectID, stream, blockID)
	if err != nil {
		return fmt.Errorf("read projections for block %s: %w", blockID, err)
	}
	var settled []bstore.DecisionProjection
	for rows.Next() {
		var p bstore.DecisionProjection
		var targetJSON string
		if err := rows.Scan(&p.Stream, &p.Locale, &p.Status, &targetJSON,
			&p.DecisionStatus, &p.DecisionBasis, &p.DecisionTargetHash); err != nil {
			rows.Close()
			return fmt.Errorf("scan projection for block %s: %w", blockID, err)
		}
		p.TargetText = bstore.TargetTextFromJSON(targetJSON)
		settled = append(settled, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	now := time.Now().UTC().Format(time.RFC3339)
	for _, p := range settled {
		status, event, changed := bstore.SettleDecisionProjection(p, contentHash)
		if !changed {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE translations SET target_json = json_set(target_json, '$.status', ?), updated_at=?
			 WHERE project_id=? AND stream=? AND block_id=? AND locale=?`,
			status, now, projectID, p.Stream, blockID, p.Locale); err != nil {
			return fmt.Errorf("settle projection for block %s: %w", blockID, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO block_history
				(project_id, block_id, locale, change_type, origin, author, stream, created_at)
			 VALUES (?,?,?,?,'','system',?,?)`,
			projectID, blockID, p.Locale, event, p.Stream, now); err != nil {
			return fmt.Errorf("log settled projection for block %s: %w", blockID, err)
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
