package terms

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/graph"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/storage"
	termsschema "github.com/neokapi/neokapi/terms/schema"
)

// ErrConceptIDRequired is returned when a concept is added without an ID.
var ErrConceptIDRequired = errors.New("concept ID is required")

// migrationsTable names the bookkeeping table that records which schema
// migrations this database has already applied.
//
// It keeps the historical "termbase" spelling on purpose. The name is written
// into every terms database ever created, so it is persisted state, not an
// internal identifier. Renaming it would make Migrate find no bookkeeping table
// in an existing database, conclude that nothing had been applied, and replay
// every migration against a populated schema. The package is called `terms` and
// the prose says "terms"; this one string stays as it is until there is a
// migration that renames the table deliberately.
const migrationsTable = "termbase_migrations"

// SQLiteStore is a persistent terms store backed by SQLite.
type SQLiteStore struct {
	db *storage.DB
}

// NewSQLiteStore opens (or creates) a SQLite-backed terms store.
// Use ":memory:" for an in-memory database (useful for testing).
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := storage.Migrate(db, migrationsTable, tbMigrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// NewSQLiteStoreFromDB creates a SQLiteStore from an already-opened database.
// This allows sharing a single DB file across content memory and terms.
func NewSQLiteStoreFromDB(db *storage.DB) (*SQLiteStore, error) {
	if err := storage.Migrate(db, migrationsTable, tbMigrations); err != nil {
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

// tbMigrations is the framework SQLite terms schema. Each migration renders
// from the shared descriptors in terms/schema (byte-identical to the
// historical hand-written DDL — golden-tested), so the SQLite and Postgres
// backends cannot drift. The portable FTS path is the tb_terms_trigram
// contentless index (populated by the tb_terms_trigram_a{i,d,u} triggers,
// queried via MATCH from the Search/Lookup paths): its trigram tokenizer is
// identical across cgo and no-cgo builds, so a .db created by one binary stays
// queryable by the other.
var tbMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "terms schema with project/stream support and FTS5 indexes",
		SQL:         termsschema.RenderTermsSQLiteV1(),
	},
	{
		Version:     2,
		Description: "add source column to concepts and competitor_term column to terms",
		SQL:         termsschema.RenderTermsSQLiteV2(),
	},
	{
		Version:     3,
		Description: "persisted concept relations and term validity columns",
		SQL:         termsschema.RenderTermsSQLiteV3(),
	},
}

// validityToColumns flattens a validity into its three column values:
// nullable RFC3339 bounds and a JSON object of tags ('{}' when empty).
func validityToColumns(v *graph.Validity) (validFrom, validTo *string, tags string) {
	tags = "{}"
	if v == nil {
		return nil, nil, tags
	}
	if v.ValidFrom != nil {
		s := v.ValidFrom.Format(time.RFC3339Nano)
		validFrom = &s
	}
	if v.ValidTo != nil {
		s := v.ValidTo.Format(time.RFC3339Nano)
		validTo = &s
	}
	if len(v.Tags) > 0 {
		if b, err := json.Marshal(v.Tags); err == nil {
			tags = string(b)
		}
	}
	return validFrom, validTo, tags
}

// validityFromColumns rebuilds a validity from its column values. An entirely
// unbounded, tagless validity round-trips to nil (semantically identical:
// it matches every scope).
func validityFromColumns(validFrom, validTo sql.NullString, tags string) *graph.Validity {
	var v graph.Validity
	if validFrom.Valid && validFrom.String != "" {
		if t, err := time.Parse(time.RFC3339Nano, validFrom.String); err == nil {
			v.ValidFrom = &t
		}
	}
	if validTo.Valid && validTo.String != "" {
		if t, err := time.Parse(time.RFC3339Nano, validTo.String); err == nil {
			v.ValidTo = &t
		}
	}
	if tags != "" && tags != "{}" {
		// Best-effort, matching the nullable validFrom/validTo parses above:
		// validity tags are optional relation metadata, so a malformed blob
		// degrades to no tags rather than failing the read. validityFromColumns
		// returns no error by design; mandatory columns (properties, created_at,
		// updated_at) are propagated by the row scanners instead.
		_ = json.Unmarshal([]byte(tags), &v.Tags)
	}
	if v.ValidFrom == nil && v.ValidTo == nil && len(v.Tags) == 0 {
		return nil
	}
	return &v
}

// parseStoredTime parses an RFC3339 timestamp read back from one of our own
// rows. An empty string (a NULL or never-set column) is not an error — it
// yields the zero time; a non-empty but unparseable value is stored corruption
// and is returned so the caller can surface it rather than silently substitute
// the zero time.
func parseStoredTime(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, s)
}

// AddConcept inserts or updates a concept with all its terms using an empty stream.
func (tb *SQLiteStore) AddConcept(ctx context.Context, concept Concept) error {
	return tb.AddConceptWithStream(ctx, concept, "")
}

