package termbase

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gokapi/gokapi/bowrain/storage"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/sievepen"
	fw "github.com/gokapi/gokapi/core/termbase"
)

// SQLiteTermBase is a persistent termbase backed by SQLite.
type SQLiteTermBase struct {
	db *storage.DB
}

// NewSQLiteTermBase opens (or creates) a SQLite-backed termbase.
func NewSQLiteTermBase(dbPath string) (*SQLiteTermBase, error) {
	db, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := storage.Migrate(db, tbMigrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	return &SQLiteTermBase{db: db}, nil
}

var tbMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "termbase schema",
		SQL: `
		CREATE TABLE IF NOT EXISTS tb_concepts (
			id          TEXT PRIMARY KEY,
			domain      TEXT NOT NULL DEFAULT '',
			definition  TEXT NOT NULL DEFAULT '',
			properties  TEXT,
			created_at  TEXT NOT NULL,
			updated_at  TEXT NOT NULL
		);
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
		`,
	},
	{
		Version:     2,
		Description: "add project_id column to concepts",
		SQL:         `ALTER TABLE tb_concepts ADD COLUMN project_id TEXT NOT NULL DEFAULT '';`,
	},
}

// AddConcept inserts or updates a concept with all its terms.
func (tb *SQLiteTermBase) AddConcept(concept fw.Concept) error {
	if concept.ID == "" {
		return fmt.Errorf("concept ID is required")
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

	// Upsert concept.
	_, err = tx.Exec(`
		INSERT INTO tb_concepts (id, project_id, domain, definition, properties, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_id = excluded.project_id,
			domain = excluded.domain,
			definition = excluded.definition,
			properties = excluded.properties,
			updated_at = excluded.updated_at
	`, concept.ID, concept.ProjectID, concept.Domain, concept.Definition,
		nullableString(propsJSON),
		concept.CreatedAt.Format(time.RFC3339),
		concept.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("upsert concept: %w", err)
	}

	// Replace all terms for this concept.
	_, err = tx.Exec("DELETE FROM tb_terms WHERE concept_id = ?", concept.ID)
	if err != nil {
		return fmt.Errorf("delete old terms: %w", err)
	}

	for _, term := range concept.Terms {
		_, err = tx.Exec(`
			INSERT INTO tb_terms (concept_id, text, text_lower, locale, status, part_of_speech, gender, note)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		`, concept.ID, term.Text, strings.ToLower(term.Text),
			string(term.Locale), string(term.Status),
			term.PartOfSpeech, term.Gender, term.Note)
		if err != nil {
			return fmt.Errorf("insert term: %w", err)
		}
	}

	return tx.Commit()
}

// GetConcept retrieves a concept by ID.
func (tb *SQLiteTermBase) GetConcept(id string) (fw.Concept, bool) {
	concept, err := tb.scanConcept(id)
	if err != nil {
		return fw.Concept{}, false
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
func (tb *SQLiteTermBase) Lookup(sourceText string, opts fw.LookupOptions) []fw.TermMatch {
	if sourceText == "" {
		return nil
	}

	opts = fw.ApplyLookupDefaults(opts)
	modeEnabled := fw.MatchModesEnabled(opts.MatchModes)
	normalizedSource := fw.NormalizeTerm(sourceText)
	var matches []fw.TermMatch

	// Exact match (indexed).
	if modeEnabled[model.MatchStrategyExact] {
		matches = append(matches, tb.queryExactTerms(sourceText, opts)...)
	}

	// Normalized match (indexed via text_lower).
	if modeEnabled[model.MatchStrategyNormalized] && len(matches) == 0 {
		matches = append(matches, tb.queryNormalizedTerms(normalizedSource, opts)...)
	}

	// Fuzzy match (scan).
	if modeEnabled[model.MatchStrategyFuzzy] && len(matches) == 0 {
		matches = append(matches, tb.queryFuzzyTerms(normalizedSource, opts)...)
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	return matches
}

// LookupAll finds all terms appearing in the given text.
func (tb *SQLiteTermBase) LookupAll(sourceText string, opts fw.LookupOptions) []fw.TermMatch {
	if sourceText == "" {
		return nil
	}

	opts = fw.ApplyLookupDefaults(opts)
	var matches []fw.TermMatch
	lowerSource := strings.ToLower(sourceText)

	// Get all terms for the source locale.
	terms, err := tb.queryTermsByLocale(opts.SourceLocale, opts.Domains, opts.StatusFilter, opts)
	if err != nil {
		return nil
	}

	// Track seen text to deduplicate: project-scoped entries take precedence.
	type matchKey struct {
		text string
		pos  int
	}
	seen := make(map[matchKey]int) // key -> index in matches

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

			m := fw.TermMatch{
				Concept:   entry.concept,
				Term:      entry.term,
				Score:     1.0,
				MatchType: model.MatchStrategyExact,
				Position:  model.TextRange{Start: pos, End: pos + len(searchFor)},
			}

			if existingIdx, exists := seen[key]; exists {
				// Project-scoped concept takes precedence over workspace-scoped.
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

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Position.Start != matches[j].Position.Start {
			return matches[i].Position.Start < matches[j].Position.Start
		}
		return matches[i].Position.End > matches[j].Position.End
	})

	return matches
}

// Search performs a case-insensitive text search across concepts.
func (tb *SQLiteTermBase) Search(query, sourceLocale, targetLocale string, offset, limit int) ([]fw.Concept, int) {
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
		args = append(args, sourceLocale)
	}
	if targetLocale != "" {
		where += " AND c.id IN (SELECT concept_id FROM tb_terms WHERE locale = ?)"
		args = append(args, targetLocale)
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

	// Collect IDs first to release the connection before loading concepts.
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}

	var concepts []fw.Concept
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
	_ = tb.db.QueryRow("SELECT COUNT(*) FROM tb_concepts").Scan(&count)
	return count
}

// Concepts returns all concepts.
func (tb *SQLiteTermBase) Concepts() []fw.Concept {
	rows, err := tb.db.Query("SELECT id FROM tb_concepts ORDER BY id")
	if err != nil {
		return nil
	}
	defer rows.Close()

	// Collect IDs first to release the connection before loading concepts.
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			continue
		}
		ids = append(ids, id)
	}

	var concepts []fw.Concept
	for _, id := range ids {
		if c, err := tb.scanConcept(id); err == nil {
			concepts = append(concepts, c)
		}
	}
	return concepts
}

