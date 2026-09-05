package memory

import (
	"cmp"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/storage"
	"github.com/neokapi/neokapi/memory/schema"
)

// migrationsTable names the bookkeeping table that records which schema
// migrations this database has already applied.
//
// It keeps the historical "sievepen" spelling on purpose. The name is written
// into every content-memory database ever created, so it is persisted state,
// not an internal identifier. Renaming it would make Migrate find no
// bookkeeping table in an existing database, conclude that nothing had been
// applied, and replay every migration against a populated schema. The package
// is called `memory` and the prose says "content memory"; this one string stays
// as it is until there is a migration that renames the table deliberately.
const migrationsTable = "sievepen_migrations"

// Sentinel errors for content-memory entry validation.
var (
	ErrEntryIDRequired   = errors.New("entry ID is required")
	ErrEntryNoVariants   = errors.New("entry must have at least one variant")
	ErrSessionIDRequired = errors.New("import session ID is required")
	ErrSessionFileKey    = errors.New("import session file_key is required")
	ErrImportSessionMiss = errors.New("import session not found")
)

// SQLiteStore is a multilingual, persistent content memory backed by SQLite.
// Each entry has a map of language variants (locale → []model.Run) plus
// normalized match keys per variant for tiered lookup.
type SQLiteStore struct {
	db *storage.DB

	fuzzyMu    sync.Mutex
	fuzzyIndex fuzzyIndexState
}

// fuzzyIndexState is what the store knows about tm_variant_trigram, the FTS5
// side-table the fuzzy tier queries for candidates. The bulk write path
// deliberately leaves it unpopulated, so an empty MATCH result does not mean
// "no match" — the store has to know which it is before it can trust one.
type fuzzyIndexState int

const (
	fuzzyIndexUnknown fuzzyIndexState = iota
	fuzzyIndexReady
	fuzzyIndexStale
)

// Compile-time checks that SQLiteStore satisfies the content memory interfaces.
var (
	_ ContentMemory = (*SQLiteStore)(nil)
	_ Store         = (*SQLiteStore)(nil)
)

// NewSQLiteStore opens (or creates) a SQLite-backed content memory.
// Use ":memory:" for an in-memory database (useful for testing).
func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := storage.Open(dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := storage.Migrate(db, migrationsTable, memoryMigrations); err != nil {
		db.Close()
		return nil, fmt.Errorf("migrate schema: %w", err)
	}

	return &SQLiteStore{db: db}, nil
}

// NewSQLiteStoreFromDB creates a SQLiteStore from an already-opened database.
// This allows sharing a single DB file across content memory and terms.
func NewSQLiteStoreFromDB(db *storage.DB) (*SQLiteStore, error) {
	if err := storage.Migrate(db, migrationsTable, memoryMigrations); err != nil {
		return nil, fmt.Errorf("migrate schema: %w", err)
	}
	return &SQLiteStore{db: db}, nil
}

// memoryMigrations defines the multilingual content memory schema. Every entry is symmetric:
// there is no "source"/"target", only a set of language variants. Match keys
// (plain, structural, generalized) are computed per variant at write time and
// indexed for tiered lookup. The DDL is rendered from the shared schema
// descriptors (memory/schema) so the SQLite and Postgres backends cannot
// drift; the rendered SQLite DDL is byte-identical to the historical
// migrations (golden-tested in memory/schema).
//
// tm_variant_search uses storage.FTSWordTokenizer, which resolves to the ICU
// tokenizer under cgo builds and unicode61 under no-cgo builds (the ICU
// tokenizer is a cgo-only extension). A .db whose FTS table was created with
// one tokenizer cannot be FTS-word-queried by a binary built with the other;
// the trigram table stays portable.
var memoryMigrations = []storage.Migration{
	{
		Version:     1,
		Description: "multilingual content memory schema with per-variant match keys and import sessions",
		SQL:         schema.RenderMemorySQLiteV1(storage.FTSWordTokenizer),
	},
	{
		Version:     2,
		Description: "add concept_id to entity mappings for terms cross-reference",
		SQL:         schema.RenderMemorySQLiteV2(),
	},
	{
		Version:     3,
		Description: "precomputed has_codes flag on tm_entries for fast facet queries",
		SQL:         schema.RenderMemorySQLiteV3(),
	},
	{
		Version: 4,
		// An answer is approved somewhere, and where it was approved is what
		// qualifies it: one source string can carry a different reviewed
		// wording per collection, and only the point tells them apart.
		//
		// Additive, so nothing is copied and nothing is lost. An entry written
		// before the column existed reads as the project's default point, which
		// is a true statement about it rather than a placeholder — a seed, an
		// import and an ad-hoc addition are bound to no location, and that is
		// exactly what the empty point means. The entries that DO have a point,
		// the ones read back out of committed translations, are re-learned with
		// it the next time the record is absorbed.
		Description: "the context point an entry's answer was approved at",
		SQL:         schema.RenderMemorySQLiteV4(),
	},
	{
		Version: 5,
		// The corpus already held every version of a block's answer: a changed
		// source writes a new entry beside the old rather than replacing it.
		// What it could not do was say the two were the same block, because the
		// only key was the text — and the text is the thing that moved.
		//
		// The unit is that link. It is resolved by reconciliation, so it
		// survives an edit and a reorder, and it is the same identity a decision
		// and a history entry are filed under.
		//
		// Additive, and deliberately not backfilled. An entry written before the
		// column existed has no unit and never will: the corpus seeds from
		// committed bundles rather than from live content, so nothing walks the
		// repo re-reconciling old answers. Version lookups therefore begin from
		// here rather than reaching backwards, which is the honest reading of an
		// empty unit — not "no block", but "approved before the chain existed".
		Description: "the durable block identity an entry's answer was approved for",
		SQL:         schema.RenderMemorySQLiteV5(),
	},
}

// DB returns the underlying database for direct access.
func (tm *SQLiteStore) DB() *storage.DB { return tm.db }

// Close closes the database connection.
func (tm *SQLiteStore) Close() error { return tm.db.Close() }

// Count returns the total number of entries.
func (tm *SQLiteStore) Count(ctx context.Context) (int, error) {
	var count int
	if err := tm.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tm_entries").Scan(&count); err != nil {
		return 0, fmt.Errorf("count entries: %w", err)
	}
	return count, nil
}

// Add inserts or updates a multilingual content-memory entry with an empty stream.
func (tm *SQLiteStore) Add(ctx context.Context, entry Entry) error {
	return tm.AddWithStream(ctx, entry, "")
}