// AddConceptWithStream inserts or updates a concept associated with a stream.
func (tb *SQLiteStore) AddConceptWithStream(ctx context.Context, concept Concept, stream string) error {
	if concept.ID == "" {
		return ErrConceptIDRequired
	}
	if err := validateConceptTermStatuses(concept); err != nil {
		return err
	}
	concept = NormalizedConcept(concept)

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
		source = TermSourceTerminology
	}

	_, err = tx.ExecContext(ctx, `
		INSERT INTO tb_concepts (id, project_id, stream, domain, definition, properties, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_id = excluded.project_id,
			stream = excluded.stream,
			domain = excluded.domain,
			definition = excluded.definition,
			properties = excluded.properties,
			source = excluded.source,
			updated_at = excluded.updated_at
	`, concept.ID, concept.ProjectID, stream, concept.Domain, concept.Definition,
		nullableString(propsJSON), string(source),
		concept.CreatedAt.Format(time.RFC3339),
		concept.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("upsert concept: %w", err)
	}

	_, err = tx.ExecContext(ctx, "DELETE FROM tb_terms WHERE concept_id = ?", concept.ID)
	if err != nil {
		return fmt.Errorf("delete old terms: %w", err)
	}

	for _, term := range concept.Terms {
		competitorInt := 0
		if term.CompetitorTerm {
			competitorInt = 1
		}
		validFrom, validTo, tags := validityToColumns(term.Validity)
		_, err = tx.ExecContext(ctx, `
			INSERT INTO tb_terms (concept_id, text, text_lower, locale, status, part_of_speech, gender, note, competitor_term, valid_from, valid_to, tags)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, concept.ID, term.Text, strings.ToLower(term.Text),
			string(term.Locale), string(term.Status),
			term.PartOfSpeech, term.Gender, term.Note, competitorInt,
			validFrom, validTo, tags)
		if err != nil {
			return fmt.Errorf("insert term: %w", err)
		}
	}

	return tx.Commit()
}

// GetConcept retrieves a concept by ID.
func (tb *SQLiteStore) GetConcept(ctx context.Context, id string) (Concept, bool, error) {
	concept, err := tb.scanConcept(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Concept{}, false, nil
	}
	if err != nil {
		return Concept{}, false, err
	}
	return concept, true, nil
}

// DeleteConcept removes a concept by ID.
func (tb *SQLiteStore) DeleteConcept(ctx context.Context, id string) error {
	result, err := tb.db.ExecContext(ctx, "DELETE FROM tb_concepts WHERE id = ?", id)
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
func (tb *SQLiteStore) AddRelation(ctx context.Context, rel ConceptRelation) error {
	return tb.AddRelationWithStream(ctx, rel, "")
}

// AddRelationWithStream inserts or updates a relation associated with a stream.
func (tb *SQLiteStore) AddRelationWithStream(ctx context.Context, rel ConceptRelation, stream string) error {
	if err := ValidateRelation(rel); err != nil {
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
	validFrom, validTo, tags := validityToColumns(rel.Validity)

	// created_at is deliberately not updated on conflict: an upsert preserves
	// the original creation time, like AddConcept does for concepts.
	_, err := tb.db.ExecContext(ctx, `
		INSERT INTO tb_relations (id, source_id, target_id, relation, note, stream, valid_from, valid_to, tags, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			source_id = excluded.source_id,
			target_id = excluded.target_id,
			relation = excluded.relation,
			note = excluded.note,
			stream = excluded.stream,
			valid_from = excluded.valid_from,
			valid_to = excluded.valid_to,
			tags = excluded.tags
	`, rel.ID, rel.SourceID, rel.TargetID, rel.RelationType, rel.Note, stream,
		validFrom, validTo, tags, rel.CreatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("upsert relation: %w", err)
	}
	return nil
}

// requireConcept returns an error if the concept does not exist.
func (tb *SQLiteStore) requireConcept(ctx context.Context, role, id string) error {
	var n int
	if err := tb.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tb_concepts WHERE id = ?", id).Scan(&n); err != nil {
		return fmt.Errorf("check %s concept: %w", role, err)
	}
	if n == 0 {
		return fmt.Errorf("%s concept not found: %s", role, id)
	}
	return nil
}

// DeleteRelation removes a relation by ID.
func (tb *SQLiteStore) DeleteRelation(ctx context.Context, id string) error {
	result, err := tb.db.ExecContext(ctx, "DELETE FROM tb_relations WHERE id = ?", id)
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
func (tb *SQLiteStore) RelationsOf(ctx context.Context, conceptID string, scope *graph.Scope) ([]ConceptRelation, error) {
	rows, err := tb.db.QueryContext(ctx, `
		SELECT id, source_id, target_id, relation, note, valid_from, valid_to, tags, created_at
		FROM tb_relations WHERE source_id = ? OR target_id = ? ORDER BY id
	`, conceptID, conceptID)
	if err != nil {
		return nil, fmt.Errorf("query relations: %w", err)
	}
	defer rows.Close()
	return scanRelations(rows, scope)
}

// ListRelations returns all relations, filtered by the validity scope when one
// is given.
func (tb *SQLiteStore) ListRelations(ctx context.Context, scope *graph.Scope) ([]ConceptRelation, error) {
	notShadow, arg := NotShadowSQL("id", "?")
	rows, err := tb.db.QueryContext(ctx, `
		SELECT id, source_id, target_id, relation, note, valid_from, valid_to, tags, created_at
		FROM tb_relations WHERE `+notShadow+` ORDER BY id
	`, arg)
	if err != nil {
		return nil, fmt.Errorf("list relations: %w", err)
	}
	defer rows.Close()
	return scanRelations(rows, scope)
}

// RelationsForStream returns the relations touching the concept (either
// direction) whose stream is the given stream or one of its ancestors, with
// the same chain semantics as SearchForStream: relations from earlier streams
// in the chain sort first. When scope is non-nil, relations whose validity
// does not match the scope are filtered out.
func (tb *SQLiteStore) RelationsForStream(ctx context.Context, conceptID string, stream string, streamChain []string, scope *graph.Scope) ([]ConceptRelation, error) {
	streams := []string{stream}
	streams = append(streams, streamChain...)

	placeholders := make([]string, len(streams))
	args := []any{conceptID, conceptID}
	for i, s := range streams {
		placeholders[i] = "?"
		args = append(args, s)
	}

	var caseExpr strings.Builder
	caseExpr.WriteString("CASE stream")
	for i, s := range streams {
		caseExpr.WriteString(fmt.Sprintf(" WHEN ? THEN %d", i))
		args = append(args, s)
	}
	caseExpr.WriteString(fmt.Sprintf(" ELSE %d END", len(streams)))

	q := fmt.Sprintf(`
		SELECT id, source_id, target_id, relation, note, valid_from, valid_to, tags, created_at
		FROM tb_relations
		WHERE (source_id = ? OR target_id = ?) AND stream IN (%s)
		ORDER BY %s, id
	`, strings.Join(placeholders, ","), caseExpr.String())
	rows, err := tb.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query relations for stream: %w", err)
	}
	defer rows.Close()
	return scanRelations(rows, scope)
}

// scanRelations reads relation rows, rebuilding validity and applying the
// optional scope filter.
func scanRelations(rows *sql.Rows, scope *graph.Scope) ([]ConceptRelation, error) {
	var out []ConceptRelation
	for rows.Next() {
		var rel ConceptRelation
		var tags, createdStr string
		var validFrom, validTo sql.NullString
		if err := rows.Scan(&rel.ID, &rel.SourceID, &rel.TargetID, &rel.RelationType, &rel.Note, &validFrom, &validTo, &tags, &createdStr); err != nil {
			return nil, fmt.Errorf("scan relation: %w", err)
		}
		rel.Validity = validityFromColumns(validFrom, validTo, tags)
		var perr error
		if rel.CreatedAt, perr = parseStoredTime(createdStr); perr != nil {
			return nil, fmt.Errorf("relation %s: parse created_at: %w", rel.ID, perr)
		}
		if !MatchesScope(rel.Validity, scope) {
			continue
		}
		out = append(out, rel)
	}
	return out, rows.Err()
}

// Lookup finds terms matching the source text.
func (tb *SQLiteStore) Lookup(ctx context.Context, sourceText string, opts LookupOptions) ([]TermMatch, error) {
	return LookupTiered(ctx, sourceText, opts, TermCandidateSource{
		Exact:           tb.queryExactTerms,
		Normalized:      tb.queryNormalizedTerms,
		FuzzyCandidates: tb.queryFuzzyTerms,
		Concept:         tb.scanConcept,
	})
}

// LookupAll finds all terms appearing in the given text. The locale/domain/
// status/project/source filtering happens in SQL (queryTermsByLocale); the
// position scan, validity-scope filter and project-priority de-duplication live
// in the shared LookupAllTiered.
func (tb *SQLiteStore) LookupAll(ctx context.Context, sourceText string, opts LookupOptions) ([]TermMatch, error) {
	if sourceText == "" {
		return nil, nil
	}
	opts = ApplyLookupDefaults(opts)
	terms, err := tb.queryTermsByLocale(ctx, opts.SourceLocale, opts.Domains, opts.StatusFilter, opts)
	if err != nil {
		return nil, err
	}
	return LookupAllTiered(sourceText, opts, terms), nil
}

// Search performs a ranked full-text search across concepts and terms.
func (tb *SQLiteStore) Search(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]Concept, int, error) {
	sourceLocale, targetLocale = model.NormalizeLocale(sourceLocale), model.NormalizeLocale(targetLocale)
	if query != "" {
		concepts, total, err := tb.searchFTS5(ctx, query, sourceLocale, targetLocale, offset, limit)
		if err == nil {
			return concepts, total, nil
		}
	}
	return tb.searchLike(ctx, query, sourceLocale, targetLocale, offset, limit)
}

func (tb *SQLiteStore) searchFTS5(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]Concept, int, error) {
	trigramQuery := `"` + strings.ReplaceAll(query, `"`, `""`) + `"`

	localeWhere := ""
	var localeArgs []any
	if sourceLocale != "" {
		localeWhere += " AND c.id IN (SELECT concept_id FROM tb_terms WHERE locale = ?)"
		localeArgs = append(localeArgs, string(sourceLocale))
	}
	if targetLocale != "" {
		localeWhere += " AND c.id IN (SELECT concept_id FROM tb_terms WHERE locale = ?)"
		localeArgs = append(localeArgs, string(targetLocale))
	}

	countQ := `SELECT COUNT(DISTINCT t.concept_id)
		FROM tb_terms t
		JOIN tb_concepts c ON t.concept_id = c.id
		WHERE t.id IN (SELECT rowid FROM tb_terms_trigram WHERE tb_terms_trigram MATCH ?)` + localeWhere
	countArgs := append([]any{trigramQuery}, localeArgs...)
	var total int
	if err := tb.db.QueryRowContext(ctx, countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	q := `SELECT DISTINCT t.concept_id
		FROM tb_terms t
		JOIN tb_concepts c ON t.concept_id = c.id
		WHERE t.id IN (SELECT rowid FROM tb_terms_trigram WHERE tb_terms_trigram MATCH ?)` +
		localeWhere + ` ORDER BY c.updated_at DESC LIMIT ? OFFSET ?`
	args := append([]any{trigramQuery}, localeArgs...)
	args = append(args, limit, offset)

	rows, err := tb.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	ids, err := scanConceptIDs(rows)
	if err != nil {
		return nil, 0, err
	}

	concepts, err := tb.loadConcepts(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	return concepts, total, nil
}

func (tb *SQLiteStore) searchLike(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]Concept, int, error) {
	where := "1=1"
	var args []any

	if query != "" {
		where += ` AND (LOWER(c.definition) LIKE ? OR LOWER(c.domain) LIKE ?
			OR c.id IN (SELECT concept_id FROM tb_terms WHERE text_lower LIKE ?))`
		pattern := "%" + strings.ToLower(query) + "%"
		args = append(args, pattern, pattern, pattern)
	}

	if sourceLocale != "" {
		where += " AND c.id IN (SELECT concept_id FROM tb_terms WHERE locale = ?)"
		args = append(args, string(sourceLocale))
	}
	if targetLocale != "" {
		where += " AND c.id IN (SELECT concept_id FROM tb_terms WHERE locale = ?)"
		args = append(args, string(targetLocale))
	}

	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	if err := tb.db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT c.id) FROM tb_concepts c WHERE "+where, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count search results: %w", err)
	}

	q := fmt.Sprintf(`SELECT DISTINCT c.id FROM tb_concepts c WHERE %s ORDER BY c.updated_at DESC LIMIT ? OFFSET ?`, where)
	args = append(args, limit, offset)
	rows, err := tb.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, total, fmt.Errorf("search concepts: %w", err)
	}
	defer rows.Close()

	ids, err := scanConceptIDs(rows)
	if err != nil {
		return nil, total, err
	}

	concepts, err := tb.loadConcepts(ctx, ids)
	if err != nil {
		return nil, total, err
	}
	return concepts, total, nil
}

