package sievepen

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/gokapi/gokapi/bowrain/storage"
	"github.com/gokapi/gokapi/core/model"
	fw "github.com/gokapi/gokapi/core/sievepen"
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
			entities        TEXT,
			properties      TEXT,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (workspace_id, id)
		);
		CREATE INDEX IF NOT EXISTS idx_tm_ws_general ON tm_entries(workspace_id, source_general, source_locale, target_locale);
		CREATE INDEX IF NOT EXISTS idx_tm_ws_struct  ON tm_entries(workspace_id, source_struct, source_locale, target_locale);
		CREATE INDEX IF NOT EXISTS idx_tm_ws_plain   ON tm_entries(workspace_id, source_plain, source_locale, target_locale);
		`,
	},
}

// Add inserts or updates a translation memory entry.
func (tm *PostgresTM) Add(entry fw.TMEntry) error {
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
		INSERT INTO tm_entries (id, workspace_id, source_coded, target_coded,
			source_plain, source_struct, source_general,
			source_locale, target_locale,
			entities, properties,
			created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		ON CONFLICT (workspace_id, id) DO UPDATE SET
			source_coded = EXCLUDED.source_coded,
			target_coded = EXCLUDED.target_coded,
			source_plain = EXCLUDED.source_plain,
			source_struct = EXCLUDED.source_struct,
			source_general = EXCLUDED.source_general,
			source_locale = EXCLUDED.source_locale,
			target_locale = EXCLUDED.target_locale,
			entities = EXCLUDED.entities,
			properties = EXCLUDED.properties,
			updated_at = EXCLUDED.updated_at
	`, entry.ID, tm.workspaceID,
		string(sourceJSON), string(targetJSON),
		fw.NormalizeText(entry.SourceText()),
		fw.NormalizeText(entry.SourceStructural()),
		fw.NormalizeText(entry.SourceGeneralized()),
		string(entry.SourceLocale), string(entry.TargetLocale),
		nullableString(entitiesJSON), nullableString(propertiesJSON),
		entry.CreatedAt, entry.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert entry: %w", err)
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

	// Fuzzy matching: scan all entries for locale pair.
	allEntries, err := tm.queryLocale(sourceLocale, targetLocale)
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

func (tm *PostgresTM) queryExact(column, value string, sourceLocale, targetLocale model.LocaleID) ([]fw.TMEntry, error) {
	query := fmt.Sprintf(`
		SELECT id, source_coded, target_coded, source_locale, target_locale,
			entities, properties, created_at, updated_at
		FROM tm_entries
		WHERE workspace_id = $1 AND %s = $2 AND source_locale = $3 AND target_locale = $4
	`, column)

	rows, err := tm.db.Query(query, tm.workspaceID, value, string(sourceLocale), string(targetLocale))
	if err != nil {
		return nil, fmt.Errorf("query exact: %w", err)
	}
	defer rows.Close()

	return scanTMEntries(rows)
}

func (tm *PostgresTM) queryLocale(sourceLocale, targetLocale model.LocaleID) ([]fw.TMEntry, error) {
	rows, err := tm.db.Query(`
		SELECT id, source_coded, target_coded, source_locale, target_locale,
			entities, properties, created_at, updated_at
		FROM tm_entries
		WHERE workspace_id = $1 AND source_locale = $2 AND target_locale = $3
	`, tm.workspaceID, string(sourceLocale), string(targetLocale))
	if err != nil {
		return nil, fmt.Errorf("query locale: %w", err)
	}
	defer rows.Close()

	return scanTMEntries(rows)
}

// Delete removes an entry by ID.
func (tm *PostgresTM) Delete(id string) error {
	result, err := tm.db.Exec("DELETE FROM tm_entries WHERE workspace_id = $1 AND id = $2", tm.workspaceID, id)
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
	_ = tm.db.QueryRow("SELECT COUNT(*) FROM tm_entries WHERE workspace_id = $1", tm.workspaceID).Scan(&count)
	return count
}

// Close is a no-op for PostgresTM since the connection is shared.
func (tm *PostgresTM) Close() error {
	return nil
}

// SearchEntries performs a case-insensitive substring search on source/target text.
func (tm *PostgresTM) SearchEntries(query, sourceLocale, targetLocale string, offset, limit int) ([]fw.TMEntry, int) {
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
	_ = tm.db.QueryRow("SELECT COUNT(*) FROM tm_entries WHERE "+where, countArgs...).Scan(&total)

	q := fmt.Sprintf(`SELECT id, source_coded, target_coded, source_locale, target_locale,
		entities, properties, created_at, updated_at
		FROM tm_entries WHERE %s ORDER BY updated_at DESC LIMIT $%d OFFSET $%d`, where, argN, argN+1)
	args = append(args, limit, offset)
	rows, err := tm.db.Query(q, args...)
	if err != nil {
		return nil, total
	}
	defer rows.Close()

	entries, _ := scanTMEntries(rows)
	return entries, total
}

// GetEntry fetches a single entry by ID.
func (tm *PostgresTM) GetEntry(id string) (fw.TMEntry, bool) {
	rows, err := tm.db.Query(`
		SELECT id, source_coded, target_coded, source_locale, target_locale,
			entities, properties, created_at, updated_at
		FROM tm_entries WHERE workspace_id = $1 AND id = $2
	`, tm.workspaceID, id)
	if err != nil {
		return fw.TMEntry{}, false
	}
	defer rows.Close()

	entries, err := scanTMEntries(rows)
	if err != nil || len(entries) == 0 {
		return fw.TMEntry{}, false
	}
	return entries[0], true
}

// Entries returns all entries for this workspace.
func (tm *PostgresTM) Entries() []fw.TMEntry {
	rows, err := tm.db.Query(`
		SELECT id, source_coded, target_coded, source_locale, target_locale,
			entities, properties, created_at, updated_at
		FROM tm_entries WHERE workspace_id = $1 ORDER BY id
	`, tm.workspaceID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	entries, _ := scanTMEntries(rows)
	return entries
}

// scanTMEntries is a shared scanner for both SQLite and PostgreSQL.
func scanTMEntries(rows *sql.Rows) ([]fw.TMEntry, error) {
	var entries []fw.TMEntry
	for rows.Next() {
		var entry fw.TMEntry
		var sourceJSON, targetJSON string
		var srcLocale, tgtLocale string
		var entitiesJSON, propertiesJSON *string
		var createdAt, updatedAt time.Time

		if err := rows.Scan(&entry.ID, &sourceJSON, &targetJSON,
			&srcLocale, &tgtLocale,
			&entitiesJSON, &propertiesJSON,
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

