// Package sievepen provides a PostgreSQL-backed implementation of
// neokapi's multilingual translation memory. It mirrors the SQLite
// implementation in the framework module, with workspace_id as a
// composite PK component on every table for multi-tenant isolation.
package sievepen

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/model"
	fw "github.com/neokapi/neokapi/sievepen"
)

// PostgresTM is a persistent, multilingual translation memory backed by
// PostgreSQL. All workspace TMs share the same database, isolated by
// workspace_id.
type PostgresTM struct {
	db          *storage.PgDB
	workspaceID string
}

// NewPostgresTMFromDB creates a PostgresTM using an existing shared PgDB
// connection. workspaceID scopes all entries to a specific workspace.
func NewPostgresTMFromDB(db *storage.PgDB, workspaceID string) (*PostgresTM, error) {
	if err := storage.MigratePostgresNS(db, "tm_schema_migrations", tmMigrationsPg); err != nil {
		return nil, fmt.Errorf("migrate TM schema: %w", err)
	}
	return &PostgresTM{db: db, workspaceID: workspaceID}, nil
}

// tmMigrationsPg holds the evolution of the TM Postgres schema. Version 4
// wipes the legacy bilingual schema and creates the multilingual tables.
var tmMigrationsPg = []storage.Migration{
	{
		Version:     1,
		Description: "legacy bilingual TM schema (superseded by v4)",
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
			note            TEXT NOT NULL DEFAULT '',
			properties      TEXT,
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (workspace_id, id)
		);
		`,
	},
	{
		Version:     2,
		Description: "add stream column (legacy)",
		SQL:         `ALTER TABLE tm_entries ADD COLUMN IF NOT EXISTS stream TEXT NOT NULL DEFAULT '';`,
	},
	{
		Version:     3,
		Description: "placeholder (legacy tsvector)",
		SQL:         `SELECT 1;`,
	},
	{
		Version:     4,
		Description: "multilingual TM schema rewrite — variants, entities per locale, import sessions",
		SQL: `
		DROP TABLE IF EXISTS tm_entity_mappings CASCADE;
		DROP TABLE IF EXISTS tm_entry_origins CASCADE;
		DROP TABLE IF EXISTS tm_entries CASCADE;

		CREATE TABLE tm_entries (
			workspace_id    TEXT NOT NULL,
			id              TEXT NOT NULL,
			project_id      TEXT NOT NULL DEFAULT '',
			stream          TEXT NOT NULL DEFAULT '',
			hint_src_lang   TEXT NOT NULL DEFAULT '',
			properties      JSONB NOT NULL DEFAULT '{}'::jsonb,
			note            TEXT NOT NULL DEFAULT '',
			created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			PRIMARY KEY (workspace_id, id)
		);
		CREATE INDEX idx_tm_ws_project ON tm_entries(workspace_id, project_id);
		CREATE INDEX idx_tm_ws_stream  ON tm_entries(workspace_id, stream);
		CREATE INDEX idx_tm_ws_updated ON tm_entries(workspace_id, updated_at DESC);

		CREATE TABLE tm_variants (
			workspace_id TEXT NOT NULL,
			entry_id     TEXT NOT NULL,
			locale       TEXT NOT NULL,
			coded        TEXT NOT NULL,
			plain        TEXT NOT NULL,
			struct_key   TEXT NOT NULL,
			general_key  TEXT NOT NULL,
			PRIMARY KEY (workspace_id, entry_id, locale),
			FOREIGN KEY (workspace_id, entry_id) REFERENCES tm_entries(workspace_id, id) ON DELETE CASCADE
		);
		CREATE INDEX idx_tm_var_ws_locale      ON tm_variants(workspace_id, locale);
		CREATE INDEX idx_tm_var_plain_loc      ON tm_variants(workspace_id, plain, locale);
		CREATE INDEX idx_tm_var_struct_loc     ON tm_variants(workspace_id, struct_key, locale);
		CREATE INDEX idx_tm_var_general_loc    ON tm_variants(workspace_id, general_key, locale);

		CREATE EXTENSION IF NOT EXISTS pg_trgm;
		CREATE INDEX idx_tm_var_trgm_plain   ON tm_variants USING gin (plain gin_trgm_ops);
		CREATE INDEX idx_tm_var_trgm_struct  ON tm_variants USING gin (struct_key gin_trgm_ops);
		CREATE INDEX idx_tm_var_trgm_general ON tm_variants USING gin (general_key gin_trgm_ops);

		ALTER TABLE tm_variants ADD COLUMN search_tsv tsvector
			GENERATED ALWAYS AS (to_tsvector('simple', plain)) STORED;
		CREATE INDEX idx_tm_var_search_tsv ON tm_variants USING gin (search_tsv);

		CREATE TABLE tm_entry_entities (
			workspace_id   TEXT NOT NULL,
			entry_id       TEXT NOT NULL,
			placeholder_id TEXT NOT NULL,
			entity_type    TEXT NOT NULL,
			PRIMARY KEY (workspace_id, entry_id, placeholder_id),
			FOREIGN KEY (workspace_id, entry_id) REFERENCES tm_entries(workspace_id, id) ON DELETE CASCADE
		);
		CREATE INDEX idx_tm_entities_type ON tm_entry_entities(workspace_id, entity_type);

		CREATE TABLE tm_entry_entity_values (
			workspace_id   TEXT NOT NULL,
			entry_id       TEXT NOT NULL,
			placeholder_id TEXT NOT NULL,
			locale         TEXT NOT NULL,
			text_value     TEXT NOT NULL DEFAULT '',
			start_pos      INTEGER NOT NULL DEFAULT 0,
			end_pos        INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (workspace_id, entry_id, placeholder_id, locale),
			FOREIGN KEY (workspace_id, entry_id, placeholder_id)
				REFERENCES tm_entry_entities(workspace_id, entry_id, placeholder_id) ON DELETE CASCADE
		);
		CREATE INDEX idx_tm_entity_values_text ON tm_entry_entity_values(workspace_id, text_value, locale);

		CREATE TABLE tm_import_sessions (
			workspace_id      TEXT NOT NULL,
			id                TEXT NOT NULL,
			file_key          TEXT NOT NULL,
			file_hash         TEXT NOT NULL DEFAULT '',
			file_size_bytes   BIGINT NOT NULL DEFAULT 0,
			imported_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			imported_by       TEXT NOT NULL DEFAULT '',
			tool_name         TEXT NOT NULL DEFAULT '',
			tool_version      TEXT NOT NULL DEFAULT '',
			seg_type          TEXT NOT NULL DEFAULT '',
			admin_lang        TEXT NOT NULL DEFAULT '',
			src_lang          TEXT NOT NULL DEFAULT '',
			data_type         TEXT NOT NULL DEFAULT '',
			original_format   TEXT NOT NULL DEFAULT '',
			original_encoding TEXT NOT NULL DEFAULT '',
			entry_count       INTEGER NOT NULL DEFAULT 0,
			properties        JSONB NOT NULL DEFAULT '{}'::jsonb,
			PRIMARY KEY (workspace_id, id)
		);
		CREATE INDEX idx_tm_sessions_hash ON tm_import_sessions(workspace_id, file_hash);
		CREATE INDEX idx_tm_sessions_time ON tm_import_sessions(workspace_id, imported_at DESC);

		CREATE TABLE tm_entry_origins (
			workspace_id TEXT NOT NULL,
			entry_id     TEXT NOT NULL,
			ordinal      INTEGER NOT NULL,
			source       TEXT NOT NULL,
			key          TEXT NOT NULL DEFAULT '',
			reference    TEXT NOT NULL DEFAULT '',
			added_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			added_by     TEXT NOT NULL DEFAULT '',
			session_id   TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (workspace_id, entry_id, ordinal),
			FOREIGN KEY (workspace_id, entry_id) REFERENCES tm_entries(workspace_id, id) ON DELETE CASCADE
		);
		CREATE INDEX idx_tm_origin_source  ON tm_entry_origins(workspace_id, source);
		CREATE INDEX idx_tm_origin_key     ON tm_entry_origins(workspace_id, key);
		CREATE INDEX idx_tm_origin_session ON tm_entry_origins(workspace_id, session_id);
		`,
	},
	{
		Version:     5,
		Description: "add concept_id to entity mappings for termbase cross-reference",
		SQL: `
		ALTER TABLE tm_entry_entities ADD COLUMN IF NOT EXISTS concept_id TEXT NOT NULL DEFAULT '';
		CREATE INDEX IF NOT EXISTS idx_tm_entities_concept ON tm_entry_entities(workspace_id, concept_id);
		`,
	},
}

// --- basic ---

// Close is a no-op for PostgresTM; the connection is shared.
func (tm *PostgresTM) Close() error { return nil }

// Count returns the total number of entries for this workspace.
func (tm *PostgresTM) Count(ctx context.Context) (int, error) {
	var count int
	if err := tm.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tm_entries WHERE workspace_id = $1",
		tm.workspaceID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count entries: %w", err)
	}
	return count, nil
}

// --- writes ---

// Add inserts or updates a multilingual TM entry.
func (tm *PostgresTM) Add(ctx context.Context, entry fw.TMEntry) error {
	return tm.AddWithStream(ctx, entry, "")
}

// AddWithStream inserts or updates a multilingual TM entry on a given stream.
func (tm *PostgresTM) AddWithStream(ctx context.Context, entry fw.TMEntry, stream string) error {
	if entry.ID == "" {
		return errors.New("entry ID is required")
	}
	if len(entry.Variants) == 0 {
		return errors.New("entry must have at least one variant")
	}

	now := time.Now()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = now
	}

	propsJSON := []byte("{}")
	if len(entry.Properties) > 0 {
		if b, err := json.Marshal(entry.Properties); err == nil {
			propsJSON = b
		}
	}

	if _, err := tm.db.ExecContext(ctx, `
		INSERT INTO tm_entries
			(workspace_id, id, project_id, stream, hint_src_lang, properties, note, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9)
		ON CONFLICT (workspace_id, id) DO UPDATE SET
			project_id    = EXCLUDED.project_id,
			stream        = EXCLUDED.stream,
			hint_src_lang = EXCLUDED.hint_src_lang,
			properties    = EXCLUDED.properties,
			note          = EXCLUDED.note,
			updated_at    = EXCLUDED.updated_at
	`, tm.workspaceID, entry.ID, entry.ProjectID, stream,
		string(entry.HintSrcLang), string(propsJSON), entry.Note,
		entry.CreatedAt, entry.UpdatedAt); err != nil {
		return fmt.Errorf("upsert entry: %w", err)
	}

	// Replace variants.
	if _, err := tm.db.ExecContext(ctx,
		"DELETE FROM tm_variants WHERE workspace_id = $1 AND entry_id = $2",
		tm.workspaceID, entry.ID); err != nil {
		return fmt.Errorf("delete variants: %w", err)
	}
	for loc, runs := range entry.Variants {
		if len(runs) == 0 {
			continue
		}
		coded, err := json.Marshal(runs)
		if err != nil {
			return fmt.Errorf("marshal variant %s: %w", loc, err)
		}
		plain := fw.NormalizeText(model.FlattenRuns(runs))
		sk := fw.NormalizeText(model.RunsStructuralText(runs))
		gk := fw.NormalizeText(model.RunsGeneralizedText(runs))
		if _, err := tm.db.ExecContext(ctx, `INSERT INTO tm_variants
			(workspace_id, entry_id, locale, coded, plain, struct_key, general_key)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			tm.workspaceID, entry.ID, string(loc), string(coded), plain, sk, gk); err != nil {
			return fmt.Errorf("insert variant %s: %w", loc, err)
		}
	}

	// Replace entities + values. CASCADE on tm_entry_entities removes values.
	if _, err := tm.db.ExecContext(ctx,
		"DELETE FROM tm_entry_entities WHERE workspace_id = $1 AND entry_id = $2",
		tm.workspaceID, entry.ID); err != nil {
		return fmt.Errorf("delete entities: %w", err)
	}
	for _, em := range entry.Entities {
		if em.PlaceholderID == "" {
			continue
		}
		if _, err := tm.db.ExecContext(ctx, `INSERT INTO tm_entry_entities
			(workspace_id, entry_id, placeholder_id, entity_type, concept_id) VALUES ($1, $2, $3, $4, $5)`,
			tm.workspaceID, entry.ID, em.PlaceholderID, string(em.Type), em.ConceptID); err != nil {
			return fmt.Errorf("insert entity: %w", err)
		}
		for loc, val := range em.Values {
			if _, err := tm.db.ExecContext(ctx, `INSERT INTO tm_entry_entity_values
				(workspace_id, entry_id, placeholder_id, locale, text_value, start_pos, end_pos)
				VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				tm.workspaceID, entry.ID, em.PlaceholderID, string(loc),
				val.Text, val.Start, val.End); err != nil {
				return fmt.Errorf("insert entity value: %w", err)
			}
		}
	}

	// Replace origins.
	if _, err := tm.db.ExecContext(ctx,
		"DELETE FROM tm_entry_origins WHERE workspace_id = $1 AND entry_id = $2",
		tm.workspaceID, entry.ID); err != nil {
		return fmt.Errorf("delete origins: %w", err)
	}
	for i, o := range entry.Origins {
		addedAt := o.AddedAt
		if addedAt.IsZero() {
			addedAt = now
		}
		if _, err := tm.db.ExecContext(ctx, `INSERT INTO tm_entry_origins
			(workspace_id, entry_id, ordinal, source, key, reference, added_at, added_by, session_id)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			tm.workspaceID, entry.ID, i, o.Source, o.Key, o.Reference,
			addedAt, o.AddedBy, o.SessionID); err != nil {
			return fmt.Errorf("insert origin: %w", err)
		}
	}

	return nil
}

