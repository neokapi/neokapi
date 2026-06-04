package termbase

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/sievepen"
)

// ErrConceptIDRequired is returned when a concept is added without an ID.
var ErrConceptIDRequired = errors.New("concept ID is required")

// SQLiteTermBase is a persistent termbase backed by SQLite.
type SQLiteTermBase struct {
	db *storage.DB
}

// NewSQLiteTermBase opens (or creates) a SQLite-backed termbase.
// Use ":memory:" for an in-memory database (useful for testing).
func NewSQLiteTermBase(dbPath string) (*SQLiteTermBase, error) {
	db, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := storage.Migrate(db, "termbase_migrations", tbMigrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	return &SQLiteTermBase{db: db}, nil
}

// NewSQLiteTermBaseFromDB creates a SQLiteTermBase from an already-opened database.
// This allows sharing a single DB file across TM and termbase.
func NewSQLiteTermBaseFromDB(db *storage.DB) (*SQLiteTermBase, error) {
	if err := storage.Migrate(db, "termbase_migrations", tbMigrations); err != nil {
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return &SQLiteTermBase{db: db}, nil
}

var tbMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "termbase schema with project/stream support and FTS5 indexes",
		// The portable FTS path is the tb_terms_trigram contentless index
		// below: it is populated by the tb_terms_trigram_a{i,d,u} triggers and
		// queried via MATCH from the Search/Lookup paths. The trigram tokenizer
		// is identical across cgo and no-cgo builds, so a .db created by one
		// binary stays queryable by the other.
		SQL: `
		CREATE TABLE IF NOT EXISTS tb_concepts (
			id          TEXT PRIMARY KEY,
			project_id  TEXT NOT NULL DEFAULT '',
			stream      TEXT NOT NULL DEFAULT '',
			domain      TEXT NOT NULL DEFAULT '',
			definition  TEXT NOT NULL DEFAULT '',
			properties  TEXT,
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_tb_concepts_stream ON tb_concepts(stream);

		CREATE TABLE IF NOT EXISTS tb_terms (
			id            INTEGER PRIMARY KEY AUTOINCREMENT,
			concept_id    TEXT NOT NULL REFERENCES tb_concepts(id) ON DELETE CASCADE,
			text          TEXT NOT NULL,
			text_lower    TEXT NOT NULL,
			locale        TEXT NOT NULL,
			status        TEXT NOT NULL DEFAULT 'approved',
			part_of_speech TEXT NOT NULL DEFAULT '',
			gender        TEXT NOT NULL DEFAULT '',
			note          TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_tb_terms_concept ON tb_terms(concept_id);
		CREATE INDEX IF NOT EXISTS idx_tb_terms_locale ON tb_terms(locale);
		CREATE INDEX IF NOT EXISTS idx_tb_terms_text ON tb_terms(text_lower, locale);

		-- FTS5 trigram index for fuzzy term matching.
		CREATE VIRTUAL TABLE IF NOT EXISTS tb_terms_trigram USING fts5(
			text_lower,
			content='tb_terms', content_rowid='id',
			tokenize='trigram'
		);

		CREATE TRIGGER tb_terms_trigram_ai AFTER INSERT ON tb_terms BEGIN
			INSERT INTO tb_terms_trigram(rowid, text_lower) VALUES (new.id, new.text_lower);
		END;
		CREATE TRIGGER tb_terms_trigram_ad AFTER DELETE ON tb_terms BEGIN
			INSERT INTO tb_terms_trigram(tb_terms_trigram, rowid, text_lower)
			VALUES ('delete', old.id, old.text_lower);
		END;
		CREATE TRIGGER tb_terms_trigram_au AFTER UPDATE ON tb_terms BEGIN
			INSERT INTO tb_terms_trigram(tb_terms_trigram, rowid, text_lower)
			VALUES ('delete', old.id, old.text_lower);
			INSERT INTO tb_terms_trigram(rowid, text_lower) VALUES (new.id, new.text_lower);
		END;
		`,
	},
	{
		Version:     2,
		Description: "add source column to concepts and competitor_term column to terms",
		SQL: `
		ALTER TABLE tb_concepts ADD COLUMN source TEXT NOT NULL DEFAULT 'terminology';
		ALTER TABLE tb_terms ADD COLUMN competitor_term INTEGER NOT NULL DEFAULT 0;
		`,
	},
}

