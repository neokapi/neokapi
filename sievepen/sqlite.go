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
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/storage"
)

// Sentinel errors for TM entry validation.
var (
	ErrEntryIDRequired   = errors.New("entry ID is required")
	ErrEntryNoVariants   = errors.New("entry must have at least one variant")
	ErrSessionIDRequired = errors.New("import session ID is required")
	ErrSessionFileKey    = errors.New("import session file_key is required")
	ErrImportSessionMiss = errors.New("import session not found")
)

// SQLiteTM is a multilingual, persistent translation memory backed by SQLite.
// Each entry has a map of language variants (locale → Fragment) plus
// normalized match keys per variant for tiered lookup.
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

// tmMigrations defines the multilingual TM schema. Every entry is
// symmetric: there is no "source"/"target", only a set of language
// variants. Match keys (plain, structural, generalized) are computed
// per variant at write time and indexed for tiered lookup.
var tmMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "multilingual TM schema with per-variant match keys and import sessions",
		// tm_variant_search uses storage.FTSWordTokenizer, which resolves to the
		// ICU tokenizer under cgo builds and unicode61 under no-cgo builds (the
		// ICU tokenizer is a cgo-only extension). A .db whose FTS table was
		// created with one tokenizer cannot be FTS-word-queried by a binary built
		// with the other; the trigram table below stays portable.
		SQL: fmt.Sprintf(`
		CREATE TABLE IF NOT EXISTS tm_entries (
			id              TEXT PRIMARY KEY,
			project_id      TEXT NOT NULL DEFAULT '',
			stream          TEXT NOT NULL DEFAULT '',
			hint_src_lang   TEXT NOT NULL DEFAULT '',
			properties      TEXT NOT NULL DEFAULT '',
			note            TEXT NOT NULL DEFAULT '',
			created_at      TEXT NOT NULL,
			updated_at      TEXT NOT NULL
		);
		CREATE INDEX IF NOT EXISTS idx_tm_project ON tm_entries(project_id);
		CREATE INDEX IF NOT EXISTS idx_tm_updated ON tm_entries(updated_at DESC);
		CREATE INDEX IF NOT EXISTS idx_tm_stream  ON tm_entries(stream);

		CREATE TABLE IF NOT EXISTS tm_variants (
			entry_id    TEXT NOT NULL REFERENCES tm_entries(id) ON DELETE CASCADE,
			locale      TEXT NOT NULL,
			coded       TEXT NOT NULL,
			plain       TEXT NOT NULL,
			struct_key  TEXT NOT NULL,
			general_key TEXT NOT NULL,
			PRIMARY KEY (entry_id, locale)
		);
		CREATE INDEX IF NOT EXISTS idx_tm_var_locale      ON tm_variants(locale);
		CREATE INDEX IF NOT EXISTS idx_tm_var_plain_loc   ON tm_variants(plain, locale);
		CREATE INDEX IF NOT EXISTS idx_tm_var_struct_loc  ON tm_variants(struct_key, locale);
		CREATE INDEX IF NOT EXISTS idx_tm_var_general_loc ON tm_variants(general_key, locale);

		CREATE VIRTUAL TABLE IF NOT EXISTS tm_variant_search USING fts5(
			text,
			locale UNINDEXED,
			entry_id UNINDEXED,
			tokenize='%s'
		);

		CREATE VIRTUAL TABLE IF NOT EXISTS tm_variant_trigram USING fts5(
			plain, struct_key, general_key,
			locale UNINDEXED,
			entry_id UNINDEXED,
			tokenize='trigram'
		);

		CREATE TABLE IF NOT EXISTS tm_entry_entities (
			entry_id       TEXT NOT NULL REFERENCES tm_entries(id) ON DELETE CASCADE,
			placeholder_id TEXT NOT NULL,
			entity_type    TEXT NOT NULL,
			PRIMARY KEY (entry_id, placeholder_id)
		);
		CREATE INDEX IF NOT EXISTS idx_entities_type ON tm_entry_entities(entity_type);

		CREATE TABLE IF NOT EXISTS tm_entry_entity_values (
			entry_id       TEXT NOT NULL,
			placeholder_id TEXT NOT NULL,
			locale         TEXT NOT NULL,
			text_value     TEXT NOT NULL DEFAULT '',
			start_pos      INTEGER NOT NULL DEFAULT 0,
			end_pos        INTEGER NOT NULL DEFAULT 0,
			PRIMARY KEY (entry_id, placeholder_id, locale),
			FOREIGN KEY (entry_id, placeholder_id)
				REFERENCES tm_entry_entities(entry_id, placeholder_id) ON DELETE CASCADE
		);
		CREATE INDEX IF NOT EXISTS idx_entity_values_text ON tm_entry_entity_values(text_value, locale);

		CREATE TABLE IF NOT EXISTS tm_import_sessions (
			id                    TEXT PRIMARY KEY,
			file_key              TEXT NOT NULL,
			file_hash             TEXT NOT NULL DEFAULT '',
			file_size_bytes       INTEGER NOT NULL DEFAULT 0,
			imported_at           TEXT NOT NULL,
			imported_by           TEXT NOT NULL DEFAULT '',
			tool_name             TEXT NOT NULL DEFAULT '',
			tool_version          TEXT NOT NULL DEFAULT '',
			seg_type              TEXT NOT NULL DEFAULT '',
			admin_lang            TEXT NOT NULL DEFAULT '',
			src_lang              TEXT NOT NULL DEFAULT '',
			data_type             TEXT NOT NULL DEFAULT '',
			original_format       TEXT NOT NULL DEFAULT '',
			original_encoding     TEXT NOT NULL DEFAULT '',
			entry_count           INTEGER NOT NULL DEFAULT 0,
			properties_json       TEXT NOT NULL DEFAULT ''
		);
		CREATE INDEX IF NOT EXISTS idx_sessions_file_hash   ON tm_import_sessions(file_hash);
		CREATE INDEX IF NOT EXISTS idx_sessions_imported_at ON tm_import_sessions(imported_at DESC);

		CREATE TABLE IF NOT EXISTS tm_entry_origins (
			entry_id   TEXT NOT NULL REFERENCES tm_entries(id) ON DELETE CASCADE,
			ordinal    INTEGER NOT NULL,
			source     TEXT NOT NULL,
			key        TEXT NOT NULL DEFAULT '',
			reference  TEXT NOT NULL DEFAULT '',
			added_at   TEXT NOT NULL,
			added_by   TEXT NOT NULL DEFAULT '',
			session_id TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (entry_id, ordinal)
		);
		CREATE INDEX IF NOT EXISTS idx_origins_source  ON tm_entry_origins(source);
		CREATE INDEX IF NOT EXISTS idx_origins_key     ON tm_entry_origins(key);
		CREATE INDEX IF NOT EXISTS idx_origins_session ON tm_entry_origins(session_id);
		`, storage.FTSWordTokenizer),
	},
	{
		Version:     2,
		Description: "add concept_id to entity mappings for termbase cross-reference",
		SQL: `
		ALTER TABLE tm_entry_entities ADD COLUMN concept_id TEXT NOT NULL DEFAULT '';
		CREATE INDEX IF NOT EXISTS idx_entities_concept ON tm_entry_entities(concept_id);
		`,
	},
	{
		Version:     3,
		Description: "precomputed has_codes flag on tm_entries for fast facet queries",
		SQL: `
		ALTER TABLE tm_entries ADD COLUMN has_codes INTEGER NOT NULL DEFAULT 0;
		`,
	},
}

// DB returns the underlying database for direct access.
func (tm *SQLiteTM) DB() *storage.DB { return tm.db }

// Close closes the database connection.
func (tm *SQLiteTM) Close() error { return tm.db.Close() }

// Count returns the total number of entries.
func (tm *SQLiteTM) Count() int {
	var count int
	if err := tm.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM tm_entries").Scan(&count); err != nil {
		slog.Warn("TM count query failed", "error", err)
		return 0
	}
	return count
}