// Delete removes an entry by ID.
func (tm *PostgresTM) Delete(ctx context.Context, id string) error {
	result, err := tm.db.ExecContext(ctx,
		"DELETE FROM tm_entries WHERE workspace_id = $1 AND id = $2",
		tm.workspaceID, id)
	if err != nil {
		return fmt.Errorf("delete entry: %w", err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("entry not found: %s", id)
	}
	return nil
}

// --- lookup ---

// Lookup searches for matches using tiered matching.
func (tm *PostgresTM) Lookup(ctx context.Context, source *model.Block, sourceLocale, targetLocale model.LocaleID, opts fw.LookupOptions) ([]fw.TMMatch, error) {
	if source == nil {
		return nil, nil
	}
	opts = fw.ApplyDefaults(opts)
	runs := source.Source
	if len(runs) == 0 {
		return nil, nil
	}
	plainKey := fw.NormalizeText(model.FlattenRuns(runs))
	structKey := fw.NormalizeText(model.RunsStructuralText(runs))
	generalKey := fw.NormalizeText(model.RunsGeneralizedText(runs))
	entityAnnotations := fw.ExtractEntityAnnotations(source)
	return tm.tieredLookup(ctx, plainKey, structKey, generalKey, entityAnnotations, sourceLocale, targetLocale, opts)
}

// LookupSegment searches for matches against a specific segment of the
// source block. See TranslationMemory.LookupSegment for the contract.
func (tm *PostgresTM) LookupSegment(ctx context.Context, source *model.Block, segmentIdx int, sourceLocale, targetLocale model.LocaleID, opts fw.LookupOptions) ([]fw.TMMatch, error) {
	if source == nil {
		return nil, nil
	}
	runs := source.SourceSegmentRuns(segmentIdx)
	if len(runs) == 0 {
		return nil, nil
	}
	opts = fw.ApplyDefaults(opts)
	plainKey := fw.NormalizeText(model.FlattenRuns(runs))
	structKey := fw.NormalizeText(model.RunsStructuralText(runs))
	generalKey := fw.NormalizeText(model.RunsGeneralizedText(runs))
	entityAnnotations := fw.ExtractEntityAnnotations(source)
	return tm.tieredLookup(ctx, plainKey, structKey, generalKey, entityAnnotations, sourceLocale, targetLocale, opts)
}

// LookupText searches for plain-text matches.
func (tm *PostgresTM) LookupText(ctx context.Context, source string, sourceLocale, targetLocale model.LocaleID, opts fw.LookupOptions) ([]fw.TMMatch, error) {
	opts = fw.ApplyDefaults(opts)
	opts.MatchModes = []fw.MatchMode{fw.MatchModePlain}
	normalized := fw.NormalizeText(source)
	return tm.tieredLookup(ctx, normalized, normalized, normalized, nil, sourceLocale, targetLocale, opts)
}

func (tm *PostgresTM) tieredLookup(ctx context.Context, plainKey, structKey, generalKey string, entityAnnotations []*model.EntityAnnotation, sourceLocale, targetLocale model.LocaleID, opts fw.LookupOptions) ([]fw.TMMatch, error) {
	var matches []fw.TMMatch
	seen := make(map[string]bool)
	modeEnabled := fw.MatchModesEnabled(opts.MatchModes)

	add := func(entry fw.TMEntry, score float64, mt fw.MatchType) {
		if seen[entry.ID] {
			return
		}
		if !entry.HasLocale(targetLocale) {
			return
		}
		seen[entry.ID] = true
		var adaptations []fw.EntityAdaptation
		if mt == fw.MatchGeneralizedExact || mt == fw.MatchGeneralizedFuzzy {
			adaptations = fw.ComputeEntityAdaptations(entry, sourceLocale, targetLocale, entityAnnotations)
		}
		matches = append(matches, fw.TMMatch{
			Entry:             entry,
			Score:             score,
			MatchType:         mt,
			ProjectID:         entry.ProjectID,
			EntityAdaptations: adaptations,
		})
	}

	if modeEnabled[fw.MatchModeGeneralized] {
		entries, err := tm.queryExactVariant(ctx, "general_key", generalKey, sourceLocale, opts)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			add(e, 1.0, fw.MatchGeneralizedExact)
		}
	}
	if modeEnabled[fw.MatchModeStructural] {
		entries, err := tm.queryExactVariant(ctx, "struct_key", structKey, sourceLocale, opts)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			add(e, 1.0, fw.MatchStructuralExact)
		}
	}
	if modeEnabled[fw.MatchModePlain] {
		entries, err := tm.queryExactVariant(ctx, "plain", plainKey, sourceLocale, opts)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			add(e, 1.0, fw.MatchExact)
		}
	}

	if len(matches) > 0 && opts.MinScore >= 1.0 {
		return fw.LimitResults(matches, opts.MaxResults), nil
	}

	candidates, err := tm.queryFuzzyCandidates(ctx, plainKey, structKey, generalKey, sourceLocale, opts)
	if err != nil {
		return nil, err
	}
	for _, entry := range candidates {
		if seen[entry.ID] {
			continue
		}
		srcRuns := entry.Variant(sourceLocale)
		if len(srcRuns) == 0 {
			continue
		}
		var bestScore float64
		var bestType fw.MatchType
		if modeEnabled[fw.MatchModeGeneralized] {
			s := fw.LevenshteinRatio(generalKey, fw.NormalizeText(model.RunsGeneralizedText(srcRuns)))
			if s >= opts.MinScore && s > bestScore {
				bestScore = s
				bestType = fw.MatchGeneralizedFuzzy
			}
		}
		if modeEnabled[fw.MatchModeStructural] {
			s := fw.LevenshteinRatio(structKey, fw.NormalizeText(model.RunsStructuralText(srcRuns)))
			if s >= opts.MinScore && s > bestScore {
				bestScore = s
				bestType = fw.MatchStructuralFuzzy
			}
		}
		if modeEnabled[fw.MatchModePlain] {
			s := fw.LevenshteinRatio(plainKey, fw.NormalizeText(model.FlattenRuns(srcRuns)))
			if s >= opts.MinScore && s > bestScore {
				bestScore = s
				bestType = fw.MatchFuzzy
			}
		}
		if bestScore < opts.MinScore {
			continue
		}
		if opts.ProjectID != "" && entry.ProjectID == opts.ProjectID && bestScore < 1.0 {
			bestScore += 0.03
			if bestScore > 1.0 {
				bestScore = 1.0
			}
		}
		add(entry, bestScore, bestType)
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

func (tm *PostgresTM) queryExactVariant(ctx context.Context, column, key string, sourceLocale model.LocaleID, opts fw.LookupOptions) ([]fw.TMEntry, error) {
	q := fmt.Sprintf(`
		SELECT DISTINCT v.entry_id
		FROM tm_variants v
		INNER JOIN tm_entries e ON e.workspace_id = v.workspace_id AND e.id = v.entry_id
		WHERE v.workspace_id = $1 AND v.%s = $2 AND v.locale = $3
	`, column)
	args := []any{tm.workspaceID, key, string(sourceLocale)}
	argN := 4
	switch opts.ProjectScope {
	case fw.ProjectScopeOnly:
		q += fmt.Sprintf(" AND e.project_id = $%d", argN)
		args = append(args, opts.ProjectID)
	case fw.ProjectScopeExclude:
		q += fmt.Sprintf(" AND e.project_id != $%d", argN)
		args = append(args, opts.ProjectID)
	}
	q += " LIMIT 200"
	rows, err := tm.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query exact variant: %w", err)
	}
	defer rows.Close()
	ids, err := scanStringColumn(rows)
	if err != nil {
		return nil, err
	}
	return tm.loadEntriesByIDs(ctx, ids)
}

