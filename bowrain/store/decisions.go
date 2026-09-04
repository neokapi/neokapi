package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/id"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
)

// The decision ledger — the server side of core/state. A decision is a FACT
// (who, when, which rung, and the hashes of the pairing it blesses — the
// translation, and the source it was blessed for); the
// unit_decisions table folds the event log to latest-per-(item, unit, variant),
// block_history keeps the events, and target_json.status is a written
// PROJECTION of the ledger so every existing status reader keeps working.
// Freshness is derived at the point of use, never stored: a decision whose
// TargetHash no longer matches the current translation is stale, and a source
// edit demotes the projection (storeBlocks, "decision.stale").

// resolveItemIDPg is the item a decision names, by identity rather than address.
//
// Decisions arrive keyed by item NAME — that is the vocabulary of the producer
// that read the file — and are stored keyed by the item's id, so a rename moves
// the address and the approvals stay put.
//
// A name this stream does not hold yet gets a row. The ledger is allowed to
// arrive before the content it judges ("stays ledger-only and heals when the
// content arrives"), and a decision that names a file is itself the assertion
// that the file exists. Minting the row is what gives the key something stable
// to point at; without it every ungrounded decision would key on the empty
// string and collide with every other one.
func resolveItemIDPg(ctx context.Context, tx Runner, projectID, stream, itemName string) (string, error) {
	if itemName == "" {
		return "", nil
	}
	var itemID string
	err := tx.QueryRowContext(ctx,
		`SELECT id FROM items WHERE project_id=$1 AND stream=$2 AND name=$3`,
		projectID, stream, itemName).Scan(&itemID)
	if err == nil {
		return itemID, nil
	}
	if err != sql.ErrNoRows {
		return "", fmt.Errorf("resolve item %q: %w", itemName, err)
	}
	itemID = id.New()
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO items (id, project_id, stream, name, format, item_type, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,'','file',NOW(),NOW())
		 ON CONFLICT (project_id, stream, name) DO UPDATE SET name=EXCLUDED.name
		 RETURNING id`,
		itemID, projectID, stream, itemName).Scan(&itemID); err != nil {
		return "", fmt.Errorf("record item %q for its decisions: %w", itemName, err)
	}
	return itemID, nil
}

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
//
// The comparison lives in core/venue because the freshness ref's
// decisions component is folded from the same field list. Two lists would let a
// record the store skips as unchanged still move the component, and every
// subsequent push would then be refused for a change nobody made.
func DecisionUnchanged(old, next venue.UnitDecision) bool {
	return venue.SameDecision(old, next)
}

// UpsertUnitDecisions implements platstore.DecisionStore: idempotent
// last-writer-wins upsert keyed by (item, unit, variant). A record older than
// the stored one (by Updated) is skipped — both ends send full sets, so a
// stale replay must not roll a newer decision back. Each record that actually
// changes appends a block_history event and re-projects the target status,
// when the unit resolves to a stored block and the decision still blesses the
// current translation.
func (s *PostgresStore) UpsertUnitDecisions(ctx context.Context, projectID, stream string, decisions []venue.UnitDecision) (int, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin decisions tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit; the commit error is what matters

	changed, err := upsertUnitDecisionsTx(ctx, tx, projectID, stream, decisions)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit decisions: %w", err)
	}
	return changed, nil
}

// upsertUnitDecisionsTx is the work, on whatever executor the caller brings.
func upsertUnitDecisionsTx(ctx context.Context, tx Runner, projectID, stream string, decisions []venue.UnitDecision) (int, error) {
	if len(decisions) == 0 {
		return 0, nil
	}
	stream = storeutil.DefaultStream(stream)

	changed := 0
	now := time.Now().UTC()
	for _, d := range decisions {
		if d.Unit == "" || d.Variant == "" {
			continue
		}

		itemID, err := resolveItemIDPg(ctx, tx, projectID, stream, d.ItemName)
		if err != nil {
			return changed, err
		}

		var old venue.UnitDecision
		var haveOld bool
		row := tx.QueryRowContext(ctx,
			`SELECT status, target_hash, content_hash, review_state, decided_by, decided_at, note, parked, assignee, updated
			 FROM unit_decisions
			 WHERE project_id=$1 AND stream=$2 AND item_id=$3 AND unit=$4 AND variant=$5`,
			projectID, stream, itemID, d.Unit, d.Variant)
		switch err := row.Scan(&old.Status, &old.TargetHash, &old.ContentHash, &old.ReviewState, &old.DecidedBy,
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
				(project_id, stream, item_id, item_name, unit, variant, status, target_hash, content_hash, review_state,
				 decided_by, decided_at, note, parked, assignee, updated, updated_at)
			 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)
			 ON CONFLICT (project_id, stream, item_id, unit, variant) DO UPDATE SET
				item_name=EXCLUDED.item_name,
				status=EXCLUDED.status, target_hash=EXCLUDED.target_hash,
				content_hash=EXCLUDED.content_hash,
				review_state=EXCLUDED.review_state, decided_by=EXCLUDED.decided_by,
				decided_at=EXCLUDED.decided_at, note=EXCLUDED.note, parked=EXCLUDED.parked,
				assignee=EXCLUDED.assignee, updated=EXCLUDED.updated, updated_at=EXCLUDED.updated_at`,
			projectID, stream, itemID, d.ItemName, d.Unit, d.Variant, d.Status, d.TargetHash, d.ContentHash, d.ReviewState,
			d.DecidedBy, d.DecidedAt, d.Note, d.Parked, d.Assignee, d.Updated, now); err != nil {
			return changed, fmt.Errorf("upsert decision %s/%s: %w", d.Unit, d.Variant, err)
		}
		changed++

		// Resolve the stored block the unit names. A decision for content this
		// store has never held stays ledger-only and heals when the content
		// arrives.
		var blockID, blockHash string
		if d.ItemName != "" {
			err := tx.QueryRowContext(ctx,
				`SELECT id, content_hash FROM blocks WHERE project_id=$1 AND stream=$2 AND item_name=$3 AND source_id=$4`,
				projectID, stream, d.ItemName, d.Unit).Scan(&blockID, &blockHash)
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

		// Project the status — only while BOTH halves of the pairing the
		// decision blessed still hold: the translation the row currently
		// carries, and the source it was blessed for. A decision arriving
		// against source this store has since rewritten is recorded but moves
		// nothing; projecting it would stamp an approval onto wording nobody
		// approved. An empty basis is unknown, not stale, and projects as before.
		if d.Status == "" {
			continue
		}
		if d.ContentHash != "" && blockHash != "" && d.ContentHash != blockHash {
			continue
		}
		var targetJSON string
		err = tx.QueryRowContext(ctx,
			`SELECT target_json FROM translations WHERE project_id=$1 AND stream=$2 AND block_id=$3 AND locale=$4`,
			projectID, stream, blockID, d.Variant).Scan(&targetJSON)
		if errors.Is(err, sql.ErrNoRows) {
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

	return changed, nil
}

// ListUnitDecisions implements platstore.DecisionStore.
func (s *PostgresStore) ListUnitDecisions(ctx context.Context, projectID, stream string) ([]venue.UnitDecision, error) {
	return listUnitDecisionsTx(ctx, s.db, projectID, stream)
}

// listUnitDecisionsTx is the work, on whatever executor the caller brings. A
// push asserts its expected ledger ref on the transaction it is about to write
// through, so the assertion is a compare-and-swap rather than a look and a hope.
func listUnitDecisionsTx(ctx context.Context, tx Querier, projectID, stream string) ([]venue.UnitDecision, error) {
	stream = storeutil.DefaultStream(stream)
	rows, err := tx.QueryContext(ctx,
		`SELECT item_name, unit, variant, status, target_hash, content_hash, review_state,
			decided_by, decided_at, note, parked, assignee, updated
		 FROM unit_decisions WHERE project_id=$1 AND stream=$2
		 ORDER BY item_name, unit, variant`,
		projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("list decisions: %w", err)
	}
	defer rows.Close()

	var out []venue.UnitDecision
	for rows.Next() {
		d := venue.UnitDecision{ProjectID: projectID, Stream: stream}
		if err := rows.Scan(&d.ItemName, &d.Unit, &d.Variant, &d.Status, &d.TargetHash, &d.ContentHash,
			&d.ReviewState, &d.DecidedBy, &d.DecidedAt, &d.Note, &d.Parked, &d.Assignee, &d.Updated); err != nil {
			return nil, fmt.Errorf("scan decision: %w", err)
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// GetUnitDecision implements platstore.UnitDecisionReader.
func (s *PostgresStore) GetUnitDecision(ctx context.Context, projectID, stream, itemName, unit, variant string) (*venue.UnitDecision, error) {
	stream = storeutil.DefaultStream(stream)
	d := venue.UnitDecision{ProjectID: projectID, Stream: stream}
	err := s.db.QueryRowContext(ctx,
		`SELECT item_name, unit, variant, status, target_hash, content_hash, review_state,
			decided_by, decided_at, note, parked, assignee, updated
		 FROM unit_decisions
		 WHERE project_id=$1 AND stream=$2 AND item_name=$3 AND unit=$4 AND variant=$5`,
		projectID, stream, itemName, unit, variant).
		Scan(&d.ItemName, &d.Unit, &d.Variant, &d.Status, &d.TargetHash, &d.ContentHash,
			&d.ReviewState, &d.DecidedBy, &d.DecidedAt, &d.Note, &d.Parked, &d.Assignee, &d.Updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get decision: %w", err)
	}
	return &d, nil
}

// TallyDecisionBasis implements platstore.DecisionStore. The basis a decision
// blessed and the block's current source hash are the same value, so grading is
// a plain equality join on (project, item, unit → source_id); a decision whose
// unit resolves to no stored block is not graded at all, because there is
// nothing to grade it against. Only translatable blocks are counted: the
// dashboard's denominators exclude the rest, and a stale count outside the
// denominator would withhold a scope for content nobody ships.
//
// Owed is the same grading with the draft basis read beside the decision: a
// stale decision whose row records no draft against the current source, on a
// unit that carries a target for the variant. The target condition keeps the
// count inside the translated denominator it is subtracted from; a stale
// decision on a unit with no target is pending as untranslated already.
func (s *PostgresStore) TallyDecisionBasis(ctx context.Context, projectID, stream string) ([]platstore.DecisionBasisTally, error) {
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
		 WHERE d.project_id=$1 AND d.stream=$2 AND b.translatable
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

// RecordDraftBases implements platstore.DecisionStore. One UPDATE per unit,
// keyed on the item the unit belongs to by identity rather than by name, so a
// stamp lands on the right row across a rename. A unit with no ledger row
// matches nothing and is left without one: a row carrying only a draft basis
// would read as a decision with an unknown basis.
func (s *PostgresStore) RecordDraftBases(ctx context.Context, projectID, stream string, drafts []platstore.DraftBasis) error {
	if len(drafts) == 0 {
		return nil
	}
	stream = storeutil.DefaultStream(stream)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin draft basis tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op after Commit; the commit error is what matters

	now := time.Now().UTC()
	for _, d := range drafts {
		if d.ItemName == "" || d.Unit == "" || d.Variant == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE unit_decisions SET draft_basis=$6, updated_at=$7
			 WHERE project_id=$1 AND stream=$2 AND unit=$4 AND variant=$5
			   AND item_id = (SELECT id FROM items WHERE project_id=$1 AND stream=$2 AND name=$3)`,
			projectID, stream, d.ItemName, d.Unit, d.Variant, d.SourceHash, now); err != nil {
			return fmt.Errorf("record draft basis %s/%s: %w", d.Unit, d.Variant, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit draft bases: %w", err)
	}
	return nil
}

// ListDraftBases implements platstore.DecisionStore.
func (s *PostgresStore) ListDraftBases(ctx context.Context, projectID, stream string) ([]platstore.DraftBasis, error) {
	stream = storeutil.DefaultStream(stream)
	rows, err := s.db.QueryContext(ctx,
		`SELECT item_name, unit, variant, draft_basis
		 FROM unit_decisions WHERE project_id=$1 AND stream=$2 AND draft_basis <> ''
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

// The block_history change types a re-derived projection files. Both are
// written by "system": nobody reviewed anything, the source moved under a
// decision that was already recorded, and the flip is logged so it is auditable
// rather than silent.
const (
	// DecisionStaleEvent — the source moved away from the wording a decision
	// blessed, so the projection dropped to the presence baseline.
	DecisionStaleEvent = "decision.stale"
	// DecisionRestoredEvent — the source moved back to wording a recorded
	// decision blesses, so that decision applies again.
	DecisionRestoredEvent = "decision.restored"
)

// The edit_reason each event carries, in the log the audit trail reads.
const (
	decisionStaleReason    = "source content changed"
	decisionRestoredReason = "source content restored"
)

// DecisionProjection is one target row of a block whose SOURCE content just
// changed, paired with the decision recorded for that unit and locale.
type DecisionProjection struct {
	Stream string
	Locale string
	// Status is the rung the row currently projects.
	Status string
	// TargetText is the translation the row holds now — the target half of the
	// pairing a decision blessed.
	TargetText string
	// DecisionStatus, DecisionBasis and DecisionTargetHash are the recorded
	// decision for (unit, locale); all empty when the ledger holds none.
	DecisionStatus     string
	DecisionBasis      string
	DecisionTargetHash string
}

// SettleDecisionProjection reports the status a target row must project once
// its block's source has moved to contentHash, and the history event that
// records the move. Shared by both backends so they cannot disagree about when
// an approval applies.
//
// A decision is about a PAIRING — this translation, of this source — and it is
// a fact that is never rewritten. So the projection is re-derived rather than
// only demoted: when the source moves away from the wording a decision blessed,
// the projection drops to the presence baseline; when it moves BACK to that
// wording with the translation still intact, the same decision applies again
// and the unit needs no second review. A decision with no recorded basis grades
// nothing — unknown is not a match — and leaves a row that is not an approval
// exactly where it stands.
func SettleDecisionProjection(p DecisionProjection, contentHash string) (status, event string, changed bool) {
	if p.DecisionStatus != "" && p.DecisionBasis != "" && p.DecisionBasis == contentHash &&
		(p.DecisionTargetHash == "" || state.TargetHash(p.TargetText) == p.DecisionTargetHash) {
		if p.DecisionStatus == p.Status {
			return p.Status, "", false
		}
		return p.DecisionStatus, DecisionRestoredEvent, true
	}
	if p.Status == string(model.TargetStatusReviewed) || p.Status == string(model.TargetStatusSignedOff) {
		return string(model.TargetStatusTranslated), DecisionStaleEvent, true
	}
	return p.Status, "", false
}

// DecisionEventReason names the edit_reason a settled projection files.
func DecisionEventReason(event string) string {
	if event == DecisionRestoredEvent {
		return decisionRestoredReason
	}
	return decisionStaleReason
}

// TargetTextFromJSON reads the plain target text out of a stored target_json
// payload. An unreadable payload yields "", which grades as a target that
// cannot match any recorded hash.
func TargetTextFromJSON(targetJSON string) string {
	var tgt model.Target
	if err := json.Unmarshal([]byte(targetJSON), &tgt); err != nil {
		return ""
	}
	return model.RunsText(tgt.Runs)
}

// settleDecisionProjectionsPg re-derives the projected statuses of every target
// of a block whose SOURCE content just changed, against the decision ledger.
// Runs inside the storeBlocks transaction, scoped to the stream whose source
// moved, and only when the content hash actually moved. A branch holding the
// same block id at its own content is judged by its own ledger.
func settleDecisionProjectionsPg(ctx context.Context, tx Runner, projectID, stream, blockID, contentHash string) error {
	// The ledger keys on the unit identity (item + source_id), not the block id.
	var itemID, unit string
	if err := tx.QueryRowContext(ctx,
		`SELECT item_id, source_id FROM blocks WHERE project_id=$1 AND stream=$2 AND id=$3`,
		projectID, stream, blockID).Scan(&itemID, &unit); err != nil {
		return fmt.Errorf("look up unit for block %s: %w", blockID, err)
	}

	rows, err := tx.QueryContext(ctx,
		`SELECT t.stream, t.locale, COALESCE(t.target_json->>'status',''), t.target_json,
			COALESCE(d.status,''), COALESCE(d.content_hash,''), COALESCE(d.target_hash,'')
		 FROM translations t
		 LEFT JOIN unit_decisions d
			ON d.project_id=t.project_id AND d.stream=t.stream
			AND d.item_id=$4 AND d.unit=$5 AND d.variant=t.locale
		 WHERE t.project_id=$1 AND t.stream=$2 AND t.block_id=$3`,
		projectID, stream, blockID, itemID, unit)
	if err != nil {
		return fmt.Errorf("read projections for block %s: %w", blockID, err)
	}
	var settled []DecisionProjection
	for rows.Next() {
		var p DecisionProjection
		var targetJSON string
		if err := rows.Scan(&p.Stream, &p.Locale, &p.Status, &targetJSON,
			&p.DecisionStatus, &p.DecisionBasis, &p.DecisionTargetHash); err != nil {
			rows.Close()
			return fmt.Errorf("scan projection for block %s: %w", blockID, err)
		}
		p.TargetText = TargetTextFromJSON(targetJSON)
		settled = append(settled, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	now := time.Now().UTC()
	for _, p := range settled {
		status, event, changed := SettleDecisionProjection(p, contentHash)
		if !changed {
			continue
		}
		if _, err := tx.ExecContext(ctx,
			`UPDATE translations SET target_json = jsonb_set(target_json, '{status}', to_jsonb($5::text)), updated_at=$6
			 WHERE project_id=$1 AND stream=$2 AND block_id=$3 AND locale=$4`,
			projectID, p.Stream, blockID, p.Locale, status, now); err != nil {
			return fmt.Errorf("settle projection for block %s: %w", blockID, err)
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO block_history
				(project_id, block_id, locale, change_type, origin, author, actor_role, edit_reason, stream, created_at)
			 VALUES ($1,$2,$3,$4,'','system','system',$5,$6,$7)`,
			projectID, blockID, p.Locale, event, DecisionEventReason(event), p.Stream, now); err != nil {
			return fmt.Errorf("log settled projection for block %s: %w", blockID, err)
		}
	}
	return nil
}