// SearchForStream performs a ranked full-text search with stream inheritance.
func (tb *SQLiteStore) SearchForStream(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, stream string, streamChain []string, offset, limit int) ([]Concept, int, error) {
	sourceLocale, targetLocale = model.NormalizeLocale(sourceLocale), model.NormalizeLocale(targetLocale)
	if query != "" {
		concepts, total, err := tb.searchFTS5ForStream(ctx, query, sourceLocale, targetLocale, stream, streamChain, offset, limit)
		if err == nil {
			return concepts, total, nil
		}
	}
	return tb.searchLikeForStream(ctx, query, sourceLocale, targetLocale, stream, streamChain, offset, limit)
}

func (tb *SQLiteStore) searchFTS5ForStream(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, stream string, streamChain []string, offset, limit int) ([]Concept, int, error) {
	streams := []string{stream}
	streams = append(streams, streamChain...)

	placeholders := make([]string, len(streams))
	var args []any
	for i, s := range streams {
		placeholders[i] = "?"
		args = append(args, s)
	}

	trigramQuery := `"` + strings.ReplaceAll(query, `"`, `""`) + `"`
	where := "c.stream IN (" + strings.Join(placeholders, ",") + ")"
	where += ` AND c.id IN (SELECT t.concept_id FROM tb_terms t
		WHERE t.id IN (SELECT rowid FROM tb_terms_trigram WHERE tb_terms_trigram MATCH ?))`
	args = append(args, trigramQuery)

	if sourceLocale != "" {
		where += " AND c.id IN (SELECT concept_id FROM tb_terms WHERE locale = ?)"
		args = append(args, string(sourceLocale))
	}
	if targetLocale != "" {
		where += " AND c.id IN (SELECT concept_id FROM tb_terms WHERE locale = ?)"
		args = append(args, string(targetLocale))
	}

	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	if err := tb.db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT c.id) FROM tb_concepts c WHERE "+where, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	var caseExpr strings.Builder
	caseExpr.WriteString("CASE c.stream")
	for i, s := range streams {
		caseExpr.WriteString(fmt.Sprintf(" WHEN ? THEN %d", i))
		args = append(args, s)
	}
	caseExpr.WriteString(fmt.Sprintf(" ELSE %d END", len(streams)))

	q := fmt.Sprintf(`SELECT DISTINCT c.id FROM tb_concepts c WHERE %s ORDER BY %s, c.updated_at DESC LIMIT ? OFFSET ?`, where, caseExpr.String())
	args = append(args, limit, offset)
	rows, err := tb.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	ids, err := scanConceptIDs(rows)
	if err != nil {
		return nil, 0, err
	}

	concepts, err := tb.loadConcepts(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	return concepts, total, nil
}

