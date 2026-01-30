package pensieve

import (
	"database/sql"
	"fmt"
	"sort"
	"time"

	"github.com/gokapi/gokapi/core/model"

	// Pure Go SQLite driver.
	_ "modernc.org/sqlite"
)

// SQLiteTM is a persistent translation memory backed by SQLite.
type SQLiteTM struct {
	db *sql.DB
}

// NewSQLiteTM opens (or creates) a SQLite-backed translation memory at the given path.
// Use ":memory:" for an in-memory database (useful for testing).
func NewSQLiteTM(dbPath string) (*SQLiteTM, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := createSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &SQLiteTM{db: db}, nil
}

func createSchema(db *sql.DB) error {
	schema := `
	CREATE TABLE IF NOT EXISTS tm_entries (
		id            TEXT PRIMARY KEY,
		source        TEXT NOT NULL,
		target        TEXT NOT NULL,
		source_locale TEXT NOT NULL,
		target_locale TEXT NOT NULL,
		created_at    TEXT NOT NULL,
		updated_at    TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS tm_properties (
		entry_id TEXT NOT NULL,
		key      TEXT NOT NULL,
		value    TEXT NOT NULL,
		PRIMARY KEY (entry_id, key),
		FOREIGN KEY (entry_id) REFERENCES tm_entries(id) ON DELETE CASCADE
	);
	CREATE INDEX IF NOT EXISTS idx_tm_entries_locales ON tm_entries(source_locale, target_locale);
	`
	_, err := db.Exec(schema)
	return err
}

// Add inserts or updates a translation memory entry.
func (tm *SQLiteTM) Add(entry TMEntry) error {
	if entry.ID == "" {
		return fmt.Errorf("entry ID is required")
	}

	now := time.Now()
	if entry.CreatedAt.IsZero() {
		entry.CreatedAt = now
	}
	if entry.UpdatedAt.IsZero() {
		entry.UpdatedAt = now
	}

	tx, err := tm.db.Begin()
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO tm_entries (id, source, target, source_locale, target_locale, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			source = excluded.source,
			target = excluded.target,
			source_locale = excluded.source_locale,
			target_locale = excluded.target_locale,
			updated_at = excluded.updated_at
	`, entry.ID, entry.Source, entry.Target,
		string(entry.SourceLocale), string(entry.TargetLocale),
		entry.CreatedAt.Format(time.RFC3339), entry.UpdatedAt.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert entry: %w", err)
	}

	// Replace properties.
	_, err = tx.Exec("DELETE FROM tm_properties WHERE entry_id = ?", entry.ID)
	if err != nil {
		return fmt.Errorf("delete properties: %w", err)
	}
	for k, v := range entry.Properties {
		_, err = tx.Exec("INSERT INTO tm_properties (entry_id, key, value) VALUES (?, ?, ?)",
			entry.ID, k, v)
		if err != nil {
			return fmt.Errorf("insert property: %w", err)
		}
	}

	return tx.Commit()
}

// Lookup searches for matches of the given source text.
func (tm *SQLiteTM) Lookup(source string, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error) {
	if opts.MinScore <= 0 {
		opts.MinScore = 0.7
	}
	if opts.MaxResults <= 0 {
		opts.MaxResults = 10
	}

	normalizedSource := normalizeText(source)

	rows, err := tm.db.Query(`
		SELECT id, source, target, source_locale, target_locale, created_at, updated_at
		FROM tm_entries
		WHERE source_locale = ? AND target_locale = ?
	`, string(sourceLocale), string(targetLocale))
	if err != nil {
		return nil, fmt.Errorf("query entries: %w", err)
	}
	defer rows.Close()

	var matches []TMMatch
	for rows.Next() {
		var entry TMEntry
		var srcLocale, tgtLocale string
		var createdStr, updatedStr string
		if err := rows.Scan(&entry.ID, &entry.Source, &entry.Target,
			&srcLocale, &tgtLocale, &createdStr, &updatedStr); err != nil {
			return nil, fmt.Errorf("scan entry: %w", err)
		}
		entry.SourceLocale = model.LocaleID(srcLocale)
		entry.TargetLocale = model.LocaleID(tgtLocale)
		entry.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		entry.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

		normalizedEntry := normalizeText(entry.Source)
		var score float64
		var matchType MatchType

		if normalizedEntry == normalizedSource {
			score = 1.0
			matchType = MatchExact
		} else {
			score = LevenshteinRatio(normalizedSource, normalizedEntry)
			matchType = MatchFuzzy
		}

		if score >= opts.MinScore {
			matches = append(matches, TMMatch{
				Entry:     entry,
				Score:     score,
				MatchType: matchType,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate rows: %w", err)
	}

	sort.Slice(matches, func(i, j int) bool {
		return matches[i].Score > matches[j].Score
	})

	if len(matches) > opts.MaxResults {
		matches = matches[:opts.MaxResults]
	}

	return matches, nil
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
	tm.db.QueryRow("SELECT COUNT(*) FROM tm_entries").Scan(&count)
	return count
}

// Close closes the database connection.
func (tm *SQLiteTM) Close() error {
	return tm.db.Close()
}

// Entries returns all entries. Used for export operations.
func (tm *SQLiteTM) Entries() []TMEntry {
	rows, err := tm.db.Query(`
		SELECT id, source, target, source_locale, target_locale, created_at, updated_at
		FROM tm_entries ORDER BY id
	`)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var entries []TMEntry
	for rows.Next() {
		var entry TMEntry
		var srcLocale, tgtLocale string
		var createdStr, updatedStr string
		if err := rows.Scan(&entry.ID, &entry.Source, &entry.Target,
			&srcLocale, &tgtLocale, &createdStr, &updatedStr); err != nil {
			continue
		}
		entry.SourceLocale = model.LocaleID(srcLocale)
		entry.TargetLocale = model.LocaleID(tgtLocale)
		entry.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
		entry.UpdatedAt, _ = time.Parse(time.RFC3339, updatedStr)

		// Load properties.
		propRows, err := tm.db.Query("SELECT key, value FROM tm_properties WHERE entry_id = ?", entry.ID)
		if err == nil {
			entry.Properties = make(map[string]string)
			for propRows.Next() {
				var k, v string
				if propRows.Scan(&k, &v) == nil {
					entry.Properties[k] = v
				}
			}
			propRows.Close()
		}
		entries = append(entries, entry)
	}
	return entries
}