// Add inserts or updates a multilingual TM entry with an empty stream.
func (tm *SQLiteTM) Add(entry TMEntry) error {
	return tm.AddWithStream(entry, "")
}

// AddWithStream inserts or updates a multilingual TM entry associated with a
// stream (e.g., a git branch name). All variants, entities, entity values,
// and origins are replaced atomically inside a single transaction so that a
// partial failure can't leave orphan rows and bulk imports aren't gated by
// per-statement fsync latency.
func (tm *SQLiteTM) AddWithStream(entry TMEntry, stream string) error {
	tx, err := tm.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	if err := tm.addInTx(tx, entry, stream); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// BulkAddWithStream inserts or updates many TM entries inside a single
// transaction, using prepared statements that are reused across all rows.
// The FTS5 trigram (fuzzy-candidate) index is NOT populated in the bulk
// path — for large corpora its n-gram build cost dominates everything
// else. Call RebuildFuzzyIndex() at the end of the import to repopulate
// it in a single set-based SELECT INTO, which is orders of magnitude
// faster than row-by-row inserts.
func (tm *SQLiteTM) BulkAddWithStream(entries []TMEntry, stream string) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := tm.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	stmts, err := prepareBulkStmts(tx)
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	defer stmts.Close()

	for i := range entries {
		if err := stmts.addEntry(&entries[i], stream); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("bulk add entry %s: %w", entries[i].ID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// RebuildFuzzyIndex rebuilds the FTS5 trigram index (tm_variant_trigram)
// from the current contents of tm_variants. This is the recommended way
// to populate the fuzzy-candidate index after a bulk load — per-row
// inserts during the bulk pass are prohibitively slow because FTS5
// trigram tokenization allocates heavily on every insert.
//
// Until this is called, fuzzy lookups fall back to length-filtered
// scanning over tm_variants, which is functional but slower on huge TMs.
func (tm *SQLiteTM) RebuildFuzzyIndex() error {
	if _, err := tm.db.ExecContext(context.Background(), `DELETE FROM tm_variant_trigram`); err != nil {
		return fmt.Errorf("clear fuzzy index: %w", err)
	}
	if _, err := tm.db.ExecContext(context.Background(), `INSERT INTO tm_variant_trigram
		(plain, struct_key, general_key, locale, entry_id)
		SELECT plain, struct_key, general_key, locale, entry_id FROM tm_variants`); err != nil {
		return fmt.Errorf("rebuild fuzzy index: %w", err)
	}
	return nil
}

// RebuildSearchIndex rebuilds the FTS5 word-search index
// (tm_variant_search) in a single set-based INSERT … SELECT. Like
// RebuildFuzzyIndex this is a post-bulk-load step — the bulk path
// deliberately skips per-row FTS5 inserts because FTS5 ICU
// tokenization is expensive.
func (tm *SQLiteTM) RebuildSearchIndex() error {
	if _, err := tm.db.ExecContext(context.Background(), `DELETE FROM tm_variant_search`); err != nil {
		return fmt.Errorf("clear search index: %w", err)
	}
	if _, err := tm.db.ExecContext(context.Background(), `INSERT INTO tm_variant_search
		(text, locale, entry_id)
		SELECT plain, locale, entry_id FROM tm_variants`); err != nil {
		return fmt.Errorf("rebuild search index: %w", err)
	}
	return nil
}

// bulkStmts holds the set of prepared statements used by BulkAddWithStream.
// Each BulkAdd call prepares them once and reuses across all entries.
// Note: the FTS5 search tables (tm_variant_search, tm_variant_trigram)
// are deliberately NOT maintained here — see BulkAddWithStream doc
// comment for rationale. Call RebuildFuzzyIndex() and RebuildSearchIndex()
// after the bulk import to populate them in one set-based pass.
type bulkStmts struct {
	upsertEntry     *sql.Stmt
	delVariants     *sql.Stmt
	insVariant      *sql.Stmt
	delEntities     *sql.Stmt
	delEntityValues *sql.Stmt
	insEntity       *sql.Stmt
	insEntityValue  *sql.Stmt
	delOrigins      *sql.Stmt
	insOrigin       *sql.Stmt
}

func prepareBulkStmts(tx *sql.Tx) (*bulkStmts, error) {
	s := &bulkStmts{}

	var err error
	if s.upsertEntry, err = tx.PrepareContext(context.Background(), `INSERT INTO tm_entries
		(id, project_id, stream, hint_src_lang, properties, note, has_codes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_id    = excluded.project_id,
			stream        = excluded.stream,
			hint_src_lang = excluded.hint_src_lang,
			properties    = excluded.properties,
			note          = excluded.note,
			has_codes     = excluded.has_codes,
			updated_at    = excluded.updated_at`); err != nil {
		return nil, fmt.Errorf("prepare upsert: %w", err)
	}

	if s.delVariants, err = tx.PrepareContext(context.Background(), `DELETE FROM tm_variants WHERE entry_id = ?`); err != nil {
		return nil, err
	}
	if s.insVariant, err = tx.PrepareContext(context.Background(), `INSERT INTO tm_variants
		(entry_id, locale, coded, plain, struct_key, general_key) VALUES (?, ?, ?, ?, ?, ?)`); err != nil {
		return nil, err
	}
	if s.delEntities, err = tx.PrepareContext(context.Background(), `DELETE FROM tm_entry_entities WHERE entry_id = ?`); err != nil {
		return nil, err
	}
	if s.delEntityValues, err = tx.PrepareContext(context.Background(), `DELETE FROM tm_entry_entity_values WHERE entry_id = ?`); err != nil {
		return nil, err
	}
	if s.insEntity, err = tx.PrepareContext(context.Background(), `INSERT INTO tm_entry_entities
		(entry_id, placeholder_id, entity_type, concept_id) VALUES (?, ?, ?, ?)`); err != nil {
		return nil, err
	}
	if s.insEntityValue, err = tx.PrepareContext(context.Background(), `INSERT INTO tm_entry_entity_values
		(entry_id, placeholder_id, locale, text_value, start_pos, end_pos) VALUES (?, ?, ?, ?, ?, ?)`); err != nil {
		return nil, err
	}
	if s.delOrigins, err = tx.PrepareContext(context.Background(), `DELETE FROM tm_entry_origins WHERE entry_id = ?`); err != nil {
		return nil, err
	}
	if s.insOrigin, err = tx.PrepareContext(context.Background(), `INSERT INTO tm_entry_origins
		(entry_id, ordinal, source, key, reference, added_at, added_by, session_id)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *bulkStmts) Close() {
	for _, st := range []*sql.Stmt{
		s.upsertEntry,
		s.delVariants,
		s.insVariant,
		s.delEntities, s.delEntityValues,
		s.insEntity, s.insEntityValue,
		s.delOrigins, s.insOrigin,
	} {
		if st != nil {
			_ = st.Close()
		}
	}
}

// addEntry is the prepared-statement counterpart of addInTx used by the
// bulk-import hot path. It mirrors the same upsert-and-replace semantics.
func (s *bulkStmts) addEntry(entry *TMEntry, stream string) error {
	if entry.ID == "" {
		return ErrEntryIDRequired
	}
	if len(entry.Variants) == 0 {
		return ErrEntryNoVariants
	}

	now := time.Now()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = now
	}

	var propertiesJSON string
	if len(entry.Properties) > 0 {
		b, err := json.Marshal(entry.Properties)
		if err != nil {
			return fmt.Errorf("marshal properties: %w", err)
		}
		propertiesJSON = string(b)
	}

	hasCodes := 0
	for _, runs := range entry.Variants {
		for _, r := range runs {
			if r.Text == nil {
				hasCodes = 1
				break
			}
		}
		if hasCodes == 1 {
			break
		}
	}

	if _, err := s.upsertEntry.ExecContext(context.Background(),
		entry.ID, entry.ProjectID, stream, string(entry.HintSrcLang),
		propertiesJSON, entry.Note, hasCodes,
		entry.CreatedAt.Format(time.RFC3339), entry.UpdatedAt.Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("upsert entry: %w", err)
	}

	if _, err := s.delVariants.ExecContext(context.Background(), entry.ID); err != nil {
		return fmt.Errorf("delete variants: %w", err)
	}

	for locale, runs := range entry.Variants {
		if len(runs) == 0 {
			continue
		}
		// Fast path: runs that are a single TextRun are stored as raw
		// plain text — TMX imports are overwhelmingly plain text, and
		// skipping the JSON wrapper cuts both CPU time and row size
		// meaningfully. On read the plain-text storage form is detected
		// by the absence of a leading '[' bracket.
		var coded, plain, structKey, generalKey string
		if isPlainTextRuns(runs) {
			plain = NormalizeText(runs[0].Text.Text)
			coded = plain
			structKey = plain
			generalKey = plain
		} else {
			b, err := json.Marshal(runs)
			if err != nil {
				return fmt.Errorf("marshal variant %s: %w", locale, err)
			}
			coded = string(b)
			plain = NormalizeText(model.FlattenRuns(runs))
			structKey = NormalizeText(model.RunsStructuralText(runs))
			generalKey = NormalizeText(model.RunsGeneralizedText(runs))
		}

		if _, err := s.insVariant.ExecContext(context.Background(), entry.ID, string(locale), coded, plain, structKey, generalKey); err != nil {
			return fmt.Errorf("insert variant %s: %w", locale, err)
		}
	}

	if _, err := s.delEntities.ExecContext(context.Background(), entry.ID); err != nil {
		return fmt.Errorf("delete entities: %w", err)
	}
	if _, err := s.delEntityValues.ExecContext(context.Background(), entry.ID); err != nil {
		return fmt.Errorf("delete entity values: %w", err)
	}
	for _, em := range entry.Entities {
		if em.PlaceholderID == "" {
			continue
		}
		if _, err := s.insEntity.ExecContext(context.Background(), entry.ID, em.PlaceholderID, string(em.Type), em.ConceptID); err != nil {
			return fmt.Errorf("insert entity: %w", err)
		}
		for loc, val := range em.Values {
			if _, err := s.insEntityValue.ExecContext(context.Background(), entry.ID, em.PlaceholderID, string(loc), val.Text, val.Start, val.End); err != nil {
				return fmt.Errorf("insert entity value: %w", err)
			}
		}
	}

	if _, err := s.delOrigins.ExecContext(context.Background(), entry.ID); err != nil {
		return fmt.Errorf("delete origins: %w", err)
	}
	for i, o := range entry.Origins {
		addedAt := o.AddedAt
		if addedAt.IsZero() {
			addedAt = now
		}
		if _, err := s.insOrigin.ExecContext(context.Background(),
			entry.ID, i, o.Source, o.Key, o.Reference,
			addedAt.Format(time.RFC3339), o.AddedBy, o.SessionID,
		); err != nil {
			return fmt.Errorf("insert origin: %w", err)
		}
	}
	return nil
}

// addInTx performs the full upsert of an entry (header + variants +
// entities + origins) against the given transaction. It is the shared
// implementation used by AddWithStream and BulkAddWithStream.
func (tm *SQLiteTM) addInTx(tx *sql.Tx, entry TMEntry, stream string) error {
	if entry.ID == "" {
		return ErrEntryIDRequired
	}
	if len(entry.Variants) == 0 {
		return ErrEntryNoVariants
	}

	now := time.Now()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = now
	}

	var propertiesJSON string
	if len(entry.Properties) > 0 {
		b, err := json.Marshal(entry.Properties)
		if err != nil {
			return fmt.Errorf("marshal properties: %w", err)
		}
		propertiesJSON = string(b)
	}

	hasCodes := 0
	for _, runs := range entry.Variants {
		for _, r := range runs {
			if r.Text == nil {
				hasCodes = 1
				break
			}
		}
		if hasCodes == 1 {
			break
		}
	}

	if _, err := tx.ExecContext(context.Background(), `
		INSERT INTO tm_entries (id, project_id, stream, hint_src_lang, properties, note, has_codes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_id    = excluded.project_id,
			stream        = excluded.stream,
			hint_src_lang = excluded.hint_src_lang,
			properties    = excluded.properties,
			note          = excluded.note,
			has_codes     = excluded.has_codes,
			updated_at    = excluded.updated_at
	`, entry.ID, entry.ProjectID, stream, string(entry.HintSrcLang),
		propertiesJSON, entry.Note, hasCodes,
		entry.CreatedAt.Format(time.RFC3339), entry.UpdatedAt.Format(time.RFC3339)); err != nil {
		return fmt.Errorf("upsert entry: %w", err)
	}

	// Replace variants. We also maintain the two FTS5 side-tables manually
	// (they are not content= external FTS, so triggers aren't wired).
	if _, err := tx.ExecContext(context.Background(), "DELETE FROM tm_variants WHERE entry_id = ?", entry.ID); err != nil {
		return fmt.Errorf("delete variants: %w", err)
	}
	if _, err := tx.ExecContext(context.Background(), "DELETE FROM tm_variant_search WHERE entry_id = ?", entry.ID); err != nil {
		return fmt.Errorf("delete variant_search: %w", err)
	}
	if _, err := tx.ExecContext(context.Background(), "DELETE FROM tm_variant_trigram WHERE entry_id = ?", entry.ID); err != nil {
		return fmt.Errorf("delete variant_trigram: %w", err)
	}

	for locale, runs := range entry.Variants {
		if len(runs) == 0 {
			continue
		}
		coded, err := json.Marshal(runs)
		if err != nil {
			return fmt.Errorf("marshal variant %s: %w", locale, err)
		}
		plain := NormalizeText(model.FlattenRuns(runs))
		structKey := NormalizeText(model.RunsStructuralText(runs))
		generalKey := NormalizeText(model.RunsGeneralizedText(runs))

		if _, err := tx.ExecContext(context.Background(), `INSERT INTO tm_variants
			(entry_id, locale, coded, plain, struct_key, general_key)
			VALUES (?, ?, ?, ?, ?, ?)`,
			entry.ID, string(locale), string(coded), plain, structKey, generalKey); err != nil {
			return fmt.Errorf("insert variant %s: %w", locale, err)
		}
		if _, err := tx.ExecContext(context.Background(), `INSERT INTO tm_variant_search (text, locale, entry_id)
			VALUES (?, ?, ?)`, plain, string(locale), entry.ID); err != nil {
			return fmt.Errorf("insert variant_search: %w", err)
		}
		if _, err := tx.ExecContext(context.Background(), `INSERT INTO tm_variant_trigram (plain, struct_key, general_key, locale, entry_id)
			VALUES (?, ?, ?, ?, ?)`, plain, structKey, generalKey, string(locale), entry.ID); err != nil {
			return fmt.Errorf("insert variant_trigram: %w", err)
		}
	}

	// Replace entities + per-locale entity values.
	if _, err := tx.ExecContext(context.Background(), "DELETE FROM tm_entry_entities WHERE entry_id = ?", entry.ID); err != nil {
		return fmt.Errorf("delete entities: %w", err)
	}
	if _, err := tx.ExecContext(context.Background(), "DELETE FROM tm_entry_entity_values WHERE entry_id = ?", entry.ID); err != nil {
		return fmt.Errorf("delete entity values: %w", err)
	}
	for _, em := range entry.Entities {
		if em.PlaceholderID == "" {
			continue
		}
		if _, err := tx.ExecContext(context.Background(), `INSERT INTO tm_entry_entities
			(entry_id, placeholder_id, entity_type, concept_id) VALUES (?, ?, ?, ?)`,
			entry.ID, em.PlaceholderID, string(em.Type), em.ConceptID); err != nil {
			return fmt.Errorf("insert entity: %w", err)
		}
		for loc, val := range em.Values {
			if _, err := tx.ExecContext(context.Background(), `INSERT INTO tm_entry_entity_values
				(entry_id, placeholder_id, locale, text_value, start_pos, end_pos)
				VALUES (?, ?, ?, ?, ?, ?)`,
				entry.ID, em.PlaceholderID, string(loc),
				val.Text, val.Start, val.End); err != nil {
				return fmt.Errorf("insert entity value: %w", err)
			}
		}
	}

	// Replace origins.
	if _, err := tx.ExecContext(context.Background(), "DELETE FROM tm_entry_origins WHERE entry_id = ?", entry.ID); err != nil {
		return fmt.Errorf("delete origins: %w", err)
	}
	for i, o := range entry.Origins {
		addedAt := o.AddedAt
		if addedAt.IsZero() {
			addedAt = now
		}
		if _, err := tx.ExecContext(context.Background(), `INSERT INTO tm_entry_origins
			(entry_id, ordinal, source, key, reference, added_at, added_by, session_id)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			entry.ID, i, o.Source, o.Key, o.Reference,
			addedAt.Format(time.RFC3339), o.AddedBy, o.SessionID); err != nil {
			return fmt.Errorf("insert origin: %w", err)
		}
	}

	return nil
}

// Delete removes an entry by ID together with every child row it owns, in a
// single transaction. All deletes — the indexed child tables (tm_variants,
// tm_entry_entities, tm_entry_entity_values, tm_entry_origins), the two
// manually-maintained FTS5 side-tables (tm_variant_search, tm_variant_trigram),
// and the main tm_entries row — are issued explicitly so correctness does not
// depend on ON DELETE CASCADE (and therefore on the foreign_keys pragma state).
// On any error the whole transaction is rolled back, so a partial delete can
// never leave orphaned child rows.
func (tm *SQLiteTM) Delete(id string) error {
	tx, err := tm.db.BeginTx(context.Background(), nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	// Delete children first so the row is gone regardless of FK enforcement.
	// tm_entry_entity_values is removed before tm_entry_entities so the delete
	// is correct even when its composite-FK cascade is disabled.
	childTables := []struct{ name, sql string }{
		{"variant_search", "DELETE FROM tm_variant_search WHERE entry_id = ?"},
		{"variant_trigram", "DELETE FROM tm_variant_trigram WHERE entry_id = ?"},
		{"variants", "DELETE FROM tm_variants WHERE entry_id = ?"},
		{"entity values", "DELETE FROM tm_entry_entity_values WHERE entry_id = ?"},
		{"entities", "DELETE FROM tm_entry_entities WHERE entry_id = ?"},
		{"origins", "DELETE FROM tm_entry_origins WHERE entry_id = ?"},
	}
	for _, ct := range childTables {
		if _, err := tx.ExecContext(context.Background(), ct.sql, id); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("delete %s: %w", ct.name, err)
		}
	}

	result, err := tx.ExecContext(context.Background(), "DELETE FROM tm_entries WHERE id = ?", id)
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("delete entry: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		_ = tx.Rollback()
		return fmt.Errorf("rows affected: %w", err)
	}
	if rows == 0 {
		_ = tx.Rollback()
		return fmt.Errorf("entry not found: %s", id)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// --- Lookup ---

// Lookup searches for matches using tiered matching with the full content
// model. Matches are found among entries whose Variants[sourceLocale] exists
// and matches the source Block; returned entries that lack a variant for
// targetLocale are skipped.
func (tm *SQLiteTM) Lookup(source *model.Block, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error) {
	if source == nil {
		return nil, nil
	}

	opts = ApplyDefaults(opts)
	runs := source.Source
	if len(runs) == 0 {
		return nil, nil
	}

	plainKey := NormalizeText(model.FlattenRuns(runs))
	structKey := NormalizeText(model.RunsStructuralText(runs))
	generalKey := NormalizeText(model.RunsGeneralizedText(runs))
	entityAnnotations := ExtractEntityAnnotations(source)

	return tm.tieredLookup(plainKey, structKey, generalKey, entityAnnotations, sourceLocale, targetLocale, opts)
}

// LookupSegment searches for matches against a specific segment of the
// source block. See TranslationMemory.LookupSegment for the contract.
func (tm *SQLiteTM) LookupSegment(source *model.Block, segmentIdx int, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error) {
	if source == nil {
		return nil, nil
	}
	runs := source.SourceSegmentRuns(segmentIdx)
	if len(runs) == 0 {
		return nil, nil
	}
	opts = ApplyDefaults(opts)
	plainKey := NormalizeText(model.FlattenRuns(runs))
	structKey := NormalizeText(model.RunsStructuralText(runs))
	generalKey := NormalizeText(model.RunsGeneralizedText(runs))
	// Entity annotations carry block-level context; keep them so the
	// generalized (entity-aware) tier still works inside a segment.
	entityAnnotations := ExtractEntityAnnotations(source)
	return tm.tieredLookup(plainKey, structKey, generalKey, entityAnnotations, sourceLocale, targetLocale, opts)
}

// LookupText searches for matches using plain text only.
func (tm *SQLiteTM) LookupText(source string, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error) {
	opts = ApplyDefaults(opts)
	opts.MatchModes = []MatchMode{MatchModePlain}
	normalized := NormalizeText(source)
	return tm.tieredLookup(normalized, normalized, normalized, nil, sourceLocale, targetLocale, opts)
}

func (tm *SQLiteTM) tieredLookup(plainKey, structKey, generalKey string, entityAnnotations []*model.EntityAnnotation, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error) {
	var matches []TMMatch
	seen := make(map[string]bool)
	modeEnabled := MatchModesEnabled(opts.MatchModes)

	add := func(entry TMEntry, score float64, mt MatchType) {
		if seen[entry.ID] {
			return
		}
		// Entry must have the requested target variant.
		if !entry.HasLocale(targetLocale) {
			return
		}
		seen[entry.ID] = true
		var adaptations []EntityAdaptation
		if mt == MatchGeneralizedExact || mt == MatchGeneralizedFuzzy {
			adaptations = ComputeEntityAdaptations(entry, sourceLocale, targetLocale, entityAnnotations)
		}
		matches = append(matches, TMMatch{
			Entry:             entry,
			Score:             score,
			MatchType:         mt,
			ProjectID:         entry.ProjectID,
			EntityAdaptations: adaptations,
		})
	}

	// Tier 1-3: exact matches via indexed variant columns.
	if modeEnabled[MatchModeGeneralized] {
		entries, err := tm.queryExactVariant("general_key", generalKey, sourceLocale, opts)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			add(e, 1.0, MatchGeneralizedExact)
		}
	}
	if modeEnabled[MatchModeStructural] {
		entries, err := tm.queryExactVariant("struct_key", structKey, sourceLocale, opts)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			add(e, 1.0, MatchStructuralExact)
		}
	}
	if modeEnabled[MatchModePlain] {
		entries, err := tm.queryExactVariant("plain", plainKey, sourceLocale, opts)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			add(e, 1.0, MatchExact)
		}
	}

	if len(matches) > 0 && opts.MinScore >= 1.0 {
		return LimitResults(matches, opts.MaxResults), nil
	}

	// Tier 4-6: fuzzy candidates via trigram + Levenshtein scoring.
	candidates, err := tm.queryFuzzyCandidates(plainKey, structKey, generalKey, sourceLocale, opts)
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
		var bestType MatchType
		if modeEnabled[MatchModeGeneralized] {
			s := LevenshteinRatio(generalKey, NormalizeText(model.RunsGeneralizedText(srcRuns)))
			if s >= opts.MinScore && s > bestScore {
				bestScore = s
				bestType = MatchGeneralizedFuzzy
			}
		}
		if modeEnabled[MatchModeStructural] {
			s := LevenshteinRatio(structKey, NormalizeText(model.RunsStructuralText(srcRuns)))
			if s >= opts.MinScore && s > bestScore {
				bestScore = s
				bestType = MatchStructuralFuzzy
			}
		}
		if modeEnabled[MatchModePlain] {
			s := LevenshteinRatio(plainKey, NormalizeText(model.FlattenRuns(srcRuns)))
			if s >= opts.MinScore && s > bestScore {
				bestScore = s
				bestType = MatchFuzzy
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

// queryExactVariant finds entries whose source-locale variant matches the
// given normalized key on the specified column (plain/struct_key/general_key).
func (tm *SQLiteTM) queryExactVariant(column, key string, sourceLocale model.LocaleID, opts LookupOptions) ([]TMEntry, error) {
	where := fmt.Sprintf("v.%s = ? AND v.locale = ?", column)
	args := []any{key, string(sourceLocale)}
	entryWhere := ""
	entryWhere, args = appendSQLiteProjectFilter(entryWhere, args, opts.ProjectID, opts.ProjectScope)

	q := fmt.Sprintf(`
		SELECT DISTINCT e.id
		FROM tm_variants v
		INNER JOIN tm_entries e ON e.id = v.entry_id
		WHERE %s%s
		LIMIT 200
	`, where, entryWhere)

	rows, err := tm.db.QueryContext(context.Background(), q, args...)
	if err != nil {
		return nil, fmt.Errorf("query exact variant: %w", err)
	}
	defer rows.Close()

	ids, err := scanIDs(rows)
	if err != nil {
		return nil, err
	}
	return tm.loadEntriesByIDs(ids)
}

// queryFuzzyCandidates returns entry candidates for fuzzy matching filtered
// by source locale, using trigram MATCH where available and falling back to
// length-filtered scanning.
func (tm *SQLiteTM) queryFuzzyCandidates(plainKey, structKey, generalKey string, sourceLocale model.LocaleID, opts LookupOptions) ([]TMEntry, error) {
	if entries, err := tm.queryTrigramCandidates(plainKey, structKey, generalKey, sourceLocale, opts); err == nil {
		return entries, nil
	}
	return tm.queryLengthFiltered(plainKey, sourceLocale, opts)
}

func (tm *SQLiteTM) queryTrigramCandidates(plainKey, structKey, generalKey string, sourceLocale model.LocaleID, opts LookupOptions) ([]TMEntry, error) {
	// FTS5 MATCH on the trigram table returns candidate rows with entry_id
	// unindexed column that we project out.
	q := `
		SELECT DISTINCT entry_id FROM tm_variant_trigram
		WHERE tm_variant_trigram MATCH ? AND locale = ?
		LIMIT 200
	`
	var ids []string
	for _, key := range []string{plainKey, structKey, generalKey} {
		tq := BuildTrigramQuery(key)
		if tq == "" {
			continue
		}
		rows, err := tm.db.QueryContext(context.Background(), q, tq, string(sourceLocale))
		if err != nil {
			return nil, fmt.Errorf("trigram query: %w", err)
		}
		for rows.Next() {
			var id string
			if err := rows.Scan(&id); err == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
	}
	ids = uniqueStrings(ids)
	if len(ids) == 0 {
		return nil, nil
	}
	entries, err := tm.loadEntriesByIDs(ids)
	if err != nil {
		return nil, err
	}
	return tm.filterByProject(entries, opts), nil
}

func (tm *SQLiteTM) queryLengthFiltered(plainKey string, sourceLocale model.LocaleID, opts LookupOptions) ([]TMEntry, error) {
	keyLen := len([]rune(plainKey))
	minLen := int(float64(keyLen) * 0.7)
	maxLen := int(float64(keyLen) * 1.3)
	if minLen < 0 {
		minLen = 0
	}

	rows, err := tm.db.QueryContext(context.Background(), `
		SELECT DISTINCT entry_id FROM tm_variants
		WHERE locale = ? AND LENGTH(plain) BETWEEN ? AND ?
		LIMIT 500
	`, string(sourceLocale), minLen, maxLen)
	if err != nil {
		return nil, fmt.Errorf("length-filtered query: %w", err)
	}
	defer rows.Close()
	ids, err := scanIDs(rows)
	if err != nil {
		return nil, err
	}
	entries, err := tm.loadEntriesByIDs(ids)
	if err != nil {
		return nil, err
	}
	return tm.filterByProject(entries, opts), nil
}

func (tm *SQLiteTM) filterByProject(entries []TMEntry, opts LookupOptions) []TMEntry {
	if opts.ProjectScope == ProjectScopeAll {
		return entries
	}
	out := entries[:0]
	for _, e := range entries {
		switch opts.ProjectScope {
		case ProjectScopeOnly:
			if e.ProjectID == opts.ProjectID {
				out = append(out, e)
			}
		case ProjectScopeExclude:
			if e.ProjectID != opts.ProjectID {
				out = append(out, e)
			}
		}
	}
	return out
}

// BuildTrigramQuery builds an FTS5 trigram MATCH expression for candidate retrieval.
// For multi-word text, uses OR of individual words (each as a substring match).
// For text without word boundaries (CJK, single words), uses overlapping windows.
func BuildTrigramQuery(s string) string {
	if s == "" {
		return ""
	}
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
		where += " AND e.project_id = ?"
		args = append(args, projectID)
	case ProjectScopeExclude:
		where += " AND e.project_id != ?"
		args = append(args, projectID)
	}
	return where, args
}

// --- Entry loading ---

// loadEntriesByIDs fetches full multilingual entries for the given IDs,
// batching variant/entity/origin queries to avoid N+1 fetches.
func (tm *SQLiteTM) loadEntriesByIDs(ids []string) ([]TMEntry, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(ids)-1) + "?"
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := tm.db.QueryContext(context.Background(), `
		SELECT id, project_id, hint_src_lang, properties, note, created_at, updated_at
		FROM tm_entries WHERE id IN (`+placeholders+`)
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("load entries: %w", err)
	}
	defer rows.Close()
	return tm.scanEntriesWithChildren(rows)
}

// scanEntriesWithChildren scans entry rows and then batch-loads variants,
// entities, and origins for all of them, stitching children onto the entries.
// Expected column order: id, project_id, hint_src_lang, properties, note, created_at, updated_at.
func (tm *SQLiteTM) scanEntriesWithChildren(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]TMEntry, error) {
	var entries []TMEntry
	for rows.Next() {
		var e TMEntry
		var propsJSON, hint, note, createdStr, updatedStr string
		if err := rows.Scan(&e.ID, &e.ProjectID, &hint, &propsJSON, &note, &createdStr, &updatedStr); err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		e.HintSrcLang = model.LocaleID(hint)
		e.Note = note
		e.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		e.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)
		if propsJSON != "" {
			_ = json.Unmarshal([]byte(propsJSON), &e.Properties)
		}
		e.Variants = make(map[model.LocaleID][]model.Run)
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate entries: %w", err)
	}
	if len(entries) == 0 {
		return nil, nil
	}

	byID := make(map[string]int, len(entries))
	idArgs := make([]any, len(entries))
	for i, e := range entries {
		byID[e.ID] = i
		idArgs[i] = e.ID
	}
	placeholders := strings.Repeat("?,", len(entries)-1) + "?"

	// Variants. The `coded` column may be either:
	//   - A JSON-encoded []Run (when there are inline codes), identified
	//     by a leading '[' character.
	//   - A plain-text string (fast path used by bulk imports), stored
	//     as-is and materialised as a single TextRun on read.
	varRows, err := tm.db.QueryContext(context.Background(), `SELECT entry_id, locale, coded FROM tm_variants
		WHERE entry_id IN (`+placeholders+`) ORDER BY entry_id, locale`, idArgs...)
	if err == nil {
		for varRows.Next() {
			var eid, loc, coded string
			if err := varRows.Scan(&eid, &loc, &coded); err != nil {
				continue
			}
			runs := decodeVariantRuns(coded)
			if idx, ok := byID[eid]; ok {
				entries[idx].Variants[model.LocaleID(loc)] = runs
			}
		}
		varRows.Close()
	}

	// Entities + per-locale values. Single join query keeps us at O(1) round trips.
	entRows, err := tm.db.QueryContext(context.Background(), `
		SELECT e.entry_id, e.placeholder_id, e.entity_type, e.concept_id,
			v.locale, v.text_value, v.start_pos, v.end_pos
		FROM tm_entry_entities e
		LEFT JOIN tm_entry_entity_values v
			ON v.entry_id = e.entry_id AND v.placeholder_id = e.placeholder_id
		WHERE e.entry_id IN (`+placeholders+`)
		ORDER BY e.entry_id, e.placeholder_id, v.locale
	`, idArgs...)
	if err == nil {
		// Map (entry index, placeholder_id) → entity slice index.
		type entKey struct {
			entryIdx int
			pid      string
		}
		entIdx := make(map[entKey]int)
		for entRows.Next() {
			var eid, pid, etype, conceptID string
			var loc, textVal *string
			var startPos, endPos *int
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
				entries[idx].Entities = append(entries[idx].Entities, EntityMapping{
					PlaceholderID: pid,
					ConceptID:     conceptID,
					Type:          model.EntityType(etype),
					Values:        make(map[model.LocaleID]EntityValue),
				})
				emIdx = len(entries[idx].Entities) - 1
				entIdx[key] = emIdx
			}
			if loc != nil && *loc != "" {
				val := EntityValue{}
				if textVal != nil {
					val.Text = *textVal
				}
				if startPos != nil {
					val.Start = *startPos
				}
				if endPos != nil {
					val.End = *endPos
				}
				entries[idx].Entities[emIdx].Values[model.LocaleID(*loc)] = val
			}
		}
		entRows.Close()
	}

	// Origins.
	originRows, err := tm.db.QueryContext(context.Background(), `SELECT entry_id, source, key, reference, added_at, added_by, session_id
		FROM tm_entry_origins WHERE entry_id IN (`+placeholders+`)
		ORDER BY entry_id, ordinal`, idArgs...)
	if err == nil {
		for originRows.Next() {
			var eid string
			var o Origin
			var addedAtStr string
			if err := originRows.Scan(&eid, &o.Source, &o.Key, &o.Reference, &addedAtStr, &o.AddedBy, &o.SessionID); err != nil {
				continue
			}
			o.AddedAt, _ = time.Parse(time.RFC3339, addedAtStr)
			if idx, ok := byID[eid]; ok {
				entries[idx].Origins = append(entries[idx].Origins, o)
			}
		}
		originRows.Close()
	}

	return entries, nil
}

func scanIDs(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]string, error) {
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return ids, nil
}

func uniqueStrings(in []string) []string {
	if len(in) <= 1 {
		return in
	}
	seen := make(map[string]struct{}, len(in))
	out := in[:0]
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// GetEntry fetches a single entry by ID with all its variants populated.
func (tm *SQLiteTM) GetEntry(id string) (TMEntry, bool) {
	entries, err := tm.loadEntriesByIDs([]string{id})
	if err != nil || len(entries) == 0 {
		return TMEntry{}, false
	}
	return entries[0], true
}

// Entries returns all entries. Used for export operations.
func (tm *SQLiteTM) Entries() []TMEntry {
	rows, err := tm.db.QueryContext(context.Background(), `SELECT id FROM tm_entries ORDER BY id`)
	if err != nil {
		return nil
	}
	defer rows.Close()
	ids, err := scanIDs(rows)
	if err != nil {
		return nil
	}
	entries, err := tm.loadEntriesByIDs(ids)
	if err != nil {
		return nil
	}
	return entries
}

// --- Search ---

// SearchEntries performs a ranked full-text search across variant text.
// See TMStore for parameter semantics.
func (tm *SQLiteTM) SearchEntries(query, anyLocale, requireLocale string, offset, limit int) ([]TMEntry, int) {
	return tm.SearchEntriesFiltered(query, anyLocale, requireLocale, SearchFilter{}, offset, limit)
}

// SearchEntriesFiltered performs a search with additional facet filters.
func (tm *SQLiteTM) SearchEntriesFiltered(query, anyLocale, requireLocale string, filter SearchFilter, offset, limit int) ([]TMEntry, int) {
	return tm.searchInternal(query, anyLocale, requireLocale, "", nil, filter, offset, limit)
}

// SearchEntriesForStream performs a ranked full-text search with stream
// inheritance. streamChain is the ordered list of ancestor streams to
// search; entries from earlier streams take priority.
func (tm *SQLiteTM) SearchEntriesForStream(query, anyLocale, requireLocale, stream string, streamChain []string, offset, limit int) ([]TMEntry, int) {
	return tm.searchInternal(query, anyLocale, requireLocale, stream, streamChain, SearchFilter{}, offset, limit)
}

// searchInternal runs a multilingual search across tm_variants, optionally
// filtered by stream chain (for Search*ForStream callers). It returns
// entries in a deterministic order (FTS5 BM25 rank when query is set,
// updated_at DESC otherwise), with stream priority applied when provided.
func (tm *SQLiteTM) searchInternal(query, anyLocale, requireLocale, stream string, streamChain []string, filter SearchFilter, offset, limit int) ([]TMEntry, int) {
	// Build WHERE clauses for the entry-level join.
	var args []any
	clauses := []string{"1=1"}

	// Text search.
	if query != "" {
		sub := `e.id IN (
			SELECT entry_id FROM tm_variant_search
			WHERE tm_variant_search MATCH ?`
		args = append(args, query)
		if anyLocale != "" {
			sub += " AND locale = ?"
			args = append(args, anyLocale)
		}
		sub += ")"
		clauses = append(clauses, sub)
	} else if anyLocale != "" {
		// Without a text query, require the entry to have a variant in anyLocale.
		clauses = append(clauses, "e.id IN (SELECT entry_id FROM tm_variants WHERE locale = ?)")
		args = append(args, anyLocale)
	}

	// Additional required locale.
	if requireLocale != "" {
		clauses = append(clauses, "e.id IN (SELECT entry_id FROM tm_variants WHERE locale = ?)")
		args = append(args, requireLocale)
	}

	// Stream inheritance.
	streamClause, streamCase, streamArgs, orderArgs := buildStreamFilter(stream, streamChain)
	if streamClause != "" {
		clauses = append(clauses, streamClause)
		args = append(args, streamArgs...)
	}

	// Project / entity / session filters.
	filterClause, filterArgs := filterWhere(filter)
	if filterClause != "" {
		clauses = append(clauses, strings.TrimPrefix(filterClause, " AND "))
		args = append(args, filterArgs...)
	}

	where := strings.Join(clauses, " AND ")

	// Count total.
	var total int
	countArgs := make([]any, len(args))
	copy(countArgs, args)
	if err := tm.db.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM tm_entries e WHERE "+where, countArgs...).Scan(&total); err != nil {
		return nil, 0
	}
	if total == 0 {
		return nil, 0
	}

	// Page query — select IDs in the proper order.
	orderBy := "e.updated_at DESC"
	if streamCase != "" {
		orderBy = streamCase + ", " + orderBy
	}

	pageArgs := make([]any, 0, len(args)+len(orderArgs)+2)
	pageArgs = append(pageArgs, args...)
	pageArgs = append(pageArgs, orderArgs...)
	pageArgs = append(pageArgs, limit, offset)

	q := fmt.Sprintf("SELECT e.id FROM tm_entries e WHERE %s ORDER BY %s LIMIT ? OFFSET ?", where, orderBy)
	rows, err := tm.db.QueryContext(context.Background(), q, pageArgs...)
	if err != nil {
		return nil, total
	}
	defer rows.Close()
	ids, err := scanIDs(rows)
	if err != nil {
		return nil, total
	}
	entries, err := tm.loadEntriesByIDs(ids)
	if err != nil {
		return nil, total
	}
	// Preserve the SQL-ordered ID sequence.
	ordered := orderEntriesByIDs(entries, ids)
	return ordered, total
}

func orderEntriesByIDs(entries []TMEntry, ids []string) []TMEntry {
	if len(entries) == 0 {
		return entries
	}
	index := make(map[string]int, len(entries))
	for i, e := range entries {
		index[e.ID] = i
	}
	out := make([]TMEntry, 0, len(ids))
	for _, id := range ids {
		if idx, ok := index[id]; ok {
			out = append(out, entries[idx])
		}
	}
	return out
}

func buildStreamFilter(stream string, streamChain []string) (whereClause, orderClause string, whereArgs, orderArgs []any) {
	if stream == "" && len(streamChain) == 0 {
		return "", "", nil, nil
	}
	streams := append([]string{stream}, streamChain...)
	placeholders := make([]string, len(streams))
	whereArgs = make([]any, len(streams))
	for i, s := range streams {
		placeholders[i] = "?"
		whereArgs[i] = s
	}
	whereClause = "e.stream IN (" + strings.Join(placeholders, ",") + ")"

	var b strings.Builder
	b.WriteString("CASE e.stream")
	orderArgs = make([]any, 0, len(streams))
	for i, s := range streams {
		fmt.Fprintf(&b, " WHEN ? THEN %d", i)
		orderArgs = append(orderArgs, s)
	}
	fmt.Fprintf(&b, " ELSE %d END", len(streams))
	orderClause = b.String()
	return
}

// filterWhere builds additional WHERE clauses and args for SearchFilter,
// expecting the outer query aliases tm_entries as "e".
func filterWhere(filter SearchFilter) (string, []any) {
	var clauses []string
	var args []any
	if filter.ProjectID != "" {
		clauses = append(clauses, "e.project_id = ?")
		args = append(args, filter.ProjectID)
	}
	if len(filter.SessionIDs) > 0 {
		placeholders := make([]string, len(filter.SessionIDs))
		for i, sid := range filter.SessionIDs {
			placeholders[i] = "?"
			args = append(args, sid)
		}
		clauses = append(clauses,
			"e.id IN (SELECT entry_id FROM tm_entry_origins WHERE session_id IN ("+strings.Join(placeholders, ",")+"))")
	}
	if len(filter.EntityTypes) > 0 {
		placeholders := make([]string, len(filter.EntityTypes))
		for i, et := range filter.EntityTypes {
			placeholders[i] = "?"
			args = append(args, et)
		}
		clauses = append(clauses,
			"e.id IN (SELECT entry_id FROM tm_entry_entities WHERE entity_type IN ("+strings.Join(placeholders, ",")+"))")
	}
	if len(filter.EntityValues) > 0 {
		pairs := make([]string, len(filter.EntityValues))
		for i, ev := range filter.EntityValues {
			pairs[i] = "(v.text_value = ? AND ee.entity_type = ?)"
			args = append(args, ev.Value, ev.Type)
		}
		clauses = append(clauses,
			"e.id IN (SELECT v.entry_id FROM tm_entry_entity_values v INNER JOIN tm_entry_entities ee "+
				"ON ee.entry_id = v.entry_id AND ee.placeholder_id = v.placeholder_id WHERE "+
				strings.Join(pairs, " OR ")+")")
	}
	if filter.HasCodes != nil {
		if *filter.HasCodes {
			clauses = append(clauses, "e.has_codes = 1")
		} else {
			clauses = append(clauses, "e.has_codes = 0")
		}
	}
	if len(clauses) == 0 {
		return "", nil
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

// --- Facet stats ---

// FacetStats returns aggregated facet data across the full TM.
func (tm *SQLiteTM) FacetStats() FacetData {
	return tm.FacetStatsFiltered("", "", "", SearchFilter{})
}

// FacetStatsFiltered returns facet counts scoped to entries matching the
// given search query and filter.
func (tm *SQLiteTM) FacetStatsFiltered(query, anyLocale, requireLocale string, filter SearchFilter) FacetData {
	subWhere, subArgs := tm.buildFacetSubquery(query, anyLocale, requireLocale, filter)

	data := FacetData{}

	// Run all facet queries concurrently. SQLite WAL mode allows parallel
	// readers, and the connection pool has room for all 5 queries.
	ctx := context.Background()
	var wg sync.WaitGroup

	// Locale facets.
	wg.Add(1)
	go func() {
		defer wg.Done()
		var localeQ string
		if subWhere == "1=1" {
			localeQ = `SELECT locale, COUNT(*) FROM tm_variants GROUP BY locale ORDER BY COUNT(*) DESC`
		} else {
			localeQ = `SELECT v.locale, COUNT(DISTINCT v.entry_id) FROM tm_variants v
				INNER JOIN tm_entries e ON e.id = v.entry_id
				WHERE ` + subWhere + `
				GROUP BY v.locale ORDER BY COUNT(DISTINCT v.entry_id) DESC`
		}
		if rows, err := tm.db.QueryContext(ctx, localeQ, subArgs...); err == nil {
			var locales []LocaleFacet
			for rows.Next() {
				var lf LocaleFacet
				if err := rows.Scan(&lf.Locale, &lf.Count); err == nil {
					locales = append(locales, lf)
				}
			}
			rows.Close()
			data.Locales = locales
		}
	}()

	// Project facets.
	wg.Add(1)
	go func() {
		defer wg.Done()
		projQ := `SELECT e.project_id, COUNT(*) FROM tm_entries e WHERE ` + subWhere + ` GROUP BY e.project_id ORDER BY COUNT(*) DESC`
		if rows, err := tm.db.QueryContext(ctx, projQ, subArgs...); err == nil {
			var projects []ProjectFacet
			for rows.Next() {
				var pf ProjectFacet
				if err := rows.Scan(&pf.ProjectID, &pf.Count); err == nil {
					projects = append(projects, pf)
				}
			}
			rows.Close()
			data.Projects = projects
		}
	}()

	// Entity type facets.
	wg.Add(1)
	go func() {
		defer wg.Done()
		etQ := `SELECT ent.entity_type, COUNT(DISTINCT ent.entry_id)
			FROM tm_entry_entities ent
			INNER JOIN tm_entries e ON e.id = ent.entry_id
			WHERE ` + subWhere + `
			GROUP BY ent.entity_type ORDER BY COUNT(DISTINCT ent.entry_id) DESC`
		if rows, err := tm.db.QueryContext(ctx, etQ, subArgs...); err == nil {
			var types []EntityTypeFacet
			for rows.Next() {
				var ef EntityTypeFacet
				if err := rows.Scan(&ef.Type, &ef.Count); err == nil {
					types = append(types, ef)
				}
			}
			rows.Close()
			data.EntityTypes = types
		}
	}()

	// Import session facets.
	wg.Add(1)
	go func() {
		defer wg.Done()
		sessQ := `SELECT s.id, s.file_key, s.tool_name, s.imported_at, COUNT(DISTINCT o.entry_id)
			FROM tm_import_sessions s
			INNER JOIN tm_entry_origins o ON o.session_id = s.id
			INNER JOIN tm_entries e ON e.id = o.entry_id
			WHERE ` + subWhere + `
			GROUP BY s.id ORDER BY COUNT(DISTINCT o.entry_id) DESC`
		if rows, err := tm.db.QueryContext(ctx, sessQ, subArgs...); err == nil {
			var sessions []ImportSessionFacet
			for rows.Next() {
				var sf ImportSessionFacet
				var importedAtStr string
				if err := rows.Scan(&sf.SessionID, &sf.FileKey, &sf.ToolName, &importedAtStr, &sf.Count); err == nil {
					sf.ImportedAt, _ = time.Parse(time.RFC3339, importedAtStr)
					sessions = append(sessions, sf)
				}
			}
			rows.Close()
			data.ImportSessions = sessions
		}
	}()

	// Inline code facets.
	wg.Add(1)
	go func() {
		defer wg.Done()
		codeQ := `SELECT
			SUM(CASE WHEN e.has_codes = 1 THEN 1 ELSE 0 END),
			SUM(CASE WHEN e.has_codes = 0 THEN 1 ELSE 0 END)
			FROM tm_entries e WHERE ` + subWhere
		_ = tm.db.QueryRowContext(ctx, codeQ, subArgs...).Scan(&data.HasCodes, &data.NoCodes)
	}()

	wg.Wait()
	return data
}

// buildFacetSubquery builds a WHERE clause (using alias `e`) that matches the
// same entries SearchEntriesFiltered would return.
func (tm *SQLiteTM) buildFacetSubquery(query, anyLocale, requireLocale string, filter SearchFilter) (string, []any) {
	var args []any
	var clauses []string

	if query != "" {
		sub := `e.id IN (SELECT entry_id FROM tm_variant_search WHERE tm_variant_search MATCH ?`
		args = append(args, query)
		if anyLocale != "" {
			sub += " AND locale = ?"
			args = append(args, anyLocale)
		}
		sub += ")"
		clauses = append(clauses, sub)
	} else if anyLocale != "" {
		clauses = append(clauses, "e.id IN (SELECT entry_id FROM tm_variants WHERE locale = ?)")
		args = append(args, anyLocale)
	}
	if requireLocale != "" {
		clauses = append(clauses, "e.id IN (SELECT entry_id FROM tm_variants WHERE locale = ?)")
		args = append(args, requireLocale)
	}
	if fc, fa := filterWhere(filter); fc != "" {
		clauses = append(clauses, strings.TrimPrefix(fc, " AND "))
		args = append(args, fa...)
	}
	if len(clauses) == 0 {
		return "1=1", nil
	}
	return strings.Join(clauses, " AND "), args
}

// LocaleStats returns per-locale entry counts across the full TM.
func (tm *SQLiteTM) LocaleStats() []LocaleFacet {
	rows, err := tm.db.QueryContext(context.Background(), `
		SELECT locale, COUNT(DISTINCT entry_id) FROM tm_variants
		GROUP BY locale ORDER BY COUNT(DISTINCT entry_id) DESC
	`)
	if err != nil {
		slog.Warn("TM locale stats query failed", "error", err)
		return nil
	}
	defer rows.Close()
	var out []LocaleFacet
	for rows.Next() {
		var lf LocaleFacet
		if err := rows.Scan(&lf.Locale, &lf.Count); err == nil {
			out = append(out, lf)
		}
	}
	return out
}

// ActivityStats returns daily entry counts over time based on created_at.
func (tm *SQLiteTM) ActivityStats() []ActivityStat {
	rows, err := tm.db.QueryContext(context.Background(),
		`SELECT DATE(created_at) AS day, COUNT(*) FROM tm_entries GROUP BY day ORDER BY day`,
	)
	if err != nil {
		slog.Warn("TM activity stats query failed", "error", err)
		return nil
	}
	defer rows.Close()
	var out []ActivityStat
	for rows.Next() {
		var s ActivityStat
		if err := rows.Scan(&s.Date, &s.Count); err == nil {
			out = append(out, s)
		}
	}
	return out
}

// --- Import sessions ---

// CreateImportSession inserts a new import session row.
func (tm *SQLiteTM) CreateImportSession(session ImportSession) error {
	if session.ID == "" {
		return ErrSessionIDRequired
	}
	if session.FileKey == "" {
		return ErrSessionFileKey
	}
	if session.ImportedAt.IsZero() {
		session.ImportedAt = time.Now()
	}
	propsJSON := ""
	if len(session.Properties) > 0 {
		b, err := json.Marshal(session.Properties)
		if err != nil {
			return fmt.Errorf("marshal session properties: %w", err)
		}
		propsJSON = string(b)
	}
	_, err := tm.db.ExecContext(context.Background(), `INSERT INTO tm_import_sessions
		(id, file_key, file_hash, file_size_bytes, imported_at, imported_by,
		 tool_name, tool_version, seg_type, admin_lang, src_lang, data_type,
		 original_format, original_encoding, entry_count, properties_json)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		session.ID, session.FileKey, session.FileHash, session.FileSizeBytes,
		session.ImportedAt.Format(time.RFC3339), session.ImportedBy,
		session.ToolName, session.ToolVersion, session.SegType,
		session.AdminLang, session.SrcLang, session.DataType,
		session.OriginalFormat, session.OriginalEncoding, session.EntryCount,
		propsJSON)
	if err != nil {
		return fmt.Errorf("insert import session: %w", err)
	}
	return nil
}

// GetImportSession fetches a session by ID.
func (tm *SQLiteTM) GetImportSession(id string) (ImportSession, bool) {
	return tm.querySingleSession("SELECT "+sessionColumns+" FROM tm_import_sessions WHERE id = ?", id)
}

// FindImportSessionByHash returns the most recent session matching the hash.
func (tm *SQLiteTM) FindImportSessionByHash(hash string) (ImportSession, bool) {
	if hash == "" {
		return ImportSession{}, false
	}
	return tm.querySingleSession(
		"SELECT "+sessionColumns+" FROM tm_import_sessions WHERE file_hash = ? ORDER BY imported_at DESC LIMIT 1",
		hash)
}

// ListImportSessions returns all sessions ordered by imported_at DESC.
func (tm *SQLiteTM) ListImportSessions() []ImportSession {
	rows, err := tm.db.QueryContext(context.Background(), "SELECT "+sessionColumns+" FROM tm_import_sessions ORDER BY imported_at DESC")
	if err != nil {
		return nil
	}
	defer rows.Close()
	var out []ImportSession
	for rows.Next() {
		s, ok := scanSession(rows)
		if ok {
			out = append(out, s)
		}
	}
	return out
}

// UpdateImportSessionCount sets the entry_count on a session.
func (tm *SQLiteTM) UpdateImportSessionCount(id string, count int) error {
	res, err := tm.db.ExecContext(context.Background(), `UPDATE tm_import_sessions SET entry_count = ? WHERE id = ?`, count, id)
	if err != nil {
		return fmt.Errorf("update session count: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrImportSessionMiss
	}
	return nil
}

// DeleteImportSession removes a session row. Origins referencing it have
// their session_id cleared to empty (no true FK SET NULL — emulated here).
func (tm *SQLiteTM) DeleteImportSession(id string) error {
	if _, err := tm.db.ExecContext(context.Background(), `UPDATE tm_entry_origins SET session_id = '' WHERE session_id = ?`, id); err != nil {
		return fmt.Errorf("clear origin session_id: %w", err)
	}
	res, err := tm.db.ExecContext(context.Background(), `DELETE FROM tm_import_sessions WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrImportSessionMiss
	}
	return nil
}

const sessionColumns = `id, file_key, file_hash, file_size_bytes, imported_at,
	imported_by, tool_name, tool_version, seg_type, admin_lang, src_lang,
	data_type, original_format, original_encoding, entry_count, properties_json`

type sessionScanner interface {
	Scan(dest ...any) error
}

func scanSession(sc sessionScanner) (ImportSession, bool) {
	var s ImportSession
	var importedAtStr, propsJSON string
	if err := sc.Scan(&s.ID, &s.FileKey, &s.FileHash, &s.FileSizeBytes,
		&importedAtStr, &s.ImportedBy, &s.ToolName, &s.ToolVersion,
		&s.SegType, &s.AdminLang, &s.SrcLang, &s.DataType,
		&s.OriginalFormat, &s.OriginalEncoding, &s.EntryCount, &propsJSON); err != nil {
		return ImportSession{}, false
	}
	s.ImportedAt, _ = time.Parse(time.RFC3339, importedAtStr)
	if propsJSON != "" {
		_ = json.Unmarshal([]byte(propsJSON), &s.Properties)
	}
	return s, true
}

func (tm *SQLiteTM) querySingleSession(q string, args ...any) (ImportSession, bool) {
	row := tm.db.QueryRowContext(context.Background(), q, args...)
	return scanSession(row)
}
