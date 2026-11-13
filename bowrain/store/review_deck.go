package store

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
)

var _ platstore.ReviewDeckStore = (*PostgresStore)(nil)

// reviewDeckFrom is the pending-review queue of one language (the predicate
// ListPendingReview applies) with the rows a deck's facts are computed from
// beside each translation: its pre-review, its ledger row and its facts.
//
// $1 project, $2 stream, $3 locale, $4 policy; a collection scope binds $5.
const reviewDeckFrom = ` FROM blocks b JOIN translations t
	ON t.project_id = b.project_id AND t.block_id = b.id AND t.stream = $2
	LEFT JOIN items i
	ON i.project_id = b.project_id AND i.stream = $2 AND i.name = b.item_name
	LEFT JOIN pre_reviews pr
	ON pr.project_id = b.project_id AND pr.stream = $2 AND pr.block_id = b.id AND pr.locale = t.locale
	LEFT JOIN unit_decisions d
	ON b.source_id <> '' AND d.project_id = b.project_id AND d.stream = $2
		AND d.item_name = b.item_name AND d.unit = b.source_id AND d.variant = t.locale
	LEFT JOIN review_deck_facts f
	ON f.project_id = b.project_id AND f.stream = $2 AND f.block_id = b.id AND f.locale = t.locale
	WHERE b.project_id = $1 AND b.stream = $2 AND b.translatable AND t.text <> ''
	AND COALESCE(t.target_json->>'status', '') <> 'established' AND t.locale = $3`

// reviewDeckFresh holds when a translation's facts were computed from the
// rows as they stand, under the policy asked for, within the last day.
const reviewDeckFresh = `(f.block_id IS NOT NULL
	AND f.target_updated_at = t.updated_at AND f.source_revision = b.source_revision
	AND f.policy = $4
	AND f.prereview_at IS NOT DISTINCT FROM pr.created_at
	AND f.decision_at IS NOT DISTINCT FROM d.updated_at
	AND f.computed_at > NOW() - INTERVAL '1 day')`

func reviewDeckArgs(q platstore.ReviewDeckQuery) (scope string, args []any) {
	args = []any{q.ProjectID, storeutil.DefaultStream(q.Stream), q.Locale, q.Policy}
	if q.CollectionID != nil {
		scope = ` AND COALESCE(i.collection_id, '') = $5`
		args = append(args, *q.CollectionID)
	}
	return scope, args
}

// UnclassifiedReview returns the pending translations of the deck whose facts
// are missing or no longer hold. See platstore.ReviewDeckStore.
func (s *PostgresStore) UnclassifiedReview(ctx context.Context, q platstore.ReviewDeckQuery, limit int) ([]platstore.ReviewDeckEntry, error) {
	scope, args := reviewDeckArgs(q)
	//nolint:gosec // the skeleton is fixed literals and $N tokens; every value binds through args
	query := `SELECT b.id, b.item_name, t.locale, COALESCE(i.collection_id, ''),
		t.updated_at, b.source_revision, pr.created_at, d.updated_at,
		COALESCE(d.review_state, ''), COALESCE(d.basis, ''), COALESCE(d.revision, '')` +
		reviewDeckFrom + scope + ` AND NOT ` + reviewDeckFresh +
		` ORDER BY ` + BlockListOrder + fmt.Sprintf(` LIMIT $%d`, len(args)+1)
	rows, err := s.db.DB.QueryContext(ctx, query, append(args, limit)...)
	if err != nil {
		return nil, fmt.Errorf("list unclassified review: %w", err)
	}
	defer rows.Close()
	var out []platstore.ReviewDeckEntry
	for rows.Next() {
		var e platstore.ReviewDeckEntry
		var pr, d sql.NullTime
		if err := rows.Scan(&e.BlockID, &e.ItemName, &e.Locale, &e.CollectionID,
			&e.Basis.TargetUpdatedAt, &e.Basis.SourceRevision, &pr, &d,
			&e.DecisionState, &e.DecisionBasis, &e.DecisionRevision); err != nil {
			return nil, fmt.Errorf("scan unclassified review: %w", err)
		}
		e.Basis.PreReviewAt = nullTimePtr(pr)
		e.Basis.DecisionAt = nullTimePtr(d)
		out = append(out, e)
	}
	return out, rows.Err()
}

func nullTimePtr(t sql.NullTime) *time.Time {
	if !t.Valid {
		return nil
	}
	v := t.Time
	return &v
}