func (tb *SQLiteStore) searchLikeForStream(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, stream string, streamChain []string, offset, limit int) ([]Concept, int, error) {
	streams := []string{stream}
	streams = append(streams, streamChain...)

	placeholders := make([]string, len(streams))
	var args []any
	for i, s := range streams {
		placeholders[i] = "?"
		args = append(args, s)
	}

	where := "c.stream IN (" + strings.Join(placeholders, ",") + ")"

	if query != "" {
		where += ` AND (LOWER(c.definition) LIKE ? OR LOWER(c.domain) LIKE ?
			OR c.id IN (SELECT concept_id FROM tb_terms WHERE text_lower LIKE ?))`
		pattern := "%" + strings.ToLower(query) + "%"
		args = append(args, pattern, pattern, pattern)
	}

	if sourceLocale != "" {
		where += " AND c.id IN (SELECT concept_id FROM tb_terms WHERE locale = ?)"
		args = append(args, string(sourceLocale))
	}
	if targetLocale != "" {
		where += " AND c.id IN (SELECT concept_id FROM tb_terms WHERE locale = ?)"
		args = append(args, string(targetLocale))
	}

	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	if err := tb.db.QueryRowContext(ctx, "SELECT COUNT(DISTINCT c.id) FROM tb_concepts c WHERE "+where, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count search results: %w", err)
	}

	var caseExpr strings.Builder
	caseExpr.WriteString("CASE c.stream")
	for i, s := range streams {
		caseExpr.WriteString(fmt.Sprintf(" WHEN ? THEN %d", i))
		args = append(args, s)
	}
	caseExpr.WriteString(fmt.Sprintf(" ELSE %d END", len(streams)))

	q := fmt.Sprintf(`SELECT DISTINCT c.id FROM tb_concepts c WHERE %s ORDER BY %s, c.updated_at DESC LIMIT ? OFFSET ?`, where, caseExpr.String())
	args = append(args, limit, offset)
	rows, err := tb.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, total, fmt.Errorf("search concepts: %w", err)
	}
	defer rows.Close()

	ids, err := scanConceptIDs(rows)
	if err != nil {
		return nil, total, err
	}

	concepts, err := tb.loadConcepts(ctx, ids)
	if err != nil {
		return nil, total, err
	}
	return concepts, total, nil
}

