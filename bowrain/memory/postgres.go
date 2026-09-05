// Package memory provides a PostgreSQL-backed implementation of
// neokapi's multilingual content memory. It mirrors the SQLite
// implementation in the framework module, with workspace_id as a
// composite PK component on every table for multi-tenant isolation.
package memory

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
	"github.com/neokapi/neokapi/core/model"
	fw "github.com/neokapi/neokapi/memory"
	"github.com/neokapi/neokapi/memory/schema"
)

// PostgresStore is a persistent, multilingual content memory backed by
// PostgreSQL. All workspace Memories share the same database, isolated by
// workspace_id.
type PostgresStore struct {
	db          *storage.PgDB
	workspaceID string
}

// PostgresStore answers a block's version chain, so the platform translate
// assembly and the review context both reach the prior version they bind for.
var _ fw.VersionReader = (*PostgresStore)(nil)

// NewPostgresStoreFromDB creates a PostgresStore using an existing shared PgDB
// connection. workspaceID scopes all entries to a specific workspace.
func NewPostgresStoreFromDB(db *storage.PgDB, workspaceID string) (*PostgresStore, error) {
	if err := storage.MigratePostgresNS(db, "tm_schema_migrations", Migrations); err != nil {
		return nil, fmt.Errorf("migrate content memory schema: %w", err)
	}
	return &PostgresStore{db: db, workspaceID: workspaceID}, nil
}

// Migrations is the content memory Postgres schema as a single consolidated
// baseline, rendered from the shared schema descriptors (memory/schema) — the
// same source the framework SQLite backend renders from, so the two cannot
// drift (statement-set tested in memory/schema).
//
// LEDGER — every version this subsystem has ever issued, now folded in:
//
//	1  bilingual content memory schema      (retired before consolidation)
//	2  bilingual schema, incremental        (retired before consolidation)
//	3  bilingual schema, incremental        (retired before consolidation)
//	4  multilingual content memory schema — variants, entities per locale, import sessions
//	5  add concept_id to entity mappings for terms cross-reference
//
// Versions 1-3 were already gone before this change: v4 dropped their tables
// outright, so a fresh database never applied them. Their numbers are recorded
// here because a retired number stays spent — reusing 1 would make a database
// that still remembers it skip the new work entirely.
//
// The DROP TABLE statements that opened v4 are NOT carried into the baseline.
// They cleared the legacy bilingual tables during that single upgrade; a
// baseline is designed to be re-applied, so keeping them would mean every
// migrate pass quietly destroyed the content memory it exists to preserve.
//
// Baseline is version 6, above every number issued. Retired numbers are never
// reused; the next migration is version 8.
//
//	7  the version chain: unit, point, and the governing context per origin
//
// Version 7 is an ALTER rather than an edit to the baseline. A live database
// has already recorded 6, so the runner skips it forever; folding a column into
// the baseline would reach a fresh database and no other, and the server would
// keep answering an empty chain wherever it already runs.
var Migrations = []storage.Migration{
	{
		Version:     6,
		Description: "content memory baseline (folds 4-5; 1-3 retired earlier)",
		SQL:         schema.RenderMemoryPostgresBaseline("workspace_id"),
	},
	{
		Version:     7,
		Description: "version chain: unit, point, and the origin's governing context",
		SQL:         schema.RenderMemoryPostgresVersionChain("workspace_id"),
	},
}

// --- basic ---

// Close is a no-op for PostgresStore; the connection is shared.
func (tm *PostgresStore) Close() error { return nil }

// Count returns the total number of entries for this workspace.
func (tm *PostgresStore) Count(ctx context.Context) (int, error) {
	var count int
	if err := tm.db.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM tm_entries WHERE workspace_id = $1",
		tm.workspaceID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count entries: %w", err)
	}
	return count, nil
}

// --- writes ---

// Add inserts or updates a multilingual content-memory entry.
func (tm *PostgresStore) Add(ctx context.Context, entry fw.Entry) error {
	return tm.AddWithStream(ctx, entry, "")
}

