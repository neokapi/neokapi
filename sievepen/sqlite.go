package sievepen

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
)

// Sentinel errors for TM entry validation.
var (
	ErrEntryIDRequired     = errors.New("entry ID is required")
	ErrEntrySourceRequired = errors.New("entry source Fragment is required")
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

	if err := storage.Migrate(db, "sievepen_migrations", tmMigrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	return &SQLiteTM{db: db}, nil
}

// NewSQLiteTMFromDB creates a SQLiteTM from an already-opened database.
// This allows sharing a single DB file across TM and termbase.
func NewSQLiteTMFromDB(db *storage.DB) (*SQLiteTM, error) {
	if err := storage.Migrate(db, "sievepen_migrations", tmMigrations); err != nil {
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return &SQLiteTM{db: db}, nil
}

var tmMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "content-aware TM schema with project/stream support and FTS5 indexes",
		SQL: `
		CREATE TABLE IF NOT EXISTS tm_entries (
			id              TEXT PRIMARY KEY,
			project_id      TEXT NOT NULL DEFAULT '',
			stream          TEXT NOT NULL DEFAULT '',
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
		CREATE INDEX IF NOT EXISTS idx_tm_stream  ON tm_entries(stream, source_locale, target_locale);

		-- FTS5 trigram index for fuzzy candidate retrieval.
		CREATE VIRTUAL TABLE IF NOT EXISTS tm_trigram USING fts5(
			source_plain, source_struct, source_general,
			content='tm_entries', content_rowid='rowid',
			tokenize='trigram'
		);

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

// Add inserts or updates a translation memory entry with an empty stream.
func (tm *SQLiteTM) Add(entry TMEntry) error {
	return tm.AddWithStream(entry, "")
}

// AddWithStream inserts or updates a translation memory entry associated with a stream.
// The stream parameter is a persistence concern (e.g., a git branch name) not stored
// in the framework TMEntry type.
func (tm *SQLiteTM) AddWithStream(entry TMEntry, stream string) error {
	if entry.ID == "" {
		return ErrEntryIDRequired
	}
	if entry.Source == nil {
		return ErrEntrySourceRequired
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
		INSERT INTO tm_entries (id, project_id, stream, source_coded, target_coded,
			source_plain, source_struct, source_general,
			source_locale, target_locale,
			entities, properties,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_id = excluded.project_id,
			stream = excluded.stream,
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
	`, entry.ID, entry.ProjectID, stream,
		string(sourceJSON), string(targetJSON),
		NormalizeText(entry.SourceText()),
		NormalizeText(entry.SourceStructural()),
		NormalizeText(entry.SourceGeneralized()),
		string(entry.SourceLocale), string(entry.TargetLocale),
		nullableString(entitiesJSON), nullableString(propertiesJSON),
		entry.CreatedAt.Format(time.RFC3339), entry.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert entry: %w", err)
	}

	return nil
}

// Lookup searches for matches using tiered matching with the full content model.
func (tm *SQLiteTM) Lookup(source *model.Block, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error) {
	if source == nil {
		return nil, nil
	}

	opts = ApplyDefaults(opts)
	frag := source.FirstFragment()
	if frag == nil {
		return nil, nil
	}

	plainKey := NormalizeText(frag.Text())
	structKey := NormalizeText(frag.StructuralText())
	generalKey := NormalizeText(frag.GeneralizedText())
	entityAnnotations := ExtractEntityAnnotations(source)

	return tm.tieredLookup(plainKey, structKey, generalKey, entityAnnotations, sourceLocale, targetLocale, opts)
}

// LookupText searches for matches using plain text only.
func (tm *SQLiteTM) LookupText(source string, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error) {
	opts = ApplyDefaults(opts)
	opts.MatchModes = []MatchMode{MatchModePlain}
	normalizedSource := NormalizeText(source)
	return tm.tieredLookup(normalizedSource, normalizedSource, normalizedSource, nil, sourceLocale, targetLocale, opts)
}

