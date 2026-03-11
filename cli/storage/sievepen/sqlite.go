// Package sievepen provides a SQLite-backed TranslationMemory for CLI use.
// This is a simplified version of bowrain/sievepen without project_id,
// stream, or workspace scoping — designed for single-user file-based use.
package sievepen

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gokapi/gokapi/cli/storage"
	"github.com/gokapi/gokapi/core/model"
	fw "github.com/gokapi/gokapi/core/sievepen"
)

// SQLiteTM is a persistent translation memory backed by SQLite with
// content-aware matching using derived keys (plain, structural, generalized).
type SQLiteTM struct {
	db *storage.DB
}

// NewSQLiteTM opens (or creates) a SQLite-backed translation memory.
// Use ":memory:" for an in-memory database (useful for testing).
func NewSQLiteTM(dbPath string) (*SQLiteTM, error) {
	db, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := storage.Migrate(db, tmMigrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	return &SQLiteTM{db: db}, nil
}

var tmMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "content-aware TM schema",
		SQL: `
		CREATE TABLE IF NOT EXISTS tm_entries (
			id              TEXT PRIMARY KEY,
			source_coded    TEXT NOT NULL,
			target_coded    TEXT NOT NULL,
			source_plain    TEXT NOT NULL,
			source_struct   TEXT NOT NULL,
			source_general  TEXT NOT NULL,
			source_locale   TEXT NOT NULL,
			target_locale   TEXT NOT NULL,
			entities        TEXT,
			properties      TEXT,
			created_at      TEXT NOT NULL,
			updated_at      TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_tm_general ON tm_entries(source_general, source_locale, target_locale);
		CREATE INDEX IF NOT EXISTS idx_tm_struct  ON tm_entries(source_struct, source_locale, target_locale);
		CREATE INDEX IF NOT EXISTS idx_tm_plain   ON tm_entries(source_plain, source_locale, target_locale);
		`,
	},
	{
		Version:     2,
		Description: "FTS5 trigram indexes for fuzzy candidate retrieval",
		SQL: `
		CREATE VIRTUAL TABLE IF NOT EXISTS tm_trigram USING fts5(
			source_plain, source_struct, source_general,
			content='tm_entries', content_rowid='rowid',
			tokenize='trigram'
		);

		-- Populate from existing data.
		INSERT INTO tm_trigram(rowid, source_plain, source_struct, source_general)
			SELECT rowid, source_plain, source_struct, source_general FROM tm_entries;

		-- Keep FTS5 in sync via triggers.
		CREATE TRIGGER tm_trigram_ai AFTER INSERT ON tm_entries BEGIN
			INSERT INTO tm_trigram(rowid, source_plain, source_struct, source_general)
			VALUES (new.rowid, new.source_plain, new.source_struct, new.source_general);
		END;
		CREATE TRIGGER tm_trigram_ad AFTER DELETE ON tm_entries BEGIN
			INSERT INTO tm_trigram(tm_trigram, rowid, source_plain, source_struct, source_general)
			VALUES ('delete', old.rowid, old.source_plain, old.source_struct, old.source_general);
		END;
		CREATE TRIGGER tm_trigram_au AFTER UPDATE ON tm_entries BEGIN
			INSERT INTO tm_trigram(tm_trigram, rowid, source_plain, source_struct, source_general)
			VALUES ('delete', old.rowid, old.source_plain, old.source_struct, old.source_general);
			INSERT INTO tm_trigram(rowid, source_plain, source_struct, source_general)
			VALUES (new.rowid, new.source_plain, new.source_struct, new.source_general);
		END;

		-- FTS5 word-based index for UI search with BM25 ranking.
		CREATE VIRTUAL TABLE IF NOT EXISTS tm_search USING fts5(
			source_text, target_text,
			content='tm_entries', content_rowid='rowid',
			tokenize='unicode61'
		);

		INSERT INTO tm_search(rowid, source_text, target_text)
			SELECT rowid, source_plain, target_coded FROM tm_entries;

		CREATE TRIGGER tm_search_ai AFTER INSERT ON tm_entries BEGIN
			INSERT INTO tm_search(rowid, source_text, target_text)
			VALUES (new.rowid, new.source_plain, new.target_coded);
		END;
		CREATE TRIGGER tm_search_ad AFTER DELETE ON tm_entries BEGIN
			INSERT INTO tm_search(tm_search, rowid, source_text, target_text)
			VALUES ('delete', old.rowid, old.source_plain, old.target_coded);
		END;
		CREATE TRIGGER tm_search_au AFTER UPDATE ON tm_entries BEGIN
			INSERT INTO tm_search(tm_search, rowid, source_text, target_text)
			VALUES ('delete', old.rowid, old.source_plain, old.target_coded);
			INSERT INTO tm_search(rowid, source_text, target_text)
			VALUES (new.rowid, new.source_plain, new.target_coded);
		END;
		`,
	},
}