// AddConcept inserts or updates a concept with all its terms using an empty stream.
func (tb *SQLiteTermBase) AddConcept(concept Concept) error {
	return tb.AddConceptWithStream(concept, "")
}

// AddConceptWithStream inserts or updates a concept associated with a stream.
func (tb *SQLiteTermBase) AddConceptWithStream(concept Concept, stream string) error {
	if concept.ID == "" {
		return ErrConceptIDRequired
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

	tx, err := tb.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	source := concept.Source
	if source == "" {
		source = TermSourceTerminology
	}

	_, err = tx.Exec(`
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

	_, err = tx.Exec("DELETE FROM tb_terms WHERE concept_id = ?", concept.ID)
	if err != nil {
		return fmt.Errorf("delete old terms: %w", err)
	}

	for _, term := range concept.Terms {
		competitorInt := 0
		if term.CompetitorTerm {
			competitorInt = 1
		}
		_, err = tx.Exec(`
			INSERT INTO tb_terms (concept_id, text, text_lower, locale, status, part_of_speech, gender, note, competitor_term)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		`, concept.ID, term.Text, strings.ToLower(term.Text),
			string(term.Locale), string(term.Status),
			term.PartOfSpeech, term.Gender, term.Note, competitorInt)
		if err != nil {
			return fmt.Errorf("insert term: %w", err)
		}
	}

	return tx.Commit()
}

// GetConcept retrieves a concept by ID.
func (tb *SQLiteTermBase) GetConcept(id string) (Concept, bool) {
	concept, err := tb.scanConcept(id)
	if err != nil {
		return Concept{}, false
	}
	return concept, true
}

// DeleteConcept removes a concept by ID.
func (tb *SQLiteTermBase) DeleteConcept(id string) error {
	result, err := tb.db.Exec("DELETE FROM tb_concepts WHERE id = ?", id)
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

// Lookup finds terms matching the source text.
func (tb *SQLiteTermBase) Lookup(sourceText string, opts LookupOptions) []TermMatch {
	if sourceText == "" {
		return nil
	}

	opts = ApplyLookupDefaults(opts)
	modeEnabled := MatchModesEnabled(opts.MatchModes)
	normalizedSource := NormalizeTerm(sourceText)
	var matches []TermMatch

	if modeEnabled[model.MatchStrategyExact] {
		matches = append(matches, tb.queryExactTerms(sourceText, opts)...)
	}

	if modeEnabled[model.MatchStrategyNormalized] && len(matches) == 0 {
		matches = append(matches, tb.queryNormalizedTerms(normalizedSource, opts)...)
	}

	if modeEnabled[model.MatchStrategyFuzzy] && len(matches) == 0 {
		matches = append(matches, tb.queryFuzzyTerms(normalizedSource, opts)...)
	}

	slices.SortFunc(matches, func(a, b TermMatch) int {
		return cmp.Compare(b.Score, a.Score)
	})

	return matches
}

// LookupAll finds all terms appearing in the given text.
func (tb *SQLiteTermBase) LookupAll(sourceText string, opts LookupOptions) []TermMatch {
	if sourceText == "" {
		return nil
	}

	opts = ApplyLookupDefaults(opts)
	var matches []TermMatch
	lowerSource := strings.ToLower(sourceText)

	terms, err := tb.queryTermsByLocale(opts.SourceLocale, opts.Domains, opts.StatusFilter, opts)
	if err != nil {
		return nil
	}

	type matchKey struct {
		text string
		pos  int
	}
	seen := make(map[matchKey]int)

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
			key := matchKey{text: searchFor, pos: pos}

			m := TermMatch{
				Concept:   entry.concept,
				Term:      entry.term,
				Score:     1.0,
				MatchType: model.MatchStrategyExact,
				Position:  model.TextRange{Start: pos, End: pos + len(searchFor)},
			}

			if existingIdx, exists := seen[key]; exists {
				if opts.ProjectID != "" && entry.concept.ProjectID == opts.ProjectID &&
					matches[existingIdx].Concept.ProjectID != opts.ProjectID {
					matches[existingIdx] = m
				}
			} else {
				seen[key] = len(matches)
				matches = append(matches, m)
			}

			offset = pos + len(searchFor)
		}
	}

	slices.SortFunc(matches, func(a, b TermMatch) int {
		if c := cmp.Compare(a.Position.Start, b.Position.Start); c != 0 {
			return c
		}
		return cmp.Compare(b.Position.End, a.Position.End)
	})

	return matches
}

// Search performs a ranked full-text search across concepts and terms.
func (tb *SQLiteTermBase) Search(query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]Concept, int) {
	if query != "" {
		concepts, total, err := tb.searchFTS5(query, sourceLocale, targetLocale, offset, limit)
		if err == nil {
			return concepts, total
		}
	}
	return tb.searchLike(query, sourceLocale, targetLocale, offset, limit)
}

func (tb *SQLiteTermBase) searchFTS5(query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]Concept, int, error) {
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
	if err := tb.db.QueryRow(countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	q := `SELECT DISTINCT t.concept_id
		FROM tb_terms t
		JOIN tb_concepts c ON t.concept_id = c.id
		WHERE t.id IN (SELECT rowid FROM tb_terms_trigram WHERE tb_terms_trigram MATCH ?)` +
		localeWhere + ` ORDER BY c.updated_at DESC LIMIT ? OFFSET ?`
	args := append([]any{trigramQuery}, localeArgs...)
	args = append(args, limit, offset)

	rows, err := tb.db.Query(q, args...)
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

	var concepts []Concept
	for _, id := range ids {
		if c, err := tb.scanConcept(id); err == nil {
			concepts = append(concepts, c)
		}
	}
	return concepts, total, nil
}

func (tb *SQLiteTermBase) searchLike(query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]Concept, int) {
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
	_ = tb.db.QueryRow("SELECT COUNT(DISTINCT c.id) FROM tb_concepts c WHERE "+where, countArgs...).Scan(&total)

	q := fmt.Sprintf(`SELECT DISTINCT c.id FROM tb_concepts c WHERE %s ORDER BY c.updated_at DESC LIMIT ? OFFSET ?`, where)
	args = append(args, limit, offset)
	rows, err := tb.db.Query(q, args...)
	if err != nil {
		return nil, total
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
		return nil, total
	}

	var concepts []Concept
	for _, id := range ids {
		if c, err := tb.scanConcept(id); err == nil {
			concepts = append(concepts, c)
		}
	}
	return concepts, total
}

// SearchForStream performs a ranked full-text search with stream inheritance.
func (tb *SQLiteTermBase) SearchForStream(query string, sourceLocale, targetLocale model.LocaleID, stream string, streamChain []string, offset, limit int) ([]Concept, int) {
	if query != "" {
		concepts, total, err := tb.searchFTS5ForStream(query, sourceLocale, targetLocale, stream, streamChain, offset, limit)
		if err == nil {
			return concepts, total
		}
	}
	return tb.searchLikeForStream(query, sourceLocale, targetLocale, stream, streamChain, offset, limit)
}

func (tb *SQLiteTermBase) searchFTS5ForStream(query string, sourceLocale, targetLocale model.LocaleID, stream string, streamChain []string, offset, limit int) ([]Concept, int, error) {
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
	if err := tb.db.QueryRow("SELECT COUNT(DISTINCT c.id) FROM tb_concepts c WHERE "+where, countArgs...).Scan(&total); err != nil {
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
	rows, err := tb.db.Query(q, args...)
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

	var concepts []Concept
	for _, id := range ids {
		if c, err := tb.scanConcept(id); err == nil {
			concepts = append(concepts, c)
		}
	}
	return concepts, total, nil
}

func (tb *SQLiteTermBase) searchLikeForStream(query string, sourceLocale, targetLocale model.LocaleID, stream string, streamChain []string, offset, limit int) ([]Concept, int) {
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
	_ = tb.db.QueryRow("SELECT COUNT(DISTINCT c.id) FROM tb_concepts c WHERE "+where, countArgs...).Scan(&total)

	var caseExpr strings.Builder
	caseExpr.WriteString("CASE c.stream")
	for i, s := range streams {
		caseExpr.WriteString(fmt.Sprintf(" WHEN ? THEN %d", i))
		args = append(args, s)
	}
	caseExpr.WriteString(fmt.Sprintf(" ELSE %d END", len(streams)))

	q := fmt.Sprintf(`SELECT DISTINCT c.id FROM tb_concepts c WHERE %s ORDER BY %s, c.updated_at DESC LIMIT ? OFFSET ?`, where, caseExpr.String())
	args = append(args, limit, offset)
	rows, err := tb.db.Query(q, args...)
	if err != nil {
		return nil, total
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
		return nil, total
	}

	var concepts []Concept
	for _, id := range ids {
		if c, err := tb.scanConcept(id); err == nil {
			concepts = append(concepts, c)
		}
	}
	return concepts, total
}

// Count returns the total number of concepts.
func (tb *SQLiteTermBase) Count() int {
	var count int
	if err := tb.db.QueryRow("SELECT COUNT(*) FROM tb_concepts").Scan(&count); err != nil {
		slog.Warn("termbase count query failed", "error", err)
		return 0
	}
	return count
}

// Concepts returns all concepts.
func (tb *SQLiteTermBase) Concepts() []Concept {
	rows, err := tb.db.Query("SELECT id FROM tb_concepts ORDER BY id")
	if err != nil {
		return nil
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
		return nil
	}

	var concepts []Concept
	for _, id := range ids {
		if c, err := tb.scanConcept(id); err == nil {
			concepts = append(concepts, c)
		}
	}
	return concepts
}

// LocaleStat holds the term count for a single locale.
type LocaleStat struct {
	Locale string `json:"locale"`
	Count  int    `json:"count"`
}

// LocaleStats returns the number of terms grouped by locale.
func (tb *SQLiteTermBase) LocaleStats() []LocaleStat {
	rows, err := tb.db.Query(
		"SELECT locale, COUNT(*) FROM tb_terms GROUP BY locale ORDER BY COUNT(*) DESC",
	)
	if err != nil {
		slog.Warn("termbase locale stats query failed", "error", err)
		return nil
	}
	defer rows.Close()
	var stats []LocaleStat
	for rows.Next() {
		var s LocaleStat
		if err := rows.Scan(&s.Locale, &s.Count); err != nil {
			continue
		}
		stats = append(stats, s)
	}
	return stats
}

// ActivityStat holds the concept count for a date bucket.
type ActivityStat struct {
	Date  string `json:"date"` // YYYY-MM-DD
	Count int    `json:"count"`
}

// ActivityStats returns daily concept counts over time based on created_at.
func (tb *SQLiteTermBase) ActivityStats() []ActivityStat {
	rows, err := tb.db.Query(
		"SELECT DATE(created_at) AS day, COUNT(*) FROM tb_concepts GROUP BY day ORDER BY day",
	)
	if err != nil {
		slog.Warn("termbase activity stats query failed", "error", err)
		return nil
	}
	defer rows.Close()
	var stats []ActivityStat
	for rows.Next() {
		var s ActivityStat
		if err := rows.Scan(&s.Date, &s.Count); err != nil {
			continue
		}
		stats = append(stats, s)
	}
	return stats
}

// DB returns the underlying database for direct access (e.g., seeding).
func (tb *SQLiteTermBase) DB() *storage.DB { return tb.db }

// Close closes the database connection.
func (tb *SQLiteTermBase) Close() error {
	return tb.db.Close()
}

// --- internal helpers ---

func (tb *SQLiteTermBase) scanConcept(id string) (Concept, error) {
	var c Concept
	var propsJSON *string
	var createdStr, updatedStr, source string

	err := tb.db.QueryRow(`
		SELECT id, project_id, domain, definition, properties, source, created_at, updated_at
		FROM tb_concepts WHERE id = ?
	`, id).Scan(&c.ID, &c.ProjectID, &c.Domain, &c.Definition, &propsJSON, &source, &createdStr, &updatedStr)
	if err != nil {
		return Concept{}, err
	}

	c.Source = TermSource(source)
	c.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

	if propsJSON != nil && *propsJSON != "" {
		_ = json.Unmarshal([]byte(*propsJSON), &c.Properties)
	}

	rows, err := tb.db.Query(`
		SELECT text, locale, status, part_of_speech, gender, note, competitor_term
		FROM tb_terms WHERE concept_id = ?
	`, id)
	if err != nil {
		return c, fmt.Errorf("query terms for concept %s: %w", id, err)
	}
	defer rows.Close()

	for rows.Next() {
		var t Term
		var locale, status string
		var competitorInt int
		if err := rows.Scan(&t.Text, &locale, &status, &t.PartOfSpeech, &t.Gender, &t.Note, &competitorInt); err != nil {
			continue
		}
		t.Locale = model.LocaleID(locale)
		t.Status = model.TermStatus(status)
		t.CompetitorTerm = competitorInt != 0
		c.Terms = append(c.Terms, t)
	}
	if err := rows.Err(); err != nil {
		return c, fmt.Errorf("iterate terms: %w", err)
	}

	return c, nil
}

type termWithConcept struct {
	concept Concept
	term    Term
}

func (tb *SQLiteTermBase) queryExactTerms(sourceText string, opts LookupOptions) []TermMatch {
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
			SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note
			FROM tb_terms t JOIN tb_concepts c ON t.concept_id = c.id
			WHERE %s
		`, where)
	} else {
		q = fmt.Sprintf(`
			SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note
			FROM tb_terms t
			WHERE %s
		`, where)
	}

	rows, err := tb.db.Query(q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	return tb.scanTermMatches(rows, 1.0, model.MatchStrategyExact, opts)
}

func (tb *SQLiteTermBase) queryNormalizedTerms(normalizedSource string, opts LookupOptions) []TermMatch {
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
			SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note
			FROM tb_terms t JOIN tb_concepts c ON t.concept_id = c.id
			WHERE %s
		`, where)
	} else {
		q = fmt.Sprintf(`
			SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note
			FROM tb_terms t
			WHERE %s
		`, where)
	}

	rows, err := tb.db.Query(q, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	return tb.scanTermMatches(rows, 0.95, model.MatchStrategyNormalized, opts)
}

func (tb *SQLiteTermBase) queryFuzzyTerms(normalizedSource string, opts LookupOptions) []TermMatch {
	matches := tb.queryFuzzyTrigramCandidates(normalizedSource, opts)
	if matches != nil {
		return matches
	}
	return tb.queryFuzzyFullScan(normalizedSource, opts)
}

func (tb *SQLiteTermBase) queryFuzzyTrigramCandidates(normalizedSource string, opts LookupOptions) []TermMatch {
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
			SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note
			FROM tb_terms t JOIN tb_concepts c ON t.concept_id = c.id
			WHERE %s LIMIT 200
		`, where)
	} else {
		q = fmt.Sprintf(`
			SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note
			FROM tb_terms t
			WHERE %s LIMIT 200
		`, where)
	}

	rows, err := tb.db.Query(q, args...)
	if err != nil {
		// A query error is not the same as "no candidates"; surface it
		// instead of silently returning an empty match set.
		slog.Warn("termbase fuzzy candidate query failed", "error", err)
		return nil
	}
	defer rows.Close()

	return tb.scoreFuzzyCandidates(rows, normalizedSource, opts)
}