func (tm *SQLiteTM) tieredLookup(plainKey, structKey, generalKey string, entityAnnotations []*model.EntityAnnotation, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error) {
	var matches []TMMatch
	seen := make(map[string]bool)
	modeEnabled := MatchModesEnabled(opts.MatchModes)

	// Tier 1-3: Exact matches (indexed lookups).
	if modeEnabled[MatchModeGeneralized] {
		exactMatches, err := tm.queryExact("source_general", generalKey, sourceLocale, targetLocale, opts)
		if err != nil {
			return nil, err
		}
		for _, entry := range exactMatches {
			if !seen[entry.ID] {
				seen[entry.ID] = true
				adaptations := ComputeEntityAdaptations(entry, entityAnnotations)
				matches = append(matches, TMMatch{
					Entry:             entry,
					Score:             1.0,
					MatchType:         MatchGeneralizedExact,
					ProjectID:         entry.ProjectID,
					EntityAdaptations: adaptations,
				})
			}
		}
	}

	if modeEnabled[MatchModeStructural] {
		exactMatches, err := tm.queryExact("source_struct", structKey, sourceLocale, targetLocale, opts)
		if err != nil {
			return nil, err
		}
		for _, entry := range exactMatches {
			if !seen[entry.ID] {
				seen[entry.ID] = true
				matches = append(matches, TMMatch{
					Entry:     entry,
					Score:     1.0,
					MatchType: MatchStructuralExact,
					ProjectID: entry.ProjectID,
				})
			}
		}
	}

	if modeEnabled[MatchModePlain] {
		exactMatches, err := tm.queryExact("source_plain", plainKey, sourceLocale, targetLocale, opts)
		if err != nil {
			return nil, err
		}
		for _, entry := range exactMatches {
			if !seen[entry.ID] {
				seen[entry.ID] = true
				matches = append(matches, TMMatch{
					Entry:     entry,
					Score:     1.0,
					MatchType: MatchExact,
					ProjectID: entry.ProjectID,
				})
			}
		}
	}

	if len(matches) > 0 && opts.MinScore >= 1.0 {
		return LimitResults(matches, opts.MaxResults), nil
	}

	// Tier 4-6: Fuzzy matches (trigram candidate retrieval + Levenshtein scoring).
	allEntries, err := tm.queryFuzzyCandidates(plainKey, structKey, generalKey, sourceLocale, targetLocale, opts)
	if err != nil {
		return nil, err
	}

	for _, entry := range allEntries {
		if seen[entry.ID] {
			continue
		}

		var bestScore float64
		var bestType MatchType
		var adaptations []EntityAdaptation

		if modeEnabled[MatchModeGeneralized] {
			score := LevenshteinRatio(generalKey, NormalizeText(entry.SourceGeneralized()))
			if score >= opts.MinScore && score > bestScore {
				bestScore = score
				bestType = MatchGeneralizedFuzzy
				adaptations = ComputeEntityAdaptations(entry, entityAnnotations)
			}
		}
		if modeEnabled[MatchModeStructural] {
			score := LevenshteinRatio(structKey, NormalizeText(entry.SourceStructural()))
			if score >= opts.MinScore && score > bestScore {
				bestScore = score
				bestType = MatchStructuralFuzzy
				adaptations = nil
			}
		}
		if modeEnabled[MatchModePlain] {
			score := LevenshteinRatio(plainKey, NormalizeText(entry.SourceText()))
			if score >= opts.MinScore && score > bestScore {
				bestScore = score
				bestType = MatchFuzzy
				adaptations = nil
			}
		}

		if bestScore >= opts.MinScore {
			seen[entry.ID] = true
			score := bestScore
			if opts.ProjectID != "" && entry.ProjectID == opts.ProjectID && score < 1.0 {
				score += 0.03
				if score > 1.0 {
					score = 1.0
				}
			}
			matches = append(matches, TMMatch{
				Entry:             entry,
				Score:             score,
				MatchType:         bestType,
				ProjectID:         entry.ProjectID,
				EntityAdaptations: adaptations,
			})
		}
	}

	slices.SortFunc(matches, func(a, b TMMatch) int {
		pa := MatchTypePriority(a.MatchType)
		pb := MatchTypePriority(b.MatchType)
		if c := cmp.Compare(pa, pb); c != 0 {
			return c
		}
		return cmp.Compare(b.Score, a.Score)
	})

	return LimitResults(matches, opts.MaxResults), nil
}