// Add inserts or updates a translation memory entry.
func (tm *SQLiteTM) Add(entry fw.TMEntry) error {
	if entry.ID == "" {
		return fmt.Errorf("entry ID is required")
	}
	if entry.Source == nil {
		return fmt.Errorf("entry source Fragment is required")
	}

	now := time.Now()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = now
	}

	sourceJSON, err := json.Marshal(entry.Source)
	if err != nil {
		return fmt.Errorf("marshal source: %w", err)
	}
	targetJSON, err := json.Marshal(entry.Target)
	if err != nil {
		return fmt.Errorf("marshal target: %w", err)
	}

	var entitiesJSON, propertiesJSON []byte
	if len(entry.Entities) > 0 {
		entitiesJSON, _ = json.Marshal(entry.Entities)
	}
	if len(entry.Properties) > 0 {
		propertiesJSON, _ = json.Marshal(entry.Properties)
	}

	_, err = tm.db.Exec(`
		INSERT INTO tm_entries (id, source_coded, target_coded,
			source_plain, source_struct, source_general,
			source_locale, target_locale,
			entities, properties,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			source_coded = excluded.source_coded,
			target_coded = excluded.target_coded,
			source_plain = excluded.source_plain,
			source_struct = excluded.source_struct,
			source_general = excluded.source_general,
			source_locale = excluded.source_locale,
			target_locale = excluded.target_locale,
			entities = excluded.entities,
			properties = excluded.properties,
			updated_at = excluded.updated_at
	`, entry.ID,
		string(sourceJSON), string(targetJSON),
		fw.NormalizeText(entry.SourceText()),
		fw.NormalizeText(entry.SourceStructural()),
		fw.NormalizeText(entry.SourceGeneralized()),
		string(entry.SourceLocale), string(entry.TargetLocale),
		nullableString(entitiesJSON), nullableString(propertiesJSON),
		entry.CreatedAt.Format(time.RFC3339), entry.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert entry: %w", err)
	}

	return nil
}

// Lookup searches for matches using tiered matching with the full content model.
func (tm *SQLiteTM) Lookup(source *model.Block, sourceLocale, targetLocale model.LocaleID, opts fw.LookupOptions) ([]fw.TMMatch, error) {
	if source == nil {
		return nil, nil
	}

	opts = fw.ApplyDefaults(opts)
	frag := source.FirstFragment()
	if frag == nil {
		return nil, nil
	}

	plainKey := fw.NormalizeText(frag.Text())
	structKey := fw.NormalizeText(frag.StructuralText())
	generalKey := fw.NormalizeText(frag.GeneralizedText())
	entityAnnotations := fw.ExtractEntityAnnotations(source)

	return tm.tieredLookup(plainKey, structKey, generalKey, entityAnnotations, sourceLocale, targetLocale, opts)
}

// LookupText searches for matches using plain text only.
func (tm *SQLiteTM) LookupText(source string, sourceLocale, targetLocale model.LocaleID, opts fw.LookupOptions) ([]fw.TMMatch, error) {
	opts = fw.ApplyDefaults(opts)
	opts.MatchModes = []fw.MatchMode{fw.MatchModePlain}
	normalizedSource := fw.NormalizeText(source)
	return tm.tieredLookup(normalizedSource, normalizedSource, normalizedSource, nil, sourceLocale, targetLocale, opts)
}

