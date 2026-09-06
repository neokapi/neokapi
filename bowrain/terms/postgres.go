package terms

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	fw "github.com/neokapi/neokapi/terms"
	termsschema "github.com/neokapi/neokapi/terms/schema"
)

// PostgresStore is a persistent terms backed by PostgreSQL.
// All workspace terms stores share the same database, isolated by workspace_id.
type PostgresStore struct {
	db          *storage.PgDB
	workspaceID string
}

// NewPostgresStoreFromDB creates a PostgresStore using an existing shared PgDB connection.
func NewPostgresStoreFromDB(db *storage.PgDB, workspaceID string) (*PostgresStore, error) {
	if err := storage.MigratePostgresNS(db, "tb_schema_migrations", Migrations); err != nil {
		return nil, fmt.Errorf("migrate terms schema: %w", err)
	}
	return &PostgresStore{db: db, workspaceID: workspaceID}, nil
}

// Migrations is the bowrain Postgres terms schema as a single consolidated
// baseline. It renders from the shared descriptors in terms/schema, the same
// source the framework SQLite backend renders from, so the two backends cannot
// drift (semantic equivalence is statement-set tested in terms/schema). The
// fuzzy infrastructure (pg_trgm GIN + a tsvector column/trigger) is the Postgres
// analogue of the SQLite FTS5 trigram virtual table.
//
// LEDGER — every version this subsystem has ever issued, now folded in:
//
//	1  terms schema
//	2  add stream column to concepts
//	3  pg_trgm trigram index for fuzzy term matching + tsvector for UI search
//	4  brand knowledge graph: concept source, term competitor/validity, persisted relations
//
// Baseline is version 5, above every number issued, so a database that already
// recorded 4 applies it once more. This subsystem is why that matters: terms
// migrates lazily on first workspace access, not at boot, so when its
// bookkeeping was emptied on 2026-08-01 the replay of version 2 failed with
// `column "stream" of relation "tb_concepts" already exists` — inside a
// translation job, which reported only "terms resolution failed" and carried
// on without enforcement. The baseline is idempotent, so the same replay now
// repairs the ledger instead of taking enforcement offline.
//
// Retired numbers are never reused. The next migration is version 6.
var Migrations = []storage.Migration{
	{
		Version:     5,
		Description: "terms baseline (folds 1-4)",
		SQL:         termsschema.RenderTermsPostgresBaseline("workspace_id"),
	},
}

// pgValidityColumns flattens a validity into its three column values:
// nullable TIMESTAMPTZ bounds and a JSON object of tags ('{}' when empty).
func pgValidityColumns(v *graph.Validity) (validFrom, validTo *time.Time, tags string) {
	tags = "{}"
	if v == nil {
		return nil, nil, tags
	}
	validFrom = v.ValidFrom
	validTo = v.ValidTo
	if len(v.Tags) > 0 {
		if b, err := json.Marshal(v.Tags); err == nil {
			tags = string(b)
		}
	}
	return validFrom, validTo, tags
}

// pgValidityFromColumns rebuilds a validity from its column values. An entirely
// unbounded, tagless validity round-trips to nil (semantically identical:
// it matches every scope).
func pgValidityFromColumns(validFrom, validTo sql.NullTime, tags string) *graph.Validity {
	var v graph.Validity
	if validFrom.Valid {
		t := validFrom.Time
		v.ValidFrom = &t
	}
	if validTo.Valid {
		t := validTo.Time
		v.ValidTo = &t
	}
	if tags != "" && tags != "{}" {
		// Best-effort, matching the annotated SQLite original in
		// terms/sqlite.go: validity tags are optional relation metadata, so a
		// malformed blob degrades to no tags rather than failing the read.
		// This function returns no error by design; mandatory columns
		// (properties, created_at, updated_at) are propagated by the row
		// scanners instead.
		_ = json.Unmarshal([]byte(tags), &v.Tags)
	}
	if v.ValidFrom == nil && v.ValidTo == nil && len(v.Tags) == 0 {
		return nil
	}
	return &v
}

// AddConcept inserts or updates a concept with all its terms using an empty stream.
func (tb *PostgresStore) AddConcept(ctx context.Context, concept fw.Concept) error {
	return tb.AddConceptWithStream(ctx, concept, "")
}

