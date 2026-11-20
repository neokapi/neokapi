package store

import (
	"context"
	"database/sql"
	"errors"
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
// The language's waiting translations are read first, on their own, and the
// blocks joined to them by key. Read the other way round, the planner walks
// every block of the project and probes each one's translations in every
// language, which costs a large project hundreds of milliseconds per query.
//
// $1 project, $2 stream, $3 the languages (an array), $4 policy; a collection
// scope binds $5.
const reviewDeckFrom = ` FROM (SELECT t.* FROM translations t
		WHERE t.project_id = $1 AND t.stream = $2 AND t.locale = ANY($3) AND t.text <> ''` + reviewDeckPending + `
		OFFSET 0) t
	JOIN blocks b
	ON b.project_id = t.project_id AND b.stream = $2 AND b.id = t.block_id` +
	reviewDeckJoins + reviewDeckWhere + reviewDeckPending

// reviewDeckPending keeps, of reviewDeckTranslations, those waiting for
// review: an established translation has been approved.
const reviewDeckPending = ` AND COALESCE(t.target_json->>'status', '') <> 'established'`

// reviewDeckTranslations is every translation of one language, reviewed or
// not, joined as reviewDeckFrom joins it.
const reviewDeckTranslations = ` FROM blocks b JOIN translations t
	ON t.project_id = b.project_id AND t.block_id = b.id AND t.stream = $2` +
	reviewDeckJoins + reviewDeckWhere

// reviewDeckJoins are the rows beside each translation of a deck query.
const reviewDeckJoins = `
	LEFT JOIN items i
	ON i.project_id = b.project_id AND i.stream = $2 AND i.name = b.item_name
	LEFT JOIN pre_reviews pr
	ON pr.project_id = b.project_id AND pr.stream = $2 AND pr.block_id = b.id AND pr.locale = t.locale
	LEFT JOIN unit_decisions d
	ON b.source_id <> '' AND d.project_id = b.project_id AND d.stream = $2
		AND d.item_name = b.item_name AND d.unit = b.source_id AND d.variant = t.locale
	LEFT JOIN review_deck_facts f
	ON f.project_id = b.project_id AND f.stream = $2 AND f.block_id = b.id AND f.locale = t.locale`

// reviewDeckWhere keeps the translatable blocks of the project and the
// non-empty translations of the languages.
const reviewDeckWhere = `
	WHERE b.project_id = $1 AND b.stream = $2 AND b.translatable AND t.text <> ''
	AND t.locale = ANY($3)`

// reviewDeckFresh holds when a translation's facts were computed from the
// rows as they stand, under the policy asked for, within the last day.
const reviewDeckFresh = `(f.block_id IS NOT NULL
	AND f.target_updated_at = t.updated_at AND f.source_revision = b.source_revision
	AND f.policy = $4
	AND f.prereview_at IS NOT DISTINCT FROM pr.created_at
	AND f.decision_at IS NOT DISTINCT FROM d.updated_at
	AND f.computed_at > NOW() - INTERVAL '1 day')`

// reviewDeckSame names the translations a reviewer decides once: the same
// source, codes included (its revision), and the same translation. A block
// stored before it had a source revision stands alone.
const reviewDeckSame = `(CASE WHEN b.source_revision = '' THEN b.id ELSE b.source_revision END), t.text`

// reviewDeckPriority is the SQL rank of a row's collection tier, 0 high,
// 1 normal, 2 low, binding the tiers' collection ids through args.
func reviewDeckPriority(q platstore.ReviewDeckQuery, args []any) (string, []any) {
	expr := "CASE"
	if len(q.HighCollections) > 0 {
		args = append(args, q.HighCollections)
		expr += fmt.Sprintf(` WHEN COALESCE(i.collection_id, '') = ANY($%d) THEN 0`, len(args))
	}
	if len(q.LowCollections) > 0 {
		args = append(args, q.LowCollections)
		expr += fmt.Sprintf(` WHEN COALESCE(i.collection_id, '') = ANY($%d) THEN 2`, len(args))
	}
	// An expression rather than the bare literal: ORDER BY 1 names the first
	// column.
	if expr == "CASE" {
		return "CAST(1 AS integer)", args
	}
	return expr + " ELSE 1 END", args
}

// reviewDeckArgs binds a deck query's scope for its one language.
func reviewDeckArgs(q platstore.ReviewDeckQuery) (scope string, args []any) {
	return reviewDeckLocalesArgs(q, []string{q.Locale})
}

// reviewDeckLocalesArgs binds a deck query's scope for several languages at
// once, in place of q.Locale.
func reviewDeckLocalesArgs(q platstore.ReviewDeckQuery, locales []string) (scope string, args []any) {
	args = []any{q.ProjectID, storeutil.DefaultStream(q.Stream), locales, q.Policy}
	if q.CollectionID != nil {
		scope = ` AND COALESCE(i.collection_id, '') = $5`
		args = append(args, *q.CollectionID)
	}
	if len(q.ExcludeCollections) > 0 {
		args = append(args, q.ExcludeCollections)
		scope += fmt.Sprintf(` AND COALESCE(i.collection_id, '') <> ALL($%d)`, len(args))
	}
	if q.Rule != "" {
		args = append(args, q.Rule)
		scope += fmt.Sprintf(` AND f.rules @> jsonb_build_array(jsonb_build_object('key', $%d::text))`, len(args))
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
		group_rank, deck_group, reason, findings, failing, term_compliance, voice_score, voice_bar, rules, computed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18::jsonb, NOW())
		ON CONFLICT (project_id, stream, block_id, locale) DO UPDATE SET
			target_updated_at = EXCLUDED.target_updated_at, source_revision = EXCLUDED.source_revision,
			prereview_at = EXCLUDED.prereview_at, decision_at = EXCLUDED.decision_at, policy = EXCLUDED.policy,
			group_rank = EXCLUDED.group_rank, deck_group = EXCLUDED.deck_group, reason = EXCLUDED.reason,
			findings = EXCLUDED.findings, failing = EXCLUDED.failing, term_compliance = EXCLUDED.term_compliance,
			voice_score = EXCLUDED.voice_score, voice_bar = EXCLUDED.voice_bar, rules = EXCLUDED.rules,
			computed_at = NOW()`)
	if err != nil {
		return fmt.Errorf("record review deck facts: %w", err)
	}
	defer stmt.Close()
	for _, f := range facts {
		if _, err := stmt.ExecContext(ctx, projectID, stream, f.BlockID, f.Locale,
			f.Basis.TargetUpdatedAt, f.Basis.SourceRevision, f.Basis.PreReviewAt, f.Basis.DecisionAt, f.Policy,
			f.GroupRank, f.Group, f.Reason, f.Findings, f.Failing, string(f.TermCompliance), f.VoiceScore, f.VoiceBar,
			platstore.EncodeRuleTallies(f.Rules)); err != nil {
			return fmt.Errorf("record review deck facts of block %s: %w", f.BlockID, err)
		}
	}
	return tx.Commit()
}

// ListReviewDeck returns a page of the deck's classified translations. See
// platstore.ReviewDeckStore.
func (s *PostgresStore) ListReviewDeck(ctx context.Context, q platstore.ReviewDeckQuery, groupRank int, after *platstore.ReviewDeckKey, limit int) ([]platstore.ReviewDeckRow, error) {
	scope, args := reviewDeckArgs(q)
	prio, args := reviewDeckPriority(q, args)
	// The inner query ranks each set of copies over the whole deck, so a set
	// is dealt once, at its first card; the outer one pages that.
	var outer string
	if groupRank >= 0 {
		args = append(args, groupRank)
		outer += fmt.Sprintf(` AND x.group_rank = $%d`, len(args))
	}
	if after != nil {
		n := len(args)
		args = append(args, after.Priority, after.GroupRank, after.ItemName, after.Position, after.BlockID)
		outer += fmt.Sprintf(` AND (x.prio, x.group_rank, x.item_name, x.position, x.id) > ($%d, $%d, $%d, $%d, $%d)`, n+1, n+2, n+3, n+4, n+5)
	}
	args = append(args, limit)
	order := prio + `, f.group_rank, b.item_name, b.position, b.id`
	//nolint:gosec // the skeleton is fixed literals and $N tokens; every value binds through args
	query := `SELECT x.id, x.item_name, x.locale, x.collection_id, x.position,
		x.updated_at, x.source_revision, x.policy, x.group_rank, x.deck_group, x.reason,
		x.findings, x.failing, x.term_compliance, x.voice_score, x.voice_bar, x.prio
		FROM (SELECT b.id, b.item_name, t.locale, COALESCE(i.collection_id, '') AS collection_id, b.position,
			t.updated_at, b.source_revision, f.policy, f.group_rank, f.deck_group, f.reason,
			f.findings, f.failing, f.term_compliance, f.voice_score, f.voice_bar, ` + prio + ` AS prio,
			row_number() OVER (PARTITION BY ` + reviewDeckSame + ` ORDER BY ` + order + `) AS copy` +
		reviewDeckFrom + scope + ` AND ` + reviewDeckFresh + ` AND f.group_rank >= 0) x
		WHERE x.copy = 1` + outer +
		fmt.Sprintf(` ORDER BY x.prio, x.group_rank, x.item_name, x.position, x.id LIMIT $%d`, len(args))
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
			&term, &voice, &bar, &r.Priority); err != nil {
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

// ReviewDeckCopies returns the other pending translations with the same
// source and translation as each block. See platstore.ReviewDeckStore.
func (s *PostgresStore) ReviewDeckCopies(ctx context.Context, q platstore.ReviewDeckQuery, blockIDs []string, limit int) (map[string][]platstore.PendingReviewRef, error) {
	out := map[string][]platstore.PendingReviewRef{}
	if len(blockIDs) == 0 || limit <= 0 {
		return out, nil
	}
	args := []any{q.ProjectID, storeutil.DefaultStream(q.Stream), q.Locale, blockIDs}
	var scope string
	if len(q.ExcludeCollections) > 0 {
		args = append(args, q.ExcludeCollections)
		scope = fmt.Sprintf(` AND COALESCE(i.collection_id, '') <> ALL($%d)`, len(args))
	}
	args = append(args, limit)
	//nolint:gosec // the skeleton is fixed literals and $N tokens; every value binds through args
	query := `SELECT r.id, b.id, b.item_name, COALESCE(i.collection_id, '')
		FROM blocks r
		JOIN translations rt ON rt.project_id = r.project_id AND rt.stream = $2 AND rt.block_id = r.id AND rt.locale = $3
		JOIN blocks b ON b.project_id = r.project_id AND b.stream = $2 AND b.source_revision = r.source_revision
			AND b.id <> r.id AND b.translatable
		JOIN translations t ON t.project_id = b.project_id AND t.stream = $2 AND t.block_id = b.id AND t.locale = $3
			AND t.text = rt.text AND COALESCE(t.target_json->>'status', '') <> 'established'
		LEFT JOIN items i ON i.project_id = b.project_id AND i.stream = $2 AND i.name = b.item_name
		WHERE r.project_id = $1 AND r.stream = $2 AND r.id = ANY($4) AND r.source_revision <> '' AND rt.text <> ''` +
		scope + fmt.Sprintf(` ORDER BY r.id, b.item_name, b.position, b.id LIMIT $%d`, len(args))
	rows, err := s.db.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list review deck copies: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var of string
		ref := platstore.PendingReviewRef{Locale: q.Locale}
		if err := rows.Scan(&of, &ref.BlockID, &ref.ItemName, &ref.CollectionID); err != nil {
			return nil, fmt.Errorf("scan review deck copies: %w", err)
		}
		out[of] = append(out[of], ref)
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
	out, err := s.CountReviewDeckByLocale(ctx, q, []string{q.Locale})
	if err != nil {
		return platstore.ReviewDeckCounts{ByRank: map[int]int{}}, err
	}
	return out[q.Locale], nil
}

// CountReviewDeckByLocale counts the deck of each language as CountReviewDeck
// does, in one query. See platstore.ReviewDeckStore.
func (s *PostgresStore) CountReviewDeckByLocale(ctx context.Context, q platstore.ReviewDeckQuery, locales []string) (map[string]platstore.ReviewDeckCounts, error) {
	scope, args := reviewDeckLocalesArgs(q, locales)
	prio, args := reviewDeckPriority(q, args)
	out := make(map[string]platstore.ReviewDeckCounts, len(locales))
	for _, loc := range locales {
		out[loc] = platstore.ReviewDeckCounts{ByRank: map[int]int{}}
	}
	// A set of copies counts once, at the card ListReviewDeck deals for it; a
	// fresh row ranked below 0 is set aside and is not in the deck.
	rank := `CASE WHEN ` + reviewDeckFresh + ` THEN f.group_rank ELSE -1 END`
	//nolint:gosec // the skeleton is fixed literals and $N tokens; every value binds through args
	query := `SELECT locale, rank, count(*) FROM (SELECT DISTINCT ON (t.locale, ` + reviewDeckSame + `) t.locale, ` + rank + ` AS rank` +
		reviewDeckFrom + scope + ` AND NOT (` + reviewDeckFresh + ` AND f.group_rank < 0)
		ORDER BY t.locale, ` + reviewDeckSame + `, (` + rank + ` < 0), ` + prio + `, ` + rank + `) x GROUP BY locale, rank`
	rows, err := s.db.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return out, fmt.Errorf("count review deck: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var loc string
		var rank, n int
		if err := rows.Scan(&loc, &rank, &n); err != nil {
			return out, fmt.Errorf("scan review deck count: %w", err)
		}
		c, ok := out[loc]
		if !ok {
			continue
		}
		if rank < 0 {
			c.Unclassified += n
		} else {
			c.ByRank[rank] += n
		}
		out[loc] = c
	}
	return out, rows.Err()
}

// CountReviewDeckRules counts the findings of each rule over one group of the
// deck. See platstore.ReviewDeckStore.
func (s *PostgresStore) CountReviewDeckRules(ctx context.Context, q platstore.ReviewDeckQuery, groupRank, limit int) ([]platstore.RuleTally, error) {
	out, err := s.CountReviewDeckRulesByLocale(ctx, q, []string{q.Locale}, groupRank, limit)
	if err != nil {
		return nil, err
	}
	if rules := out[q.Locale]; rules != nil {
		return rules, nil
	}
	return []platstore.RuleTally{}, nil
}

// CountReviewDeckRulesByLocale counts the findings of each rule over one
// group of each language's deck, as CountReviewDeckRules does, in one query.
// See platstore.ReviewDeckStore.
func (s *PostgresStore) CountReviewDeckRulesByLocale(ctx context.Context, q platstore.ReviewDeckQuery, locales []string, groupRank, limit int) (map[string][]platstore.RuleTally, error) {
	scope, args := reviewDeckLocalesArgs(q, locales)
	prio, args := reviewDeckPriority(q, args)
	args = append(args, groupRank)
	rank := len(args)
	args = append(args, limit)
	// A set of copies counts once, at its first card in deck order; each of
	// its rules then counts what that translation's facts hold. Each language
	// keeps its limit rules with the most findings.
	//nolint:gosec // the skeleton is fixed literals and $N tokens; every value binds through args
	query := `SELECT locale, key, rule, subject, message, concept_id, fails, n FROM (
		SELECT x.locale, r->>'key' AS key, MIN(COALESCE(r->>'rule', '')) AS rule,
		MIN(COALESCE(r->>'subject', '')) AS subject, MIN(COALESCE(r->>'message', '')) AS message,
		MAX(COALESCE(r->>'concept_id', '')) AS concept_id,
		COALESCE(BOOL_OR((r->>'fails')::boolean), FALSE) AS fails, SUM(COALESCE((r->>'count')::integer, 0)) AS n,
		row_number() OVER (PARTITION BY x.locale ORDER BY SUM(COALESCE((r->>'count')::integer, 0)) DESC, r->>'key') AS place
		FROM (SELECT DISTINCT ON (t.locale, ` + reviewDeckSame + `) t.locale, f.rules` +
		reviewDeckFrom + scope + ` AND ` + reviewDeckFresh + fmt.Sprintf(` AND f.group_rank = $%d`, rank) +
		` ORDER BY t.locale, ` + reviewDeckSame + `, ` + prio + `, b.item_name, b.position, b.id) x
		CROSS JOIN LATERAL jsonb_array_elements(x.rules) r
		WHERE COALESCE(r->>'key', '') <> ''
		GROUP BY x.locale, r->>'key') y` +
		fmt.Sprintf(` WHERE place <= $%d ORDER BY locale, place`, len(args))
	rows, err := s.db.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("count review deck rules: %w", err)
	}
	defer rows.Close()
	out := map[string][]platstore.RuleTally{}
	for rows.Next() {
		var loc string
		var t platstore.RuleTally
		if err := rows.Scan(&loc, &t.Key, &t.Rule, &t.Subject, &t.Message, &t.ConceptID, &t.Fails, &t.Count); err != nil {
			return nil, fmt.Errorf("scan review deck rule count: %w", err)
		}
		out[loc] = append(out[loc], t)
	}
	return out, rows.Err()
}

// CountReviewDeckByCollection counts the deck of each collection, with every
// translation of the language in it. See platstore.ReviewDeckStore.
func (s *PostgresStore) CountReviewDeckByCollection(ctx context.Context, q platstore.ReviewDeckQuery) (map[string]platstore.ReviewDeckCollectionCounts, error) {
	return s.countReviewDeckCollections(ctx, q, `COALESCE(i.collection_id, '')`)
}

// CountReviewDeckLanguage counts the language as one collection. See
// platstore.ReviewDeckStore.
func (s *PostgresStore) CountReviewDeckLanguage(ctx context.Context, q platstore.ReviewDeckQuery) (platstore.ReviewDeckCollectionCounts, error) {
	out, err := s.countReviewDeckCollections(ctx, q, `''`)
	if err != nil {
		return platstore.ReviewDeckCollectionCounts{}, err
	}
	c, ok := out[""]
	if !ok {
		c.ByRank = map[int]int{}
	}
	return c, nil
}

// countReviewDeckCollections counts the deck grouped by coll, an SQL
// expression naming each row's group.
func (s *PostgresStore) countReviewDeckCollections(ctx context.Context, q platstore.ReviewDeckQuery, coll string) (map[string]platstore.ReviewDeckCollectionCounts, error) {
	scope, args := reviewDeckArgs(q)
	// Rank -3 is an established translation, -2 one set aside, -1 one whose
	// facts are missing or no longer hold.
	rank := `CASE
			WHEN COALESCE(t.target_json->>'status', '') = 'established' THEN -3
			WHEN ` + reviewDeckFresh + ` AND f.group_rank < 0 THEN -2
			WHEN ` + reviewDeckFresh + ` THEN f.group_rank
			ELSE -1 END`
	// A set of copies in a collection counts once, as its collection's deck
	// deals it: waiting while any copy waits.
	//nolint:gosec // the skeleton is fixed literals and $N tokens; every value binds through args
	query := `SELECT coll, rank, count(*) FROM (SELECT DISTINCT ON (coll, ` + reviewDeckSame + `)
			` + coll + ` AS coll, ` + rank + ` AS rank` +
		reviewDeckTranslations + scope + `
		ORDER BY coll, ` + reviewDeckSame + `, CASE WHEN ` + rank + ` >= 0 THEN 0 ELSE -` + rank + ` END, ` + rank + `) x
		GROUP BY coll, rank`
	rows, err := s.db.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("count review deck by collection: %w", err)
	}
	defer rows.Close()
	out := map[string]platstore.ReviewDeckCollectionCounts{}
	for rows.Next() {
		var coll string
		var rank, n int
		if err := rows.Scan(&coll, &rank, &n); err != nil {
			return nil, fmt.Errorf("scan review deck count by collection: %w", err)
		}
		c, ok := out[coll]
		if !ok {
			c.ByRank = map[int]int{}
		}
		c.Translations += n
		switch {
		case rank == -1:
			c.Unclassified += n
		case rank >= 0:
			c.ByRank[rank] += n
		}
		out[coll] = c
	}
	return out, rows.Err()
}

// reviewDeckWaiting keeps, of reviewDeckFrom, the translations in the deck:
// a fresh row ranked below 0 is set aside.
const reviewDeckWaiting = ` AND NOT (` + reviewDeckFresh + ` AND f.group_rank < 0)`

// ListReviewDeckPages returns the deck's pages with their waiting counts.
// See platstore.ReviewDeckStore.
func (s *PostgresStore) ListReviewDeckPages(ctx context.Context, q platstore.ReviewDeckQuery, limit int) ([]platstore.ReviewDeckPage, error) {
	out, err := s.ListReviewDeckPagesByLocale(ctx, q, []string{q.Locale}, limit)
	if err != nil {
		return nil, err
	}
	return out[q.Locale], nil
}

// ListReviewDeckPagesByLocale returns each language's pages as
// ListReviewDeckPages does, in one query. See platstore.ReviewDeckStore.
func (s *PostgresStore) ListReviewDeckPagesByLocale(ctx context.Context, q platstore.ReviewDeckQuery, locales []string, limit int) (map[string][]platstore.ReviewDeckPage, error) {
	scope, args := reviewDeckLocalesArgs(q, locales)
	prio, args := reviewDeckPriority(q, args)
	args = append(args, limit)
	//nolint:gosec // the skeleton is fixed literals and $N tokens; every value binds through args
	query := `SELECT locale, item_name, collection_id, component, prio, waiting, findings, first_block FROM (
		SELECT x.locale, x.item_name, x.collection_id, x.component, x.prio, count(*) AS waiting,
		count(*) FILTER (WHERE x.failing) AS findings,
		(array_agg(x.id ORDER BY x.position, x.id))[1] AS first_block,
		row_number() OVER (PARTITION BY x.locale ORDER BY x.prio, x.item_name, min(x.position), x.component) AS place
		FROM (SELECT t.locale, b.id, b.item_name, COALESCE(i.collection_id, '') AS collection_id,
			COALESCE(b.properties::jsonb->>'component', '') AS component, b.position, ` + prio + ` AS prio,
			COALESCE(` + reviewDeckFresh + ` AND f.failing > 0, false) AS failing` +
		reviewDeckFrom + scope + reviewDeckWaiting + `) x
		GROUP BY x.locale, x.item_name, x.collection_id, x.component, x.prio) y` +
		fmt.Sprintf(` WHERE place <= $%d ORDER BY locale, place`, len(args))
	rows, err := s.db.DB.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list review deck pages: %w", err)
	}
	defer rows.Close()
	out := map[string][]platstore.ReviewDeckPage{}
	for rows.Next() {
		var loc string
		var p platstore.ReviewDeckPage
		if err := rows.Scan(&loc, &p.ItemName, &p.CollectionID, &p.Component, &p.Priority, &p.Waiting, &p.Findings, &p.FirstBlockID); err != nil {
			return nil, fmt.Errorf("scan review deck page: %w", err)
		}
		out[loc] = append(out[loc], p)
	}
	return out, rows.Err()
}

// NextReviewInItem returns the first translation waiting in the item after
// the block. See platstore.ReviewDeckStore.
func (s *PostgresStore) NextReviewInItem(ctx context.Context, q platstore.ReviewDeckQuery, itemName, afterBlockID string) (string, error) {
	scope, args := reviewDeckArgs(q)
	args = append(args, itemName, afterBlockID)
	n := len(args)
	query := `SELECT b.id` + reviewDeckFrom + scope + reviewDeckWaiting +
		fmt.Sprintf(` AND b.item_name = $%d AND (b.position, b.id) > (
			SELECT a.position, a.id FROM blocks a WHERE a.project_id = $1 AND a.stream = $2 AND a.id = $%d)
		ORDER BY b.position, b.id LIMIT 1`, n-1, n)
	var id string
	err := s.db.DB.QueryRowContext(ctx, query, args...).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("next review in item: %w", err)
	}
	return id, nil
}

// reviewDeckCompactAt is the number of a project's change rows past which
// reading its revision folds them into its base.
const reviewDeckCompactAt = 500

// ReviewDeckRevision returns the project's review deck revision: its base
// plus its rows in review_deck_changes, read in one snapshot. See
// platstore.ReviewDeckStore.
func (s *PostgresStore) ReviewDeckRevision(ctx context.Context, projectID string) (int64, error) {
	var base, pending int64
	err := s.db.DB.QueryRowContext(ctx, `SELECT
		COALESCE((SELECT base FROM review_deck_revisions WHERE project_id = $1), 0),
		(SELECT count(*) FROM review_deck_changes WHERE project_id = $1)`, projectID).Scan(&base, &pending)
	if err != nil {
		return 0, fmt.Errorf("read review deck revision: %w", err)
	}
	if pending > reviewDeckCompactAt {
		// One statement deletes the rows it sees and adds their number to the
		// base, so the sum a reader sees stays as it was.
		if _, err := s.db.DB.ExecContext(ctx, `WITH gone AS (
				DELETE FROM review_deck_changes WHERE project_id = $1 RETURNING 1)
			INSERT INTO review_deck_revisions (project_id, base) VALUES ($1, (SELECT count(*) FROM gone))
			ON CONFLICT (project_id) DO UPDATE SET base = review_deck_revisions.base + EXCLUDED.base`, projectID); err != nil {
			return 0, fmt.Errorf("compact review deck revision: %w", err)
		}
	}
	return base + pending, nil
}