func (tm *PostgresTM) queryFuzzyCandidates(ctx context.Context, plainKey, structKey, generalKey string, sourceLocale model.LocaleID, _ fw.LookupOptions) ([]fw.TMEntry, error) {
	q := `
		SELECT DISTINCT entry_id FROM tm_variants
		WHERE workspace_id = $1 AND locale = $2
			AND (plain % $3 OR struct_key % $4 OR general_key % $5)
		LIMIT 200
	`
	rows, err := tm.db.QueryContext(ctx, q,
		tm.workspaceID, string(sourceLocale), plainKey, structKey, generalKey)
	if err != nil {
		return tm.queryLengthFiltered(ctx, plainKey, sourceLocale)
	}
	defer rows.Close()
	ids, err := scanStringColumn(rows)
	if err != nil {
		return nil, err
	}
	return tm.loadEntriesByIDs(ctx, ids)
}

func (tm *PostgresTM) queryLengthFiltered(ctx context.Context, plainKey string, sourceLocale model.LocaleID) ([]fw.TMEntry, error) {
	keyLen := len([]rune(plainKey))
	minLen := int(float64(keyLen) * 0.7)
	maxLen := int(float64(keyLen) * 1.3)
	if minLen < 0 {
		minLen = 0
	}
	rows, err := tm.db.QueryContext(ctx, `
		SELECT DISTINCT entry_id FROM tm_variants
		WHERE workspace_id = $1 AND locale = $2 AND CHAR_LENGTH(plain) BETWEEN $3 AND $4
		LIMIT 500
	`, tm.workspaceID, string(sourceLocale), minLen, maxLen)
	if err != nil {
		return nil, fmt.Errorf("length-filtered query: %w", err)
	}
	defer rows.Close()
	ids, err := scanStringColumn(rows)
	if err != nil {
		return nil, err
	}
	return tm.loadEntriesByIDs(ctx, ids)
}