// AddConceptWithStream inserts or updates a concept associated with a stream.
func (tb *PostgresStore) AddConceptWithStream(ctx context.Context, concept fw.Concept, stream string) error {
	if concept.ID == "" {
		return errors.New("concept ID is required")
	}
	// Mirror the framework backends: each term status must be a known
	// lifecycle value (an empty status is allowed — callers may leave it unset).
	for _, t := range concept.Terms {
		if t.Status != "" && !fw.KnownTermStatus(t.Status) {
			return fmt.Errorf("term %q (%s): unknown status %q", t.Text, t.Locale, t.Status)
		}
	}
	concept = fw.NormalizedConcept(concept)

	now := time.Now()
	if concept.CreatedAt.IsZero() {
		concept.CreatedAt = now
	}
	if concept.UpdatedAt.IsZero() {
		concept.UpdatedAt = now
	}

	var propsJSON []byte
	if len(concept.Properties) > 0 {
		propsJSON, _ = json.Marshal(concept.Properties)
	}

	tx, err := tb.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	source := concept.Source
	if source == "" {
		source = fw.TermSourceTerminology
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO tb_concepts (id, workspace_id, stream, domain, definition, properties, source, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		ON CONFLICT (workspace_id, id) DO UPDATE SET
			stream = EXCLUDED.stream,
			domain = EXCLUDED.domain,
			definition = EXCLUDED.definition,
			properties = EXCLUDED.properties,
			source = EXCLUDED.source,
			updated_at = EXCLUDED.updated_at
	`, concept.ID, tb.workspaceID, stream, concept.Domain, concept.Definition,
		nullableString(propsJSON), string(source),
		concept.CreatedAt, concept.UpdatedAt)
	if err != nil {
		return fmt.Errorf("upsert concept: %w", err)
	}

	// Replace all terms for this concept.
	_, err = tx.ExecContext(ctx, "DELETE FROM tb_terms WHERE workspace_id = $1 AND concept_id = $2", tb.workspaceID, concept.ID)
	if err != nil {
		return fmt.Errorf("delete old terms: %w", err)
	}

	for _, term := range concept.Terms {
		validFrom, validTo, tags := pgValidityColumns(term.Validity)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO tb_terms (workspace_id, concept_id, text, text_lower, locale, status, part_of_speech, gender, note, competitor_term, valid_from, valid_to, tags)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		`, tb.workspaceID, concept.ID, term.Text, strings.ToLower(term.Text),
			string(term.Locale), string(term.Status),
			term.PartOfSpeech, term.Gender, term.Note, term.CompetitorTerm,
			validFrom, validTo, tags)
		if err != nil {
			return fmt.Errorf("insert term: %w", err)
		}
	}

	return tx.Commit()
}

// GetConcept retrieves a concept by ID.
func (tb *PostgresStore) GetConcept(ctx context.Context, id string) (fw.Concept, bool, error) {
	concept, err := tb.scanConcept(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return fw.Concept{}, false, nil
	}
	if err != nil {
		return fw.Concept{}, false, err
	}
	return concept, true, nil
}

// DeleteConcept removes a concept by ID.
func (tb *PostgresStore) DeleteConcept(ctx context.Context, id string) error {
	result, err := tb.db.ExecContext(ctx, "DELETE FROM tb_concepts WHERE workspace_id = $1 AND id = $2", tb.workspaceID, id)
	if err != nil {
		return fmt.Errorf("delete concept: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("concept not found: %s", id)
	}
	return nil
}

// AddRelation inserts or updates (by ID) a relation between two concepts
// using an empty stream.
func (tb *PostgresStore) AddRelation(ctx context.Context, rel fw.ConceptRelation) error {
	return tb.AddRelationWithStream(ctx, rel, "")
}

// AddRelationWithStream inserts or updates a relation associated with a stream.
func (tb *PostgresStore) AddRelationWithStream(ctx context.Context, rel fw.ConceptRelation, stream string) error {
	if err := fw.ValidateRelation(rel); err != nil {
		return err
	}
	if err := tb.requireConcept(ctx, "source", rel.SourceID); err != nil {
		return err
	}
	if err := tb.requireConcept(ctx, "target", rel.TargetID); err != nil {
		return err
	}

	if rel.CreatedAt.IsZero() {
		rel.CreatedAt = time.Now()
	}
	validFrom, validTo, tags := pgValidityColumns(rel.Validity)

	// created_at is deliberately not updated on conflict: an upsert preserves
	// the original creation time, like AddConcept does for concepts.
	_, err := tb.db.ExecContext(ctx, `
		INSERT INTO tb_relations (id, workspace_id, source_id, target_id, relation, note, stream, valid_from, valid_to, tags, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		ON CONFLICT (workspace_id, id) DO UPDATE SET
			source_id = EXCLUDED.source_id,
			target_id = EXCLUDED.target_id,
			relation = EXCLUDED.relation,
			note = EXCLUDED.note,
			stream = EXCLUDED.stream,
			valid_from = EXCLUDED.valid_from,
			valid_to = EXCLUDED.valid_to,
			tags = EXCLUDED.tags
	`, rel.ID, tb.workspaceID, rel.SourceID, rel.TargetID, rel.RelationType, rel.Note, stream,
		validFrom, validTo, tags, rel.CreatedAt)
	if err != nil {
		return fmt.Errorf("upsert relation: %w", err)
	}
	return nil
}

// requireConcept returns an error if the concept does not exist.
func (tb *PostgresStore) requireConcept(ctx context.Context, role, id string) error {
	var n int
	if err := tb.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tb_concepts WHERE workspace_id = $1 AND id = $2", tb.workspaceID, id).Scan(&n); err != nil {
		return fmt.Errorf("check %s concept: %w", role, err)
	}
	if n == 0 {
		return fmt.Errorf("%s concept not found: %s", role, id)
	}
	return nil
}

