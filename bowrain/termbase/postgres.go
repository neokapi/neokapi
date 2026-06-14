package termbase

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/sievepen"
	fw "github.com/neokapi/neokapi/termbase"
)

// PostgresTermBase is a persistent termbase backed by PostgreSQL.
// All workspace termbases share the same database, isolated by workspace_id.
type PostgresTermBase struct {
	db          *storage.PgDB
	workspaceID string
}

// NewPostgresTermBaseFromDB creates a PostgresTermBase using an existing shared PgDB connection.
func NewPostgresTermBaseFromDB(db *storage.PgDB, workspaceID string) (*PostgresTermBase, error) {
	if err := storage.MigratePostgresNS(db, "tb_schema_migrations", tbMigrationsPg); err != nil {
		return nil, fmt.Errorf("migrate termbase schema: %w", err)
	}
	return &PostgresTermBase{db: db, workspaceID: workspaceID}, nil
}

var tbMigrationsPg = []storage.Migration{
	{
		Version:     1,
		Description: "termbase schema",
		SQL: `
		CREATE TABLE IF NOT EXISTS tb_concepts (
			id           TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			domain       TEXT NOT NULL DEFAULT '',
			definition   TEXT NOT NULL DEFAULT '',
			properties   TEXT,
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (workspace_id, id)
		);
		CREATE TABLE IF NOT EXISTS tb_terms (
			id            SERIAL PRIMARY KEY,
			workspace_id  TEXT NOT NULL,
			concept_id    TEXT NOT NULL,
			text          TEXT NOT NULL,
			text_lower    TEXT NOT NULL,
			locale        TEXT NOT NULL,
			status        TEXT NOT NULL DEFAULT 'approved',
			part_of_speech TEXT NOT NULL DEFAULT '',
			gender        TEXT NOT NULL DEFAULT '',
			note          TEXT NOT NULL DEFAULT '',
			FOREIGN KEY (workspace_id, concept_id) REFERENCES tb_concepts(workspace_id, id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_tb_terms_ws_concept ON tb_terms(workspace_id, concept_id);
		CREATE INDEX IF NOT EXISTS idx_tb_terms_ws_locale ON tb_terms(workspace_id, locale);
		CREATE INDEX IF NOT EXISTS idx_tb_terms_ws_text ON tb_terms(workspace_id, text_lower, locale);
		`,
	},
	{
		Version:     2,
		Description: "add stream column to concepts",
		SQL: `ALTER TABLE tb_concepts ADD COLUMN stream TEXT NOT NULL DEFAULT '';
		CREATE INDEX IF NOT EXISTS idx_tb_concepts_ws_stream ON tb_concepts(workspace_id, stream);`,
	},
	{
		Version:     3,
		Description: "pg_trgm trigram index for fuzzy term matching + tsvector for UI search",
		SQL: `
		CREATE EXTENSION IF NOT EXISTS pg_trgm;

		CREATE INDEX IF NOT EXISTS idx_tb_terms_trgm ON tb_terms USING gin (text_lower gin_trgm_ops);

		ALTER TABLE tb_terms ADD COLUMN search_tsv tsvector;
		UPDATE tb_terms SET search_tsv = to_tsvector('simple', text_lower);
		CREATE INDEX IF NOT EXISTS idx_tb_terms_search_tsv ON tb_terms USING gin (search_tsv);

		CREATE OR REPLACE FUNCTION tb_terms_search_tsv_update() RETURNS trigger AS $$
		BEGIN
			NEW.search_tsv := to_tsvector('simple', NEW.text_lower);
			RETURN NEW;
		END $$ LANGUAGE plpgsql;

		DROP TRIGGER IF EXISTS tb_terms_search_tsv_trigger ON tb_terms;
		CREATE TRIGGER tb_terms_search_tsv_trigger BEFORE INSERT OR UPDATE ON tb_terms
			FOR EACH ROW EXECUTE FUNCTION tb_terms_search_tsv_update();
		`,
	},
	{
		Version:     4,
		Description: "brand knowledge graph: concept source, term competitor/validity, persisted relations",
		// Schema parity with the framework SQLite backend (AD-021): the source
		// column on concepts, the competitor flag and validity columns on terms,
		// and the workspace-scoped relations table. The stream column mirrors
		// the tb_concepts stream pattern so relations can participate in
		// stream-scoped shadows (pilots) like concepts do.
		SQL: `
		ALTER TABLE tb_concepts ADD COLUMN source TEXT NOT NULL DEFAULT 'terminology';

		ALTER TABLE tb_terms ADD COLUMN competitor_term BOOLEAN NOT NULL DEFAULT FALSE;
		ALTER TABLE tb_terms ADD COLUMN valid_from TIMESTAMPTZ;
		ALTER TABLE tb_terms ADD COLUMN valid_to TIMESTAMPTZ;
		ALTER TABLE tb_terms ADD COLUMN tags JSONB NOT NULL DEFAULT '{}';

		CREATE TABLE IF NOT EXISTS tb_relations (
			id           TEXT NOT NULL,
			workspace_id TEXT NOT NULL,
			source_id    TEXT NOT NULL,
			target_id    TEXT NOT NULL,
			relation     TEXT NOT NULL,
			note         TEXT NOT NULL DEFAULT '',
			stream       TEXT NOT NULL DEFAULT '',
			valid_from   TIMESTAMPTZ,
			valid_to     TIMESTAMPTZ,
			tags         JSONB NOT NULL DEFAULT '{}',
			created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (workspace_id, id),
			FOREIGN KEY (workspace_id, source_id) REFERENCES tb_concepts(workspace_id, id) ON DELETE CASCADE,
			FOREIGN KEY (workspace_id, target_id) REFERENCES tb_concepts(workspace_id, id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_tb_relations_ws_source ON tb_relations(workspace_id, source_id);
		CREATE INDEX IF NOT EXISTS idx_tb_relations_ws_target ON tb_relations(workspace_id, target_id);
		CREATE INDEX IF NOT EXISTS idx_tb_relations_ws_stream ON tb_relations(workspace_id, stream);
		`,
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
		_ = json.Unmarshal([]byte(tags), &v.Tags)
	}
	if v.ValidFrom == nil && v.ValidTo == nil && len(v.Tags) == 0 {
		return nil
	}
	return &v
}

