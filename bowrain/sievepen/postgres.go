package sievepen

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/model"
	fw "github.com/neokapi/neokapi/sievepen"
)

// PostgresTM is a persistent translation memory backed by PostgreSQL.
// All workspace TMs share the same PostgreSQL database, isolated by workspace_id.
type PostgresTM struct {
	db          *storage.PgDB
	workspaceID string
}

// NewPostgresTMFromDB creates a PostgresTM using an existing shared PgDB connection.
// workspaceID scopes all entries to a specific workspace.
func NewPostgresTMFromDB(db *storage.PgDB, workspaceID string) (*PostgresTM, error) {
	if err := storage.MigratePostgresNS(db, "tm_schema_migrations", tmMigrationsPg); err != nil {
		return nil, fmt.Errorf("migrate TM schema: %w", err)
	}
	return &PostgresTM{db: db, workspaceID: workspaceID}, nil
}

var tmMigrationsPg = []storage.Migration{
	{
		Version:     1,
		Description: "content-aware TM schema",
		SQL: `
		CREATE TABLE IF NOT EXISTS tm_entries (
			id              TEXT NOT NULL,
			workspace_id    TEXT NOT NULL,
			source_coded    TEXT NOT NULL,
			target_coded    TEXT NOT NULL,
			source_plain    TEXT NOT NULL,
			source_struct   TEXT NOT NULL,
			source_general  TEXT NOT NULL,
			source_locale   TEXT NOT NULL,
			target_locale   TEXT NOT NULL,
			properties      TEXT,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (workspace_id, id)
		);
		CREATE INDEX IF NOT EXISTS idx_tm_ws_general ON tm_entries(workspace_id, source_general, source_locale, target_locale);
		CREATE INDEX IF NOT EXISTS idx_tm_ws_struct  ON tm_entries(workspace_id, source_struct, source_locale, target_locale);
		CREATE INDEX IF NOT EXISTS idx_tm_ws_plain   ON tm_entries(workspace_id, source_plain, source_locale, target_locale);

		CREATE TABLE IF NOT EXISTS tm_entity_mappings (
			workspace_id   TEXT NOT NULL,
			entry_id       TEXT NOT NULL,
			ordinal        INTEGER NOT NULL,
			placeholder_id TEXT NOT NULL,
			entity_type    TEXT NOT NULL,
			source_value   TEXT NOT NULL DEFAULT '',
			source_start   INTEGER NOT NULL DEFAULT 0,
			source_end     INTEGER NOT NULL DEFAULT 0,
			target_value   TEXT NOT NULL DEFAULT '',
			target_start   INTEGER NOT NULL DEFAULT 0,
			target_end     INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (workspace_id, entry_id, ordinal),
			FOREIGN KEY (workspace_id, entry_id) REFERENCES tm_entries(workspace_id, id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_tm_ent_type ON tm_entity_mappings(workspace_id, entity_type);
		CREATE INDEX IF NOT EXISTS idx_tm_ent_value_type ON tm_entity_mappings(workspace_id, source_value, entity_type);
		`,
	},
	{
		Version:     2,
		Description: "add stream column",
		SQL: `ALTER TABLE tm_entries ADD COLUMN stream TEXT NOT NULL DEFAULT '';
		CREATE INDEX IF NOT EXISTS idx_tm_ws_stream ON tm_entries(workspace_id, stream, source_locale, target_locale);`,
	},
	{
		Version:     3,
		Description: "pg_trgm trigram indexes for fuzzy candidate retrieval + tsvector for UI search",
		SQL: `
		CREATE EXTENSION IF NOT EXISTS pg_trgm;
		CREATE EXTENSION IF NOT EXISTS fuzzystrmatch;

		CREATE INDEX IF NOT EXISTS idx_tm_trgm_plain   ON tm_entries USING gin (source_plain gin_trgm_ops);
		CREATE INDEX IF NOT EXISTS idx_tm_trgm_struct   ON tm_entries USING gin (source_struct gin_trgm_ops);
		CREATE INDEX IF NOT EXISTS idx_tm_trgm_general  ON tm_entries USING gin (source_general gin_trgm_ops);

		ALTER TABLE tm_entries ADD COLUMN search_tsv tsvector;
		UPDATE tm_entries SET search_tsv = to_tsvector('simple', source_plain || ' ' || COALESCE(target_coded, ''));
		CREATE INDEX IF NOT EXISTS idx_tm_search_tsv ON tm_entries USING gin (search_tsv);

		CREATE OR REPLACE FUNCTION tm_search_tsv_update() RETURNS trigger AS $$
		BEGIN
			NEW.search_tsv := to_tsvector('simple', NEW.source_plain || ' ' || COALESCE(NEW.target_coded, ''));
			RETURN NEW;
		END $$ LANGUAGE plpgsql;

		DROP TRIGGER IF EXISTS tm_search_tsv_trigger ON tm_entries;
		CREATE TRIGGER tm_search_tsv_trigger BEFORE INSERT OR UPDATE ON tm_entries
			FOR EACH ROW EXECUTE FUNCTION tm_search_tsv_update();
		`,
	},
}