// DeleteRelation removes a relation by ID.
func (tb *PostgresStore) DeleteRelation(ctx context.Context, id string) error {
	result, err := tb.db.ExecContext(ctx, "DELETE FROM tb_relations WHERE workspace_id = $1 AND id = $2", tb.workspaceID, id)
	if err != nil {
		return fmt.Errorf("delete relation: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("relation not found: %s", id)
	}
	return nil
}

// RelationsOf returns all relations touching the concept, in either direction,
// filtered by the validity scope when one is given.
func (tb *PostgresStore) RelationsOf(ctx context.Context, conceptID string, scope *graph.Scope) ([]fw.ConceptRelation, error) {
	rows, err := tb.db.QueryContext(ctx, `
		SELECT id, source_id, target_id, relation, note, valid_from, valid_to, tags, created_at
		FROM tb_relations WHERE workspace_id = $1 AND (source_id = $2 OR target_id = $2) ORDER BY id
	`, tb.workspaceID, conceptID)
	if err != nil {
		return nil, fmt.Errorf("query relations: %w", err)
	}
	defer rows.Close()
	return pgScanRelations(rows, scope)
}

// ListRelations returns all relations, filtered by the validity scope when one
// is given.
func (tb *PostgresStore) ListRelations(ctx context.Context, scope *graph.Scope) ([]fw.ConceptRelation, error) {
	notShadow, shadowArg := fw.NotShadowSQL("id", "$2")
	rows, err := tb.db.QueryContext(ctx, `
		SELECT id, source_id, target_id, relation, note, valid_from, valid_to, tags, created_at
		FROM tb_relations WHERE workspace_id = $1 AND `+notShadow+` ORDER BY id
	`, tb.workspaceID, shadowArg)
	if err != nil {
		return nil, fmt.Errorf("list relations: %w", err)
	}
	defer rows.Close()
	return pgScanRelations(rows, scope)
}

// RelationsForStream returns the relations touching the concept (either
// direction) whose stream is the given stream or one of its ancestors, with
// the same chain semantics as SearchForStream: relations from earlier streams
// in the chain sort first. When scope is non-nil, relations whose validity
// does not match the scope are filtered out.
func (tb *PostgresStore) RelationsForStream(ctx context.Context, conceptID string, stream string, streamChain []string, scope *graph.Scope) ([]fw.ConceptRelation, error) {
	streams := []string{stream}
	streams = append(streams, streamChain...)

	args := []any{tb.workspaceID, conceptID}
	argN := 3

	placeholders := make([]string, len(streams))
	for i, s := range streams {
		placeholders[i] = fmt.Sprintf("$%d", argN)
		args = append(args, s)
		argN++
	}

	var caseExpr strings.Builder
	caseExpr.WriteString("CASE stream")
	for i, s := range streams {
		caseExpr.WriteString(fmt.Sprintf(" WHEN $%d THEN %d", argN, i))
		args = append(args, s)
		argN++
	}
	caseExpr.WriteString(fmt.Sprintf(" ELSE %d END", len(streams)))

	q := fmt.Sprintf(`
		SELECT id, source_id, target_id, relation, note, valid_from, valid_to, tags, created_at
		FROM tb_relations
		WHERE workspace_id = $1 AND (source_id = $2 OR target_id = $2) AND stream IN (%s)
		ORDER BY %s, id
	`, strings.Join(placeholders, ","), caseExpr.String())
	rows, err := tb.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query relations for stream: %w", err)
	}
	defer rows.Close()
	return pgScanRelations(rows, scope)
}

// pgScanRelations reads relation rows, rebuilding validity and applying the
// optional scope filter.
func pgScanRelations(rows *sql.Rows, scope *graph.Scope) ([]fw.ConceptRelation, error) {
	var out []fw.ConceptRelation
	for rows.Next() {
		var rel fw.ConceptRelation
		var tags string
		var validFrom, validTo sql.NullTime
		if err := rows.Scan(&rel.ID, &rel.SourceID, &rel.TargetID, &rel.RelationType, &rel.Note, &validFrom, &validTo, &tags, &rel.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan relation: %w", err)
		}
		rel.Validity = pgValidityFromColumns(validFrom, validTo, tags)
		if !fw.MatchesScope(rel.Validity, scope) {
			continue
		}
		out = append(out, rel)
	}
	return out, rows.Err()
}

// Lookup finds terms matching the source text. The tier gating, validity-scope
// filter, status filter, Levenshtein scoring and sort live in the shared
// fw.LookupTiered, so the Postgres and SQLite backends rank identically and both
// honor term validity (valid_from/valid_to) by construction.
func (tb *PostgresStore) Lookup(ctx context.Context, sourceText string, opts fw.LookupOptions) ([]fw.TermMatch, error) {
	return fw.LookupTiered(ctx, sourceText, opts, fw.TermCandidateSource{
		Exact:           tb.queryExactTerms,
		Normalized:      tb.queryNormalizedTerms,
		FuzzyCandidates: tb.queryFuzzyTerms,
		Concept:         tb.scanConcept,
	})
}