// RecordReviewDeckFacts stores the facts of translations. See
// platstore.ReviewDeckStore.
func (s *PostgresStore) RecordReviewDeckFacts(ctx context.Context, projectID, stream string, facts []platstore.ReviewDeckFacts) error {
	if len(facts) == 0 {
		return nil
	}
	stream = storeutil.DefaultStream(stream)
	tx, err := s.db.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("record review deck facts: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	stmt, err := tx.PrepareContext(ctx, `INSERT INTO review_deck_facts (project_id, stream, block_id, locale,
		target_updated_at, source_revision, prereview_at, decision_at, policy,
		group_rank, deck_group, reason, findings, failing, term_compliance, voice_score, voice_bar, computed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NOW())
		ON CONFLICT (project_id, stream, block_id, locale) DO UPDATE SET
			target_updated_at = EXCLUDED.target_updated_at, source_revision = EXCLUDED.source_revision,
			prereview_at = EXCLUDED.prereview_at, decision_at = EXCLUDED.decision_at, policy = EXCLUDED.policy,
			group_rank = EXCLUDED.group_rank, deck_group = EXCLUDED.deck_group, reason = EXCLUDED.reason,
			findings = EXCLUDED.findings, failing = EXCLUDED.failing, term_compliance = EXCLUDED.term_compliance,
			voice_score = EXCLUDED.voice_score, voice_bar = EXCLUDED.voice_bar, computed_at = NOW()`)
	if err != nil {
		return fmt.Errorf("record review deck facts: %w", err)
	}
	defer stmt.Close()
	for _, f := range facts {
		if _, err := stmt.ExecContext(ctx, projectID, stream, f.BlockID, f.Locale,
			f.Basis.TargetUpdatedAt, f.Basis.SourceRevision, f.Basis.PreReviewAt, f.Basis.DecisionAt, f.Policy,
			f.GroupRank, f.Group, f.Reason, f.Findings, f.Failing, string(f.TermCompliance), f.VoiceScore, f.VoiceBar); err != nil {
			return fmt.Errorf("record review deck facts of block %s: %w", f.BlockID, err)
		}
	}
	return tx.Commit()
}

// ListReviewDeck returns a page of the deck's classified translations. See
// platstore.ReviewDeckStore.
func (s *PostgresStore) ListReviewDeck(ctx context.Context, q platstore.ReviewDeckQuery, groupRank int, after *platstore.ReviewDeckKey, limit int) ([]platstore.ReviewDeckRow, error) {
	scope, args := reviewDeckArgs(q)
	if groupRank >= 0 {
		args = append(args, groupRank)
		scope += fmt.Sprintf(` AND f.group_rank = $%d`, len(args))
	} else {
		scope += ` AND f.group_rank >= 0`
	}
	if after != nil {
		n := len(args)
		args = append(args, after.GroupRank, after.ItemName, after.Position, after.BlockID)
		scope += fmt.Sprintf(` AND (f.group_rank, b.item_name, b.position, b.id) > ($%d, $%d, $%d, $%d)`, n+1, n+2, n+3, n+4)
	}
	args = append(args, limit)
	//nolint:gosec // the skeleton is fixed literals and $N tokens; every value binds through args
	query := `SELECT b.id, b.item_name, t.locale, COALESCE(i.collection_id, ''), b.position,
		t.updated_at, b.source_revision, f.policy, f.group_rank, f.deck_group, f.reason,
		f.findings, f.failing, f.term_compliance, f.voice_score, f.voice_bar` +
		reviewDeckFrom + scope + ` AND ` + reviewDeckFresh +
		fmt.Sprintf(` ORDER BY f.group_rank, b.item_name, b.position, b.id LIMIT $%d`, len(args))
	rows, err := s.db.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list review deck: %w", err)
	}
	defer rows.Close()
	var out []platstore.ReviewDeckRow
	for rows.Next() {
		var r platstore.ReviewDeckRow
		var term string
		var voice, bar sql.NullInt64
		if err := rows.Scan(&r.BlockID, &r.ItemName, &r.Locale, &r.CollectionID, &r.Position,
			&r.Facts.Basis.TargetUpdatedAt, &r.Facts.Basis.SourceRevision, &r.Facts.Policy,
			&r.Facts.GroupRank, &r.Facts.Group, &r.Facts.Reason, &r.Facts.Findings, &r.Facts.Failing,
			&term, &voice, &bar); err != nil {
			return nil, fmt.Errorf("scan review deck: %w", err)
		}
		r.Facts.BlockID, r.Facts.Locale = r.BlockID, r.Locale
		r.Facts.TermCompliance = platstore.TermCompliance(term)
		r.Facts.VoiceScore = nullIntPtr(voice)
		r.Facts.VoiceBar = nullIntPtr(bar)
		out = append(out, r)
	}
	return out, rows.Err()
}

func nullIntPtr(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

// CountReviewDeck counts the deck by group, and what is not classified. See
// platstore.ReviewDeckStore.
func (s *PostgresStore) CountReviewDeck(ctx context.Context, q platstore.ReviewDeckQuery) (platstore.ReviewDeckCounts, error) {
	scope, args := reviewDeckArgs(q)
	out := platstore.ReviewDeckCounts{ByRank: map[int]int{}}
	//nolint:gosec // the skeleton is fixed literals and $N tokens; every value binds through args
	// A fresh row ranked below 0 is set aside and is not in the deck.
	query := `SELECT CASE WHEN ` + reviewDeckFresh + ` THEN f.group_rank ELSE -1 END AS rank, count(*)` +
		reviewDeckFrom + scope + ` AND NOT (` + reviewDeckFresh + ` AND f.group_rank < 0) GROUP BY rank`
	rows, err := s.db.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return out, fmt.Errorf("count review deck: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var rank, n int
		if err := rows.Scan(&rank, &n); err != nil {
			return out, fmt.Errorf("scan review deck count: %w", err)
		}
		if rank < 0 {
			out.Unclassified += n
			continue
		}
		out.ByRank[rank] += n
	}
	return out, rows.Err()
}