// Add inserts or updates a translation memory entry with an empty stream.
func (tm *PostgresTM) Add(entry fw.TMEntry) error {
	return tm.AddWithStream(entry, "")
}

// AddWithStream inserts or updates a translation memory entry associated with a stream.
func (tm *PostgresTM) AddWithStream(entry fw.TMEntry, stream string) error {
	if entry.ID == "" {
		return errors.New("entry ID is required")
	}
	if entry.Source == nil {
		return errors.New("entry source Fragment is required")
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

	var propertiesJSON []byte
	if len(entry.Properties) > 0 {
		propertiesJSON, _ = json.Marshal(entry.Properties)
	}

	_, err = tm.db.ExecContext(context.Background(), `
		INSERT INTO tm_entries (id, workspace_id, stream, source_coded, target_coded,
			source_plain, source_struct, source_general,
			source_locale, target_locale,
			properties,
			created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (workspace_id, id) DO UPDATE SET
			stream = EXCLUDED.stream,
			source_coded = EXCLUDED.source_coded,
			target_coded = EXCLUDED.target_coded,
			source_plain = EXCLUDED.source_plain,
			source_struct = EXCLUDED.source_struct,
			source_general = EXCLUDED.source_general,
			source_locale = EXCLUDED.source_locale,
			target_locale = EXCLUDED.target_locale,
			properties = EXCLUDED.properties,
			updated_at = EXCLUDED.updated_at
	`, entry.ID, tm.workspaceID, stream,
		string(sourceJSON), string(targetJSON),
		fw.NormalizeText(entry.SourceText()),
		fw.NormalizeText(entry.SourceStructural()),
		fw.NormalizeText(entry.SourceGeneralized()),
		string(entry.SourceLocale), string(entry.TargetLocale),
		nullableString(propertiesJSON),
		entry.CreatedAt, entry.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert entry: %w", err)
	}

	// Replace entity mappings — single source of truth.
	if _, err := tm.db.ExecContext(context.Background(),
		"DELETE FROM tm_entity_mappings WHERE workspace_id = $1 AND entry_id = $2",
		tm.workspaceID, entry.ID); err != nil {
		return fmt.Errorf("delete entity mappings: %w", err)
	}
	for i, em := range entry.Entities {
		if _, err := tm.db.ExecContext(context.Background(), `INSERT INTO tm_entity_mappings
			(workspace_id, entry_id, ordinal, placeholder_id, entity_type,
			 source_value, source_start, source_end,
			 target_value, target_start, target_end)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			tm.workspaceID, entry.ID, i, em.PlaceholderID, string(em.Type),
			em.SourceValue, em.SourcePos.Start, em.SourcePos.End,
			em.TargetValue, em.TargetPos.Start, em.TargetPos.End); err != nil {
			return fmt.Errorf("insert entity mapping: %w", err)
		}
	}

	return nil
}

// Lookup searches for matches using tiered matching with the full content model.
func (tm *PostgresTM) Lookup(source *model.Block, sourceLocale, targetLocale model.LocaleID, opts fw.LookupOptions) ([]fw.TMMatch, error) {
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
func (tm *PostgresTM) LookupText(source string, sourceLocale, targetLocale model.LocaleID, opts fw.LookupOptions) ([]fw.TMMatch, error) {
	opts = fw.ApplyDefaults(opts)
	opts.MatchModes = []fw.MatchMode{fw.MatchModePlain}
	normalizedSource := fw.NormalizeText(source)
	return tm.tieredLookup(normalizedSource, normalizedSource, normalizedSource, nil, sourceLocale, targetLocale, opts)
}

func (tm *PostgresTM) tieredLookup(plainKey, structKey, generalKey string, entityAnnotations []*model.EntityAnnotation, sourceLocale, targetLocale model.LocaleID, opts fw.LookupOptions) ([]fw.TMMatch, error) {
	var matches []fw.TMMatch
	seen := make(map[string]bool)

	modeEnabled := fw.MatchModesEnabled(opts.MatchModes)

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

	// Fuzzy matching: trigram candidate retrieval + Levenshtein scoring.
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

	slices.SortFunc(matches, func(a, b fw.TMMatch) int {
		pa := fw.MatchTypePriority(a.MatchType)
		pb := fw.MatchTypePriority(b.MatchType)
		if c := cmp.Compare(pa, pb); c != 0 {
			return c
		}
		return cmp.Compare(b.Score, a.Score)
	})

	return fw.LimitResults(matches, opts.MaxResults), nil
}

func (tm *PostgresTM) queryExact(column, value string, sourceLocale, targetLocale model.LocaleID) ([]fw.TMEntry, error) {
	query := fmt.Sprintf(`
		SELECT id, source_coded, target_coded, source_locale, target_locale,
			properties, created_at, updated_at
		FROM tm_entries
		WHERE workspace_id = $1 AND %s = $2 AND source_locale = $3 AND target_locale = $4
	`, column)

	rows, err := tm.db.QueryContext(context.Background(), query, tm.workspaceID, value, string(sourceLocale), string(targetLocale))
	if err != nil {
		return nil, fmt.Errorf("query exact: %w", err)
	}
	defer rows.Close()

	return tm.scanTMEntries(rows)
}

// queryFuzzyCandidates uses pg_trgm indexes to retrieve a limited set of
// candidate entries for Levenshtein scoring, replacing the previous full table scan.
// Falls back to length-based pre-filtering if pg_trgm is unavailable.
func (tm *PostgresTM) queryFuzzyCandidates(plainKey, structKey, generalKey string, sourceLocale, targetLocale model.LocaleID) ([]fw.TMEntry, error) {
	entries, err := tm.queryTrigramCandidates(plainKey, structKey, generalKey, sourceLocale, targetLocale)
	if err == nil {
		return entries, nil
	}

	// Fallback: length-based pre-filtering.
	return tm.queryLengthFiltered(plainKey, sourceLocale, targetLocale)
}

func (tm *PostgresTM) queryTrigramCandidates(plainKey, structKey, generalKey string, sourceLocale, targetLocale model.LocaleID) ([]fw.TMEntry, error) {
	// Use pg_trgm similarity operator (%) on all three key columns.
	// Set a low threshold to maximize recall; final scoring is done in Go.
	rows, err := tm.db.QueryContext(context.Background(), `
		SELECT DISTINCT ON (id) id, source_coded, target_coded, source_locale, target_locale,
			properties, created_at, updated_at
		FROM tm_entries
		WHERE workspace_id = $1
			AND source_locale = $2 AND target_locale = $3
			AND (source_plain % $4 OR source_struct % $5 OR source_general % $6)
		LIMIT 200
	`, tm.workspaceID, string(sourceLocale), string(targetLocale),
		plainKey, structKey, generalKey)
	if err != nil {
		return nil, fmt.Errorf("trigram query: %w", err)
	}
	defer rows.Close()

	return tm.scanTMEntries(rows)
}

func (tm *PostgresTM) queryLengthFiltered(plainKey string, sourceLocale, targetLocale model.LocaleID) ([]fw.TMEntry, error) {
	keyLen := len([]rune(plainKey))
	minLen := int(float64(keyLen) * 0.7)
	maxLen := int(float64(keyLen) * 1.3)
	if minLen < 0 {
		minLen = 0
	}

	rows, err := tm.db.QueryContext(context.Background(), `
		SELECT id, source_coded, target_coded, source_locale, target_locale,
			properties, created_at, updated_at
		FROM tm_entries
		WHERE workspace_id = $1 AND source_locale = $2 AND target_locale = $3
			AND LENGTH(source_plain) BETWEEN $4 AND $5
		LIMIT 500
	`, tm.workspaceID, string(sourceLocale), string(targetLocale), minLen, maxLen)
	if err != nil {
		return nil, fmt.Errorf("length-filtered query: %w", err)
	}
	defer rows.Close()

	return tm.scanTMEntries(rows)
}

// Delete removes an entry by ID.
func (tm *PostgresTM) Delete(id string) error {
	result, err := tm.db.ExecContext(context.Background(), "DELETE FROM tm_entries WHERE workspace_id = $1 AND id = $2", tm.workspaceID, id)
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

// Count returns the total number of entries for this workspace.
func (tm *PostgresTM) Count() int {
	var count int
	if err := tm.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM tm_entries WHERE workspace_id = $1", tm.workspaceID).Scan(&count); err != nil {
		slog.Warn("TM count query failed", "workspace", tm.workspaceID, "error", err)
		return 0
	}
	return count
}

// Close is a no-op for PostgresTM since the connection is shared.
func (tm *PostgresTM) Close() error {
	return nil
}

// SearchEntries performs a ranked full-text search using tsvector with BM25-like ranking.
// Falls back to LIKE-based substring search if tsvector column is unavailable.
func (tm *PostgresTM) SearchEntries(query, sourceLocale, targetLocale string, offset, limit int) ([]fw.TMEntry, int) {
	if query != "" {
		entries, total, err := tm.searchTSVector(query, sourceLocale, targetLocale, offset, limit)
		if err == nil {
			return entries, total
		}
	}
	return tm.pgSearchLike(query, sourceLocale, targetLocale, offset, limit)
}

func (tm *PostgresTM) searchTSVector(query, sourceLocale, targetLocale string, offset, limit int) ([]fw.TMEntry, int, error) {
	where := "workspace_id = $1 AND search_tsv @@ plainto_tsquery('simple', $2)"
	args := []any{tm.workspaceID, query}
	argN := 3

	if sourceLocale != "" {
		where += fmt.Sprintf(" AND source_locale = $%d", argN)
		args = append(args, sourceLocale)
		argN++
	}
	if targetLocale != "" {
		where += fmt.Sprintf(" AND target_locale = $%d", argN)
		args = append(args, targetLocale)
		argN++
	}

	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	if err := tm.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM tm_entries WHERE "+where, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	q := fmt.Sprintf(`SELECT id, source_coded, target_coded, source_locale, target_locale,
		properties, created_at, updated_at
		FROM tm_entries WHERE %s
		ORDER BY ts_rank(search_tsv, plainto_tsquery('simple', $2)) DESC
		LIMIT $%d OFFSET $%d`, where, argN, argN+1)
	args = append(args, limit, offset)
	rows, err := tm.db.QueryContext(context.Background(), q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries, err := tm.scanTMEntries(rows)
	return entries, total, err
}

func (tm *PostgresTM) pgSearchLike(query, sourceLocale, targetLocale string, offset, limit int) ([]fw.TMEntry, int) {
	where := "workspace_id = $1"
	args := []any{tm.workspaceID}
	argN := 2

	if query != "" {
		where += fmt.Sprintf(" AND (LOWER(source_plain) LIKE $%d OR LOWER(target_coded) LIKE $%d)", argN, argN+1)
		pattern := "%" + strings.ToLower(query) + "%"
		args = append(args, pattern, pattern)
		argN += 2
	}
	if sourceLocale != "" {
		where += fmt.Sprintf(" AND source_locale = $%d", argN)
		args = append(args, sourceLocale)
		argN++
	}
	if targetLocale != "" {
		where += fmt.Sprintf(" AND target_locale = $%d", argN)
		args = append(args, targetLocale)
		argN++
	}

	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	_ = tm.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM tm_entries WHERE "+where, countArgs...).Scan(&total)

	q := fmt.Sprintf(`SELECT id, source_coded, target_coded, source_locale, target_locale,
		properties, created_at, updated_at
		FROM tm_entries WHERE %s ORDER BY updated_at DESC LIMIT $%d OFFSET $%d`, where, argN, argN+1)
	args = append(args, limit, offset)
	rows, err := tm.db.QueryContext(context.Background(), q, args...)
	if err != nil {
		return nil, total
	}
	defer rows.Close()

	entries, _ := tm.scanTMEntries(rows)
	return entries, total
}

// SearchEntriesForStream performs a ranked full-text search with stream
// inheritance. Uses tsvector when a query is provided, falls back to LIKE.
// The streamChain is an ordered list of ancestor streams to search.
// Entries from earlier streams in the chain take priority.
func (tm *PostgresTM) SearchEntriesForStream(query, sourceLocale, targetLocale, stream string, streamChain []string, offset, limit int) ([]fw.TMEntry, int) {
	if query != "" {
		entries, total, err := tm.searchTSVectorForStream(query, sourceLocale, targetLocale, stream, streamChain, offset, limit)
		if err == nil {
			return entries, total
		}
	}
	return tm.pgSearchLikeForStream(query, sourceLocale, targetLocale, stream, streamChain, offset, limit)
}

func (tm *PostgresTM) searchTSVectorForStream(query, sourceLocale, targetLocale, stream string, streamChain []string, offset, limit int) ([]fw.TMEntry, int, error) {
	streams := []string{stream}
	streams = append(streams, streamChain...)

	where := "workspace_id = $1 AND search_tsv @@ plainto_tsquery('simple', $2)"
	args := []any{tm.workspaceID, query}
	argN := 3

	// Stream filter.
	placeholders := make([]string, len(streams))
	for i, s := range streams {
		placeholders[i] = fmt.Sprintf("$%d", argN)
		args = append(args, s)
		argN++
	}
	where += " AND stream IN (" + strings.Join(placeholders, ",") + ")"

	if sourceLocale != "" {
		where += fmt.Sprintf(" AND source_locale = $%d", argN)
		args = append(args, sourceLocale)
		argN++
	}
	if targetLocale != "" {
		where += fmt.Sprintf(" AND target_locale = $%d", argN)
		args = append(args, targetLocale)
		argN++
	}

	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	if err := tm.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM tm_entries WHERE "+where, countArgs...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Build CASE expression for stream priority ordering.
	var caseExpr strings.Builder
	caseExpr.WriteString("CASE stream")
	for i, s := range streams {
		caseExpr.WriteString(fmt.Sprintf(" WHEN $%d THEN %d", argN, i))
		args = append(args, s)
		argN++
	}
	caseExpr.WriteString(fmt.Sprintf(" ELSE %d END", len(streams)))

	q := fmt.Sprintf(`SELECT id, source_coded, target_coded, source_locale, target_locale,
		properties, created_at, updated_at
		FROM tm_entries WHERE %s
		ORDER BY %s, ts_rank(search_tsv, plainto_tsquery('simple', $2)) DESC
		LIMIT $%d OFFSET $%d`, where, caseExpr.String(), argN, argN+1)
	args = append(args, limit, offset)
	rows, err := tm.db.QueryContext(context.Background(), q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	entries, err := tm.scanTMEntries(rows)
	return entries, total, err
}

func (tm *PostgresTM) pgSearchLikeForStream(query, sourceLocale, targetLocale, stream string, streamChain []string, offset, limit int) ([]fw.TMEntry, int) {
	streams := []string{stream}
	streams = append(streams, streamChain...)

	where := "workspace_id = $1"
	args := []any{tm.workspaceID}
	argN := 2

	// Stream filter.
	placeholders := make([]string, len(streams))
	for i, s := range streams {
		placeholders[i] = fmt.Sprintf("$%d", argN)
		args = append(args, s)
		argN++
	}
	where += " AND stream IN (" + strings.Join(placeholders, ",") + ")"

	if query != "" {
		where += fmt.Sprintf(" AND (LOWER(source_plain) LIKE $%d OR LOWER(target_coded) LIKE $%d)", argN, argN+1)
		pattern := "%" + strings.ToLower(query) + "%"
		args = append(args, pattern, pattern)
		argN += 2
	}
	if sourceLocale != "" {
		where += fmt.Sprintf(" AND source_locale = $%d", argN)
		args = append(args, sourceLocale)
		argN++
	}
	if targetLocale != "" {
		where += fmt.Sprintf(" AND target_locale = $%d", argN)
		args = append(args, targetLocale)
		argN++
	}

	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	_ = tm.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM tm_entries WHERE "+where, countArgs...).Scan(&total)

	// Build CASE expression for stream priority ordering.
	var caseExpr strings.Builder
	caseExpr.WriteString("CASE stream")
	for i, s := range streams {
		caseExpr.WriteString(fmt.Sprintf(" WHEN $%d THEN %d", argN, i))
		args = append(args, s)
		argN++
	}
	caseExpr.WriteString(fmt.Sprintf(" ELSE %d END", len(streams)))

	q := fmt.Sprintf(`SELECT id, source_coded, target_coded, source_locale, target_locale,
		properties, created_at, updated_at
		FROM tm_entries WHERE %s ORDER BY %s, updated_at DESC LIMIT $%d OFFSET $%d`, where, caseExpr.String(), argN, argN+1)
	args = append(args, limit, offset)
	rows, err := tm.db.QueryContext(context.Background(), q, args...)
	if err != nil {
		return nil, total
	}
	defer rows.Close()

	entries, _ := tm.scanTMEntries(rows)
	return entries, total
}

// SearchEntriesGrouped returns entries grouped by source text.
// This is a stub implementation for PostgresTM — full implementation is pending.
func (tm *PostgresTM) SearchEntriesGrouped(query, sourceLocale string, offset, limit int) ([]fw.TMEntryGroup, int) {
	// Use flat search and group in Go.
	entries, total := tm.SearchEntries(query, sourceLocale, "", offset*limit, limit*10)
	if len(entries) == 0 {
		return nil, 0
	}

	type groupInfo struct {
		entries []fw.TMEntry
	}
	groups := make(map[string]*groupInfo)
	var groupOrder []string
	for _, e := range entries {
		key := fw.NormalizeText(e.SourceText())
		g, ok := groups[key]
		if !ok {
			g = &groupInfo{}
			groups[key] = g
			groupOrder = append(groupOrder, key)
		}
		g.entries = append(g.entries, e)
	}

	totalGroups := len(groupOrder)
	if offset >= totalGroups {
		return nil, total
	}
	end := offset + limit
	if end > totalGroups {
		end = totalGroups
	}

	var result []fw.TMEntryGroup
	for _, key := range groupOrder[offset:end] {
		g := groups[key]
		first := g.entries[0]
		result = append(result, fw.TMEntryGroup{
			SourceText:   key,
			Source:       first.Source,
			SourceLocale: first.SourceLocale,
			Targets:      g.entries,
		})
	}
	return result, totalGroups
}

// SearchEntriesFiltered delegates to the unfiltered SearchEntries (filters not yet implemented for PostgresTM).
func (tm *PostgresTM) SearchEntriesFiltered(query, sourceLocale, targetLocale string, _ fw.SearchFilter, offset, limit int) ([]fw.TMEntry, int) {
	return tm.SearchEntries(query, sourceLocale, targetLocale, offset, limit)
}

// SearchEntriesGroupedFiltered delegates to the unfiltered SearchEntriesGrouped (filters not yet implemented for PostgresTM).
func (tm *PostgresTM) SearchEntriesGroupedFiltered(query, sourceLocale string, _ fw.SearchFilter, offset, limit int) ([]fw.TMEntryGroup, int) {
	return tm.SearchEntriesGrouped(query, sourceLocale, offset, limit)
}

// FacetStats returns aggregated facet data for filtering UI.
// This is a stub implementation for PostgresTM — full implementation is pending.
func (tm *PostgresTM) FacetStats() fw.FacetData {
	return fw.FacetData{}
}

// GetEntry fetches a single entry by ID.
func (tm *PostgresTM) GetEntry(id string) (fw.TMEntry, bool) {
	rows, err := tm.db.QueryContext(context.Background(), `
		SELECT id, source_coded, target_coded, source_locale, target_locale,
			properties, created_at, updated_at
		FROM tm_entries WHERE workspace_id = $1 AND id = $2
	`, tm.workspaceID, id)
	if err != nil {
		return fw.TMEntry{}, false
	}
	defer rows.Close()

	entries, err := tm.scanTMEntries(rows)
	if err != nil || len(entries) == 0 {
		return fw.TMEntry{}, false
	}
	return entries[0], true
}

// Entries returns all entries for this workspace.
func (tm *PostgresTM) Entries() []fw.TMEntry {
	rows, err := tm.db.QueryContext(context.Background(), `
		SELECT id, source_coded, target_coded, source_locale, target_locale,
			properties, created_at, updated_at
		FROM tm_entries WHERE workspace_id = $1 ORDER BY id
	`, tm.workspaceID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	entries, _ := tm.scanTMEntries(rows)
	return entries
}

// scanTMEntries scans rows from tm_entries and batch-fetches associated
// entity mappings from tm_entity_mappings for this workspace.
func (tm *PostgresTM) scanTMEntries(rows *sql.Rows) ([]fw.TMEntry, error) {
	var entries []fw.TMEntry
	for rows.Next() {
		var entry fw.TMEntry
		var sourceJSON, targetJSON string
		var srcLocale, tgtLocale string
		var propertiesJSON *string
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&entry.ID, &sourceJSON, &targetJSON,
			&srcLocale, &tgtLocale,
			&propertiesJSON,
			&createdAt, &updatedAt); err != nil {
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
		entry.CreatedAt = createdAt
		entry.UpdatedAt = updatedAt

		if propertiesJSON != nil && *propertiesJSON != "" {
			_ = json.Unmarshal([]byte(*propertiesJSON), &entry.Properties)
		}

		entries = append(entries, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	// Batch-fetch entity mappings for all loaded entries.
	if len(entries) > 0 {
		ids := make([]string, len(entries))
		for i, e := range entries {
			ids[i] = e.ID
		}
		placeholders := make([]string, len(ids))
		args := make([]any, 0, len(ids)+1)
		args = append(args, tm.workspaceID)
		for i, id := range ids {
			placeholders[i] = fmt.Sprintf("$%d", i+2)
			args = append(args, id)
		}
		q := `SELECT entry_id, placeholder_id, entity_type,
			source_value, source_start, source_end,
			target_value, target_start, target_end
			FROM tm_entity_mappings
			WHERE workspace_id = $1 AND entry_id IN (` + strings.Join(placeholders, ",") + `)
			ORDER BY entry_id, ordinal`
		mapRows, err := tm.db.QueryContext(context.Background(), q, args...)
		if err == nil {
			defer mapRows.Close()
			byID := make(map[string]int, len(entries))
			for i, e := range entries {
				byID[e.ID] = i
			}
			for mapRows.Next() {
				var eid string
				var em fw.EntityMapping
				var etype string
				if err := mapRows.Scan(&eid, &em.PlaceholderID, &etype,
					&em.SourceValue, &em.SourcePos.Start, &em.SourcePos.End,
					&em.TargetValue, &em.TargetPos.Start, &em.TargetPos.End); err != nil {
					continue
				}
				em.Type = model.EntityType(etype)
				if idx, ok := byID[eid]; ok {
					entries[idx].Entities = append(entries[idx].Entities, em)
				}
			}
		}
	}
	return entries, nil
}

func nullableString(b []byte) *string {
	if len(b) == 0 {
		return nil
	}
	s := string(b)
	return &s
}