func (tm *SQLiteTM) queryExact(column, value string, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMEntry, error) {
	where := column + " = ? AND source_locale = ? AND target_locale = ?"
	args := []any{value, string(sourceLocale), string(targetLocale)}

	where, args = appendSQLiteProjectFilter(where, args, opts.ProjectID, opts.ProjectScope)

	query := fmt.Sprintf(`
		SELECT id, project_id, source_coded, target_coded, source_locale, target_locale,
			entities, properties, created_at, updated_at
		FROM tm_entries
		WHERE %s
	`, where)

	rows, err := tm.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("query exact: %w", err)
	}
	defer rows.Close()

	return tm.scanEntries(rows)
}

func (tm *SQLiteTM) queryFuzzyCandidates(plainKey, structKey, generalKey string, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMEntry, error) {
	entries, err := tm.queryTrigramCandidates(plainKey, structKey, generalKey, sourceLocale, targetLocale, opts)
	if err == nil {
		return entries, nil
	}
	return tm.queryLengthFiltered(plainKey, sourceLocale, targetLocale, opts)
}

func (tm *SQLiteTM) queryTrigramCandidates(plainKey, structKey, generalKey string, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMEntry, error) {
	where := `e.rowid IN (
			SELECT rowid FROM tm_trigram WHERE tm_trigram MATCH ?
			UNION
			SELECT rowid FROM tm_trigram WHERE tm_trigram MATCH ?
			UNION
			SELECT rowid FROM tm_trigram WHERE tm_trigram MATCH ?
		) AND e.source_locale = ? AND e.target_locale = ?`
	args := []any{
		BuildTrigramQuery(plainKey), BuildTrigramQuery(structKey), BuildTrigramQuery(generalKey),
		string(sourceLocale), string(targetLocale),
	}

	where, args = appendSQLiteProjectFilter(where, args, opts.ProjectID, opts.ProjectScope)

	rows, err := tm.db.Query(fmt.Sprintf(`
		SELECT DISTINCT e.id, e.project_id, e.source_coded, e.target_coded, e.source_locale, e.target_locale,
			e.entities, e.properties, e.created_at, e.updated_at
		FROM tm_entries e
		WHERE %s
		LIMIT 200
	`, where), args...)
	if err != nil {
		return nil, fmt.Errorf("trigram query: %w", err)
	}
	defer rows.Close()

	return tm.scanEntries(rows)
}

func (tm *SQLiteTM) queryLengthFiltered(plainKey string, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMEntry, error) {
	keyLen := len([]rune(plainKey))
	minLen := int(float64(keyLen) * 0.7)
	maxLen := int(float64(keyLen) * 1.3)
	if minLen < 0 {
		minLen = 0
	}

	where := "source_locale = ? AND target_locale = ? AND LENGTH(source_plain) BETWEEN ? AND ?"
	args := []any{string(sourceLocale), string(targetLocale), minLen, maxLen}

	where, args = appendSQLiteProjectFilter(where, args, opts.ProjectID, opts.ProjectScope)

	rows, err := tm.db.Query(fmt.Sprintf(`
		SELECT id, project_id, source_coded, target_coded, source_locale, target_locale,
			entities, properties, created_at, updated_at
		FROM tm_entries
		WHERE %s
		LIMIT 500
	`, where), args...)
	if err != nil {
		return nil, fmt.Errorf("length-filtered query: %w", err)
	}
	defer rows.Close()

	return tm.scanEntries(rows)
}