func (tm *SQLiteTM) tieredLookup(plainKey, structKey, generalKey string, entityAnnotations []*model.EntityAnnotation, sourceLocale, targetLocale model.LocaleID, opts fw.LookupOptions) ([]fw.TMMatch, error) {
	var matches []fw.TMMatch
	seen := make(map[string]bool)
	modeEnabled := fw.MatchModesEnabled(opts.MatchModes)

	// Tier 1-3: Exact matches (indexed lookups).
	if modeEnabled[fw.MatchModeGeneralized] {
		exactMatches, err := tm.queryExact("source_general", generalKey, sourceLocale, targetLocale)
		if err != nil {
			return nil, err
		}
		for _, entry := range exactMatches {
			if !seen[entry.ID] {
				seen[entry.ID] = true
				adaptations := fw.ComputeEntityAdaptations(entry, entityAnnotations)
				matches = append(matches, fw.TMMatch{
					Entry:             entry,
					Score:             1.0,
					MatchType:         fw.MatchGeneralizedExact,
					EntityAdaptations: adaptations,
				})
			}
		}
	}

	if modeEnabled[fw.MatchModeStructural] {
		exactMatches, err := tm.queryExact("source_struct", structKey, sourceLocale, targetLocale)
		if err != nil {
			return nil, err
		}
		for _, entry := range exactMatches {
			if !seen[entry.ID] {
				seen[entry.ID] = true
				matches = append(matches, fw.TMMatch{
					Entry:     entry,
					Score:     1.0,
					MatchType: fw.MatchStructuralExact,
				})
			}
		}
	}

	if modeEnabled[fw.MatchModePlain] {
		exactMatches, err := tm.queryExact("source_plain", plainKey, sourceLocale, targetLocale)
		if err != nil {
			return nil, err
		}
		for _, entry := range exactMatches {
			if !seen[entry.ID] {
				seen[entry.ID] = true
				matches = append(matches, fw.TMMatch{
					Entry:     entry,
					Score:     1.0,
					MatchType: fw.MatchExact,
				})
			}
		}
	}

	if len(matches) > 0 && opts.MinScore >= 1.0 {
		return fw.LimitResults(matches, opts.MaxResults), nil
	}

	// Tier 4-6: Fuzzy matches (trigram candidate retrieval + Levenshtein scoring).
	allEntries, err := tm.queryFuzzyCandidates(plainKey, structKey, generalKey, sourceLocale, targetLocale)
	if err != nil {
		return nil, err
	}

	for _, entry := range allEntries {
		if seen[entry.ID] {
			continue
		}

		var bestScore float64
		var bestType fw.MatchType
		var adaptations []fw.EntityAdaptation

		if modeEnabled[fw.MatchModeGeneralized] {
			score := fw.LevenshteinRatio(generalKey, fw.NormalizeText(entry.SourceGeneralized()))
			if score >= opts.MinScore && score > bestScore {
				bestScore = score
				bestType = fw.MatchGeneralizedFuzzy
				adaptations = fw.ComputeEntityAdaptations(entry, entityAnnotations)
			}
		}
		if modeEnabled[fw.MatchModeStructural] {
			score := fw.LevenshteinRatio(structKey, fw.NormalizeText(entry.SourceStructural()))
			if score >= opts.MinScore && score > bestScore {
				bestScore = score
				bestType = fw.MatchStructuralFuzzy
				adaptations = nil
			}
		}
		if modeEnabled[fw.MatchModePlain] {
			score := fw.LevenshteinRatio(plainKey, fw.NormalizeText(entry.SourceText()))
			if score >= opts.MinScore && score > bestScore {
				bestScore = score
				bestType = fw.MatchFuzzy
				adaptations = nil
			}
		}

		if bestScore >= opts.MinScore {
			seen[entry.ID] = true
			matches = append(matches, fw.TMMatch{
				Entry:             entry,
				Score:             bestScore,
				MatchType:         bestType,
				EntityAdaptations: adaptations,
			})
		}
	}

	sort.Slice(matches, func(i, j int) bool {
		pi := fw.MatchTypePriority(matches[i].MatchType)
		pj := fw.MatchTypePriority(matches[j].MatchType)
		if pi != pj {
			return pi < pj
		}
		return matches[i].Score > matches[j].Score
	})

	return fw.LimitResults(matches, opts.MaxResults), nil
}