// AddWithStream inserts or updates a multilingual content-memory entry on a given stream.
func (tm *PostgresStore) AddWithStream(ctx context.Context, entry fw.Entry, stream string) error {
	if entry.ID == "" {
		return errors.New("entry ID is required")
	}
	if len(entry.Variants) == 0 {
		return errors.New("entry must have at least one variant")
	}
	fw.NormalizeEntryLocales(&entry)

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
			(workspace_id, id, project_id, stream, hint_src_lang, properties, note, point, unit, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6::jsonb, $7, $8, $9, $10, $11)
		ON CONFLICT (workspace_id, id) DO UPDATE SET
			project_id    = EXCLUDED.project_id,
			stream        = EXCLUDED.stream,
			hint_src_lang = EXCLUDED.hint_src_lang,
			properties    = EXCLUDED.properties,
			note          = EXCLUDED.note,
			point         = EXCLUDED.point,
			unit          = EXCLUDED.unit,
			updated_at    = EXCLUDED.updated_at
	`, tm.workspaceID, entry.ID, entry.ProjectID, stream,
		string(entry.HintSrcLang), string(propsJSON), entry.Note,
		entry.Point, entry.Unit,
		entry.CreatedAt, entry.UpdatedAt); err != nil {
		return fmt.Errorf("upsert entry: %w", err)
	}

	// Replace the variants this write carries, locale by locale: one source text
	// is taught one target locale at a time, and every locale keys the same entry
	// off the source content hash, so clearing the whole set would leave the entry
	// holding only the locale written last. Same rule as the SQLite store.
	for loc := range entry.Variants {
		if _, err := tm.db.ExecContext(ctx,
			"DELETE FROM tm_variants WHERE workspace_id = $1 AND entry_id = $2 AND locale = $3",
			tm.workspaceID, entry.ID, string(loc)); err != nil {
			return fmt.Errorf("delete variants %s: %w", loc, err)
		}
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
			(workspace_id, entry_id, ordinal, source, key, reference, added_at, added_by, session_id, context_fp)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
			tm.workspaceID, entry.ID, i, o.Source, o.Key, o.Reference,
			addedAt, o.AddedBy, o.SessionID, o.ContextFingerprint); err != nil {
			return fmt.Errorf("insert origin: %w", err)
		}
	}

	return nil
}

// Delete removes an entry by ID.
func (tm *PostgresStore) Delete(ctx context.Context, id string) error {
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

// --- versions ---

// Versions returns a block's prior answers from the server corpus.
//
// The var _ fw.VersionReader assertion above is what binds this to the
// capability: the platform translate assembly and the review context both
// type-assert for VersionReader and skip the prior-version section when the
// store lacks it, so a dropped method shows up as an empty panel rather than as
// a build failure.
//
// Entries written before the version chain existed carry no unit and answer no
// chain. Nothing backfills them: a unit is resolved by reconciliation over a
// project's own content, and inventing one from stored text would link answers
// that were never the same block. The standing decision is to reset the data
// rather than migrate it while the dogfood setup is the only tenant
// (bowrain-infra/docs/runbooks/data-reset.md).
func (tm *PostgresStore) Versions(ctx context.Context, q fw.VersionQuery, excludeID string) ([]fw.Version, error) {
	if q.Unit == "" {
		return nil, fw.ErrVersionQueryNeedsUnit
	}

	where := "workspace_id = $1 AND unit = $2"
	args := []any{tm.workspaceID, q.Unit}
	if q.Point != "" {
		where += " AND point = $3"
		args = append(args, q.Point)
	}

	rows, err := tm.db.QueryContext(ctx,
		`SELECT id FROM tm_entries WHERE `+where+` ORDER BY updated_at DESC, id DESC`, args...)
	if err != nil {
		return nil, fmt.Errorf("select versions: %w", err)
	}
	defer rows.Close()
	ids, err := scanStringColumn(rows)
	if err != nil {
		return nil, fmt.Errorf("scan version ids: %w", err)
	}
	if len(ids) == 0 {
		return nil, nil
	}

	// Hydrated through the same path every other read uses, so a version carries
	// the variants, entities and origins a caller needs to judge it.
	entries, err := tm.loadEntriesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	return fw.VersionsFrom(entries, q, excludeID), nil
}

// --- lookup ---

// Lookup searches for matches using tiered matching.
func (tm *PostgresStore) Lookup(ctx context.Context, source *model.Block, sourceLocale, targetLocale model.LocaleID, opts fw.LookupOptions) ([]fw.Match, error) {
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
// source block. See ContentMemory.LookupSegment for the contract.
func (tm *PostgresStore) LookupSegment(ctx context.Context, source *model.Block, segmentIdx int, sourceLocale, targetLocale model.LocaleID, opts fw.LookupOptions) ([]fw.Match, error) {
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
func (tm *PostgresStore) LookupText(ctx context.Context, source string, sourceLocale, targetLocale model.LocaleID, opts fw.LookupOptions) ([]fw.Match, error) {
	opts = fw.ApplyDefaults(opts)
	opts.MatchModes = []fw.MatchMode{fw.MatchModePlain}
	normalized := fw.NormalizeText(source)
	return tm.tieredLookup(ctx, normalized, normalized, normalized, nil, sourceLocale, targetLocale, opts)
}

func (tm *PostgresStore) tieredLookup(ctx context.Context, plainKey, structKey, generalKey string, entityAnnotations []*model.EntityAnnotation, sourceLocale, targetLocale model.LocaleID, opts fw.LookupOptions) ([]fw.Match, error) {
	return fw.TieredLookup(ctx, plainKey, structKey, generalKey, entityAnnotations, sourceLocale, targetLocale, opts, fw.CandidateSource{
		Exact:           tm.queryExactVariant,
		FuzzyCandidates: tm.queryFuzzyCandidates,
	})
}

func (tm *PostgresStore) queryExactVariant(ctx context.Context, column, key string, sourceLocale model.LocaleID, opts fw.LookupOptions) ([]fw.Entry, error) {
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

func (tm *PostgresStore) queryFuzzyCandidates(ctx context.Context, plainKey, structKey, generalKey string, sourceLocale model.LocaleID, _ fw.LookupOptions) ([]fw.Entry, error) {
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

func (tm *PostgresStore) queryLengthFiltered(ctx context.Context, plainKey string, sourceLocale model.LocaleID) ([]fw.Entry, error) {
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
func (tm *PostgresStore) GetEntry(ctx context.Context, id string) (fw.Entry, bool, error) {
	entries, err := tm.loadEntriesByIDs(ctx, []string{id})
	if err != nil {
		return fw.Entry{}, false, err
	}
	if len(entries) == 0 {
		return fw.Entry{}, false, nil
	}
	return entries[0], true, nil
}

// Entries returns all entries for this workspace.
func (tm *PostgresStore) Entries(ctx context.Context) ([]fw.Entry, error) {
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

func (tm *PostgresStore) loadEntriesByIDs(ctx context.Context, ids []string) ([]fw.Entry, error) {
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

	entryQ := `SELECT id, project_id, hint_src_lang, properties::text, note, point, unit, created_at, updated_at
		FROM tm_entries WHERE workspace_id = $1 AND id IN (` + inClause + `)`
	rows, err := tm.db.QueryContext(ctx, entryQ, args...)
	if err != nil {
		return nil, fmt.Errorf("load entries: %w", err)
	}
	defer rows.Close()

	var entries []fw.Entry
	for rows.Next() {
		var e fw.Entry
		var hint, propsJSON, note string
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&e.ID, &e.ProjectID, &hint, &propsJSON, &note, &e.Point, &e.Unit, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		e.HintSrcLang = model.LocaleID(hint)
		e.Note = note
		e.CreatedAt = createdAt
		e.UpdatedAt = updatedAt
		if propsJSON != "" && propsJSON != "{}" {
			if err := json.Unmarshal([]byte(propsJSON), &e.Properties); err != nil {
				return nil, fmt.Errorf("entry %s: unmarshal properties: %w", e.ID, err)
			}
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
	// A transient failure reading variants must surface as an error, not as an
	// entry with no translation — the caller reads an empty variant map as "not
	// in memory" and re-translates, discarding an approved target.
	if err != nil {
		return nil, fmt.Errorf("load variants: %w", err)
	}
	for varRows.Next() {
		var eid, loc, coded string
		if err := varRows.Scan(&eid, &loc, &coded); err != nil {
			varRows.Close()
			return nil, fmt.Errorf("scan variant: %w", err)
		}
		var runs []model.Run
		if err := json.Unmarshal([]byte(coded), &runs); err != nil {
			varRows.Close()
			return nil, fmt.Errorf("unmarshal variant runs for entry %s: %w", eid, err)
		}
		if idx, ok := byID[eid]; ok {
			entries[idx].Variants[model.LocaleID(loc)] = runs
		}
	}
	if err := varRows.Err(); err != nil {
		varRows.Close()
		return nil, fmt.Errorf("variant rows: %w", err)
	}
	varRows.Close()

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
	if err != nil {
		return nil, fmt.Errorf("load entities: %w", err)
	}
	{
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
				entRows.Close()
				return nil, fmt.Errorf("scan entity: %w", err)
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
		if err := entRows.Err(); err != nil {
			entRows.Close()
			return nil, fmt.Errorf("entity rows: %w", err)
		}
		entRows.Close()
	}

	// Origins.
	originRows, err := tm.db.QueryContext(ctx, `
		SELECT entry_id, source, key, reference, added_at, added_by, session_id, context_fp
		FROM tm_entry_origins WHERE workspace_id = $1 AND entry_id IN (`+inClause+`)
		ORDER BY entry_id, ordinal
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("load origins: %w", err)
	}
	for originRows.Next() {
		var eid string
		var o fw.Origin
		if err := originRows.Scan(&eid, &o.Source, &o.Key, &o.Reference, &o.AddedAt, &o.AddedBy, &o.SessionID, &o.ContextFingerprint); err != nil {
			originRows.Close()
			return nil, fmt.Errorf("scan origin: %w", err)
		}
		if idx, ok := byID[eid]; ok {
			entries[idx].Origins = append(entries[idx].Origins, o)
		}
	}
	if err := originRows.Err(); err != nil {
		originRows.Close()
		return nil, fmt.Errorf("origin rows: %w", err)
	}
	originRows.Close()

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
func (tm *PostgresStore) SearchEntries(ctx context.Context, params fw.SearchParams) ([]fw.Entry, int, error) {
	params.Stream = ""
	params.StreamChain = nil
	params.Filter = fw.SearchFilter{}
	return tm.searchInternal(ctx, params)
}