// Close closes the database connection.
func (tb *SQLiteTermBase) Close() error {
	return tb.db.Close()
}

// --- internal query helpers ---

func (tb *SQLiteTermBase) scanConcept(id string) (fw.Concept, error) {
	var c fw.Concept
	var propsJSON *string
	var createdStr, updatedStr string

	err := tb.db.QueryRow(`
		SELECT id, project_id, domain, definition, properties, created_at, updated_at
		FROM tb_concepts WHERE id = ?
	`, id).Scan(&c.ID, &c.ProjectID, &c.Domain, &c.Definition, &propsJSON, &createdStr, &updatedStr)
	if err != nil {
		return fw.Concept{}, err
	}

	c.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	c.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

	if propsJSON != nil && *propsJSON != "" {
		_ = json.Unmarshal([]byte(*propsJSON), &c.Properties)
	}

	// Load terms.
	rows, err := tb.db.Query(`
		SELECT text, locale, status, part_of_speech, gender, note
		FROM tb_terms WHERE concept_id = ?
	`, id)
	if err != nil {
		return c, nil
	}
	defer rows.Close()

	for rows.Next() {
		var t fw.Term
		var locale, status string
		if err := rows.Scan(&t.Text, &locale, &status, &t.PartOfSpeech, &t.Gender, &t.Note); err != nil {
			continue
		}
		t.Locale = model.LocaleID(locale)
		t.Status = model.TermStatus(status)
		c.Terms = append(c.Terms, t)
	}

	return c, nil
}

type termWithConcept struct {
	concept fw.Concept
	term    fw.Term
}