// --- entry loading ---

// GetEntry fetches a single entry by ID.
func (tm *PostgresTM) GetEntry(ctx context.Context, id string) (fw.TMEntry, bool, error) {
	entries, err := tm.loadEntriesByIDs(ctx, []string{id})
	if err != nil {
		return fw.TMEntry{}, false, err
	}
	if len(entries) == 0 {
		return fw.TMEntry{}, false, nil
	}
	return entries[0], true, nil
}

// Entries returns all entries for this workspace.
func (tm *PostgresTM) Entries(ctx context.Context) ([]fw.TMEntry, error) {
	rows, err := tm.db.QueryContext(ctx,
		"SELECT id FROM tm_entries WHERE workspace_id = $1 ORDER BY id",
		tm.workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list entry ids: %w", err)
	}
	defer rows.Close()
	ids, err := scanStringColumn(rows)
	if err != nil {
		return nil, err
	}
	return tm.loadEntriesByIDs(ctx, ids)
}

func (tm *PostgresTM) loadEntriesByIDs(ctx context.Context, ids []string) ([]fw.TMEntry, error) {
	if len(ids) == 0 {
		return nil, nil
	}

	// Build placeholders $2..$N for IDs; $1 is workspace.
	placeholders := make([]string, len(ids))
	args := make([]any, 0, len(ids)+1)
	args = append(args, tm.workspaceID)
	for i, id := range ids {
		placeholders[i] = fmt.Sprintf("$%d", i+2)
		args = append(args, id)
	}
	inClause := strings.Join(placeholders, ",")

	entryQ := `SELECT id, project_id, hint_src_lang, properties::text, note, created_at, updated_at
		FROM tm_entries WHERE workspace_id = $1 AND id IN (` + inClause + `)`
	rows, err := tm.db.QueryContext(ctx, entryQ, args...)
	if err != nil {
		return nil, fmt.Errorf("load entries: %w", err)
	}
	defer rows.Close()

	var entries []fw.TMEntry
	for rows.Next() {
		var e fw.TMEntry
		var hint, propsJSON, note string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&e.ID, &e.ProjectID, &hint, &propsJSON, &note, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		e.HintSrcLang = model.LocaleID(hint)
		e.Note = note
		e.CreatedAt = createdAt
		e.UpdatedAt = updatedAt
		if propsJSON != "" && propsJSON != "{}" {
			_ = json.Unmarshal([]byte(propsJSON), &e.Properties)
		}
		e.Variants = make(map[model.LocaleID][]model.Run)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, nil
	}

	byID := make(map[string]int, len(entries))
	for i, e := range entries {
		byID[e.ID] = i
	}

	// Variants.
	varRows, err := tm.db.QueryContext(ctx,
		`SELECT entry_id, locale, coded FROM tm_variants
		 WHERE workspace_id = $1 AND entry_id IN (`+inClause+`) ORDER BY entry_id, locale`,
		args...)
	if err == nil {
		for varRows.Next() {
			var eid, loc, coded string
			if err := varRows.Scan(&eid, &loc, &coded); err != nil {
				continue
			}
			var runs []model.Run
			if err := json.Unmarshal([]byte(coded), &runs); err == nil {
				if idx, ok := byID[eid]; ok {
					entries[idx].Variants[model.LocaleID(loc)] = runs
				}
			}
		}
		varRows.Close()
	}

	// Entities joined with values.
	entRows, err := tm.db.QueryContext(ctx, `
		SELECT e.entry_id, e.placeholder_id, e.entity_type, e.concept_id,
			v.locale, v.text_value, v.start_pos, v.end_pos
		FROM tm_entry_entities e
		LEFT JOIN tm_entry_entity_values v
			ON v.workspace_id = e.workspace_id AND v.entry_id = e.entry_id
			AND v.placeholder_id = e.placeholder_id
		WHERE e.workspace_id = $1 AND e.entry_id IN (`+inClause+`)
		ORDER BY e.entry_id, e.placeholder_id, v.locale
	`, args...)
	if err == nil {
		type entKey struct {
			entryIdx int
			pid      string
		}
		entIdx := make(map[entKey]int)
		for entRows.Next() {
			var eid, pid, etype, conceptID string
			var loc, textVal sql.NullString
			var startPos, endPos sql.NullInt64
			if err := entRows.Scan(&eid, &pid, &etype, &conceptID, &loc, &textVal, &startPos, &endPos); err != nil {
				continue
			}
			idx, ok := byID[eid]
			if !ok {
				continue
			}
			key := entKey{idx, pid}
			emIdx, exists := entIdx[key]
			if !exists {
				entries[idx].Entities = append(entries[idx].Entities, fw.EntityMapping{
					PlaceholderID: pid,
					Type:          model.EntityType(etype),
					ConceptID:     conceptID,
					Values:        make(map[model.LocaleID]fw.EntityValue),
				})
				emIdx = len(entries[idx].Entities) - 1
				entIdx[key] = emIdx
			}
			if loc.Valid && loc.String != "" {
				entries[idx].Entities[emIdx].Values[model.LocaleID(loc.String)] = fw.EntityValue{
					Text:  textVal.String,
					Start: int(startPos.Int64),
					End:   int(endPos.Int64),
				}
			}
		}
		entRows.Close()
	}

	// Origins.
	originRows, err := tm.db.QueryContext(ctx, `
		SELECT entry_id, source, key, reference, added_at, added_by, session_id
		FROM tm_entry_origins WHERE workspace_id = $1 AND entry_id IN (`+inClause+`)
		ORDER BY entry_id, ordinal
	`, args...)
	if err == nil {
		for originRows.Next() {
			var eid string
			var o fw.Origin
			if err := originRows.Scan(&eid, &o.Source, &o.Key, &o.Reference, &o.AddedAt, &o.AddedBy, &o.SessionID); err != nil {
				continue
			}
			if idx, ok := byID[eid]; ok {
				entries[idx].Origins = append(entries[idx].Origins, o)
			}
		}
		originRows.Close()
	}

	return entries, nil
}