func (tm *SQLiteTM) queryExact(column, value string, sourceLocale, targetLocale model.LocaleID) ([]fw.TMEntry, error) {
	query := fmt.Sprintf(`
		SELECT id, source_coded, target_coded, source_locale, target_locale,
			entities, properties, created_at, updated_at
		FROM tm_entries
		WHERE %s = ? AND source_locale = ? AND target_locale = ?
	`, column)

	rows, err := tm.db.Query(query, value, string(sourceLocale), string(targetLocale))
	if err != nil {
		return nil, fmt.Errorf("query exact: %w", err)
	}
	defer rows.Close()

	return tm.scanEntries(rows)
}

// queryFuzzyCandidates uses FTS5 trigram indexes to retrieve a limited set of
// candidate entries for Levenshtein scoring, replacing the previous full table scan.
// Falls back to length-based pre-filtering if FTS5 trigram is unavailable.
func (tm *SQLiteTM) queryFuzzyCandidates(plainKey, structKey, generalKey string, sourceLocale, targetLocale model.LocaleID) ([]fw.TMEntry, error) {
	// Try FTS5 trigram candidate retrieval first.
	entries, err := tm.queryTrigramCandidates(plainKey, structKey, generalKey, sourceLocale, targetLocale)
	if err == nil {
		return entries, nil
	}

	// Fallback: length-based pre-filtering. With MinScore=0.7, entries differing
	// by more than 30% in length cannot possibly match.
	return tm.queryLengthFiltered(plainKey, sourceLocale, targetLocale)
}

func (tm *SQLiteTM) queryTrigramCandidates(plainKey, structKey, generalKey string, sourceLocale, targetLocale model.LocaleID) ([]fw.TMEntry, error) {
	// Build OR query across all three key columns.
	rows, err := tm.db.Query(`
		SELECT DISTINCT e.id, e.source_coded, e.target_coded, e.source_locale, e.target_locale,
			e.entities, e.properties, e.created_at, e.updated_at
		FROM tm_entries e
		WHERE e.rowid IN (
			SELECT rowid FROM tm_trigram WHERE tm_trigram MATCH ?
			UNION
			SELECT rowid FROM tm_trigram WHERE tm_trigram MATCH ?
			UNION
			SELECT rowid FROM tm_trigram WHERE tm_trigram MATCH ?
		) AND e.source_locale = ? AND e.target_locale = ?
		LIMIT 200
	`, buildTrigramQuery(plainKey), buildTrigramQuery(structKey), buildTrigramQuery(generalKey),
		string(sourceLocale), string(targetLocale))
	if err != nil {
		return nil, fmt.Errorf("trigram query: %w", err)
	}
	defer rows.Close()

	return tm.scanEntries(rows)
}

func (tm *SQLiteTM) queryLengthFiltered(plainKey string, sourceLocale, targetLocale model.LocaleID) ([]fw.TMEntry, error) {
	keyLen := len([]rune(plainKey))
	minLen := int(float64(keyLen) * 0.7)
	maxLen := int(float64(keyLen) * 1.3)
	if minLen < 0 {
		minLen = 0
	}

	rows, err := tm.db.Query(`
		SELECT id, source_coded, target_coded, source_locale, target_locale,
			entities, properties, created_at, updated_at
		FROM tm_entries
		WHERE source_locale = ? AND target_locale = ?
			AND LENGTH(source_plain) BETWEEN ? AND ?
		LIMIT 500
	`, string(sourceLocale), string(targetLocale), minLen, maxLen)
	if err != nil {
		return nil, fmt.Errorf("length-filtered query: %w", err)
	}
	defer rows.Close()

	return tm.scanEntries(rows)
}

