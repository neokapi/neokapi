package platformconfig

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/bowrain/storage"
)

// Migrations is the platform_config schema as a single consolidated baseline.
//
// LEDGER — every version this subsystem has ever issued, now folded in:
//
//	1  platform_config singleton key/value settings
//
// The baseline is version 2, above every number issued, so a database that
// already recorded 1 applies it once more. That is deliberate: it is what
// repairs a database whose bookkeeping was emptied while its schema was left
// standing, which is the state production was in on 2026-08-01. The SQL is
// idempotent, so the repair costs nothing on a database that is already right.
//
// Retired numbers are never reused. The next migration is version 3.
var Migrations = []storage.Migration{
	{
		Version:     2,
		Description: "platform_config baseline (folds 1)",
		SQL: `CREATE TABLE IF NOT EXISTS platform_config (
			key        TEXT PRIMARY KEY,
			value      JSONB NOT NULL,
			updated_by TEXT NOT NULL DEFAULT '',
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)`,
	},
}

// Entry is one stored setting with its provenance.
type Entry struct {
	Key       string          `json:"key"`
	Value     json.RawMessage `json:"value"`
	UpdatedBy string          `json:"updated_by"`
	UpdatedAt time.Time       `json:"updated_at"`
}

// Store is the Postgres-backed instance-wide settings table. It is intentionally
// tiny: a handful of rows read into memory by the Service.
type Store struct {
	db *storage.PgDB
}

// NewStore opens (migrating) the platform_config store.
func NewStore(db *storage.PgDB) (*Store, error) {
	if err := storage.MigratePostgresNS(db, "platform_config_schema_migrations", Migrations); err != nil {
		return nil, fmt.Errorf("migrate platform_config schema: %w", err)
	}
	return &Store{db: db}, nil
}

// GetAll returns every stored key. The whole table is small enough to load at
// once, which is exactly how the Service caches it.
func (s *Store) GetAll(ctx context.Context) (map[string]json.RawMessage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT key, value FROM platform_config`)
	if err != nil {
		return nil, fmt.Errorf("platform_config: query: %w", err)
	}
	defer rows.Close()

	out := make(map[string]json.RawMessage)
	for rows.Next() {
		var key string
		var value []byte
		if err := rows.Scan(&key, &value); err != nil {
			return nil, fmt.Errorf("platform_config: scan: %w", err)
		}
		out[key] = json.RawMessage(value)
	}
	return out, rows.Err()
}

// Set upserts one key. value must be valid JSON.
func (s *Store) Set(ctx context.Context, key string, value json.RawMessage, updatedBy string) error {
	return s.setWith(ctx, s.db, key, value, updatedBy)
}

// SetMany upserts several keys atomically, so a multi-field admin save never
// half-applies.
func (s *Store) SetMany(ctx context.Context, values map[string]json.RawMessage, updatedBy string) error {
	if len(values) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("platform_config: begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for key, value := range values {
		if err := s.setWith(ctx, tx, key, value, updatedBy); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("platform_config: commit: %w", err)
	}
	return nil
}

// execer is satisfied by both *storage.PgDB (via its embedded *sql.DB) and *sql.Tx.
type execer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

func (s *Store) setWith(ctx context.Context, e execer, key string, value json.RawMessage, updatedBy string) error {
	if !json.Valid(value) {
		return fmt.Errorf("platform_config: value for %q is not valid JSON", key)
	}
	_, err := e.ExecContext(ctx, `
		INSERT INTO platform_config (key, value, updated_by, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (key) DO UPDATE
		SET value = EXCLUDED.value, updated_by = EXCLUDED.updated_by, updated_at = NOW()`,
		key, []byte(value), updatedBy)
	if err != nil {
		return fmt.Errorf("platform_config: set %q: %w", key, err)
	}
	return nil
}

// Delete removes a key, resetting it to its bootstrap default.
func (s *Store) Delete(ctx context.Context, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM platform_config WHERE key = $1`, key)
	if err != nil {
		return fmt.Errorf("platform_config: delete %q: %w", key, err)
	}
	return nil
}