func scanStringColumn(rows *sql.Rows) ([]string, error) {
	var out []string
	for rows.Next() {
		var s string
		if err := rows.Scan(&s); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// --- search ---

// SearchEntries performs a ranked full-text search across variant text.
func (tm *PostgresTM) SearchEntries(ctx context.Context, params fw.SearchParams) ([]fw.TMEntry, int, error) {
	params.Stream = ""
	params.StreamChain = nil
	params.Filter = fw.SearchFilter{}
	return tm.searchInternal(ctx, params)
}

// SearchEntriesFiltered applies additional facet filters.
func (tm *PostgresTM) SearchEntriesFiltered(ctx context.Context, params fw.SearchParams) ([]fw.TMEntry, int, error) {
	params.Stream = ""
	params.StreamChain = nil
	return tm.searchInternal(ctx, params)
}

// SearchEntriesForStream performs a search with stream inheritance.
func (tm *PostgresTM) SearchEntriesForStream(ctx context.Context, params fw.SearchParams) ([]fw.TMEntry, int, error) {
	return tm.searchInternal(ctx, params)
}

func (tm *PostgresTM) searchInternal(ctx context.Context, params fw.SearchParams) ([]fw.TMEntry, int, error) {
	query := params.Query
	anyLocale := params.AnyLocale
	requireLocale := params.RequireLocale
	stream := params.Stream
	streamChain := params.StreamChain
	filter := params.Filter
	offset := params.Offset
	limit := params.Limit
	args := []any{tm.workspaceID}
	argN := 2
	clauses := []string{"e.workspace_id = $1"}

	if query != "" {
		// Variant text search via tsvector.
		sub := fmt.Sprintf(`e.id IN (
			SELECT entry_id FROM tm_variants
			WHERE workspace_id = $1 AND search_tsv @@ plainto_tsquery('simple', $%d)`, argN)
		args = append(args, query)
		argN++
		if anyLocale != "" {
			sub += fmt.Sprintf(" AND locale = $%d", argN)
			args = append(args, anyLocale)
			argN++
		}
		sub += ")"
		clauses = append(clauses, sub)
	} else if anyLocale != "" {
		clauses = append(clauses, fmt.Sprintf(
			"e.id IN (SELECT entry_id FROM tm_variants WHERE workspace_id = $1 AND locale = $%d)", argN))
		args = append(args, anyLocale)
		argN++
	}

	if requireLocale != "" {
		clauses = append(clauses, fmt.Sprintf(
			"e.id IN (SELECT entry_id FROM tm_variants WHERE workspace_id = $1 AND locale = $%d)", argN))
		args = append(args, requireLocale)
		argN++
	}

	// Stream inheritance.
	var streamCase string
	var streamCaseArgs []any
	if stream != "" || len(streamChain) > 0 {
		streams := append([]string{stream}, streamChain...)
		placeholders := make([]string, len(streams))
		for i, s := range streams {
			placeholders[i] = fmt.Sprintf("$%d", argN)
			args = append(args, s)
			argN++
		}
		clauses = append(clauses, "e.stream IN ("+strings.Join(placeholders, ",")+")")

		var b strings.Builder
		b.WriteString("CASE e.stream")
		for i, s := range streams {
			fmt.Fprintf(&b, " WHEN $%d THEN %d", argN, i)
			streamCaseArgs = append(streamCaseArgs, s)
			argN++
		}
		fmt.Fprintf(&b, " ELSE %d END", len(streams))
		streamCase = b.String()
	}

	// Filters.
	filterClause, filterArgs, nextArgN := pgFilterWhere(filter, argN)
	if filterClause != "" {
		clauses = append(clauses, strings.TrimPrefix(filterClause, " AND "))
		args = append(args, filterArgs...)
		argN = nextArgN
	}
	_ = argN

	where := strings.Join(clauses, " AND ")

	// Count.
	countArgs := append([]any{}, args...)
	var total int
	if err := tm.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tm_entries e WHERE "+where,
		countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count search entries: %w", err)
	}
	if total == 0 {
		return nil, 0, nil
	}

	orderBy := "e.updated_at DESC"
	if streamCase != "" {
		orderBy = streamCase + ", " + orderBy
	}

	pageArgs := append([]any{}, args...)
	pageArgs = append(pageArgs, streamCaseArgs...)
	limitN := argN
	pageArgs = append(pageArgs, limit)
	offsetN := argN + 1
	pageArgs = append(pageArgs, offset)

	q := fmt.Sprintf("SELECT e.id FROM tm_entries e WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d",
		where, orderBy, limitN, offsetN)
	rows, err := tm.db.QueryContext(ctx, q, pageArgs...)
	if err != nil {
		return nil, total, fmt.Errorf("query search entries: %w", err)
	}
	defer rows.Close()
	ids, err := scanStringColumn(rows)
	if err != nil {
		return nil, total, err
	}
	entries, err := tm.loadEntriesByIDs(ctx, ids)
	if err != nil {
		return nil, total, err
	}
	return orderByIDs(entries, ids), total, nil
}

func orderByIDs(entries []fw.TMEntry, ids []string) []fw.TMEntry {
	if len(entries) == 0 {
		return entries
	}
	byID := make(map[string]int, len(entries))
	for i, e := range entries {
		byID[e.ID] = i
	}
	out := make([]fw.TMEntry, 0, len(ids))
	for _, id := range ids {
		if idx, ok := byID[id]; ok {
			out = append(out, entries[idx])
		}
	}
	return out
}

func pgFilterWhere(filter fw.SearchFilter, startN int) (string, []any, int) {
	var args []any
	var clauses []string
	argN := startN
	if filter.ProjectID != "" {
		clauses = append(clauses, fmt.Sprintf("e.project_id = $%d", argN))
		args = append(args, filter.ProjectID)
		argN++
	}
	if len(filter.SessionIDs) > 0 {
		placeholders := make([]string, len(filter.SessionIDs))
		for i, sid := range filter.SessionIDs {
			placeholders[i] = fmt.Sprintf("$%d", argN)
			args = append(args, sid)
			argN++
		}
		clauses = append(clauses,
			"e.id IN (SELECT entry_id FROM tm_entry_origins WHERE workspace_id = $1 AND session_id IN ("+strings.Join(placeholders, ",")+"))")
	}
	if len(filter.EntityTypes) > 0 {
		placeholders := make([]string, len(filter.EntityTypes))
		for i, et := range filter.EntityTypes {
			placeholders[i] = fmt.Sprintf("$%d", argN)
			args = append(args, et)
			argN++
		}
		clauses = append(clauses,
			"e.id IN (SELECT entry_id FROM tm_entry_entities WHERE workspace_id = $1 AND entity_type IN ("+strings.Join(placeholders, ",")+"))")
	}
	if len(filter.EntityValues) > 0 {
		pairs := make([]string, len(filter.EntityValues))
		for i, ev := range filter.EntityValues {
			pairs[i] = fmt.Sprintf("(v.text_value = $%d AND ee.entity_type = $%d)", argN, argN+1)
			args = append(args, ev.Value, ev.Type)
			argN += 2
		}
		clauses = append(clauses,
			"e.id IN (SELECT v.entry_id FROM tm_entry_entity_values v "+
				"INNER JOIN tm_entry_entities ee ON ee.workspace_id = v.workspace_id AND ee.entry_id = v.entry_id AND ee.placeholder_id = v.placeholder_id "+
				"WHERE v.workspace_id = $1 AND ("+strings.Join(pairs, " OR ")+"))")
	}
	if filter.HasCodes != nil {
		if *filter.HasCodes {
			clauses = append(clauses,
				"e.id IN (SELECT entry_id FROM tm_variants WHERE workspace_id = $1 AND (POSITION(E'\\ue001' IN coded) > 0 OR POSITION(E'\\ue002' IN coded) > 0 OR POSITION(E'\\ue003' IN coded) > 0))")
		} else {
			clauses = append(clauses,
				"e.id NOT IN (SELECT entry_id FROM tm_variants WHERE workspace_id = $1 AND (POSITION(E'\\ue001' IN coded) > 0 OR POSITION(E'\\ue002' IN coded) > 0 OR POSITION(E'\\ue003' IN coded) > 0))")
		}
	}
	if len(clauses) == 0 {
		return "", nil, argN
	}
	return " AND " + strings.Join(clauses, " AND "), args, argN
}

// --- facets ---

// FacetStats returns aggregated facet data across the workspace.
func (tm *PostgresTM) FacetStats(ctx context.Context) (fw.FacetData, error) {
	return tm.FacetStatsFiltered(ctx, fw.SearchParams{})
}

// FacetStatsFiltered returns facet counts scoped to matching entries.
func (tm *PostgresTM) FacetStatsFiltered(ctx context.Context, params fw.SearchParams) (fw.FacetData, error) {
	where, args := tm.buildFacetSubquery(params.Query, params.AnyLocale, params.RequireLocale, params.Filter)

	data := fw.FacetData{}

	localeQ := `SELECT v.locale, COUNT(DISTINCT v.entry_id)
		FROM tm_variants v
		INNER JOIN tm_entries e ON e.workspace_id = v.workspace_id AND e.id = v.entry_id
		WHERE ` + where + `
		GROUP BY v.locale ORDER BY COUNT(DISTINCT v.entry_id) DESC`
	if rows, err := tm.db.QueryContext(ctx, localeQ, args...); err == nil {
		for rows.Next() {
			var lf fw.LocaleFacet
			if err := rows.Scan(&lf.Locale, &lf.Count); err == nil {
				data.Locales = append(data.Locales, lf)
			}
		}
		rows.Close()
	} else {
		return data, fmt.Errorf("facet locales: %w", err)
	}

	projQ := `SELECT e.project_id, COUNT(*) FROM tm_entries e WHERE ` + where + ` GROUP BY e.project_id ORDER BY COUNT(*) DESC`
	if rows, err := tm.db.QueryContext(ctx, projQ, args...); err == nil {
		for rows.Next() {
			var pf fw.ProjectFacet
			if err := rows.Scan(&pf.ProjectID, &pf.Count); err == nil {
				data.Projects = append(data.Projects, pf)
			}
		}
		rows.Close()
	} else {
		return data, fmt.Errorf("facet projects: %w", err)
	}

	etQ := `SELECT ent.entity_type, COUNT(DISTINCT ent.entry_id)
		FROM tm_entry_entities ent
		INNER JOIN tm_entries e ON e.workspace_id = ent.workspace_id AND e.id = ent.entry_id
		WHERE ` + where + `
		GROUP BY ent.entity_type ORDER BY COUNT(DISTINCT ent.entry_id) DESC`
	if rows, err := tm.db.QueryContext(ctx, etQ, args...); err == nil {
		for rows.Next() {
			var ef fw.EntityTypeFacet
			if err := rows.Scan(&ef.Type, &ef.Count); err == nil {
				data.EntityTypes = append(data.EntityTypes, ef)
			}
		}
		rows.Close()
	} else {
		return data, fmt.Errorf("facet entity types: %w", err)
	}

	sessQ := `SELECT s.id, s.file_key, s.tool_name, s.imported_at, COUNT(DISTINCT o.entry_id)
		FROM tm_import_sessions s
		INNER JOIN tm_entry_origins o ON o.workspace_id = s.workspace_id AND o.session_id = s.id
		INNER JOIN tm_entries e ON e.workspace_id = o.workspace_id AND e.id = o.entry_id
		WHERE ` + where + `
		GROUP BY s.id, s.file_key, s.tool_name, s.imported_at
		ORDER BY COUNT(DISTINCT o.entry_id) DESC`
	if rows, err := tm.db.QueryContext(ctx, sessQ, args...); err == nil {
		for rows.Next() {
			var sf fw.ImportSessionFacet
			if err := rows.Scan(&sf.SessionID, &sf.FileKey, &sf.ToolName, &sf.ImportedAt, &sf.Count); err == nil {
				data.ImportSessions = append(data.ImportSessions, sf)
			}
		}
		rows.Close()
	} else {
		return data, fmt.Errorf("facet import sessions: %w", err)
	}

	codeQ := `SELECT
		COUNT(DISTINCT CASE WHEN EXISTS (
			SELECT 1 FROM tm_variants v
			WHERE v.workspace_id = e.workspace_id AND v.entry_id = e.id
			AND (POSITION(E'\ue001' IN v.coded) > 0 OR POSITION(E'\ue002' IN v.coded) > 0 OR POSITION(E'\ue003' IN v.coded) > 0)
		) THEN e.id END),
		COUNT(DISTINCT CASE WHEN NOT EXISTS (
			SELECT 1 FROM tm_variants v
			WHERE v.workspace_id = e.workspace_id AND v.entry_id = e.id
			AND (POSITION(E'\ue001' IN v.coded) > 0 OR POSITION(E'\ue002' IN v.coded) > 0 OR POSITION(E'\ue003' IN v.coded) > 0)
		) THEN e.id END)
		FROM tm_entries e WHERE ` + where
	if err := tm.db.QueryRowContext(ctx, codeQ, args...).Scan(&data.HasCodes, &data.NoCodes); err != nil {
		return data, fmt.Errorf("facet code counts: %w", err)
	}

	return data, nil
}

func (tm *PostgresTM) buildFacetSubquery(query, anyLocale, requireLocale string, filter fw.SearchFilter) (string, []any) {
	args := []any{tm.workspaceID}
	argN := 2
	clauses := []string{"e.workspace_id = $1"}

	if query != "" {
		sub := fmt.Sprintf(`e.id IN (
			SELECT entry_id FROM tm_variants
			WHERE workspace_id = $1 AND search_tsv @@ plainto_tsquery('simple', $%d)`, argN)
		args = append(args, query)
		argN++
		if anyLocale != "" {
			sub += fmt.Sprintf(" AND locale = $%d", argN)
			args = append(args, anyLocale)
			argN++
		}
		sub += ")"
		clauses = append(clauses, sub)
	} else if anyLocale != "" {
		clauses = append(clauses, fmt.Sprintf(
			"e.id IN (SELECT entry_id FROM tm_variants WHERE workspace_id = $1 AND locale = $%d)", argN))
		args = append(args, anyLocale)
		argN++
	}
	if requireLocale != "" {
		clauses = append(clauses, fmt.Sprintf(
			"e.id IN (SELECT entry_id FROM tm_variants WHERE workspace_id = $1 AND locale = $%d)", argN))
		args = append(args, requireLocale)
		argN++
	}
	if fc, fa, _ := pgFilterWhere(filter, argN); fc != "" {
		clauses = append(clauses, strings.TrimPrefix(fc, " AND "))
		args = append(args, fa...)
	}
	return strings.Join(clauses, " AND "), args
}

// LocaleStats returns per-locale entry counts across the workspace.
func (tm *PostgresTM) LocaleStats(ctx context.Context) ([]fw.LocaleFacet, error) {
	rows, err := tm.db.QueryContext(ctx, `
		SELECT locale, COUNT(DISTINCT entry_id) FROM tm_variants
		WHERE workspace_id = $1
		GROUP BY locale ORDER BY COUNT(DISTINCT entry_id) DESC
	`, tm.workspaceID)
	if err != nil {
		return nil, fmt.Errorf("locale stats: %w", err)
	}
	defer rows.Close()
	var out []fw.LocaleFacet
	for rows.Next() {
		var lf fw.LocaleFacet
		if err := rows.Scan(&lf.Locale, &lf.Count); err == nil {
			out = append(out, lf)
		}
	}
	return out, rows.Err()
}

// ActivityStats returns daily entry counts.
func (tm *PostgresTM) ActivityStats(ctx context.Context) ([]fw.ActivityStat, error) {
	rows, err := tm.db.QueryContext(ctx, `
		SELECT TO_CHAR(created_at, 'YYYY-MM-DD') AS day, COUNT(*)
		FROM tm_entries WHERE workspace_id = $1
		GROUP BY day ORDER BY day
	`, tm.workspaceID)
	if err != nil {
		return nil, fmt.Errorf("activity stats: %w", err)
	}
	defer rows.Close()
	var out []fw.ActivityStat
	for rows.Next() {
		var s fw.ActivityStat
		if err := rows.Scan(&s.Date, &s.Count); err == nil {
			out = append(out, s)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date < out[j].Date })
	return out, nil
}

// --- import sessions ---

// CreateImportSession inserts a new session row.
func (tm *PostgresTM) CreateImportSession(ctx context.Context, session fw.ImportSession) error {
	if session.ID == "" {
		return errors.New("import session ID is required")
	}
	if session.FileKey == "" {
		return errors.New("import session file_key is required")
	}
	if session.ImportedAt.IsZero() {
		session.ImportedAt = time.Now()
	}
	propsJSON := []byte("{}")
	if len(session.Properties) > 0 {
		if b, err := json.Marshal(session.Properties); err == nil {
			propsJSON = b
		}
	}
	_, err := tm.db.ExecContext(ctx, `INSERT INTO tm_import_sessions
		(workspace_id, id, file_key, file_hash, file_size_bytes, imported_at, imported_by,
		 tool_name, tool_version, seg_type, admin_lang, src_lang, data_type,
		 original_format, original_encoding, entry_count, properties)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17::jsonb)`,
		tm.workspaceID, session.ID, session.FileKey, session.FileHash, session.FileSizeBytes,
		session.ImportedAt, session.ImportedBy,
		session.ToolName, session.ToolVersion, session.SegType,
		session.AdminLang, session.SrcLang, session.DataType,
		session.OriginalFormat, session.OriginalEncoding, session.EntryCount,
		string(propsJSON))
	if err != nil {
		return fmt.Errorf("insert import session: %w", err)
	}
	return nil
}

// GetImportSession fetches a session by ID.
func (tm *PostgresTM) GetImportSession(ctx context.Context, id string) (fw.ImportSession, bool, error) {
	row := tm.db.QueryRowContext(ctx,
		"SELECT "+pgSessionColumns+" FROM tm_import_sessions WHERE workspace_id = $1 AND id = $2",
		tm.workspaceID, id)
	s, ok := scanPgSession(row)
	return s, ok, nil
}

// FindImportSessionByHash returns the most recent session matching the hash.
func (tm *PostgresTM) FindImportSessionByHash(ctx context.Context, hash string) (fw.ImportSession, bool, error) {
	if hash == "" {
		return fw.ImportSession{}, false, nil
	}
	row := tm.db.QueryRowContext(ctx,
		"SELECT "+pgSessionColumns+" FROM tm_import_sessions WHERE workspace_id = $1 AND file_hash = $2 ORDER BY imported_at DESC LIMIT 1",
		tm.workspaceID, hash)
	s, ok := scanPgSession(row)
	return s, ok, nil
}

// ListImportSessions returns all sessions ordered by imported_at DESC.
func (tm *PostgresTM) ListImportSessions(ctx context.Context) ([]fw.ImportSession, error) {
	rows, err := tm.db.QueryContext(ctx,
		"SELECT "+pgSessionColumns+" FROM tm_import_sessions WHERE workspace_id = $1 ORDER BY imported_at DESC",
		tm.workspaceID)
	if err != nil {
		return nil, fmt.Errorf("list import sessions: %w", err)
	}
	defer rows.Close()
	var out []fw.ImportSession
	for rows.Next() {
		if s, ok := scanPgSession(rows); ok {
			out = append(out, s)
		}
	}
	return out, rows.Err()
}

// UpdateImportSessionCount sets the entry_count on a session.
func (tm *PostgresTM) UpdateImportSessionCount(ctx context.Context, id string, count int) error {
	res, err := tm.db.ExecContext(ctx,
		"UPDATE tm_import_sessions SET entry_count = $1 WHERE workspace_id = $2 AND id = $3",
		count, tm.workspaceID, id)
	if err != nil {
		return fmt.Errorf("update session count: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("import session not found")
	}
	return nil
}

// DeleteImportSession removes a session row and clears origin.session_id.
func (tm *PostgresTM) DeleteImportSession(ctx context.Context, id string) error {
	if _, err := tm.db.ExecContext(ctx,
		"UPDATE tm_entry_origins SET session_id = '' WHERE workspace_id = $1 AND session_id = $2",
		tm.workspaceID, id); err != nil {
		return fmt.Errorf("clear origin session_id: %w", err)
	}
	res, err := tm.db.ExecContext(ctx,
		"DELETE FROM tm_import_sessions WHERE workspace_id = $1 AND id = $2",
		tm.workspaceID, id)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return errors.New("import session not found")
	}
	return nil
}

const pgSessionColumns = `id, file_key, file_hash, file_size_bytes, imported_at,
	imported_by, tool_name, tool_version, seg_type, admin_lang, src_lang,
	data_type, original_format, original_encoding, entry_count, properties::text`

// pgScanner is an alias for storage.Scanner, satisfied by *sql.Row and *sql.Rows.
type pgScanner = storage.Scanner

func scanPgSession(sc pgScanner) (fw.ImportSession, bool) {
	var s fw.ImportSession
	var propsJSON string
	if err := sc.Scan(&s.ID, &s.FileKey, &s.FileHash, &s.FileSizeBytes,
		&s.ImportedAt, &s.ImportedBy, &s.ToolName, &s.ToolVersion,
		&s.SegType, &s.AdminLang, &s.SrcLang, &s.DataType,
		&s.OriginalFormat, &s.OriginalEncoding, &s.EntryCount, &propsJSON); err != nil {
		return fw.ImportSession{}, false
	}
	if propsJSON != "" && propsJSON != "{}" {
		_ = json.Unmarshal([]byte(propsJSON), &s.Properties)
	}
	return s, true
}