// buildTrigramQuery builds an FTS5 trigram MATCH expression for candidate retrieval.
// For multi-word text, uses OR of individual words (each as a substring match).
// For text without word boundaries (CJK, single words), uses overlapping windows.
func buildTrigramQuery(s string) string {
	escape := func(w string) string {
		return `"` + strings.ReplaceAll(w, `"`, `""`) + `"`
	}

	fields := strings.Fields(s)
	if len(fields) > 1 {
		// Multi-word: OR individual words as substring matches.
		var parts []string
		for _, f := range fields {
			if len([]rune(f)) >= 3 {
				parts = append(parts, escape(f))
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, " OR ")
		}
	}

	// Single word or no word boundaries (CJK): use overlapping windows.
	runes := []rune(s)
	if len(runes) <= 5 {
		return escape(s)
	}

	windowSize := 4
	step := (len(runes) - windowSize) / 4
	if step < 1 {
		step = 1
	}
	var parts []string
	seen := make(map[string]bool)
	for i := 0; i < len(runes)-windowSize+1 && len(parts) < 6; i += step {
		w := string(runes[i : i+windowSize])
		if !seen[w] {
			seen[w] = true
			parts = append(parts, escape(w))
		}
	}
	if len(parts) == 0 {
		return escape(s)
	}
	return strings.Join(parts, " OR ")
}

func (tm *SQLiteTM) scanEntries(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]fw.TMEntry, error) {
	var entries []fw.TMEntry
	for rows.Next() {
		var entry fw.TMEntry
		var sourceJSON, targetJSON string
		var srcLocale, tgtLocale string
		var entitiesJSON, propertiesJSON *string
		var createdStr, updatedStr string

		if err := rows.Scan(&entry.ID, &sourceJSON, &targetJSON,
			&srcLocale, &tgtLocale,
			&entitiesJSON, &propertiesJSON,
			&createdStr, &updatedStr); err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}

		entry.Source = &model.Fragment{}
		if err := json.Unmarshal([]byte(sourceJSON), entry.Source); err != nil {
			return nil, fmt.Errorf("unmarshal source: %w", err)
		}
		entry.Target = &model.Fragment{}
		if err := json.Unmarshal([]byte(targetJSON), entry.Target); err != nil {
			return nil, fmt.Errorf("unmarshal target: %w", err)
		}

		entry.SourceLocale = model.LocaleID(srcLocale)
		entry.TargetLocale = model.LocaleID(tgtLocale)
		entry.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		entry.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

		if entitiesJSON != nil && *entitiesJSON != "" {
			_ = json.Unmarshal([]byte(*entitiesJSON), &entry.Entities)
		}
		if propertiesJSON != nil && *propertiesJSON != "" {
			_ = json.Unmarshal([]byte(*propertiesJSON), &entry.Properties)
		}

		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}
	return entries, nil
}

// Delete removes an entry by ID.
func (tm *SQLiteTM) Delete(id string) error {
	result, err := tm.db.Exec("DELETE FROM tm_entries WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("delete entry: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("entry not found: %s", id)
	}
	return nil
}

// Count returns the total number of entries.
func (tm *SQLiteTM) Count() int {
	var count int
	_ = tm.db.QueryRow("SELECT COUNT(*) FROM tm_entries").Scan(&count)
	return count
}

// Close closes the database connection.
func (tm *SQLiteTM) Close() error {
	return tm.db.Close()
}

// SearchEntries performs a ranked full-text search using FTS5 with BM25 ranking.
// Falls back to LIKE-based substring search if FTS5 is unavailable.
func (tm *SQLiteTM) SearchEntries(query, sourceLocale, targetLocale string, offset, limit int) ([]fw.TMEntry, int) {
	if query != "" {
		entries, total, err := tm.searchFTS5(query, sourceLocale, targetLocale, offset, limit)
		if err == nil {
			return entries, total
		}
		// Fall through to LIKE-based search.
	}
	return tm.searchLike(query, sourceLocale, targetLocale, offset, limit)
}