// LookupAll finds all terms appearing in the given text. The position scan,
// validity-scope filter and (text, position) de-duplication live in the shared
// fw.LookupAllTiered, which Postgres now inherits.
func (tb *PostgresStore) LookupAll(ctx context.Context, sourceText string, opts fw.LookupOptions) ([]fw.TermMatch, error) {
	if sourceText == "" {
		return nil, nil
	}
	opts = fw.ApplyLookupDefaults(opts)
	terms, err := tb.queryTermsByLocale(ctx, opts.SourceLocale, opts.Domains, opts.StatusFilter)
	if err != nil {
		return nil, err
	}
	return fw.LookupAllTiered(sourceText, opts, terms), nil
}

// Search performs a ranked full-text search across concepts and terms.
// Uses pg_trgm for term matching when a query is provided, falls back to LIKE.
func (tb *PostgresStore) Search(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]fw.Concept, int, error) {
	sourceLocale, targetLocale = model.NormalizeLocale(sourceLocale), model.NormalizeLocale(targetLocale)
	if query != "" {
		concepts, total, err := tb.pgSearchTrgm(ctx, query, sourceLocale, targetLocale, offset, limit)
		if err == nil {
			return concepts, total, nil
		}
	}
	return tb.pgSearchLike(ctx, query, sourceLocale, targetLocale, offset, limit)
}

func (tb *PostgresStore) pgSearchTrgm(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]fw.Concept, int, error) {
	where := "t.workspace_id = $1 AND t.text_lower % $2"
	args := []any{tb.workspaceID, strings.ToLower(query)}
	argN := 3

	if sourceLocale != "" {
		where += fmt.Sprintf(" AND t.locale = $%d", argN)
		args = append(args, string(sourceLocale))
		argN++
	}
	if targetLocale != "" {
		// Need to check that concept has a term in target locale too.
		where += fmt.Sprintf(` AND t.concept_id IN (
			SELECT concept_id FROM tb_terms WHERE workspace_id = $1 AND locale = $%d)`, argN)
		args = append(args, string(targetLocale))
		argN++
	}

	countQ := `SELECT COUNT(DISTINCT t.concept_id)
		FROM tb_terms t WHERE ` + where
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	var total int
	if err := tb.db.QueryRowContext(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// GROUP BY (not DISTINCT) so the ORDER BY similarity() expression is legal:
	// Postgres rejects "SELECT DISTINCT … ORDER BY <expr not in select list>".
	q := fmt.Sprintf(`SELECT t.concept_id
		FROM tb_terms t
		JOIN tb_concepts c ON t.workspace_id = c.workspace_id AND t.concept_id = c.id
		WHERE %s
		GROUP BY t.concept_id
		ORDER BY MAX(similarity(t.text_lower, $2)) DESC
		LIMIT $%d OFFSET $%d`, where, argN, argN+1)
	args = append(args, limit, offset)

	rows, err := tb.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, 0, fmt.Errorf("scan concept id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	concepts, err := tb.scanConcepts(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	return concepts, total, nil
}

func (tb *PostgresStore) pgSearchLike(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]fw.Concept, int, error) {
	where := "workspace_id = $1"
	args := []any{tb.workspaceID}
	argN := 2

	if query != "" {
		where += fmt.Sprintf(` AND (LOWER(c.definition) LIKE $%d OR LOWER(c.domain) LIKE $%d
			OR c.id IN (SELECT concept_id FROM tb_terms WHERE workspace_id = $1 AND text_lower LIKE $%d))`, argN, argN+1, argN+2)
		pattern := "%" + strings.ToLower(query) + "%"
		args = append(args, pattern, pattern, pattern)
		argN += 3
	}

	if sourceLocale != "" {
		where += fmt.Sprintf(" AND c.id IN (SELECT concept_id FROM tb_terms WHERE workspace_id = $1 AND locale = $%d)", argN)
		args = append(args, string(sourceLocale))
		argN++
	}
	if targetLocale != "" {
		where += fmt.Sprintf(" AND c.id IN (SELECT concept_id FROM tb_terms WHERE workspace_id = $1 AND locale = $%d)", argN)
		args = append(args, string(targetLocale))
		argN++
	}

	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	_ = tb.db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT c.id) FROM tb_concepts c WHERE "+where, countArgs...).Scan(&total)

	// No DISTINCT: c.id is the table PK (single-table query), so rows are already
	// unique — and DISTINCT would make ORDER BY c.updated_at illegal in Postgres.
	q := fmt.Sprintf(`SELECT c.id FROM tb_concepts c WHERE %s ORDER BY c.updated_at DESC LIMIT $%d OFFSET $%d`, where, argN, argN+1)
	args = append(args, limit, offset)
	rows, err := tb.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, total, fmt.Errorf("search concepts: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, total, fmt.Errorf("iterate concepts: %w", err)
	}

	concepts, err := tb.scanConcepts(ctx, ids)
	if err != nil {
		return nil, total, err
	}
	return concepts, total, nil
}