// SearchEntriesFiltered applies additional facet filters.
func (tm *PostgresStore) SearchEntriesFiltered(ctx context.Context, params fw.SearchParams) ([]fw.Entry, int, error) {
	params.Stream = ""
	params.StreamChain = nil
	return tm.searchInternal(ctx, params)
}

// SearchEntriesForStream performs a search with stream inheritance.
func (tm *PostgresStore) SearchEntriesForStream(ctx context.Context, params fw.SearchParams) ([]fw.Entry, int, error) {
	return tm.searchInternal(ctx, params)
}

func (tm *PostgresStore) searchInternal(ctx context.Context, params fw.SearchParams) ([]fw.Entry, int, error) {
	params = fw.NormalizeSearchLocales(params)
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

func orderByIDs(entries []fw.Entry, ids []string) []fw.Entry {
	if len(entries) == 0 {
		return entries
	}
	byID := make(map[string]int, len(entries))
	for i, e := range entries {
		byID[e.ID] = i
	}
	out := make([]fw.Entry, 0, len(ids))
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
func (tm *PostgresStore) FacetStats(ctx context.Context) (fw.FacetData, error) {
	return tm.FacetStatsFiltered(ctx, fw.SearchParams{})
}

// FacetStatsFiltered returns facet counts scoped to matching entries.
func (tm *PostgresStore) FacetStatsFiltered(ctx context.Context, params fw.SearchParams) (fw.FacetData, error) {
	params = fw.NormalizeSearchLocales(params)
	where, args := tm.buildFacetSubquery(params.Query, params.AnyLocale, params.RequireLocale, params.Filter)

	data := fw.FacetData{}

	localeQ := `SELECT v.locale, COUNT(DISTINCT v.entry_id)
		FROM tm_variants v
		INNER JOIN tm_entries e ON e.workspace_id = v.workspace_id AND e.id = v.entry_id
		WHERE ` + where + `
		GROUP BY v.locale ORDER BY COUNT(DISTINCT v.entry_id) DESC`
	// Each facet loop propagates its scan and iteration errors: a truncated read
	// otherwise renders as an undercount that reads as real data.
	localeRows, err := tm.db.QueryContext(ctx, localeQ, args...)
	if err != nil {
		return data, fmt.Errorf("facet locales: %w", err)
	}
	for localeRows.Next() {
		var lf fw.LocaleFacet
		if err := localeRows.Scan(&lf.Locale, &lf.Count); err != nil {
			localeRows.Close()
			return data, fmt.Errorf("scan facet locale: %w", err)
		}
		data.Locales = append(data.Locales, lf)
	}
	if err := localeRows.Err(); err != nil {
		localeRows.Close()
		return data, fmt.Errorf("facet locales: %w", err)
	}
	localeRows.Close()

	projQ := `SELECT e.project_id, COUNT(*) FROM tm_entries e WHERE ` + where + ` GROUP BY e.project_id ORDER BY COUNT(*) DESC`
	projRows, err := tm.db.QueryContext(ctx, projQ, args...)
	if err != nil {
		return data, fmt.Errorf("facet projects: %w", err)
	}
	for projRows.Next() {
		var pf fw.ProjectFacet
		if err := projRows.Scan(&pf.ProjectID, &pf.Count); err != nil {
			projRows.Close()
			return data, fmt.Errorf("scan facet project: %w", err)
		}
		data.Projects = append(data.Projects, pf)
	}
	if err := projRows.Err(); err != nil {
		projRows.Close()
		return data, fmt.Errorf("facet projects: %w", err)
	}
	projRows.Close()

	etQ := `SELECT ent.entity_type, COUNT(DISTINCT ent.entry_id)
		FROM tm_entry_entities ent
		INNER JOIN tm_entries e ON e.workspace_id = ent.workspace_id AND e.id = ent.entry_id
		WHERE ` + where + `
		GROUP BY ent.entity_type ORDER BY COUNT(DISTINCT ent.entry_id) DESC`
	etRows, err := tm.db.QueryContext(ctx, etQ, args...)
	if err != nil {
		return data, fmt.Errorf("facet entity types: %w", err)
	}
	for etRows.Next() {
		var ef fw.EntityTypeFacet
		if err := etRows.Scan(&ef.Type, &ef.Count); err != nil {
			etRows.Close()
			return data, fmt.Errorf("scan facet entity type: %w", err)
		}
		data.EntityTypes = append(data.EntityTypes, ef)
	}
	if err := etRows.Err(); err != nil {
		etRows.Close()
		return data, fmt.Errorf("facet entity types: %w", err)
	}
	etRows.Close()

	sessQ := `SELECT s.id, s.file_key, s.tool_name, s.imported_at, COUNT(DISTINCT o.entry_id)
		FROM tm_import_sessions s
		INNER JOIN tm_entry_origins o ON o.workspace_id = s.workspace_id AND o.session_id = s.id
		INNER JOIN tm_entries e ON e.workspace_id = o.workspace_id AND e.id = o.entry_id
		WHERE ` + where + `
		GROUP BY s.id, s.file_key, s.tool_name, s.imported_at
		ORDER BY COUNT(DISTINCT o.entry_id) DESC`
	sessRows, err := tm.db.QueryContext(ctx, sessQ, args...)
	if err != nil {
		return data, fmt.Errorf("facet import sessions: %w", err)
	}
	for sessRows.Next() {
		var sf fw.ImportSessionFacet
		if err := sessRows.Scan(&sf.SessionID, &sf.FileKey, &sf.ToolName, &sf.ImportedAt, &sf.Count); err != nil {
			sessRows.Close()
			return data, fmt.Errorf("scan facet import session: %w", err)
		}
		data.ImportSessions = append(data.ImportSessions, sf)
	}
	if err := sessRows.Err(); err != nil {
		sessRows.Close()
		return data, fmt.Errorf("facet import sessions: %w", err)
	}
	sessRows.Close()

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

func (tm *PostgresStore) buildFacetSubquery(query, anyLocale, requireLocale string, filter fw.SearchFilter) (string, []any) {
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
func (tm *PostgresStore) LocaleStats(ctx context.Context) ([]fw.LocaleFacet, error) {
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
func (tm *PostgresStore) ActivityStats(ctx context.Context) ([]fw.ActivityStat, error) {
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
func (tm *PostgresStore) CreateImportSession(ctx context.Context, session fw.ImportSession) error {
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
func (tm *PostgresStore) GetImportSession(ctx context.Context, id string) (fw.ImportSession, bool, error) {
	row := tm.db.QueryRowContext(ctx,
		"SELECT "+pgSessionColumns+" FROM tm_import_sessions WHERE workspace_id = $1 AND id = $2",
		tm.workspaceID, id)
	s, ok := scanPgSession(row)
	return s, ok, nil
}

// FindImportSessionByHash returns the most recent session matching the hash.
func (tm *PostgresStore) FindImportSessionByHash(ctx context.Context, hash string) (fw.ImportSession, bool, error) {
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
func (tm *PostgresStore) ListImportSessions(ctx context.Context) ([]fw.ImportSession, error) {
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
func (tm *PostgresStore) UpdateImportSessionCount(ctx context.Context, id string, count int) error {
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
func (tm *PostgresStore) DeleteImportSession(ctx context.Context, id string) error {
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
		// Best-effort: this scanner reports presence, not failure — its only
		// error channel is the bool, and returning false for a session that
		// exists would hide the session rather than the bad column. Session
		// properties are import provenance, so a malformed blob degrades to
		// none; the mandatory columns above still fail the scan.
		_ = json.Unmarshal([]byte(propsJSON), &s.Properties)
	}
	return s, true
}