// AddConcept inserts or updates a concept with all its terms using an empty stream.
func (tb *PostgresTermBase) AddConcept(ctx context.Context, concept fw.Concept) error {
	return tb.AddConceptWithStream(ctx, concept, "")
}

// AddConceptWithStream inserts or updates a concept associated with a stream.
func (tb *PostgresTermBase) AddConceptWithStream(ctx context.Context, concept fw.Concept, stream string) error {
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
func (tb *PostgresTermBase) GetConcept(ctx context.Context, id string) (fw.Concept, bool, error) {
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
func (tb *PostgresTermBase) DeleteConcept(ctx context.Context, id string) error {
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
func (tb *PostgresTermBase) AddRelation(ctx context.Context, rel fw.ConceptRelation) error {
	return tb.AddRelationWithStream(ctx, rel, "")
}

// AddRelationWithStream inserts or updates a relation associated with a stream.
func (tb *PostgresTermBase) AddRelationWithStream(ctx context.Context, rel fw.ConceptRelation, stream string) error {
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
func (tb *PostgresTermBase) requireConcept(ctx context.Context, role, id string) error {
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
func (tb *PostgresTermBase) DeleteRelation(ctx context.Context, id string) error {
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
func (tb *PostgresTermBase) RelationsOf(ctx context.Context, conceptID string, scope *graph.Scope) ([]fw.ConceptRelation, error) {
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
func (tb *PostgresTermBase) ListRelations(ctx context.Context, scope *graph.Scope) ([]fw.ConceptRelation, error) {
	rows, err := tb.db.QueryContext(ctx, `
		SELECT id, source_id, target_id, relation, note, valid_from, valid_to, tags, created_at
		FROM tb_relations WHERE workspace_id = $1 ORDER BY id
	`, tb.workspaceID)
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
func (tb *PostgresTermBase) RelationsForStream(ctx context.Context, conceptID string, stream string, streamChain []string, scope *graph.Scope) ([]fw.ConceptRelation, error) {
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

// Lookup finds terms matching the source text.
func (tb *PostgresTermBase) Lookup(ctx context.Context, sourceText string, opts fw.LookupOptions) ([]fw.TermMatch, error) {
	if sourceText == "" {
		return nil, nil
	}

	opts = fw.ApplyLookupDefaults(opts)
	modeEnabled := fw.MatchModesEnabled(opts.MatchModes)
	normalizedSource := fw.NormalizeTerm(sourceText)
	var matches []fw.TermMatch

	if modeEnabled[model.MatchStrategyExact] {
		m, err := tb.queryExactTerms(ctx, sourceText, opts)
		if err != nil {
			return nil, err
		}
		matches = append(matches, m...)
	}

	if modeEnabled[model.MatchStrategyNormalized] && len(matches) == 0 {
		m, err := tb.queryNormalizedTerms(ctx, normalizedSource, opts)
		if err != nil {
			return nil, err
		}
		matches = append(matches, m...)
	}

	if modeEnabled[model.MatchStrategyFuzzy] && len(matches) == 0 {
		m, err := tb.queryFuzzyTerms(ctx, normalizedSource, opts)
		if err != nil {
			return nil, err
		}
		matches = append(matches, m...)
	}

	slices.SortFunc(matches, func(a, b fw.TermMatch) int {
		return cmp.Compare(b.Score, a.Score)
	})

	return matches, nil
}

// LookupAll finds all terms appearing in the given text.
func (tb *PostgresTermBase) LookupAll(ctx context.Context, sourceText string, opts fw.LookupOptions) ([]fw.TermMatch, error) {
	if sourceText == "" {
		return nil, nil
	}

	opts = fw.ApplyLookupDefaults(opts)
	var matches []fw.TermMatch
	lowerSource := strings.ToLower(sourceText)

	terms, err := tb.queryTermsByLocale(ctx, opts.SourceLocale, opts.Domains, opts.StatusFilter)
	if err != nil {
		return nil, err
	}

	for _, entry := range terms {
		searchIn := sourceText
		searchFor := entry.term.Text
		if !opts.CaseSensitive {
			searchIn = lowerSource
			searchFor = strings.ToLower(entry.term.Text)
		}

		offset := 0
		for {
			idx := strings.Index(searchIn[offset:], searchFor)
			if idx < 0 {
				break
			}
			pos := offset + idx
			matches = append(matches, fw.TermMatch{
				Concept:   entry.concept,
				Term:      entry.term,
				Score:     1.0,
				MatchType: model.MatchStrategyExact,
				Position:  model.TextRange{Start: pos, End: pos + len(searchFor)},
			})
			offset = pos + len(searchFor)
		}
	}

	slices.SortFunc(matches, func(a, b fw.TermMatch) int {
		if c := cmp.Compare(a.Position.Start, b.Position.Start); c != 0 {
			return c
		}
		return cmp.Compare(b.Position.End, a.Position.End)
	})

	return matches, nil
}

// Search performs a ranked full-text search across concepts and terms.
// Uses pg_trgm for term matching when a query is provided, falls back to LIKE.
func (tb *PostgresTermBase) Search(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]fw.Concept, int, error) {
	if query != "" {
		concepts, total, err := tb.pgSearchTrgm(ctx, query, sourceLocale, targetLocale, offset, limit)
		if err == nil {
			return concepts, total, nil
		}
	}
	return tb.pgSearchLike(ctx, query, sourceLocale, targetLocale, offset, limit)
}

func (tb *PostgresTermBase) pgSearchTrgm(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]fw.Concept, int, error) {
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

	var concepts []fw.Concept
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		if c, err := tb.scanConcept(ctx, id); err == nil {
			concepts = append(concepts, c)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return concepts, total, nil
}

func (tb *PostgresTermBase) pgSearchLike(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]fw.Concept, int, error) {
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

	var concepts []fw.Concept
	for _, id := range ids {
		if c, err := tb.scanConcept(ctx, id); err == nil {
			concepts = append(concepts, c)
		}
	}
	return concepts, total, nil
}

// SearchForStream performs a ranked full-text search with stream inheritance.
// Uses pg_trgm when a query is provided, falls back to LIKE.
// The streamChain is an ordered list of ancestor streams to search.
// Concepts from earlier streams take priority.
func (tb *PostgresTermBase) SearchForStream(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, stream string, streamChain []string, offset, limit int) ([]fw.Concept, int, error) {
	if query != "" {
		concepts, total, err := tb.pgSearchTrgmForStream(ctx, query, sourceLocale, targetLocale, stream, streamChain, offset, limit)
		if err == nil {
			return concepts, total, nil
		}
	}
	return tb.pgSearchLikeForStream(ctx, query, sourceLocale, targetLocale, stream, streamChain, offset, limit)
}

func (tb *PostgresTermBase) pgSearchTrgmForStream(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, stream string, streamChain []string, offset, limit int) ([]fw.Concept, int, error) {
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

	var concepts []fw.Concept
	for _, id := range ids {
		if c, err := tb.scanConcept(ctx, id); err == nil {
			concepts = append(concepts, c)
		}
	}
	return concepts, total, nil
}

func (tb *PostgresTermBase) pgSearchLikeForStream(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, stream string, streamChain []string, offset, limit int) ([]fw.Concept, int, error) {
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

	var concepts []fw.Concept
	for _, id := range ids {
		if c, err := tb.scanConcept(ctx, id); err == nil {
			concepts = append(concepts, c)
		}
	}
	return concepts, total, nil
}

// Count returns the total number of concepts for this workspace.
func (tb *PostgresTermBase) Count(ctx context.Context) (int, error) {
	var count int
	if err := tb.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tb_concepts WHERE workspace_id = $1", tb.workspaceID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count concepts: %w", err)
	}
	return count, nil
}

// Concepts returns all concepts for this workspace.
func (tb *PostgresTermBase) Concepts(ctx context.Context) ([]fw.Concept, error) {
	rows, err := tb.db.QueryContext(ctx, "SELECT id FROM tb_concepts WHERE workspace_id = $1 ORDER BY id", tb.workspaceID)
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

	var concepts []fw.Concept
	for _, id := range ids {
		if c, err := tb.scanConcept(ctx, id); err == nil {
			concepts = append(concepts, c)
		}
	}
	return concepts, nil
}

// Close is a no-op for PostgresTermBase since the connection is shared.
func (tb *PostgresTermBase) Close() error {
	return nil
}

// --- internal helpers ---

func (tb *PostgresTermBase) scanConcept(ctx context.Context, id string) (fw.Concept, error) {
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

	if propsJSON != nil && *propsJSON != "" {
		_ = json.Unmarshal([]byte(*propsJSON), &c.Properties)
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

type pgTermWithConcept struct {
	concept fw.Concept
	term    fw.Term
}

func (tb *PostgresTermBase) queryExactTerms(ctx context.Context, sourceText string, opts fw.LookupOptions) ([]fw.TermMatch, error) {
	searchText := sourceText
	column := "t.text"
	if !opts.CaseSensitive {
		searchText = strings.ToLower(sourceText)
		column = "t.text_lower"
	}

	q := fmt.Sprintf(`
		SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note
		FROM tb_terms t
		WHERE t.workspace_id = $1 AND %s = $2 AND t.locale = $3
	`, column)

	rows, err := tb.db.QueryContext(ctx, q, tb.workspaceID, searchText, string(opts.SourceLocale))
	if err != nil {
		return nil, fmt.Errorf("query exact terms: %w", err)
	}
	defer rows.Close()

	return tb.scanTermMatches(ctx, rows, 1.0, model.MatchStrategyExact, opts), nil
}

func (tb *PostgresTermBase) queryNormalizedTerms(ctx context.Context, normalizedSource string, opts fw.LookupOptions) ([]fw.TermMatch, error) {
	rows, err := tb.db.QueryContext(ctx, `
		SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note
		FROM tb_terms t
		WHERE t.workspace_id = $1 AND t.text_lower = $2 AND t.locale = $3
	`, tb.workspaceID, normalizedSource, string(opts.SourceLocale))
	if err != nil {
		return nil, fmt.Errorf("query normalized terms: %w", err)
	}
	defer rows.Close()

	return tb.scanTermMatches(ctx, rows, 0.95, model.MatchStrategyNormalized, opts), nil
}

func (tb *PostgresTermBase) queryFuzzyTerms(ctx context.Context, normalizedSource string, opts fw.LookupOptions) ([]fw.TermMatch, error) {
	// Try pg_trgm candidate retrieval first, fall back to full scan.
	matches, err := tb.queryFuzzyTrigramCandidates(ctx, normalizedSource, opts)
	if err != nil {
		return nil, err
	}
	if matches != nil {
		return matches, nil
	}
	return tb.queryFuzzyFullScan(ctx, normalizedSource, opts)
}

func (tb *PostgresTermBase) queryFuzzyTrigramCandidates(ctx context.Context, normalizedSource string, opts fw.LookupOptions) ([]fw.TermMatch, error) {
	// Use pg_trgm similarity operator (%) with GIN index.
	rows, err := tb.db.QueryContext(ctx, `
		SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note
		FROM tb_terms t
		WHERE t.workspace_id = $1 AND t.locale = $2 AND t.text_lower % $3
		LIMIT 200
	`, tb.workspaceID, string(opts.SourceLocale), normalizedSource)
	if err != nil {
		// pg_trgm unavailable; signal fallback to the full scan (not an error).
		return nil, nil
	}
	defer rows.Close()

	return tb.pgScoreFuzzyCandidates(ctx, rows, normalizedSource, opts), nil
}

func (tb *PostgresTermBase) queryFuzzyFullScan(ctx context.Context, normalizedSource string, opts fw.LookupOptions) ([]fw.TermMatch, error) {
	keyLen := len([]rune(normalizedSource))
	minLen := int(float64(keyLen) * 0.7)
	maxLen := int(float64(keyLen) * 1.3)
	if minLen < 0 {
		minLen = 0
	}

	rows, err := tb.db.QueryContext(ctx, `
		SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note
		FROM tb_terms t
		WHERE t.workspace_id = $1 AND t.locale = $2 AND LENGTH(t.text_lower) BETWEEN $3 AND $4
		LIMIT 500
	`, tb.workspaceID, string(opts.SourceLocale), minLen, maxLen)
	if err != nil {
		return nil, fmt.Errorf("fuzzy full scan: %w", err)
	}
	defer rows.Close()

	return tb.pgScoreFuzzyCandidates(ctx, rows, normalizedSource, opts), nil
}

func (tb *PostgresTermBase) pgScoreFuzzyCandidates(ctx context.Context, rows interface {
	Next() bool
	Scan(...any) error
}, normalizedSource string, opts fw.LookupOptions) []fw.TermMatch {
	type fuzzyCandidate struct {
		row   pgScanTermRow
		score float64
	}
	var candidates []fuzzyCandidate
	for rows.Next() {
		var r pgScanTermRow
		if err := rows.Scan(&r.conceptID, &r.text, &r.locale, &r.status, &r.pos, &r.gender, &r.note); err != nil {
			continue
		}

		score := sievepen.LevenshteinRatio(normalizedSource, fw.NormalizeTerm(r.text))
		if score >= opts.MinScore && fw.MatchesStatus(model.TermStatus(r.status), opts.StatusFilter) {
			candidates = append(candidates, fuzzyCandidate{row: r, score: score})
		}
	}

	var matches []fw.TermMatch
	for _, c := range candidates {
		concept, err := tb.scanConcept(ctx, c.row.conceptID)
		if err != nil {
			continue
		}
		matches = append(matches, fw.TermMatch{
			Concept: concept,
			Term: fw.Term{
				Text:         c.row.text,
				Locale:       model.LocaleID(c.row.locale),
				Status:       model.TermStatus(c.row.status),
				PartOfSpeech: c.row.pos,
				Gender:       c.row.gender,
				Note:         c.row.note,
			},
			Score:     c.score,
			MatchType: model.MatchStrategyFuzzy,
		})
	}

	return matches
}

func (tb *PostgresTermBase) queryTermsByLocale(ctx context.Context, locale model.LocaleID, domains []string, statusFilter []model.TermStatus) ([]pgTermWithConcept, error) {
	where := "t.workspace_id = $1 AND t.locale = $2"
	args := []any{tb.workspaceID, string(locale)}
	argN := 3

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
		SELECT c.id, c.domain, c.definition, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note
		FROM tb_terms t JOIN tb_concepts c ON t.workspace_id = c.workspace_id AND t.concept_id = c.id
		WHERE %s
	`, where), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []pgTermWithConcept
	for rows.Next() {
		var cID, domain, definition, text, loc, status, pos, gender, note string
		if err := rows.Scan(&cID, &domain, &definition, &text, &loc, &status, &pos, &gender, &note); err != nil {
			continue
		}
		results = append(results, pgTermWithConcept{
			concept: fw.Concept{ID: cID, Domain: domain, Definition: definition},
			term: fw.Term{
				Text:         text,
				Locale:       model.LocaleID(loc),
				Status:       model.TermStatus(status),
				PartOfSpeech: pos,
				Gender:       gender,
				Note:         note,
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
}

func (tb *PostgresTermBase) scanTermMatches(ctx context.Context, rows interface {
	Next() bool
	Scan(...any) error
}, score float64, matchType model.MatchStrategy, opts fw.LookupOptions) []fw.TermMatch {
	var raw []pgScanTermRow
	for rows.Next() {
		var r pgScanTermRow
		if err := rows.Scan(&r.conceptID, &r.text, &r.locale, &r.status, &r.pos, &r.gender, &r.note); err != nil {
			continue
		}
		if fw.MatchesStatus(model.TermStatus(r.status), opts.StatusFilter) {
			raw = append(raw, r)
		}
	}

	var matches []fw.TermMatch
	for _, r := range raw {
		concept, err := tb.scanConcept(ctx, r.conceptID)
		if err != nil {
			continue
		}
		matches = append(matches, fw.TermMatch{
			Concept: concept,
			Term: fw.Term{
				Text:         r.text,
				Locale:       model.LocaleID(r.locale),
				Status:       model.TermStatus(r.status),
				PartOfSpeech: r.pos,
				Gender:       r.gender,
				Note:         r.note,
			},
			Score:     score,
			MatchType: matchType,
		})
	}
	return matches
}

func nullableString(b []byte) *string {
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	return &s
}