func (tb *SQLiteTermBase) queryExactTerms(sourceText string, opts fw.LookupOptions) []fw.TermMatch {
	searchText := sourceText
	column := "t.text"
	if !opts.CaseSensitive {
		searchText = strings.ToLower(sourceText)
		column = "t.text_lower"
	}

	where := fmt.Sprintf("%s = ? AND t.locale = ?", column)
	args := []any{searchText, string(opts.SourceLocale)}

	needsJoin := false
	switch opts.ProjectScope {
	case fw.ProjectScopeOnly:
		where += " AND c.project_id = ?"
		args = append(args, opts.ProjectID)
		needsJoin = true
	case fw.ProjectScopeExclude:
		where += " AND c.project_id != ?"
		args = append(args, opts.ProjectID)
		needsJoin = true
	}

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

func (tb *SQLiteTermBase) queryNormalizedTerms(normalizedSource string, opts fw.LookupOptions) []fw.TermMatch {
	where := "t.text_lower = ? AND t.locale = ?"
	args := []any{normalizedSource, string(opts.SourceLocale)}

	needsJoin := false
	switch opts.ProjectScope {
	case fw.ProjectScopeOnly:
		where += " AND c.project_id = ?"
		args = append(args, opts.ProjectID)
		needsJoin = true
	case fw.ProjectScopeExclude:
		where += " AND c.project_id != ?"
		args = append(args, opts.ProjectID)
		needsJoin = true
	}

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

func (tb *SQLiteTermBase) queryFuzzyTerms(normalizedSource string, opts fw.LookupOptions) []fw.TermMatch {
	where := "t.locale = ?"
	args := []any{string(opts.SourceLocale)}

	needsJoin := false
	switch opts.ProjectScope {
	case fw.ProjectScopeOnly:
		where += " AND c.project_id = ?"
		args = append(args, opts.ProjectID)
		needsJoin = true
	case fw.ProjectScopeExclude:
		where += " AND c.project_id != ?"
		args = append(args, opts.ProjectID)
		needsJoin = true
	}

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

	// Collect rows first to release the connection before loading concepts.
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

		score := sievepen.LevenshteinRatio(normalizedSource, fw.NormalizeTerm(r.text))
		if score >= opts.MinScore && fw.MatchesStatus(model.TermStatus(r.status), opts.StatusFilter) {
			candidates = append(candidates, fuzzyCandidate{row: r, score: score})
		}
	}

	var matches []fw.TermMatch
	for _, c := range candidates {
		concept, err := tb.scanConcept(c.row.conceptID)
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

func (tb *SQLiteTermBase) queryTermsByLocale(locale model.LocaleID, domains []string, statusFilter []model.TermStatus, opts fw.LookupOptions) ([]termWithConcept, error) {
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
	case fw.ProjectScopeOnly:
		where += " AND c.project_id = ?"
		args = append(args, opts.ProjectID)
	case fw.ProjectScopeExclude:
		where += " AND c.project_id != ?"
		args = append(args, opts.ProjectID)
	}

	rows, err := tb.db.Query(fmt.Sprintf(`
		SELECT c.id, c.project_id, c.domain, c.definition, t.text, t.locale, t.status, t.part_of_speech, t.gender, t.note
		FROM tb_terms t JOIN tb_concepts c ON t.concept_id = c.id
		WHERE %s
	`, where), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []termWithConcept
	for rows.Next() {
		var cID, projectID, domain, definition, text, loc, status, pos, gender, note string
		if err := rows.Scan(&cID, &projectID, &domain, &definition, &text, &loc, &status, &pos, &gender, &note); err != nil {
			continue
		}
		results = append(results, termWithConcept{
			concept: fw.Concept{ID: cID, ProjectID: projectID, Domain: domain, Definition: definition},
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
	return results, nil
}

// scanTermRows is a raw row from a term query, before concept loading.
type scanTermRow struct {
	conceptID, text, locale, status, pos, gender, note string
}

func (tb *SQLiteTermBase) scanTermMatches(rows interface {
	Next() bool
	Scan(...any) error
}, score float64, matchType model.MatchStrategy, opts fw.LookupOptions) []fw.TermMatch {
	// Collect all row data first to release the database connection.
	var raw []scanTermRow
	for rows.Next() {
		var r scanTermRow
		if err := rows.Scan(&r.conceptID, &r.text, &r.locale, &r.status, &r.pos, &r.gender, &r.note); err != nil {
			continue
		}
		if fw.MatchesStatus(model.TermStatus(r.status), opts.StatusFilter) {
			raw = append(raw, r)
		}
	}

	// Now load concepts (safe — rows are fully consumed).
	var matches []fw.TermMatch
	for _, r := range raw {
		concept, err := tb.scanConcept(r.conceptID)
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

// nullableString is defined in postgres.go (shared by both backends).