// AddWithStream inserts or updates a multilingual content-memory entry associated with a
// stream (e.g., a git branch name). Variants are replaced per locale (see
// variantLocaleDeletes); entities, entity values and origins are replaced
// wholesale. Everything happens inside a single transaction so that a partial
// failure can't leave orphan rows and bulk imports aren't gated by
// per-statement fsync latency.
func (tm *SQLiteStore) AddWithStream(ctx context.Context, entry Entry, stream string) error {
	tx, err := tm.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	if err := tm.addInTx(ctx, tx.Tx, entry, stream); err != nil {
		_ = tx.Rollback()
		// The statements run on the embedded *sql.Tx, past the gated handle
		// that would have named a cancellation for us; see storage.CancelledBy.
		return storage.CancelledBy(ctx, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// BulkAddWithStream inserts or updates many content-memory entries inside a single
// transaction, using prepared statements that are reused across all rows.
// The FTS5 trigram (fuzzy-candidate) index is NOT populated in the bulk
// path — for large corpora its n-gram build cost dominates everything
// else. Call RebuildFuzzyIndex(ctx) at the end of the import to repopulate
// it in a single set-based SELECT INTO, which is orders of magnitude
// faster than row-by-row inserts.
func (tm *SQLiteStore) BulkAddWithStream(ctx context.Context, entries []Entry, stream string) error {
	if len(entries) == 0 {
		return nil
	}
	tx, err := tm.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}

	// The prepared statements below live on the embedded *sql.Tx, past the gated
	// handle, so a cancellation that closes them out from under this loop has to
	// be named here; see storage.CancelledBy.
	stmts, err := prepareBulkStmts(ctx, tx.Tx)
	if err != nil {
		_ = tx.Rollback()
		return storage.CancelledBy(ctx, err)
	}
	defer stmts.Close()

	for i := range entries {
		if err := stmts.addEntry(ctx, &entries[i], stream); err != nil {
			_ = tx.Rollback()
			return storage.CancelledBy(ctx, fmt.Errorf("bulk add entry %s: %w", entries[i].ID, err))
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	tm.setFuzzyIndexState(fuzzyIndexStale)
	return nil
}

// RebuildFuzzyIndex rebuilds the FTS5 trigram index (tm_variant_trigram)
// from the current contents of tm_variants. This is the recommended way
// to populate the fuzzy-candidate index after a bulk load — per-row
// inserts during the bulk pass are prohibitively slow because FTS5
// trigram tokenization allocates heavily on every insert.
//
// Until this is called, fuzzy lookups fall back to length-filtered
// scanning over tm_variants, which is functional but slower on huge Memories.
//
// It is the largest single write the content memory makes — seconds over a
// dogfood-sized corpus, holding the write lock throughout — so ctx is the
// caller's only way to stop it, and is honoured rather than discarded.
func (tm *SQLiteStore) RebuildFuzzyIndex(ctx context.Context) error {
	if _, err := tm.db.ExecContext(ctx, `DELETE FROM tm_variant_trigram`); err != nil {
		return fmt.Errorf("clear fuzzy index: %w", err)
	}
	if _, err := tm.db.ExecContext(ctx, `INSERT INTO tm_variant_trigram
		(plain, struct_key, general_key, locale, entry_id)
		SELECT plain, struct_key, general_key, locale, entry_id FROM tm_variants`); err != nil {
		return fmt.Errorf("rebuild fuzzy index: %w", err)
	}
	tm.setFuzzyIndexState(fuzzyIndexReady)
	return nil
}

// RebuildSearchIndex rebuilds the FTS5 word-search index
// (tm_variant_search) in a single set-based INSERT … SELECT. Like
// RebuildFuzzyIndex this is a post-bulk-load step — the bulk path
// deliberately skips per-row FTS5 inserts because FTS5 ICU
// tokenization is expensive.
//
// Like RebuildFuzzyIndex it runs for seconds on a large corpus and honours ctx.
func (tm *SQLiteStore) RebuildSearchIndex(ctx context.Context) error {
	if _, err := tm.db.ExecContext(ctx, `DELETE FROM tm_variant_search`); err != nil {
		return fmt.Errorf("clear search index: %w", err)
	}
	if _, err := tm.db.ExecContext(ctx, `INSERT INTO tm_variant_search
		(text, locale, entry_id)
		SELECT plain, locale, entry_id FROM tm_variants`); err != nil {
		return fmt.Errorf("rebuild search index: %w", err)
	}
	return nil
}

func (tm *SQLiteStore) setFuzzyIndexState(state fuzzyIndexState) {
	tm.fuzzyMu.Lock()
	tm.fuzzyIndex = state
	tm.fuzzyMu.Unlock()
}

// fuzzyIndexUsable reports whether tm_variant_trigram covers all of
// tm_variants, and so whether an empty MATCH result can be believed. The
// verdict is resolved once per store and then carried by the write paths that
// change it.
func (tm *SQLiteStore) fuzzyIndexUsable(ctx context.Context) bool {
	tm.fuzzyMu.Lock()
	defer tm.fuzzyMu.Unlock()
	switch tm.fuzzyIndex {
	case fuzzyIndexReady:
		return true
	case fuzzyIndexStale:
		return false
	}
	state, err := tm.probeFuzzyIndex(ctx)
	if err != nil {
		// Leave the verdict unresolved so a cancelled or busy probe is not
		// cached; the scan path this returns to reports the real error.
		return false
	}
	tm.fuzzyIndex = state
	return state == fuzzyIndexReady
}

// probeFuzzyIndex decides the state of a store this process has not written to,
// which is how a content memory bulk-imported by an earlier run presents
// itself: variants on disk, an index that was never rebuilt.
func (tm *SQLiteStore) probeFuzzyIndex(ctx context.Context) (fuzzyIndexState, error) {
	var hasVariants, hasIndex bool
	if err := tm.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM tm_variants)`).Scan(&hasVariants); err != nil {
		return fuzzyIndexUnknown, fmt.Errorf("probe variants: %w", err)
	}
	if !hasVariants {
		return fuzzyIndexReady, nil
	}
	if err := tm.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM tm_variant_trigram)`).Scan(&hasIndex); err != nil {
		return fuzzyIndexUnknown, fmt.Errorf("probe fuzzy index: %w", err)
	}
	if !hasIndex {
		return fuzzyIndexStale, nil
	}
	return fuzzyIndexReady, nil
}

// bulkStmts holds the set of prepared statements used by BulkAddWithStream.
// Each BulkAdd call prepares them once and reuses across all entries.
// Note: the FTS5 search tables (tm_variant_search, tm_variant_trigram)
// are deliberately NOT maintained here — see BulkAddWithStream doc
// comment for rationale. Call RebuildFuzzyIndex(ctx) and RebuildSearchIndex(ctx)
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

func prepareBulkStmts(ctx context.Context, tx *sql.Tx) (*bulkStmts, error) {
	s := &bulkStmts{}

	var err error
	if s.upsertEntry, err = tx.PrepareContext(ctx, `INSERT INTO tm_entries
		(id, project_id, stream, hint_src_lang, properties, note, has_codes, point, unit, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_id    = excluded.project_id,
			stream        = excluded.stream,
			hint_src_lang = excluded.hint_src_lang,
			properties    = excluded.properties,
			note          = excluded.note,
			has_codes     = excluded.has_codes,
			point         = excluded.point,
			unit          = excluded.unit,
			updated_at    = excluded.updated_at`); err != nil {
		return nil, fmt.Errorf("prepare upsert: %w", err)
	}

	// Per locale, not per entry: see variantLocaleDeletes. The FTS5 side-tables
	// are not maintained here at all — the bulk path rebuilds them set-wise.
	if s.delVariants, err = tx.PrepareContext(ctx, `DELETE FROM tm_variants WHERE entry_id = ? AND locale = ?`); err != nil {
		return nil, err
	}
	if s.insVariant, err = tx.PrepareContext(ctx, `INSERT INTO tm_variants
		(entry_id, locale, coded, plain, struct_key, general_key) VALUES (?, ?, ?, ?, ?, ?)`); err != nil {
		return nil, err
	}
	if s.delEntities, err = tx.PrepareContext(ctx, `DELETE FROM tm_entry_entities WHERE entry_id = ?`); err != nil {
		return nil, err
	}
	if s.delEntityValues, err = tx.PrepareContext(ctx, `DELETE FROM tm_entry_entity_values WHERE entry_id = ?`); err != nil {
		return nil, err
	}
	if s.insEntity, err = tx.PrepareContext(ctx, `INSERT INTO tm_entry_entities
		(entry_id, placeholder_id, entity_type, concept_id) VALUES (?, ?, ?, ?)`); err != nil {
		return nil, err
	}
	if s.insEntityValue, err = tx.PrepareContext(ctx, `INSERT INTO tm_entry_entity_values
		(entry_id, placeholder_id, locale, text_value, start_pos, end_pos) VALUES (?, ?, ?, ?, ?, ?)`); err != nil {
		return nil, err
	}
	if s.delOrigins, err = tx.PrepareContext(ctx, `DELETE FROM tm_entry_origins WHERE entry_id = ?`); err != nil {
		return nil, err
	}
	if s.insOrigin, err = tx.PrepareContext(ctx, `INSERT INTO tm_entry_origins
		(entry_id, ordinal, source, key, reference, added_at, added_by, session_id, context_fp)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`); err != nil {
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
// bulk-import hot path. It mirrors the same upsert semantics — variants
// replaced per locale, entities and origins wholesale.
func (s *bulkStmts) addEntry(ctx context.Context, entry *Entry, stream string) error {
	if entry.ID == "" {
		return ErrEntryIDRequired
	}
	if len(entry.Variants) == 0 {
		return ErrEntryNoVariants
	}
	NormalizeEntryLocales(entry)

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

	if _, err := s.upsertEntry.ExecContext(ctx,
		entry.ID, entry.ProjectID, stream, string(entry.HintSrcLang),
		propertiesJSON, entry.Note, hasCodes, entry.Point, entry.Unit,
		entry.CreatedAt.Format(time.RFC3339), entry.UpdatedAt.Format(time.RFC3339),
	); err != nil {
		return fmt.Errorf("upsert entry: %w", err)
	}

	for locale := range entry.Variants {
		if _, err := s.delVariants.ExecContext(ctx, entry.ID, string(locale)); err != nil {
			return fmt.Errorf("delete variants %s: %w", locale, err)
		}
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
		//
		// `coded` is the authoritative content and stays VERBATIM;
		// NormalizeText applies only to the derived matching keys.
		// Normalizing coded here used to collapse line structure on
		// every bulk-imported variant (multi-line targets came back as
		// one line), which the non-bulk addInTx path never did.
		var coded, plain, structKey, generalKey string
		if isPlainTextRuns(runs) && !strings.HasPrefix(runs[0].Text.Text, "[") {
			coded = runs[0].Text.Text
			plain = NormalizeText(coded)
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

		if _, err := s.insVariant.ExecContext(ctx, entry.ID, string(locale), coded, plain, structKey, generalKey); err != nil {
			return fmt.Errorf("insert variant %s: %w", locale, err)
		}
	}

	if _, err := s.delEntities.ExecContext(ctx, entry.ID); err != nil {
		return fmt.Errorf("delete entities: %w", err)
	}
	if _, err := s.delEntityValues.ExecContext(ctx, entry.ID); err != nil {
		return fmt.Errorf("delete entity values: %w", err)
	}
	for _, em := range entry.Entities {
		if em.PlaceholderID == "" {
			continue
		}
		if _, err := s.insEntity.ExecContext(ctx, entry.ID, em.PlaceholderID, string(em.Type), em.ConceptID); err != nil {
			return fmt.Errorf("insert entity: %w", err)
		}
		for loc, val := range em.Values {
			if _, err := s.insEntityValue.ExecContext(ctx, entry.ID, em.PlaceholderID, string(loc), val.Text, val.Start, val.End); err != nil {
				return fmt.Errorf("insert entity value: %w", err)
			}
		}
	}

	if _, err := s.delOrigins.ExecContext(ctx, entry.ID); err != nil {
		return fmt.Errorf("delete origins: %w", err)
	}
	for i, o := range entry.Origins {
		addedAt := o.AddedAt
		if addedAt.IsZero() {
			addedAt = now
		}
		if _, err := s.insOrigin.ExecContext(ctx,
			entry.ID, i, o.Source, o.Key, o.Reference,
			addedAt.Format(time.RFC3339), o.AddedBy, o.SessionID, o.ContextFingerprint,
		); err != nil {
			return fmt.Errorf("insert origin: %w", err)
		}
	}
	return nil
}

// variantLocaleDeletes clears one locale of one entry across the variant table
// and the two manually-maintained FTS5 side-tables.
//
// A write replaces the locales it carries and leaves the rest of the entry
// standing. One source text is taught one target locale at a time — materialize
// runs per locale, and every locale keys the same entry off the source content
// hash — so clearing the whole variant set would leave an entry holding only
// the locale written last.
var variantLocaleDeletes = []struct{ what, sql string }{
	{"variants", "DELETE FROM tm_variants WHERE entry_id = ? AND locale = ?"},
	{"variant_search", "DELETE FROM tm_variant_search WHERE entry_id = ? AND locale = ?"},
	{"variant_trigram", "DELETE FROM tm_variant_trigram WHERE entry_id = ? AND locale = ?"},
}

// addInTx performs the full upsert of an entry (header + variants +
// entities + origins) against the given transaction. It is the shared
// implementation used by AddWithStream and BulkAddWithStream.
func (tm *SQLiteStore) addInTx(ctx context.Context, tx *sql.Tx, entry Entry, stream string) error {
	if entry.ID == "" {
		return ErrEntryIDRequired
	}
	if len(entry.Variants) == 0 {
		return ErrEntryNoVariants
	}
	NormalizeEntryLocales(&entry)

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

	if _, err := tx.ExecContext(ctx, `
		INSERT INTO tm_entries (id, project_id, stream, hint_src_lang, properties, note, has_codes, point, unit, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			project_id    = excluded.project_id,
			stream        = excluded.stream,
			hint_src_lang = excluded.hint_src_lang,
			properties    = excluded.properties,
			note          = excluded.note,
			has_codes     = excluded.has_codes,
			point         = excluded.point,
			unit          = excluded.unit,
			updated_at    = excluded.updated_at
	`, entry.ID, entry.ProjectID, stream, string(entry.HintSrcLang),
		propertiesJSON, entry.Note, hasCodes, entry.Point, entry.Unit,
		entry.CreatedAt.Format(time.RFC3339), entry.UpdatedAt.Format(time.RFC3339)); err != nil {
		return fmt.Errorf("upsert entry: %w", err)
	}

	// Replace the variants this write carries, locale by locale. We also
	// maintain the two FTS5 side-tables manually (they are not content=
	// external FTS, so triggers aren't wired).
	for locale := range entry.Variants {
		for _, del := range variantLocaleDeletes {
			if _, err := tx.ExecContext(ctx, del.sql, entry.ID, string(locale)); err != nil {
				return fmt.Errorf("delete %s %s: %w", del.what, locale, err)
			}
		}
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

		if _, err := tx.ExecContext(ctx, `INSERT INTO tm_variants
			(entry_id, locale, coded, plain, struct_key, general_key)
			VALUES (?, ?, ?, ?, ?, ?)`,
			entry.ID, string(locale), string(coded), plain, structKey, generalKey); err != nil {
			return fmt.Errorf("insert variant %s: %w", locale, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO tm_variant_search (text, locale, entry_id)
			VALUES (?, ?, ?)`, plain, string(locale), entry.ID); err != nil {
			return fmt.Errorf("insert variant_search: %w", err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO tm_variant_trigram (plain, struct_key, general_key, locale, entry_id)
			VALUES (?, ?, ?, ?, ?)`, plain, structKey, generalKey, string(locale), entry.ID); err != nil {
			return fmt.Errorf("insert variant_trigram: %w", err)
		}
	}

	// Replace entities + per-locale entity values.
	if _, err := tx.ExecContext(ctx, "DELETE FROM tm_entry_entities WHERE entry_id = ?", entry.ID); err != nil {
		return fmt.Errorf("delete entities: %w", err)
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM tm_entry_entity_values WHERE entry_id = ?", entry.ID); err != nil {
		return fmt.Errorf("delete entity values: %w", err)
	}
	for _, em := range entry.Entities {
		if em.PlaceholderID == "" {
			continue
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO tm_entry_entities
			(entry_id, placeholder_id, entity_type, concept_id) VALUES (?, ?, ?, ?)`,
			entry.ID, em.PlaceholderID, string(em.Type), em.ConceptID); err != nil {
			return fmt.Errorf("insert entity: %w", err)
		}
		for loc, val := range em.Values {
			if _, err := tx.ExecContext(ctx, `INSERT INTO tm_entry_entity_values
				(entry_id, placeholder_id, locale, text_value, start_pos, end_pos)
				VALUES (?, ?, ?, ?, ?, ?)`,
				entry.ID, em.PlaceholderID, string(loc),
				val.Text, val.Start, val.End); err != nil {
				return fmt.Errorf("insert entity value: %w", err)
			}
		}
	}

	// Replace origins.
	if _, err := tx.ExecContext(ctx, "DELETE FROM tm_entry_origins WHERE entry_id = ?", entry.ID); err != nil {
		return fmt.Errorf("delete origins: %w", err)
	}
	for i, o := range entry.Origins {
		addedAt := o.AddedAt
		if addedAt.IsZero() {
			addedAt = now
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO tm_entry_origins
			(entry_id, ordinal, source, key, reference, added_at, added_by, session_id, context_fp)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			entry.ID, i, o.Source, o.Key, o.Reference,
			addedAt.Format(time.RFC3339), o.AddedBy, o.SessionID, o.ContextFingerprint); err != nil {
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
func (tm *SQLiteStore) Delete(ctx context.Context, id string) error {
	tx, err := tm.db.BeginTx(ctx, nil)
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
		if _, err := tx.ExecContext(ctx, ct.sql, id); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("delete %s: %w", ct.name, err)
		}
	}

	result, err := tx.ExecContext(ctx, "DELETE FROM tm_entries WHERE id = ?", id)
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
func (tm *SQLiteStore) Lookup(ctx context.Context, source *model.Block, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]Match, error) {
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

	return tm.tieredLookup(ctx, plainKey, structKey, generalKey, entityAnnotations, sourceLocale, targetLocale, opts)
}

// LookupSegment searches for matches against a specific segment of the
// source block. See ContentMemory.LookupSegment for the contract.
func (tm *SQLiteStore) LookupSegment(ctx context.Context, source *model.Block, segmentIdx int, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]Match, error) {
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
	return tm.tieredLookup(ctx, plainKey, structKey, generalKey, entityAnnotations, sourceLocale, targetLocale, opts)
}

// LookupText searches for matches using plain text only.
func (tm *SQLiteStore) LookupText(ctx context.Context, source string, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]Match, error) {
	opts = ApplyDefaults(opts)
	opts.MatchModes = []MatchMode{MatchModePlain}
	normalized := NormalizeText(source)
	return tm.tieredLookup(ctx, normalized, normalized, normalized, nil, sourceLocale, targetLocale, opts)
}

func (tm *SQLiteStore) tieredLookup(ctx context.Context, plainKey, structKey, generalKey string, entityAnnotations []*model.EntityAnnotation, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]Match, error) {
	return TieredLookup(ctx, plainKey, structKey, generalKey, entityAnnotations, sourceLocale, targetLocale, opts, CandidateSource{
		Exact:           tm.queryExactVariant,
		FuzzyCandidates: tm.queryFuzzyCandidates,
	})
}

// sortMatches orders matches by score (desc), then match-type priority,
// then entry ID — the last key makes equal candidates deterministic
// instead of inheriting incidental storage order.
func sortMatches(matches []Match) []Match {
	slices.SortFunc(matches, func(a, b Match) int {
		if c := cmp.Compare(b.Score, a.Score); c != 0 {
			return c
		}
		pa := MatchTypePriority(a.MatchType)
		pb := MatchTypePriority(b.MatchType)
		if c := cmp.Compare(pa, pb); c != 0 {
			return c
		}
		return cmp.Compare(a.Entry.ID, b.Entry.ID)
	})
	return matches
}

// queryExactVariant finds entries whose source-locale variant matches the
// given normalized key on the specified column (plain/struct_key/general_key).
func (tm *SQLiteStore) queryExactVariant(ctx context.Context, column, key string, sourceLocale model.LocaleID, opts LookupOptions) ([]Entry, error) {
	where := fmt.Sprintf("v.%s = ? AND v.locale = ?", column)
	args := []any{key, string(sourceLocale)}
	entryWhere := ""
	entryWhere, args = appendSQLiteProjectFilter(entryWhere, args, opts.ProjectID, opts.ProjectScope)

	q := fmt.Sprintf(`
		SELECT DISTINCT e.id
		FROM tm_variants v
		INNER JOIN tm_entries e ON e.id = v.entry_id
		WHERE %s%s
		ORDER BY e.id
		LIMIT 200
	`, where, entryWhere)

	rows, err := tm.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query exact variant: %w", err)
	}
	defer rows.Close()

	ids, err := scanIDs(rows)
	if err != nil {
		return nil, err
	}
	return tm.loadEntriesByIDs(ctx, ids)
}

// queryFuzzyCandidates returns entry candidates for fuzzy matching filtered
// by source locale, using trigram MATCH where available and falling back to
// length-filtered scanning.
func (tm *SQLiteStore) queryFuzzyCandidates(ctx context.Context, plainKey, structKey, generalKey string, sourceLocale model.LocaleID, opts LookupOptions) ([]Entry, error) {
	if tm.fuzzyIndexUsable(ctx) {
		if entries, err := tm.queryTrigramCandidates(ctx, plainKey, structKey, generalKey, sourceLocale, opts); err == nil {
			return entries, nil
		}
	}
	return tm.queryLengthFiltered(ctx, plainKey, sourceLocale, opts)
}

func (tm *SQLiteStore) queryTrigramCandidates(ctx context.Context, plainKey, structKey, generalKey string, sourceLocale model.LocaleID, opts LookupOptions) ([]Entry, error) {
	var ids []string
	for _, key := range []string{plainKey, structKey, generalKey} {
		tq := BuildTrigramQuery(key)
		if tq == "" {
			continue
		}
		keyIDs, err := tm.trigramMatchIDs(ctx, tq, sourceLocale)
		if err != nil {
			return nil, err
		}
		ids = append(ids, keyIDs...)
	}
	ids = uniqueStrings(ids)
	if len(ids) == 0 {
		return nil, nil
	}
	entries, err := tm.loadEntriesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	return tm.filterByProject(entries, opts), nil
}

// trigramMatchIDs runs one FTS5 MATCH against the trigram table, whose entry_id
// is an unindexed column projected back out.
func (tm *SQLiteStore) trigramMatchIDs(ctx context.Context, match string, sourceLocale model.LocaleID) ([]string, error) {
	rows, err := tm.db.QueryContext(ctx, `
		SELECT DISTINCT entry_id FROM tm_variant_trigram
		WHERE tm_variant_trigram MATCH ? AND locale = ?
		LIMIT 200
	`, match, string(sourceLocale))
	if err != nil {
		return nil, fmt.Errorf("trigram query: %w", err)
	}
	defer rows.Close()
	return scanIDs(rows)
}

func (tm *SQLiteStore) queryLengthFiltered(ctx context.Context, plainKey string, sourceLocale model.LocaleID, opts LookupOptions) ([]Entry, error) {
	keyLen := len([]rune(plainKey))
	minLen := int(float64(keyLen) * 0.7)
	maxLen := int(float64(keyLen) * 1.3)
	if minLen < 0 {
		minLen = 0
	}

	rows, err := tm.db.QueryContext(ctx, `
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
	entries, err := tm.loadEntriesByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	return tm.filterByProject(entries, opts), nil
}

func (tm *SQLiteStore) filterByProject(entries []Entry, opts LookupOptions) []Entry {
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
	step := max((len(runes)-windowSize)/4, 1)
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
func (tm *SQLiteStore) loadEntriesByIDs(ctx context.Context, ids []string) ([]Entry, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	placeholders := strings.Repeat("?,", len(ids)-1) + "?"
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}

	rows, err := tm.db.QueryContext(ctx, `
		SELECT id, project_id, hint_src_lang, properties, note, point, unit, created_at, updated_at
		FROM tm_entries WHERE id IN (`+placeholders+`)
	`, args...)
	if err != nil {
		return nil, fmt.Errorf("load entries: %w", err)
	}
	defer rows.Close()
	return tm.scanEntriesWithChildren(ctx, rows)
}

// scanEntriesWithChildren scans entry rows and then batch-loads variants,
// entities, and origins for all of them, stitching children onto the entries.
// Expected column order: id, project_id, hint_src_lang, properties, note, point, unit, created_at, updated_at.
func (tm *SQLiteStore) scanEntriesWithChildren(ctx context.Context, rows interface {
	Next() bool
	Scan(...any) error
	Err() error
}) ([]Entry, error) {
	var entries []Entry
	for rows.Next() {
		var e Entry
		var propsJSON, hint, note, createdStr, updatedStr string
		if err := rows.Scan(&e.ID, &e.ProjectID, &hint, &propsJSON, &note, &e.Point, &e.Unit, &createdStr, &updatedStr); err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		e.HintSrcLang = model.LocaleID(hint)
		e.Note = note
		var perr error
		if e.CreatedAt, perr = parseStoredTime(createdStr); perr != nil {
			return nil, fmt.Errorf("entry %s: parse created_at: %w", e.ID, perr)
		}
		if e.UpdatedAt, perr = parseStoredTime(updatedStr); perr != nil {
			return nil, fmt.Errorf("entry %s: parse updated_at: %w", e.ID, perr)
		}
		if propsJSON != "" {
			if err := json.Unmarshal([]byte(propsJSON), &e.Properties); err != nil {
				return nil, fmt.Errorf("entry %s: unmarshal properties: %w", e.ID, err)
			}
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
	varRows, err := tm.db.QueryContext(ctx, `SELECT entry_id, locale, coded FROM tm_variants
		WHERE entry_id IN (`+placeholders+`) ORDER BY entry_id, locale`, idArgs...)
	if err != nil {
		return nil, fmt.Errorf("load tm variants: %w", err)
	}
	{
		for varRows.Next() {
			var eid, loc, coded string
			if err := varRows.Scan(&eid, &loc, &coded); err != nil {
				varRows.Close()
				return nil, fmt.Errorf("scan variant: %w", err)
			}
			runs := decodeVariantRuns(coded)
			if idx, ok := byID[eid]; ok {
				entries[idx].Variants[model.LocaleID(loc)] = runs
			}
		}
		if err := varRows.Err(); err != nil {
			varRows.Close()
			return nil, fmt.Errorf("iterate variants: %w", err)
		}
		varRows.Close()
	}

	// Entities + per-locale values. Single join query keeps us at O(1) round trips.
	entRows, err := tm.db.QueryContext(ctx, `
		SELECT e.entry_id, e.placeholder_id, e.entity_type, e.concept_id,
			v.locale, v.text_value, v.start_pos, v.end_pos
		FROM tm_entry_entities e
		LEFT JOIN tm_entry_entity_values v
			ON v.entry_id = e.entry_id AND v.placeholder_id = e.placeholder_id
		WHERE e.entry_id IN (`+placeholders+`)
		ORDER BY e.entry_id, e.placeholder_id, v.locale
	`, idArgs...)
	if err != nil {
		return nil, fmt.Errorf("load tm entities: %w", err)
	}
	{
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
				entRows.Close()
				return nil, fmt.Errorf("scan entity mapping: %w", err)
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
		if err := entRows.Err(); err != nil {
			entRows.Close()
			return nil, fmt.Errorf("iterate entity mappings: %w", err)
		}
		entRows.Close()
	}

	// Origins.
	originRows, err := tm.db.QueryContext(ctx, `SELECT entry_id, source, key, reference, added_at, added_by, session_id, context_fp
		FROM tm_entry_origins WHERE entry_id IN (`+placeholders+`)
		ORDER BY entry_id, ordinal`, idArgs...)
	if err != nil {
		return nil, fmt.Errorf("load tm origins: %w", err)
	}
	{
		for originRows.Next() {
			var eid string
			var o Origin
			var addedAtStr string
			if err := originRows.Scan(&eid, &o.Source, &o.Key, &o.Reference, &addedAtStr, &o.AddedBy, &o.SessionID, &o.ContextFingerprint); err != nil {
				originRows.Close()
				return nil, fmt.Errorf("scan origin: %w", err)
			}
			var perr error
			if o.AddedAt, perr = parseStoredTime(addedAtStr); perr != nil {
				originRows.Close()
				return nil, fmt.Errorf("origin for entry %s: parse added_at: %w", eid, perr)
			}
			if idx, ok := byID[eid]; ok {
				entries[idx].Origins = append(entries[idx].Origins, o)
			}
		}
		if err := originRows.Err(); err != nil {
			originRows.Close()
			return nil, fmt.Errorf("iterate origins: %w", err)
		}
		originRows.Close()
	}

	return entries, nil
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

// FullScoreEntries returns every entry whose sourceLocale variant would match
// the given source runs at score 1.0 — the exact tiers' generalized and
// structural hits, plus the plain hits whose own inline-code structure matches
// (a plain hit with a differing structure takes the tag-mismatch penalty and
// never reaches full score).
//
// It is the set the ambiguity rule compares (demoteAmbiguousExacts), which
// Lookup cannot report: once demoted, the members come back at ScoreNearExact
// or are filtered out entirely by a full-score policy, so a caller that has to
// RESOLVE the disagreement rather than avoid it cannot see who is in it. The
// content-record absorber is that caller: it reconciles the entries holding a
// stale target for a source the committed translations answer, so the answer is
// unambiguous again.
//
// Entries are returned in ID order, deduplicated across the tiers.
func (tm *SQLiteStore) FullScoreEntries(ctx context.Context, runs []model.Run, sourceLocale model.LocaleID) ([]Entry, error) {
	if len(runs) == 0 {
		return nil, nil
	}
	sourceLocale = model.NormalizeLocale(sourceLocale)
	plainKey := NormalizeText(model.FlattenRuns(runs))
	structKey := NormalizeText(model.RunsStructuralText(runs))
	generalKey := NormalizeText(model.RunsGeneralizedText(runs))

	var out []Entry
	seen := map[string]bool{}
	for _, tier := range []struct{ column, key string }{
		{"general_key", generalKey},
		{"struct_key", structKey},
		{"plain", plainKey},
	} {
		entries, err := tm.queryExactVariant(ctx, tier.column, tier.key, sourceLocale, ApplyDefaults(LookupOptions{}))
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if seen[e.ID] {
				continue
			}
			if tier.column == "plain" &&
				NormalizeText(model.RunsStructuralText(e.Variant(sourceLocale))) != structKey {
				continue // tag mismatch: below full score, so outside the ambiguity set
			}
			seen[e.ID] = true
			out = append(out, e)
		}
	}
	slices.SortFunc(out, func(a, b Entry) int { return cmp.Compare(a.ID, b.ID) })
	return out, nil
}

// GetEntry fetches a single entry by ID with all its variants populated.
func (tm *SQLiteStore) GetEntry(ctx context.Context, id string) (Entry, bool, error) {
	entries, err := tm.loadEntriesByIDs(ctx, []string{id})
	if err != nil {
		return Entry{}, false, err
	}
	if len(entries) == 0 {
		return Entry{}, false, nil
	}
	return entries[0], true, nil
}

// Entries returns all entries. Used for export operations.
func (tm *SQLiteStore) Entries(ctx context.Context) ([]Entry, error) {
	rows, err := tm.db.QueryContext(ctx, `SELECT id FROM tm_entries ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("list entry ids: %w", err)
	}
	defer rows.Close()
	ids, err := scanIDs(rows)
	if err != nil {
		return nil, err
	}
	return tm.loadEntriesByIDs(ctx, ids)
}

// --- Search ---

// SearchEntries performs a ranked full-text search across variant text.
// See SearchParams for parameter semantics.
func (tm *SQLiteStore) SearchEntries(ctx context.Context, params SearchParams) ([]Entry, int, error) {
	params.Stream = ""
	params.StreamChain = nil
	return tm.searchInternal(ctx, params)
}

// SearchEntriesFiltered performs a search with additional facet filters.
func (tm *SQLiteStore) SearchEntriesFiltered(ctx context.Context, params SearchParams) ([]Entry, int, error) {
	params.Stream = ""
	params.StreamChain = nil
	return tm.searchInternal(ctx, params)
}

// SearchEntriesForStream performs a ranked full-text search with stream
// inheritance. params.StreamChain is the ordered list of ancestor streams
// to search; entries from earlier streams take priority.
func (tm *SQLiteStore) SearchEntriesForStream(ctx context.Context, params SearchParams) ([]Entry, int, error) {
	return tm.searchInternal(ctx, params)
}

// searchInternal runs a multilingual search across tm_variants, optionally
// filtered by stream chain (for Search*ForStream callers). It returns
// entries in a deterministic order (FTS5 BM25 rank when query is set,
// updated_at DESC otherwise), with stream priority applied when provided.
func (tm *SQLiteStore) searchInternal(ctx context.Context, params SearchParams) ([]Entry, int, error) {
	params = NormalizeSearchLocales(params)
	query := params.Query
	anyLocale := params.AnyLocale
	requireLocale := params.RequireLocale
	stream := params.Stream
	streamChain := params.StreamChain
	filter := params.Filter
	offset := params.Offset
	limit := params.Limit
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
	if err := tm.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM tm_entries e WHERE "+where, countArgs...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count search results: %w", err)
	}
	if total == 0 {
		return nil, 0, nil
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
	rows, err := tm.db.QueryContext(ctx, q, pageArgs...)
	if err != nil {
		return nil, total, fmt.Errorf("page search results: %w", err)
	}
	defer rows.Close()
	ids, err := scanIDs(rows)
	if err != nil {
		return nil, total, err
	}
	entries, err := tm.loadEntriesByIDs(ctx, ids)
	if err != nil {
		return nil, total, err
	}
	// Preserve the SQL-ordered ID sequence.
	ordered := orderEntriesByIDs(entries, ids)
	return ordered, total, nil
}

func orderEntriesByIDs(entries []Entry, ids []string) []Entry {
	if len(entries) == 0 {
		return entries
	}
	index := make(map[string]int, len(entries))
	for i, e := range entries {
		index[e.ID] = i
	}
	out := make([]Entry, 0, len(ids))
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

// FacetStats returns aggregated facet data across the full content memory.
func (tm *SQLiteStore) FacetStats(ctx context.Context) (FacetData, error) {
	return tm.FacetStatsFiltered(ctx, SearchParams{})
}

// FacetStatsFiltered returns facet counts scoped to entries matching the
// given search query and filter.
func (tm *SQLiteStore) FacetStatsFiltered(ctx context.Context, params SearchParams) (FacetData, error) {
	params = NormalizeSearchLocales(params)
	subWhere, subArgs := tm.buildFacetSubquery(params.Query, params.AnyLocale, params.RequireLocale, params.Filter)

	data := FacetData{}

	// Run all facet queries concurrently. SQLite WAL mode allows parallel
	// readers, and the connection pool has room for all 5 queries. Errors
	// from the individual fan-out queries are captured and surfaced.
	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error
	recordErr := func(err error) {
		if err == nil {
			return
		}
		errMu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		errMu.Unlock()
	}

	// Locale facets.
	wg.Go(func() {
		var localeQ string
		if subWhere == "1=1" {
			localeQ = `SELECT locale, COUNT(*) FROM tm_variants GROUP BY locale ORDER BY COUNT(*) DESC`
		} else {
			localeQ = `SELECT v.locale, COUNT(DISTINCT v.entry_id) FROM tm_variants v
				INNER JOIN tm_entries e ON e.id = v.entry_id
				WHERE ` + subWhere + `
				GROUP BY v.locale ORDER BY COUNT(DISTINCT v.entry_id) DESC`
		}
		rows, err := tm.db.QueryContext(ctx, localeQ, subArgs...)
		if err != nil {
			recordErr(fmt.Errorf("locale facets: %w", err))
			return
		}
		defer rows.Close()
		var locales []LocaleFacet
		for rows.Next() {
			var lf LocaleFacet
			if err := rows.Scan(&lf.Locale, &lf.Count); err != nil {
				recordErr(fmt.Errorf("scan locale facet: %w", err))
				return
			}
			locales = append(locales, lf)
		}
		if err := rows.Err(); err != nil {
			recordErr(fmt.Errorf("locale facets: %w", err))
			return
		}
		data.Locales = locales
	})

	// Project facets.
	wg.Go(func() {
		projQ := `SELECT e.project_id, COUNT(*) FROM tm_entries e WHERE ` + subWhere + ` GROUP BY e.project_id ORDER BY COUNT(*) DESC`
		rows, err := tm.db.QueryContext(ctx, projQ, subArgs...)
		if err != nil {
			recordErr(fmt.Errorf("project facets: %w", err))
			return
		}
		defer rows.Close()
		var projects []ProjectFacet
		for rows.Next() {
			var pf ProjectFacet
			if err := rows.Scan(&pf.ProjectID, &pf.Count); err != nil {
				recordErr(fmt.Errorf("scan project facet: %w", err))
				return
			}
			projects = append(projects, pf)
		}
		if err := rows.Err(); err != nil {
			recordErr(fmt.Errorf("project facets: %w", err))
			return
		}
		data.Projects = projects
	})

	// Entity type facets.
	wg.Go(func() {
		etQ := `SELECT ent.entity_type, COUNT(DISTINCT ent.entry_id)
			FROM tm_entry_entities ent
			INNER JOIN tm_entries e ON e.id = ent.entry_id
			WHERE ` + subWhere + `
			GROUP BY ent.entity_type ORDER BY COUNT(DISTINCT ent.entry_id) DESC`
		rows, err := tm.db.QueryContext(ctx, etQ, subArgs...)
		if err != nil {
			recordErr(fmt.Errorf("entity type facets: %w", err))
			return
		}
		defer rows.Close()
		var types []EntityTypeFacet
		for rows.Next() {
			var ef EntityTypeFacet
			if err := rows.Scan(&ef.Type, &ef.Count); err != nil {
				recordErr(fmt.Errorf("scan entity type facet: %w", err))
				return
			}
			types = append(types, ef)
		}
		if err := rows.Err(); err != nil {
			recordErr(fmt.Errorf("entity type facets: %w", err))
			return
		}
		data.EntityTypes = types
	})

	// Import session facets.
	wg.Go(func() {
		sessQ := `SELECT s.id, s.file_key, s.tool_name, s.imported_at, COUNT(DISTINCT o.entry_id)
			FROM tm_import_sessions s
			INNER JOIN tm_entry_origins o ON o.session_id = s.id
			INNER JOIN tm_entries e ON e.id = o.entry_id
			WHERE ` + subWhere + `
			GROUP BY s.id ORDER BY COUNT(DISTINCT o.entry_id) DESC`
		rows, err := tm.db.QueryContext(ctx, sessQ, subArgs...)
		if err != nil {
			recordErr(fmt.Errorf("import session facets: %w", err))
			return
		}
		defer rows.Close()
		var sessions []ImportSessionFacet
		for rows.Next() {
			var sf ImportSessionFacet
			var importedAtStr string
			if err := rows.Scan(&sf.SessionID, &sf.FileKey, &sf.ToolName, &importedAtStr, &sf.Count); err != nil {
				recordErr(fmt.Errorf("scan import session facet: %w", err))
				return
			}
			var perr error
			if sf.ImportedAt, perr = parseStoredTime(importedAtStr); perr != nil {
				recordErr(fmt.Errorf("import session facet %s: parse imported_at: %w", sf.SessionID, perr))
				return
			}
			sessions = append(sessions, sf)
		}
		if err := rows.Err(); err != nil {
			recordErr(fmt.Errorf("import session facets: %w", err))
			return
		}
		data.ImportSessions = sessions
	})

	// Inline code facets.
	wg.Go(func() {
		codeQ := `SELECT
			SUM(CASE WHEN e.has_codes = 1 THEN 1 ELSE 0 END),
			SUM(CASE WHEN e.has_codes = 0 THEN 1 ELSE 0 END)
			FROM tm_entries e WHERE ` + subWhere
		if err := tm.db.QueryRowContext(ctx, codeQ, subArgs...).Scan(&data.HasCodes, &data.NoCodes); err != nil {
			recordErr(fmt.Errorf("inline code facets: %w", err))
		}
	})

	wg.Wait()
	if firstErr != nil {
		return FacetData{}, firstErr
	}
	return data, nil
}

// buildFacetSubquery builds a WHERE clause (using alias `e`) that matches the
// same entries SearchEntriesFiltered would return.
func (tm *SQLiteStore) buildFacetSubquery(query, anyLocale, requireLocale string, filter SearchFilter) (string, []any) {
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

// LocaleStats returns per-locale entry counts across the full content memory.
func (tm *SQLiteStore) LocaleStats(ctx context.Context) ([]LocaleFacet, error) {
	rows, err := tm.db.QueryContext(ctx, `
		SELECT locale, COUNT(DISTINCT entry_id) FROM tm_variants
		GROUP BY locale ORDER BY COUNT(DISTINCT entry_id) DESC
	`)
	if err != nil {
		return nil, fmt.Errorf("locale stats: %w", err)
	}
	defer rows.Close()
	var out []LocaleFacet
	for rows.Next() {
		var lf LocaleFacet
		if err := rows.Scan(&lf.Locale, &lf.Count); err != nil {
			return nil, fmt.Errorf("scan locale stat: %w", err)
		}
		out = append(out, lf)
	}
	return out, rows.Err()
}

// ActivityStats returns daily entry counts over time based on created_at.
func (tm *SQLiteStore) ActivityStats(ctx context.Context) ([]ActivityStat, error) {
	rows, err := tm.db.QueryContext(ctx,
		`SELECT DATE(created_at) AS day, COUNT(*) FROM tm_entries GROUP BY day ORDER BY day`,
	)
	if err != nil {
		return nil, fmt.Errorf("activity stats: %w", err)
	}
	defer rows.Close()
	var out []ActivityStat
	for rows.Next() {
		var s ActivityStat
		if err := rows.Scan(&s.Date, &s.Count); err != nil {
			return nil, fmt.Errorf("scan activity stat: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// --- Import sessions ---

// CreateImportSession inserts a new import session row.
func (tm *SQLiteStore) CreateImportSession(ctx context.Context, session ImportSession) error {
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
	_, err := tm.db.ExecContext(ctx, `INSERT INTO tm_import_sessions
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
func (tm *SQLiteStore) GetImportSession(ctx context.Context, id string) (ImportSession, bool, error) {
	return tm.querySingleSession(ctx, "SELECT "+sessionColumns+" FROM tm_import_sessions WHERE id = ?", id)
}

// FindImportSessionByHash returns the most recent session matching the hash.
func (tm *SQLiteStore) FindImportSessionByHash(ctx context.Context, hash string) (ImportSession, bool, error) {
	if hash == "" {
		return ImportSession{}, false, nil
	}
	return tm.querySingleSession(ctx,
		"SELECT "+sessionColumns+" FROM tm_import_sessions WHERE file_hash = ? ORDER BY imported_at DESC LIMIT 1",
		hash)
}

// ListImportSessions returns all sessions ordered by imported_at DESC.
func (tm *SQLiteStore) ListImportSessions(ctx context.Context) ([]ImportSession, error) {
	rows, err := tm.db.QueryContext(ctx, "SELECT "+sessionColumns+" FROM tm_import_sessions ORDER BY imported_at DESC")
	if err != nil {
		return nil, fmt.Errorf("list import sessions: %w", err)
	}
	defer rows.Close()
	var out []ImportSession
	for rows.Next() {
		s, ok, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, s)
		}
	}
	return out, rows.Err()
}

// UpdateImportSessionCount sets the entry_count on a session.
func (tm *SQLiteStore) UpdateImportSessionCount(ctx context.Context, id string, count int) error {
	res, err := tm.db.ExecContext(ctx, `UPDATE tm_import_sessions SET entry_count = ? WHERE id = ?`, count, id)
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
func (tm *SQLiteStore) DeleteImportSession(ctx context.Context, id string) error {
	if _, err := tm.db.ExecContext(ctx, `UPDATE tm_entry_origins SET session_id = '' WHERE session_id = ?`, id); err != nil {
		return fmt.Errorf("clear origin session_id: %w", err)
	}
	res, err := tm.db.ExecContext(ctx, `DELETE FROM tm_import_sessions WHERE id = ?`, id)
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

// scanSession scans a single import-session row. A scan failure (including no
// row) yields ok=false with a nil error — the callers treat that as "not
// found". A non-nil error means the row exists but stored corruption (a
// malformed imported_at timestamp or properties JSON) was surfaced instead of
// silently substituting zero values.
func scanSession(sc sessionScanner) (ImportSession, bool, error) {
	var s ImportSession
	var importedAtStr, propsJSON string
	if err := sc.Scan(&s.ID, &s.FileKey, &s.FileHash, &s.FileSizeBytes,
		&importedAtStr, &s.ImportedBy, &s.ToolName, &s.ToolVersion,
		&s.SegType, &s.AdminLang, &s.SrcLang, &s.DataType,
		&s.OriginalFormat, &s.OriginalEncoding, &s.EntryCount, &propsJSON); err != nil {
		return ImportSession{}, false, nil
	}
	var err error
	if s.ImportedAt, err = parseStoredTime(importedAtStr); err != nil {
		return ImportSession{}, false, fmt.Errorf("session %s: parse imported_at: %w", s.ID, err)
	}
	if propsJSON != "" {
		if err := json.Unmarshal([]byte(propsJSON), &s.Properties); err != nil {
			return ImportSession{}, false, fmt.Errorf("session %s: unmarshal properties: %w", s.ID, err)
		}
	}
	return s, true, nil
}

func (tm *SQLiteStore) querySingleSession(ctx context.Context, q string, args ...any) (ImportSession, bool, error) {
	row := tm.db.QueryRowContext(ctx, q, args...)
	return scanSession(row)
}