func (tb *SQLiteTermBase) queryFuzzyFullScan(normalizedSource string, opts LookupOptions) []TermMatch {
	keyLen := len([]rune(normalizedSource))
	minLen := int(float64(keyLen) * 0.7)
	maxLen := int(float64(keyLen) * 1.3)
	if minLen < 0 {
		minLen = 0
	}

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
			SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note
			FROM tb_terms t JOIN tb_concepts c ON t.concept_id = c.id
			WHERE %s LIMIT 500
		`, where)
	} else {
		q = fmt.Sprintf(`
			SELECT t.concept_id, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note
			FROM tb_terms t
			WHERE %s LIMIT 500
		`, where)
	}

	rows, err := tb.db.Query(q, args...)
	if err != nil {
		// A query error is not the same as "no candidates"; surface it
		// instead of silently returning an empty match set.
		slog.Warn("termbase fuzzy candidate query failed", "error", err)
		return nil
	}
	defer rows.Close()

	return tb.scoreFuzzyCandidates(rows, normalizedSource, opts)
}

func (tb *SQLiteTermBase) scoreFuzzyCandidates(rows interface {
	Next() bool
	Scan(...any) error
}, normalizedSource string, opts LookupOptions) []TermMatch {
	type fuzzyCandidate struct {
		row   scanTermRow
		score float64
	}
	var candidates []fuzzyCandidate
	for rows.Next() {
		var r scanTermRow
		if err := rows.Scan(&r.conceptID, &r.text, &r.locale, &r.status, &r.pos, &r.gender, &r.note); err != nil {
			continue
		}

		score := sievepen.LevenshteinRatio(normalizedSource, NormalizeTerm(r.text))
		if score >= opts.MinScore && MatchesStatus(model.TermStatus(r.status), opts.StatusFilter) {
			candidates = append(candidates, fuzzyCandidate{row: r, score: score})
		}
	}

	var matches []TermMatch
	for _, c := range candidates {
		concept, err := tb.scanConcept(c.row.conceptID)
		if err != nil {
			continue
		}
		matches = append(matches, TermMatch{
			Concept: concept,
			Term: Term{
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

func (tb *SQLiteTermBase) queryTermsByLocale(locale model.LocaleID, domains []string, statusFilter []model.TermStatus, opts LookupOptions) ([]termWithConcept, error) {
	where := "t.locale = ?"
	args := []any{string(locale)}

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

	rows, err := tb.db.Query(fmt.Sprintf(`
		SELECT c.id, c.project_id, c.domain, c.definition, c.source, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note, t.competitor_term
		FROM tb_terms t JOIN tb_concepts c ON t.concept_id = c.id
		WHERE %s
	`, where), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []termWithConcept
	for rows.Next() {
		var cID, projectID, domain, definition, source, text, loc, status, pos, gender, note string
		var competitorInt int
		if err := rows.Scan(&cID, &projectID, &domain, &definition, &source, &text, &loc, &status, &pos, &gender, &note, &competitorInt); err != nil {
			continue
		}
		results = append(results, termWithConcept{
			concept: Concept{ID: cID, ProjectID: projectID, Domain: domain, Definition: definition, Source: TermSource(source)},
			term: Term{
				Text:           text,
				Locale:         model.LocaleID(loc),
				Status:         model.TermStatus(status),
				PartOfSpeech:   pos,
				Gender:         gender,
				Note:           note,
				CompetitorTerm: competitorInt != 0,
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
}

func (tb *SQLiteTermBase) scanTermMatches(rows interface {
	Next() bool
	Scan(...any) error
}, score float64, matchType model.MatchStrategy, opts LookupOptions) []TermMatch {
	var raw []scanTermRow
	for rows.Next() {
		var r scanTermRow
		if err := rows.Scan(&r.conceptID, &r.text, &r.locale, &r.status, &r.pos, &r.gender, &r.note); err != nil {
			continue
		}
		if MatchesStatus(model.TermStatus(r.status), opts.StatusFilter) {
			raw = append(raw, r)
		}
	}

	var matches []TermMatch
	for _, r := range raw {
		concept, err := tb.scanConcept(r.conceptID)
		if err != nil {
			continue
		}
		matches = append(matches, TermMatch{
			Concept: concept,
			Term: Term{
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