// SearchForStream performs a ranked full-text search with stream inheritance.
// Uses pg_trgm when a query is provided, falls back to LIKE.
// The streamChain is an ordered list of ancestor streams to search.
// Concepts from earlier streams take priority.
func (tb *PostgresStore) SearchForStream(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, stream string, streamChain []string, offset, limit int) ([]fw.Concept, int, error) {
	sourceLocale, targetLocale = model.NormalizeLocale(sourceLocale), model.NormalizeLocale(targetLocale)
	if query != "" {
		concepts, total, err := tb.pgSearchTrgmForStream(ctx, query, sourceLocale, targetLocale, stream, streamChain, offset, limit)
		if err == nil {
			return concepts, total, nil
		}
	}
	return tb.pgSearchLikeForStream(ctx, query, sourceLocale, targetLocale, stream, streamChain, offset, limit)
}

func (tb *PostgresStore) pgSearchTrgmForStream(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, stream string, streamChain []string, offset, limit int) ([]fw.Concept, int, error) {
	streams := []string{stream}
	streams = append(streams, streamChain...)

	where := "c.workspace_id = $1"
	args := []any{tb.workspaceID}
	argN := 2

	// Stream filter.
	placeholders := make([]string, len(streams))
	for i, s := range streams {
		placeholders[i] = fmt.Sprintf("$%d", argN)
		args = append(args, s)
		argN++
	}
	where += " AND c.stream IN (" + strings.Join(placeholders, ",") + ")"

	where += fmt.Sprintf(` AND c.id IN (SELECT concept_id FROM tb_terms
		WHERE workspace_id = $1 AND text_lower %% $%d)`, argN)
	args = append(args, strings.ToLower(query))
	argN++

	if sourceLocale != "" {
		where += fmt.Sprintf(" AND c.id IN (SELECT concept_id FROM tb_terms WHERE workspace_id = $1 AND locale = $%d)", argN)
		args = append(args, string(sourceLocale))
		argN++
	}
	if targetLocale != "" {
		where += fmt.Sprintf(" AND c.id IN (SELECT concept_id FROM tb_terms WHERE workspace_id = $1 AND locale = $%d)", argN)
		args = append(args, string(targetLocale))
		argN++
	}

	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	if err := tb.db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT c.id) FROM tb_concepts c WHERE "+where, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Build CASE expression for stream priority ordering.
	var caseExpr strings.Builder
	caseExpr.WriteString("CASE c.stream")
	for i, s := range streams {
		caseExpr.WriteString(fmt.Sprintf(" WHEN $%d THEN %d", argN, i))
		args = append(args, s)
		argN++
	}
	caseExpr.WriteString(fmt.Sprintf(" ELSE %d END", len(streams)))

	// No DISTINCT: single-table query (c.id is the PK), and DISTINCT would make the
	// status/updated_at ORDER BY illegal in Postgres.
	q := fmt.Sprintf(`SELECT c.id FROM tb_concepts c WHERE %s ORDER BY %s, c.updated_at DESC LIMIT $%d OFFSET $%d`, where, caseExpr.String(), argN, argN+1)
	args = append(args, limit, offset)
	rows, err := tb.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	concepts, err := tb.scanConcepts(ctx, ids)
	if err != nil {
		return nil, total, err
	}
	return concepts, total, nil
}

func (tb *PostgresStore) pgSearchLikeForStream(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, stream string, streamChain []string, offset, limit int) ([]fw.Concept, int, error) {
	streams := []string{stream}
	streams = append(streams, streamChain...)

	where := "c.workspace_id = $1"
	args := []any{tb.workspaceID}
	argN := 2

	// Stream filter.
	placeholders := make([]string, len(streams))
	for i, s := range streams {
		placeholders[i] = fmt.Sprintf("$%d", argN)
		args = append(args, s)
		argN++
	}
	where += " AND c.stream IN (" + strings.Join(placeholders, ",") + ")"

	if query != "" {
		where += fmt.Sprintf(` AND (LOWER(c.definition) LIKE $%d OR LOWER(c.domain) LIKE $%d
			OR c.id IN (SELECT concept_id FROM tb_terms WHERE workspace_id = $1 AND text_lower LIKE $%d))`, argN, argN+1, argN+2)
		pattern := "%" + strings.ToLower(query) + "%"
		args = append(args, pattern, pattern, pattern)
		argN += 3
	}

	if sourceLocale != "" {
		where += fmt.Sprintf(" AND c.id IN (SELECT concept_id FROM tb_terms WHERE workspace_id = $1 AND locale = $%d)", argN)
		args = append(args, string(sourceLocale))
		argN++
	}
	if targetLocale != "" {
		where += fmt.Sprintf(" AND c.id IN (SELECT concept_id FROM tb_terms WHERE workspace_id = $1 AND locale = $%d)", argN)
		args = append(args, string(targetLocale))
		argN++
	}

	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	_ = tb.db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT c.id) FROM tb_concepts c WHERE "+where, countArgs...).Scan(&total)

	// Build CASE expression for stream priority ordering.
	var caseExpr strings.Builder
	caseExpr.WriteString("CASE c.stream")
	for i, s := range streams {
		caseExpr.WriteString(fmt.Sprintf(" WHEN $%d THEN %d", argN, i))
		args = append(args, s)
		argN++
	}
	caseExpr.WriteString(fmt.Sprintf(" ELSE %d END", len(streams)))

	// No DISTINCT: single-table query (c.id is the PK), and DISTINCT would make the
	// status/updated_at ORDER BY illegal in Postgres.
	q := fmt.Sprintf(`SELECT c.id FROM tb_concepts c WHERE %s ORDER BY %s, c.updated_at DESC LIMIT $%d OFFSET $%d`, where, caseExpr.String(), argN, argN+1)
	args = append(args, limit, offset)
	rows, err := tb.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, total, fmt.Errorf("search concepts: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, total, fmt.Errorf("iterate concepts: %w", err)
	}

	concepts, err := tb.scanConcepts(ctx, ids)
	if err != nil {
		return nil, total, err
	}
	return concepts, total, nil
}

