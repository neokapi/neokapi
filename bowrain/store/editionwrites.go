package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
)

// The edition writes a push carries beside its decisions.
//
// A project keeps how each translation came to be in its block history: the
// source a run made it from, and who wrote it. A push sends the last write of
// each translation (venue.EditionWrite), and the venue keeps the two facts it
// grades by where it keeps the same facts about its own drafts:
//
//   - the basis, on the unit's ledger row, where nobody has decided the unit:
//     the row a run of the venue's own writes for a draft it made, so a
//     translation a run on the checkout made is graded stale against the
//     source the same way (TallyDecisionBasis). A decision's basis is the
//     decision's, and a write never replaces it.
//   - the draft mark, for a translation a tool made from a recorded source, so
//     the venue's own runs count the unit as drafted against that source.
//   - the author, on the block history row this push wrote for a translation
//     the pusher wrote by hand, which separation of duties reads
//     (LastTargetAuthors).
//
// A write about another translation than the one the venue holds describes
// nothing the venue has, and is left out.

// RecordEditionWrites implements platstore.PushApplier.
func (a pushApply) RecordEditionWrites(ctx context.Context, projectID, stream, author string, writes []venue.EditionWrite) (int, error) {
	if len(writes) == 0 {
		return 0, nil
	}
	if err := a.hold(ctx, projectID, stream); err != nil {
		return 0, err
	}
	return recordEditionWritesTx(ctx, a.tx, projectID, stream, author, a.correlation, writes)
}

// recordEditionWritesTx records writes on tx. correlation names the block
// history rows the push wrote, which a translation the pusher wrote by hand is
// attributed on.
func recordEditionWritesTx(ctx context.Context, tx Runner, projectID, stream, author, correlation string, writes []venue.EditionWrite) (int, error) {
	stream = storeutil.DefaultStream(stream)
	updated := time.Now().UTC().Format(time.RFC3339)
	var bases []venue.UnitDecision
	var drafts []platstore.DraftBasis
	recorded := 0
	for _, w := range writes {
		if w.ItemName == "" || w.Unit == "" || w.Variant == "" || w.Revision == "" {
			continue
		}
		key, err := model.ParseEditionKey(w.Variant)
		if err != nil || key.Locale == "" {
			continue
		}
		var blockID string
		switch err := tx.QueryRowContext(ctx,
			`SELECT id FROM blocks WHERE project_id=$1 AND stream=$2 AND item_name=$3 AND source_id=$4`,
			projectID, stream, w.ItemName, w.Unit).Scan(&blockID); {
		case errors.Is(err, sql.ErrNoRows):
			continue // a unit the venue does not hold
		case err != nil:
			return recorded, fmt.Errorf("resolve the block of %s/%s: %w", w.ItemName, w.Unit, err)
		}

		// The translation the venue holds, when it holds one.
		held, targetHash := false, ""
		var targetJSON string
		switch err := tx.QueryRowContext(ctx,
			`SELECT target_json FROM translations WHERE project_id=$1 AND stream=$2 AND block_id=$3 AND locale=$4`,
			projectID, stream, blockID, w.Variant).Scan(&targetJSON); {
		case errors.Is(err, sql.ErrNoRows):
		case err != nil:
			return recorded, fmt.Errorf("read the translation of %s/%s: %w", w.Unit, w.Variant, err)
		default:
			var tgt model.Target
			if uerr := json.Unmarshal([]byte(targetJSON), &tgt); uerr == nil {
				if model.RunsRevision(key.Canonical(), tgt.Runs) != w.Revision {
					continue // the write describes another translation
				}
				held, targetHash = true, state.TargetHash(model.RunsText(tgt.Runs))
			}
		}

		// The unit's record.
		var reviewState, prevTarget, prevBasis string
		haveRecord := true
		switch err := tx.QueryRowContext(ctx,
			`SELECT review_state, target_hash, content_hash FROM unit_decisions
			 WHERE project_id=$1 AND stream=$2 AND unit=$4 AND variant=$5
			   AND item_id = (SELECT id FROM items WHERE project_id=$1 AND stream=$2 AND name=$3)`,
			projectID, stream, w.ItemName, w.Unit, w.Variant).Scan(&reviewState, &prevTarget, &prevBasis); {
		case errors.Is(err, sql.ErrNoRows):
			haveRecord = false
		case err != nil:
			return recorded, fmt.Errorf("read the record of %s/%s: %w", w.Unit, w.Variant, err)
		}
		decided := haveRecord && reviewState != ""
		basis := ""
		if w.HasBasis() {
			basis = w.Basis
		}
		if !decided && (basis != "" || w.KnowsNoBasis()) &&
			(!haveRecord || prevBasis != basis || prevTarget != targetHash) {
			bases = append(bases, venue.UnitDecision{
				ItemName: w.ItemName, Unit: w.Unit, Variant: w.Variant,
				TargetHash: targetHash, ContentHash: basis,
				GoverningFingerprint: w.GoverningFingerprint, Updated: updated,
			})
		}
		if w.Produced() {
			drafts = append(drafts, platstore.DraftBasis{ItemName: w.ItemName, Unit: w.Unit, Variant: w.Variant, SourceHash: w.Basis})
		}
		if held && w.ByHand() && author != "" && correlation != "" {
			if _, err := tx.ExecContext(ctx,
				`UPDATE block_history SET author=$1
				 WHERE id = (SELECT MAX(id) FROM block_history
				   WHERE project_id=$2 AND stream=$3 AND block_id=$4 AND locale=$5
				     AND change_type IN `+targetContentChangeTypes+`
				     AND correlation_id=$6)
				   AND author=''`,
				author, projectID, stream, blockID, w.Variant, correlation); err != nil {
				return recorded, fmt.Errorf("attribute the translation of %s/%s: %w", w.Unit, w.Variant, err)
			}
		}
		recorded++
	}
	// The records first: a unit nobody had a record of gets its row here, and
	// the draft marks below land on rows and create none.
	if _, err := upsertUnitDecisionsTx(ctx, tx, projectID, stream, bases); err != nil {
		return recorded, err
	}
	if len(drafts) > 0 {
		if err := recordDraftBasesTx(ctx, tx, projectID, stream, drafts); err != nil {
			return recorded, err
		}
	}
	return recorded, nil
}