func (tm *SQLiteTM) searchFTS5(query, sourceLocale, targetLocale string, offset, limit int) ([]fw.TMEntry, int, error) {
	// Build locale filter for the main table.
	localeWhere := "1=1"
	var localeArgs []any
	if sourceLocale != "" {
		localeWhere += " AND e.source_locale = ?"
		localeArgs = append(localeArgs, sourceLocale)
	}
	if targetLocale != "" {
		localeWhere += " AND e.target_locale = ?"
		localeArgs = append(localeArgs, targetLocale)
	}

	// Count matching entries.
	countQ := fmt.Sprintf(`
		SELECT COUNT(*) FROM tm_entries e
		WHERE e.rowid IN (SELECT rowid FROM tm_search WHERE tm_search MATCH ?)
		AND %s`, localeWhere)
	countArgs := append([]any{query}, localeArgs...)
	var total int
	if err := tm.db.QueryRow(countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Fetch ranked results.
	q := fmt.Sprintf(`
		SELECT e.id, e.source_coded, e.target_coded, e.source_locale, e.target_locale,
			e.entities, e.properties, e.created_at, e.updated_at
		FROM tm_entries e
		JOIN tm_search s ON s.rowid = e.rowid
		WHERE tm_search MATCH ? AND %s
		ORDER BY s.rank
		LIMIT ? OFFSET ?`, localeWhere)
	args := append([]any{query}, localeArgs...)
	args = append(args, limit, offset)

	rows, err := tm.db.Query(q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries, err := tm.scanEntries(rows)
	return entries, total, err
}

func (tm *SQLiteTM) searchLike(query, sourceLocale, targetLocale string, offset, limit int) ([]fw.TMEntry, int) {
	where := "1=1"
	var args []any
	if query != "" {
		where += " AND (LOWER(source_plain) LIKE ? OR LOWER(target_coded) LIKE ?)"
		pattern := "%" + strings.ToLower(query) + "%"
		args = append(args, pattern, pattern)
	}
	if sourceLocale != "" {
		where += " AND source_locale = ?"
		args = append(args, sourceLocale)
	}
	if targetLocale != "" {
		where += " AND target_locale = ?"
		args = append(args, targetLocale)
	}

	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	_ = tm.db.QueryRow("SELECT COUNT(*) FROM tm_entries WHERE "+where, countArgs...).Scan(&total)

	q := fmt.Sprintf(`SELECT id, source_coded, target_coded, source_locale, target_locale,
		entities, properties, created_at, updated_at
		FROM tm_entries WHERE %s ORDER BY updated_at DESC LIMIT ? OFFSET ?`, where)
	args = append(args, limit, offset)
	rows, err := tm.db.Query(q, args...)
	if err != nil {
		return nil, total
	}
	defer rows.Close()

	entries, _ := tm.scanEntries(rows)
	return entries, total
}

// GetEntry fetches a single entry by ID.
func (tm *SQLiteTM) GetEntry(id string) (fw.TMEntry, bool) {
	rows, err := tm.db.Query(`
		SELECT id, source_coded, target_coded, source_locale, target_locale,
			entities, properties, created_at, updated_at
		FROM tm_entries WHERE id = ?
	`, id)
	if err != nil {
		return fw.TMEntry{}, false
	}
	defer rows.Close()

	entries, err := tm.scanEntries(rows)
	if err != nil || len(entries) == 0 {
		return fw.TMEntry{}, false
	}
	return entries[0], true
}

// Entries returns all entries. Used for export operations.
func (tm *SQLiteTM) Entries() []fw.TMEntry {
	rows, err := tm.db.Query(`
		SELECT id, source_coded, target_coded, source_locale, target_locale,
			entities, properties, created_at, updated_at
		FROM tm_entries ORDER BY id
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	entries, _ := tm.scanEntries(rows)
	return entries
}

func nullableString(b []byte) *string {
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	return &s
}