// Count returns the total number of concepts for this workspace.
func (tb *PostgresStore) Count(ctx context.Context) (int, error) {
	var count int
	if err := tb.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tb_concepts WHERE workspace_id = $1", tb.workspaceID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count concepts: %w", err)
	}
	return count, nil
}

// Concepts returns all concepts for this workspace.
// Concepts returns the workspace's concepts. The shadow namespace is not among
// them: a stream-scoped shadow belongs to the branch it was written for, and
// this list is what the sync carries into a project's own terms.
func (tb *PostgresStore) Concepts(ctx context.Context) ([]fw.Concept, error) {
	notShadow, shadowArg := fw.NotShadowSQL("id", "$2")
	rows, err := tb.db.QueryContext(ctx,
		"SELECT id FROM tb_concepts WHERE workspace_id = $1 AND "+notShadow+" ORDER BY id",
		tb.workspaceID, shadowArg)
	if err != nil {
		return nil, fmt.Errorf("list concepts: %w", err)
	}
	defer rows.Close()

	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate concepts: %w", err)
	}

	concepts, err := tb.scanConcepts(ctx, ids)
	if err != nil {
		return nil, err
	}
	return concepts, nil
}

// Close is a no-op for PostgresStore since the connection is shared.
func (tb *PostgresStore) Close() error {
	return nil
}

// --- internal helpers ---

// scanConcepts hydrates several concepts and all their terms in two queries
// rather than the 1+2N round trips a per-id scanConcept loop runs — the term
// check hydrates the whole store on every request. The result preserves the
// caller's id order and drops ids with no concept row. A concept whose
// properties JSON is corrupt is an error, not a silently dropped row.
func (tb *PostgresStore) scanConcepts(ctx context.Context, ids []string) ([]fw.Concept, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// $1 is workspace; $2..$N are the ids.
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	args = append(args, tb.workspaceID)
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}
	inClause := strings.Join(placeholders, ",")

	conceptRows, err := tb.db.QueryContext(ctx, `
		SELECT id, domain, definition, properties, source, created_at, updated_at
		FROM tb_concepts WHERE workspace_id = $1 AND id IN (`+inClause+`)`, args...)
	if err != nil {
		return nil, fmt.Errorf("load concepts: %w", err)
	}
	defer conceptRows.Close()

	byID := make(map[string]*fw.Concept, len(ids))
	for conceptRows.Next() {
		var c fw.Concept
		var propsJSON *string
		var source string
		if err := conceptRows.Scan(&c.ID, &c.Domain, &c.Definition, &propsJSON, &source, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan concept: %w", err)
		}
		c.Source = fw.TermSource(source)
		if propsJSON != nil && *propsJSON != "" {
			if err := json.Unmarshal([]byte(*propsJSON), &c.Properties); err != nil {
				return nil, fmt.Errorf("concept %s: unmarshal properties: %w", c.ID, err)
			}
		}
		byID[c.ID] = &c
	}
	if err := conceptRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate concepts: %w", err)
	}

	termRows, err := tb.db.QueryContext(ctx, `
		SELECT concept_id, text, locale, status, part_of_speech, gender, note, competitor_term, valid_from, valid_to, tags
		FROM tb_terms WHERE workspace_id = $1 AND concept_id IN (`+inClause+`)
		ORDER BY concept_id, text`, args...)
	if err != nil {
		return nil, fmt.Errorf("load terms: %w", err)
	}
	defer termRows.Close()

	for termRows.Next() {
		var conceptID string
		var t fw.Term
		var locale, status, tags string
		var validFrom, validTo sql.NullTime
		if err := termRows.Scan(&conceptID, &t.Text, &locale, &status, &t.PartOfSpeech, &t.Gender, &t.Note, &t.CompetitorTerm, &validFrom, &validTo, &tags); err != nil {
			return nil, fmt.Errorf("scan term: %w", err)
		}
		c, ok := byID[conceptID]
		if !ok {
			continue
		}
		t.Locale = model.LocaleID(locale)
		t.Status = model.TermStatus(status)
		t.Validity = pgValidityFromColumns(validFrom, validTo, tags)
		c.Terms = append(c.Terms, t)
	}
	if err := termRows.Err(); err != nil {
		return nil, fmt.Errorf("iterate terms: %w", err)
	}

	concepts := make([]fw.Concept, 0, len(ids))
	for _, id := range ids {
		if c, ok := byID[id]; ok {
			concepts = append(concepts, *c)
		}
	}
	return concepts, nil
}