// Count returns the total number of concepts.
func (tb *SQLiteStore) Count(ctx context.Context) (int, error) {
	var count int
	if err := tb.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tb_concepts").Scan(&count); err != nil {
		return 0, fmt.Errorf("count concepts: %w", err)
	}
	return count, nil
}

// Concepts returns all concepts. Callers that write the result out — TBX
// export, JSON/CSV import diffing, concept sync — depend on it being all of
// them or an error, never a silently short list.
//
// "All" excludes the shadow namespace: a stream-scoped shadow is not a concept
// of the workspace, and every one of those callers writes what it gets
// somewhere durable.
func (tb *SQLiteStore) Concepts(ctx context.Context) ([]Concept, error) {
	notShadow, arg := NotShadowSQL("id", "?")
	rows, err := tb.db.QueryContext(ctx,
		"SELECT id FROM tb_concepts WHERE "+notShadow+" ORDER BY id", arg)
	if err != nil {
		return nil, fmt.Errorf("list concepts: %w", err)
	}
	defer rows.Close()

	ids, err := scanConceptIDs(rows)
	if err != nil {
		return nil, err
	}
	return tb.loadConcepts(ctx, ids)
}

// LocaleStat holds the term count for a single locale.
type LocaleStat struct {
	Locale string `json:"locale"`
	Count  int    `json:"count"`
}