// BuildTrigramQuery builds an FTS5 trigram MATCH expression for candidate retrieval.
// For multi-word text, uses OR of individual words (each as a substring match).
// For text without word boundaries (CJK, single words), uses overlapping windows.
func BuildTrigramQuery(s string) string {
	escape := func(w string) string {
		return `"` + strings.ReplaceAll(w, `"`, `""`) + `"`
	}

	fields := strings.Fields(s)
	if len(fields) > 1 {
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

func appendSQLiteProjectFilter(where string, args []any, projectID string, scope ProjectScope) (string, []any) {
	switch scope {
	case ProjectScopeOnly:
		where += " AND project_id = ?"
		args = append(args, projectID)
	case ProjectScopeExclude:
		where += " AND project_id != ?"
		args = append(args, projectID)
	}
	return where, args
}

func (tm *SQLiteTM) scanEntries(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]TMEntry, error) {
	var entries []TMEntry
	for rows.Next() {
		var entry TMEntry
		var sourceJSON, targetJSON string
		var srcLocale, tgtLocale string
		var entitiesJSON, propertiesJSON *string
		var createdStr, updatedStr string

		if err := rows.Scan(&entry.ID, &entry.ProjectID, &sourceJSON, &targetJSON,
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
	if err := tm.db.QueryRow("SELECT COUNT(*) FROM tm_entries").Scan(&count); err != nil {
		slog.Warn("TM count query failed", "error", err)
		return 0
	}
	return count
}

// LocalePairStat holds the entry count for a source→target locale pair.
type LocalePairStat struct {
	SourceLocale string `json:"source_locale"`
	TargetLocale string `json:"target_locale"`
	Count        int    `json:"count"`
}

// LocalePairStats returns the number of entries grouped by locale pair.
func (tm *SQLiteTM) LocalePairStats() []LocalePairStat {
	rows, err := tm.db.Query(
		"SELECT source_locale, target_locale, COUNT(*) FROM tm_entries GROUP BY source_locale, target_locale ORDER BY COUNT(*) DESC",
	)
	if err != nil {
		slog.Warn("TM locale pair stats query failed", "error", err)
		return nil
	}
	defer rows.Close()
	var stats []LocalePairStat
	for rows.Next() {
		var s LocalePairStat
		if err := rows.Scan(&s.SourceLocale, &s.TargetLocale, &s.Count); err != nil {
			continue
		}
		stats = append(stats, s)
	}
	return stats
}

// ActivityStat holds the entry count for a date bucket.
type ActivityStat struct {
	Date  string `json:"date"`  // YYYY-MM-DD
	Count int    `json:"count"`
}

// ActivityStats returns daily entry counts over time based on created_at.
func (tm *SQLiteTM) ActivityStats() []ActivityStat {
	rows, err := tm.db.Query(
		"SELECT DATE(created_at) AS day, COUNT(*) FROM tm_entries GROUP BY day ORDER BY day",
	)
	if err != nil {
		slog.Warn("TM activity stats query failed", "error", err)
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
func (tm *SQLiteTM) DB() *storage.DB { return tm.db }

// Close closes the database connection.
func (tm *SQLiteTM) Close() error {
	return tm.db.Close()
}

// SearchEntries performs a ranked full-text search using FTS5 with BM25 ranking.
// Falls back to LIKE-based substring search if FTS5 is unavailable.
func (tm *SQLiteTM) SearchEntries(query, sourceLocale, targetLocale string, offset, limit int) ([]TMEntry, int) {
	if query != "" {
		entries, total, err := tm.searchFTS5(query, sourceLocale, targetLocale, offset, limit)
		if err == nil {
			return entries, total
		}
	}
	return tm.searchLike(query, sourceLocale, targetLocale, offset, limit)
}

func (tm *SQLiteTM) searchFTS5(query, sourceLocale, targetLocale string, offset, limit int) ([]TMEntry, int, error) {
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

	countQ := "\n\t\tSELECT COUNT(*) FROM tm_entries e\n\t\tWHERE e.rowid IN (SELECT rowid FROM tm_search WHERE tm_search MATCH ?)\n\t\tAND " + localeWhere
	countArgs := append([]any{query}, localeArgs...)
	var total int
	if err := tm.db.QueryRow(countQ, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	q := fmt.Sprintf(`
		SELECT e.id, e.project_id, e.source_coded, e.target_coded, e.source_locale, e.target_locale,
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

func (tm *SQLiteTM) searchLike(query, sourceLocale, targetLocale string, offset, limit int) ([]TMEntry, int) {
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

	q := fmt.Sprintf(`SELECT id, project_id, source_coded, target_coded, source_locale, target_locale,
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

// SearchEntriesForStream performs a ranked full-text search with stream
// inheritance. The streamChain is an ordered list of streams to search (e.g.,
// ["feature/rebrand", "main", ""]). Entries from earlier streams in the chain
// take priority.
func (tm *SQLiteTM) SearchEntriesForStream(query, sourceLocale, targetLocale, stream string, streamChain []string, offset, limit int) ([]TMEntry, int) {
	if query != "" {
		entries, total, err := tm.searchFTS5ForStream(query, sourceLocale, targetLocale, stream, streamChain, offset, limit)
		if err == nil {
			return entries, total
		}
	}
	return tm.searchLikeForStream(query, sourceLocale, targetLocale, stream, streamChain, offset, limit)
}

func (tm *SQLiteTM) searchFTS5ForStream(query, sourceLocale, targetLocale, stream string, streamChain []string, offset, limit int) ([]TMEntry, int, error) {
	streams := []string{stream}
	streams = append(streams, streamChain...)

	placeholders := make([]string, len(streams))
	var args []any
	for i, s := range streams {
		placeholders[i] = "?"
		args = append(args, s)
	}

	where := "e.stream IN (" + strings.Join(placeholders, ",") + ")"
	where += " AND e.rowid IN (SELECT rowid FROM tm_search WHERE tm_search MATCH ?)"
	args = append(args, query)

	if sourceLocale != "" {
		where += " AND e.source_locale = ?"
		args = append(args, sourceLocale)
	}
	if targetLocale != "" {
		where += " AND e.target_locale = ?"
		args = append(args, targetLocale)
	}

	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	if err := tm.db.QueryRow("SELECT COUNT(*) FROM tm_entries e WHERE "+where, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	var caseExpr strings.Builder
	caseExpr.WriteString("CASE e.stream")
	for i, s := range streams {
		caseExpr.WriteString(fmt.Sprintf(" WHEN ? THEN %d", i))
		args = append(args, s)
	}
	caseExpr.WriteString(fmt.Sprintf(" ELSE %d END", len(streams)))

	q := fmt.Sprintf(`SELECT e.id, e.project_id, e.source_coded, e.target_coded, e.source_locale, e.target_locale,
		e.entities, e.properties, e.created_at, e.updated_at
		FROM tm_entries e
		JOIN tm_search s ON s.rowid = e.rowid
		WHERE %s ORDER BY %s, s.rank LIMIT ? OFFSET ?`, where, caseExpr.String())
	args = append(args, limit, offset)
	rows, err := tm.db.Query(q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries, err := tm.scanEntries(rows)
	return entries, total, err
}

func (tm *SQLiteTM) searchLikeForStream(query, sourceLocale, targetLocale, stream string, streamChain []string, offset, limit int) ([]TMEntry, int) {
	streams := []string{stream}
	streams = append(streams, streamChain...)

	placeholders := make([]string, len(streams))
	var args []any
	for i, s := range streams {
		placeholders[i] = "?"
		args = append(args, s)
	}

	where := "stream IN (" + strings.Join(placeholders, ",") + ")"

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

	var caseExpr strings.Builder
	caseExpr.WriteString("CASE stream")
	for i, s := range streams {
		caseExpr.WriteString(fmt.Sprintf(" WHEN ? THEN %d", i))
		args = append(args, s)
	}
	caseExpr.WriteString(fmt.Sprintf(" ELSE %d END", len(streams)))

	q := fmt.Sprintf(`SELECT id, project_id, source_coded, target_coded, source_locale, target_locale,
		entities, properties, created_at, updated_at
		FROM tm_entries WHERE %s ORDER BY %s, updated_at DESC LIMIT ? OFFSET ?`, where, caseExpr.String())
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
func (tm *SQLiteTM) GetEntry(id string) (TMEntry, bool) {
	rows, err := tm.db.Query(`
		SELECT id, project_id, source_coded, target_coded, source_locale, target_locale,
			entities, properties, created_at, updated_at
		FROM tm_entries WHERE id = ?
	`, id)
	if err != nil {
		return TMEntry{}, false
	}
	defer rows.Close()

	entries, err := tm.scanEntries(rows)
	if err != nil || len(entries) == 0 {
		return TMEntry{}, false
	}
	return entries[0], true
}

// Entries returns all entries. Used for export operations.
func (tm *SQLiteTM) Entries() []TMEntry {
	rows, err := tm.db.Query(`
		SELECT id, project_id, source_coded, target_coded, source_locale, target_locale,
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