func (tb *PostgresStore) scanConcept(ctx context.Context, id string) (fw.Concept, error) {
	var c fw.Concept
	var propsJSON *string
	var source string

	err := tb.db.QueryRowContext(ctx, `
		SELECT id, domain, definition, properties, source, created_at, updated_at
		FROM tb_concepts WHERE workspace_id = $1 AND id = $2
	`, tb.workspaceID, id).Scan(&c.ID, &c.Domain, &c.Definition, &propsJSON, &source, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return fw.Concept{}, err
	}

	c.Source = fw.TermSource(source)

	// tb_concepts.properties is plain TEXT with no NOT NULL, so the nil check
	// is load-bearing: a row that never had properties is not a corrupt one.
	if propsJSON != nil && *propsJSON != "" {
		if err := json.Unmarshal([]byte(*propsJSON), &c.Properties); err != nil {
			return c, fmt.Errorf("concept %s: unmarshal properties: %w", c.ID, err)
		}
	}

	rows, err := tb.db.QueryContext(ctx, `
		SELECT text, locale, status, part_of_speech, gender, note, competitor_term, valid_from, valid_to, tags
		FROM tb_terms WHERE workspace_id = $1 AND concept_id = $2
	`, tb.workspaceID, id)
	if err != nil {
		return c, fmt.Errorf("query terms for concept %s: %w", id, err)
	}
	defer rows.Close()

	for rows.Next() {
		var t fw.Term
		var locale, status, tags string
		var validFrom, validTo sql.NullTime
		if err := rows.Scan(&t.Text, &locale, &status, &t.PartOfSpeech, &t.Gender, &t.Note, &t.CompetitorTerm, &validFrom, &validTo, &tags); err != nil {
			continue
		}
		t.Locale = model.LocaleID(locale)
		t.Status = model.TermStatus(status)
		t.Validity = pgValidityFromColumns(validFrom, validTo, tags)
		c.Terms = append(c.Terms, t)
	}
	if err := rows.Err(); err != nil {
		return c, fmt.Errorf("iterate terms: %w", err)
	}

	return c, nil
}

// pgTermSelectCols is the shared 10-column term projection every candidate
// query selects, including the validity columns the shared lookup filters on.
const pgTermSelectCols = "t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note, t.valid_from, t.valid_to, t.tags"

func (tb *PostgresStore) queryExactTerms(ctx context.Context, sourceText string, opts fw.LookupOptions) ([]fw.TermCandidate, error) {
	searchText := sourceText
	column := "t.text"
	if !opts.CaseSensitive {
		searchText = strings.ToLower(sourceText)
		column = "t.text_lower"
	}

	q := fmt.Sprintf(`
		SELECT `+pgTermSelectCols+`
		FROM tb_terms t
		WHERE t.workspace_id = $1 AND %s = $2 AND t.locale = $3
	`, column)

	rows, err := tb.db.QueryContext(ctx, q, tb.workspaceID, searchText, string(opts.SourceLocale))
	if err != nil {
		return nil, fmt.Errorf("query exact terms: %w", err)
	}
	defer rows.Close()

	return pgScanTermCandidates(rows)
}

func (tb *PostgresStore) queryNormalizedTerms(ctx context.Context, normalizedSource string, opts fw.LookupOptions) ([]fw.TermCandidate, error) {
	rows, err := tb.db.QueryContext(ctx, `
		SELECT `+pgTermSelectCols+`
		FROM tb_terms t
		WHERE t.workspace_id = $1 AND t.text_lower = $2 AND t.locale = $3
	`, tb.workspaceID, normalizedSource, string(opts.SourceLocale))
	if err != nil {
		return nil, fmt.Errorf("query normalized terms: %w", err)
	}
	defer rows.Close()

	return pgScanTermCandidates(rows)
}

// queryFuzzyTerms returns the raw fuzzy candidate pool: the pg_trgm pre-filter,
// falling back to a length-bounded full scan when the trigram path yields no
// candidate rows (or pg_trgm is unavailable). Levenshtein scoring and the
// MinScore gate live in the shared fw.LookupTiered.
func (tb *PostgresStore) queryFuzzyTerms(ctx context.Context, normalizedSource string, opts fw.LookupOptions) ([]fw.TermCandidate, error) {
	cands, err := tb.queryFuzzyTrigramCandidates(ctx, normalizedSource, opts)
	if err != nil {
		return nil, err
	}
	if len(cands) > 0 {
		return cands, nil
	}
	return tb.queryFuzzyFullScan(ctx, normalizedSource, opts)
}

func (tb *PostgresStore) queryFuzzyTrigramCandidates(ctx context.Context, normalizedSource string, opts fw.LookupOptions) ([]fw.TermCandidate, error) {
	// Use pg_trgm similarity operator (%) with GIN index.
	rows, err := tb.db.QueryContext(ctx, `
		SELECT `+pgTermSelectCols+`
		FROM tb_terms t
		WHERE t.workspace_id = $1 AND t.locale = $2 AND t.text_lower % $3
		LIMIT 200
	`, tb.workspaceID, string(opts.SourceLocale), normalizedSource)
	if err != nil {
		// pg_trgm unavailable; signal fallback to the full scan (not an error).
		return nil, nil
	}
	defer rows.Close()

	return pgScanTermCandidates(rows)
}