// LocaleStats returns the number of terms grouped by locale.
func (tb *SQLiteStore) LocaleStats(ctx context.Context) ([]LocaleStat, error) {
	rows, err := tb.db.QueryContext(ctx,
		"SELECT locale, COUNT(*) FROM tb_terms GROUP BY locale ORDER BY COUNT(*) DESC",
	)
	if err != nil {
		return nil, fmt.Errorf("locale stats: %w", err)
	}
	defer rows.Close()
	var stats []LocaleStat
	for rows.Next() {
		var s LocaleStat
		if err := rows.Scan(&s.Locale, &s.Count); err != nil {
			return nil, fmt.Errorf("scan locale stat: %w", err)
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// ActivityStat holds the concept count for a date bucket.
type ActivityStat struct {
	Date  string `json:"date"` // YYYY-MM-DD
	Count int    `json:"count"`
}

// ActivityStats returns daily concept counts over time based on created_at.
func (tb *SQLiteStore) ActivityStats(ctx context.Context) ([]ActivityStat, error) {
	rows, err := tb.db.QueryContext(ctx,
		"SELECT DATE(created_at) AS day, COUNT(*) FROM tb_concepts GROUP BY day ORDER BY day",
	)
	if err != nil {
		return nil, fmt.Errorf("activity stats: %w", err)
	}
	defer rows.Close()
	var stats []ActivityStat
	for rows.Next() {
		var s ActivityStat
		if err := rows.Scan(&s.Date, &s.Count); err != nil {
			return nil, fmt.Errorf("scan activity stat: %w", err)
		}
		stats = append(stats, s)
	}
	return stats, rows.Err()
}

// DB returns the underlying database for direct access (e.g., seeding).
func (tb *SQLiteStore) DB() *storage.DB { return tb.db }

// Close closes the database connection.
func (tb *SQLiteStore) Close() error {
	return tb.db.Close()
}

// --- internal helpers ---

// scanConceptIDs collects a single-column result set of concept IDs.
func scanConceptIDs(rows *sql.Rows) ([]string, error) {
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan concept id: %w", err)
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

// loadConcepts materializes the given IDs in order. A concept that cannot be
// read fails the whole load: the callers turn the result into an export file or
// a sync payload, where a missing concept is indistinguishable from a deleted
// one.
func (tb *SQLiteStore) loadConcepts(ctx context.Context, ids []string) ([]Concept, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	concepts := make([]Concept, 0, len(ids))
	for _, id := range ids {
		c, err := tb.scanConcept(ctx, id)
		if err != nil {
			return nil, err
		}
		concepts = append(concepts, c)
	}
	return concepts, nil
}

func (tb *SQLiteStore) scanConcept(ctx context.Context, id string) (Concept, error) {
	var c Concept
	var propsJSON *string
	var createdStr, updatedStr, source string

	err := tb.db.QueryRowContext(ctx, `
		SELECT id, project_id, domain, definition, properties, source, created_at, updated_at
		FROM tb_concepts WHERE id = ?
	`, id).Scan(&c.ID, &c.ProjectID, &c.Domain, &c.Definition, &propsJSON, &source, &createdStr, &updatedStr)
	if err != nil {
		return Concept{}, fmt.Errorf("concept %s: %w", id, err)
	}

	c.Source = TermSource(source)
	if c.CreatedAt, err = parseStoredTime(createdStr); err != nil {
		return Concept{}, fmt.Errorf("concept %s: parse created_at: %w", c.ID, err)
	}
	if c.UpdatedAt, err = parseStoredTime(updatedStr); err != nil {
		return Concept{}, fmt.Errorf("concept %s: parse updated_at: %w", c.ID, err)
	}

	if propsJSON != nil && *propsJSON != "" {
		if err := json.Unmarshal([]byte(*propsJSON), &c.Properties); err != nil {
			return Concept{}, fmt.Errorf("concept %s: unmarshal properties: %w", c.ID, err)
		}
	}

	rows, err := tb.db.QueryContext(ctx, `
		SELECT text, locale, status, part_of_speech, gender, note, competitor_term, valid_from, valid_to, tags
		FROM tb_terms WHERE concept_id = ?
	`, id)
	if err != nil {
		return c, fmt.Errorf("query terms for concept %s: %w", id, err)
	}
	defer rows.Close()

	for rows.Next() {
		var t Term
		var locale, status, tags string
		var competitorInt int
		var validFrom, validTo sql.NullString
		if err := rows.Scan(&t.Text, &locale, &status, &t.PartOfSpeech, &t.Gender, &t.Note, &competitorInt, &validFrom, &validTo, &tags); err != nil {
			return c, fmt.Errorf("concept %s: scan term: %w", id, err)
		}
		t.Locale = model.LocaleID(locale)
		t.Status = model.TermStatus(status)
		t.CompetitorTerm = competitorInt != 0
		t.Validity = validityFromColumns(validFrom, validTo, tags)
		c.Terms = append(c.Terms, t)
	}
	if err := rows.Err(); err != nil {
		return c, fmt.Errorf("concept %s: iterate terms: %w", id, err)
	}

	return c, nil
}

func (tb *SQLiteStore) queryExactTerms(ctx context.Context, sourceText string, opts LookupOptions) ([]TermCandidate, error) {
	searchText := sourceText
	column := "t.text"
	if !opts.CaseSensitive {
		searchText = strings.ToLower(sourceText)
		column = "t.text_lower"
	}

	where := column + " = ? AND t.locale = ?"
	args := []any{searchText, string(opts.SourceLocale)}

	needsJoin := false
	switch opts.ProjectScope {
	case ProjectScopeOnly:
		where += " AND c.project_id = ?"
		args = append(args, opts.ProjectID)
		needsJoin = true
	case ProjectScopeExclude:
		where += " AND c.project_id != ?"
		args = append(args, opts.ProjectID)
		needsJoin = true
	}

	var sourceNeedsJoin bool
	where, args, sourceNeedsJoin = sourceFilterSQL(where, args, opts.SourceFilter)
	needsJoin = needsJoin || sourceNeedsJoin

	var q string
	if needsJoin {
		q = fmt.Sprintf(`
			SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note, t.valid_from, t.valid_to, t.tags
			FROM tb_terms t JOIN tb_concepts c ON t.concept_id = c.id
			WHERE %s
		`, where)
	} else {
		q = fmt.Sprintf(`
			SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note, t.valid_from, t.valid_to, t.tags
			FROM tb_terms t
			WHERE %s
		`, where)
	}

	rows, err := tb.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query exact terms: %w", err)
	}
	defer rows.Close()

	return scanTermCandidates(rows)
}

func (tb *SQLiteStore) queryNormalizedTerms(ctx context.Context, normalizedSource string, opts LookupOptions) ([]TermCandidate, error) {
	where := "t.text_lower = ? AND t.locale = ?"
	args := []any{normalizedSource, string(opts.SourceLocale)}

	needsJoin := false
	switch opts.ProjectScope {
	case ProjectScopeOnly:
		where += " AND c.project_id = ?"
		args = append(args, opts.ProjectID)
		needsJoin = true
	case ProjectScopeExclude:
		where += " AND c.project_id != ?"
		args = append(args, opts.ProjectID)
		needsJoin = true
	}

	var sourceNeedsJoin bool
	where, args, sourceNeedsJoin = sourceFilterSQL(where, args, opts.SourceFilter)
	needsJoin = needsJoin || sourceNeedsJoin

	var q string
	if needsJoin {
		q = fmt.Sprintf(`
			SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note, t.valid_from, t.valid_to, t.tags
			FROM tb_terms t JOIN tb_concepts c ON t.concept_id = c.id
			WHERE %s
		`, where)
	} else {
		q = fmt.Sprintf(`
			SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note, t.valid_from, t.valid_to, t.tags
			FROM tb_terms t
			WHERE %s
		`, where)
	}

	rows, err := tb.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query normalized terms: %w", err)
	}
	defer rows.Close()

	return scanTermCandidates(rows)
}

// queryFuzzyTerms returns the raw fuzzy candidate pool: the trigram pre-filter,
// falling back to a length-bounded full scan only when the trigram index yields
// no candidate rows. Levenshtein scoring and the MinScore gate live in the
// shared LookupTiered, so both backends score identically.
func (tb *SQLiteStore) queryFuzzyTerms(ctx context.Context, normalizedSource string, opts LookupOptions) ([]TermCandidate, error) {
	cands, err := tb.queryFuzzyTrigramCandidates(ctx, normalizedSource, opts)
	if err != nil {
		return nil, err
	}
	if len(cands) > 0 {
		return cands, nil
	}
	return tb.queryFuzzyFullScan(ctx, normalizedSource, opts)
}

func (tb *SQLiteStore) queryFuzzyTrigramCandidates(ctx context.Context, normalizedSource string, opts LookupOptions) ([]TermCandidate, error) {
	trigramQuery := `"` + strings.ReplaceAll(normalizedSource, `"`, `""`) + `"`

	where := `t.id IN (SELECT rowid FROM tb_terms_trigram WHERE tb_terms_trigram MATCH ?)
		AND t.locale = ?`
	args := []any{trigramQuery, string(opts.SourceLocale)}

	needsJoin := false
	switch opts.ProjectScope {
	case ProjectScopeOnly:
		where += " AND c.project_id = ?"
		args = append(args, opts.ProjectID)
		needsJoin = true
	case ProjectScopeExclude:
		where += " AND c.project_id != ?"
		args = append(args, opts.ProjectID)
		needsJoin = true
	}

	var sourceNeedsJoin bool
	where, args, sourceNeedsJoin = sourceFilterSQL(where, args, opts.SourceFilter)
	needsJoin = needsJoin || sourceNeedsJoin

	var q string
	if needsJoin {
		q = fmt.Sprintf(`
			SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note, t.valid_from, t.valid_to, t.tags
			FROM tb_terms t JOIN tb_concepts c ON t.concept_id = c.id
			WHERE %s LIMIT 200
		`, where)
	} else {
		q = fmt.Sprintf(`
			SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note, t.valid_from, t.valid_to, t.tags
			FROM tb_terms t
			WHERE %s LIMIT 200
		`, where)
	}

	rows, err := tb.db.QueryContext(ctx, q, args...)
	if err != nil {
		// A query error is not the same as "no candidates"; surface it
		// instead of silently returning an empty match set.
		return nil, fmt.Errorf("query fuzzy trigram candidates: %w", err)
	}
	defer rows.Close()

	return scanTermCandidates(rows)
}

func (tb *SQLiteStore) queryFuzzyFullScan(ctx context.Context, normalizedSource string, opts LookupOptions) ([]TermCandidate, error) {
	minLen, maxLen := FuzzyLengthWindow(len([]rune(normalizedSource)), opts.MinScore)

	where := "t.locale = ? AND LENGTH(t.text_lower) BETWEEN ? AND ?"
	args := []any{string(opts.SourceLocale), minLen, maxLen}

	needsJoin := false
	switch opts.ProjectScope {
	case ProjectScopeOnly:
		where += " AND c.project_id = ?"
		args = append(args, opts.ProjectID)
		needsJoin = true
	case ProjectScopeExclude:
		where += " AND c.project_id != ?"
		args = append(args, opts.ProjectID)
		needsJoin = true
	}

	var sourceNeedsJoin bool
	where, args, sourceNeedsJoin = sourceFilterSQL(where, args, opts.SourceFilter)
	needsJoin = needsJoin || sourceNeedsJoin

	var q string
	if needsJoin {
		q = fmt.Sprintf(`
			SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note, t.valid_from, t.valid_to, t.tags
			FROM tb_terms t JOIN tb_concepts c ON t.concept_id = c.id
			WHERE %s LIMIT 500
		`, where)
	} else {
		q = fmt.Sprintf(`
			SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note, t.valid_from, t.valid_to, t.tags
			FROM tb_terms t
			WHERE %s LIMIT 500
		`, where)
	}

	rows, err := tb.db.QueryContext(ctx, q, args...)
	if err != nil {
		// A query error is not the same as "no candidates"; surface it
		// instead of silently returning an empty match set.
		return nil, fmt.Errorf("query fuzzy full scan: %w", err)
	}
	defer rows.Close()

	return scanTermCandidates(rows)
}

func (tb *SQLiteStore) queryTermsByLocale(ctx context.Context, locale model.LocaleID, domains []string, statusFilter []model.TermStatus, opts LookupOptions) ([]LocaleTerm, error) {
	// This query is the terms half of every check. It names no stream, so it
	// must exclude the shadow namespace: a shadow written for one branch would
	// otherwise decide what the checks flag everywhere.
	notShadow, shadowArg := NotShadowSQL("c.id", "?")
	where := "t.locale = ? AND " + notShadow
	args := []any{string(locale), shadowArg}

	if len(domains) > 0 {
		placeholders := make([]string, len(domains))
		for i, d := range domains {
			placeholders[i] = "?"
			args = append(args, d)
		}
		where += " AND c.domain IN (" + strings.Join(placeholders, ",") + ")"
	}

	if len(statusFilter) > 0 {
		placeholders := make([]string, len(statusFilter))
		for i, s := range statusFilter {
			placeholders[i] = "?"
			args = append(args, string(s))
		}
		where += " AND t.status IN (" + strings.Join(placeholders, ",") + ")"
	}

	switch opts.ProjectScope {
	case ProjectScopeOnly:
		where += " AND c.project_id = ?"
		args = append(args, opts.ProjectID)
	case ProjectScopeExclude:
		where += " AND c.project_id != ?"
		args = append(args, opts.ProjectID)
	}

	where, args, _ = sourceFilterSQL(where, args, opts.SourceFilter)

	rows, err := tb.db.QueryContext(ctx, fmt.Sprintf(`
		SELECT c.id, c.project_id, c.domain, c.definition, c.source, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note, t.competitor_term, t.valid_from, t.valid_to, t.tags
		FROM tb_terms t JOIN tb_concepts c ON t.concept_id = c.id
		WHERE %s
		ORDER BY c.id, t.text
	`, where), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []LocaleTerm
	for rows.Next() {
		var cID, projectID, domain, definition, source, text, loc, status, pos, gender, note, tags string
		var competitorInt int
		var validFrom, validTo sql.NullString
		if err := rows.Scan(&cID, &projectID, &domain, &definition, &source, &text, &loc, &status, &pos, &gender, &note, &competitorInt, &validFrom, &validTo, &tags); err != nil {
			continue
		}
		validity := validityFromColumns(validFrom, validTo, tags)
		// The validity-scope filter now lives in the shared LookupAllTiered so
		// both backends honor it identically; this query keeps only the
		// SQL-expressible filters (locale/domain/status/project/source).
		results = append(results, LocaleTerm{
			Concept: Concept{ID: cID, ProjectID: projectID, Domain: domain, Definition: definition, Source: TermSource(source)},
			Term: Term{
				Text:           text,
				Locale:         model.LocaleID(loc),
				Status:         model.TermStatus(status),
				PartOfSpeech:   pos,
				Gender:         gender,
				Note:           note,
				CompetitorTerm: competitorInt != 0,
				Validity:       validity,
			},
		})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

type scanTermRow struct {
	conceptID, text, locale, status, pos, gender, note string
	validFrom, validTo                                 sql.NullString
	tags                                               string
}

// validity rebuilds the term validity from the scanned columns.
func (r scanTermRow) validity() *graph.Validity {
	return validityFromColumns(r.validFrom, r.validTo, r.tags)
}

// scanTermCandidates scans the shared 10-column term projection into raw
// candidates. Validity is reconstructed here (SQLite's TEXT RFC3339 codec); the
// shared LookupTiered applies the scope/status/score filters and hydrates the
// owning concept.
func scanTermCandidates(rows interface {
	Next() bool
	Scan(...any) error
}) ([]TermCandidate, error) {
	var out []TermCandidate
	for rows.Next() {
		var r scanTermRow
		if err := rows.Scan(&r.conceptID, &r.text, &r.locale, &r.status, &r.pos, &r.gender, &r.note, &r.validFrom, &r.validTo, &r.tags); err != nil {
			continue
		}
		out = append(out, TermCandidate{
			ConceptID: r.conceptID,
			Term: Term{
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
	return out, nil
}

// sourceFilterSQL appends a WHERE clause for SourceFilter and returns updated args.
// It requires tb_concepts to be joined as "c".
func sourceFilterSQL(where string, args []any, filter []TermSource) (string, []any, bool) {
	if len(filter) == 0 {
		return where, args, false
	}
	placeholders := make([]string, len(filter))
	for i, s := range filter {
		placeholders[i] = "?"
		src := s
		if src == "" {
			src = TermSourceTerminology
		}
		args = append(args, string(src))
	}
	where += " AND c.source IN (" + strings.Join(placeholders, ",") + ")"
	return where, args, true
}

func nullableString(b []byte) *string {
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	return &s
}