func (tb *PostgresStore) queryFuzzyFullScan(ctx context.Context, normalizedSource string, opts fw.LookupOptions) ([]fw.TermCandidate, error) {
	minLen, maxLen := fw.FuzzyLengthWindow(len([]rune(normalizedSource)), opts.MinScore)

	rows, err := tb.db.QueryContext(ctx, `
		SELECT `+pgTermSelectCols+`
		FROM tb_terms t
		WHERE t.workspace_id = $1 AND t.locale = $2 AND LENGTH(t.text_lower) BETWEEN $3 AND $4
		LIMIT 500
	`, tb.workspaceID, string(opts.SourceLocale), minLen, maxLen)
	if err != nil {
		return nil, fmt.Errorf("fuzzy full scan: %w", err)
	}
	defer rows.Close()

	return pgScanTermCandidates(rows)
}

func (tb *PostgresStore) queryTermsByLocale(ctx context.Context, locale model.LocaleID, domains []string, statusFilter []model.TermStatus) ([]fw.LocaleTerm, error) {
	// This query is the terms half of every check. It names no stream, so it
	// must exclude the shadow namespace: a shadow written for one branch would
	// otherwise decide what the checks flag everywhere.
	notShadow, shadowArg := fw.NotShadowSQL("c.id", "$3")
	where := "t.workspace_id = $1 AND t.locale = $2 AND " + notShadow
	args := []any{tb.workspaceID, string(locale), shadowArg}
	argN := 4

	if len(domains) > 0 {
		placeholders := make([]string, len(domains))
		for i, d := range domains {
			placeholders[i] = fmt.Sprintf("$%d", argN)
			args = append(args, d)
			argN++
		}
		where += " AND c.domain IN (" + strings.Join(placeholders, ",") + ")"
	}

	if len(statusFilter) > 0 {
		placeholders := make([]string, len(statusFilter))
		for i, s := range statusFilter {
			placeholders[i] = fmt.Sprintf("$%d", argN)
			args = append(args, string(s))
			argN++
		}
		where += " AND t.status IN (" + strings.Join(placeholders, ",") + ")"
	}

	rows, err := tb.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT c.id, c.domain, c.definition, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note, t.valid_from, t.valid_to, t.tags
		FROM tb_terms t JOIN tb_concepts c ON t.workspace_id = c.workspace_id AND t.concept_id = c.id
		WHERE %s
		ORDER BY c.id, t.text
	`, where), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []fw.LocaleTerm
	for rows.Next() {
		var cID, domain, definition, text, loc, status, pos, gender, note, tags string
		var validFrom, validTo sql.NullTime
		if err := rows.Scan(&cID, &domain, &definition, &text, &loc, &status, &pos, &gender, &note, &validFrom, &validTo, &tags); err != nil {
			continue
		}
		results = append(results, fw.LocaleTerm{
			Concept: fw.Concept{ID: cID, Domain: domain, Definition: definition},
			Term: fw.Term{
				Text:         text,
				Locale:       model.LocaleID(loc),
				Status:       model.TermStatus(status),
				PartOfSpeech: pos,
				Gender:       gender,
				Note:         note,
				Validity:     pgValidityFromColumns(validFrom, validTo, tags),
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

type pgScanTermRow struct {
	conceptID, text, locale, status, pos, gender, note string
	validFrom, validTo                                 sql.NullTime
	tags                                               string
}

// validity rebuilds the term validity from the scanned columns (Postgres
// TIMESTAMPTZ codec).
func (r pgScanTermRow) validity() *graph.Validity {
	return pgValidityFromColumns(r.validFrom, r.validTo, r.tags)
}

// pgScanTermCandidates scans the shared 10-column term projection into raw
// candidates. Validity is reconstructed here; the shared fw.LookupTiered
// applies the scope/status/score filters and hydrates the owning concept.
//
// It reports scan and iteration errors: a truncated candidate set is a term
// silently not matching, which lets a term/DNT check pass over violating
// content, so the rows interface must expose Err() and callers must propagate.
func pgScanTermCandidates(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]fw.TermCandidate, error) {
	var out []fw.TermCandidate
	for rows.Next() {
		var r pgScanTermRow
		if err := rows.Scan(&r.conceptID, &r.text, &r.locale, &r.status, &r.pos, &r.gender, &r.note, &r.validFrom, &r.validTo, &r.tags); err != nil {
			return nil, fmt.Errorf("scan term candidate: %w", err)
		}
		out = append(out, fw.TermCandidate{
			ConceptID: r.conceptID,
			Term: fw.Term{
				Text:         r.text,
				Locale:       model.LocaleID(r.locale),
				Status:       model.TermStatus(r.status),
				PartOfSpeech: r.pos,
				Gender:       r.gender,
				Note:         r.note,
				Validity:     r.validity(),
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate term candidates: %w", err)
	}
	return out, nil
}

func nullableString(b []byte) *string {
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	return &s
}
