package workspace

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/neokapi/neokapi/core/storage"
)

// importsMigrationsTable is the import-stamp table's own migration ledger
// inside `workspace.db`. The project registry, the widened rules, the agent
// sessions, the operation log and the context graph each keep one, which is
// what lets them share the file without replaying each other's migrations.
const importsMigrationsTable = "workspace_imports_migrations"

var importsMigrations = []storage.Migration{{
	Version:     1,
	Description: "context files a checkout has read into the workspace",
	SQL: `
CREATE TABLE IF NOT EXISTS workspace_context_imports (
    checkout TEXT NOT NULL,
    path     TEXT NOT NULL,
    digest   TEXT NOT NULL,
    read_at  TEXT NOT NULL,
    PRIMARY KEY (checkout, path)
);`,
}}

// ContextImportStamp is one context file a checkout has read into the
// workspace, and the bytes it held when it was read.
//
// The rows an import writes are shared: a project's terms, voice profiles,
// content memory and recorded decisions live in the workspace, and every
// checkout of that project reads the same ones. The stamp lives beside them for
// that reason. Keeping it in the checkout instead would let one clone's import
// decide that another clone's file has already been read, which is how a second
// branch's wording went missing (#2918).
//
// The checkout is part of the key, so two clones of one project each read their
// own files once and neither skips the other's.
type ContextImportStamp struct {
	// Checkout is the absolute path of the checkout root, normalized by
	// whatever wrote the stamp.
	Checkout string
	// Path names the file inside that checkout, project-relative and
	// slash-separated.
	Path string
	// Digest is the SHA-256 of the bytes that were read, as lower-case hex.
	Digest string
	// At is when the file was read, in UTC. A zero value takes the moment of
	// the write.
	At time.Time
}

// ErrNoImportStamp reports an import stamp that names no checkout or no file.
var ErrNoImportStamp = errors.New("workspace: an import stamp needs a checkout and a path")

// imports brings the import-stamp schema up to date on first use. The registry
// database is shared, so the migration runs from here rather than from Open,
// and a read-only workspace skips it the way Open skips the registry's.
func (w *Workspace) imports() (*storage.DB, error) {
	if w.backend.Describe().ReadOnly {
		return w.registry, nil
	}
	w.importsOnce.Do(func() {
		w.importsErr = storage.Migrate(w.registry, importsMigrationsTable, importsMigrations)
	})
	if w.importsErr != nil {
		return nil, fmt.Errorf("workspace: migrate import stamps: %w", w.importsErr)
	}
	return w.registry, nil
}

// NoteContextImports records what a checkout read, replacing whatever an
// earlier read of the same file in the same checkout recorded.
func (w *Workspace) NoteContextImports(ctx context.Context, stamps ...ContextImportStamp) error {
	if len(stamps) == 0 {
		return nil
	}
	for _, s := range stamps {
		if s.Checkout == "" || s.Path == "" {
			return ErrNoImportStamp
		}
	}
	if w.backend.Describe().ReadOnly {
		return ErrReadOnly
	}
	db, err := w.imports()
	if err != nil {
		return err
	}
	for _, s := range stamps {
		at := s.At
		if at.IsZero() {
			at = time.Now()
		}
		if _, err := db.ExecContext(ctx, `
INSERT INTO workspace_context_imports (checkout, path, digest, read_at) VALUES (?, ?, ?, ?)
ON CONFLICT(checkout, path) DO UPDATE SET
    digest = excluded.digest,
    read_at = excluded.read_at`,
			s.Checkout, s.Path, s.Digest, at.UTC().Format(time.RFC3339Nano)); err != nil {
			return fmt.Errorf("workspace: note import of %s: %w", s.Path, err)
		}
	}
	return nil
}

// ContextImports reports what one checkout has read, in path order. A checkout
// that has read nothing reports nothing.
func (w *Workspace) ContextImports(ctx context.Context, checkout string) ([]ContextImportStamp, error) {
	if checkout == "" {
		return nil, ErrNoImportStamp
	}
	db, err := w.imports()
	if err != nil {
		return nil, err
	}
	if w.backend.Describe().ReadOnly && !importTableExists(ctx, db) {
		// A workspace opened for reading alone never ran the migration, so
		// there may be no table to read. Nothing read is the honest answer.
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, `
SELECT checkout, path, digest, read_at FROM workspace_context_imports
WHERE checkout = ? ORDER BY path`, checkout)
	if err != nil {
		return nil, fmt.Errorf("workspace: list import stamps: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []ContextImportStamp
	for rows.Next() {
		var (
			s    ContextImportStamp
			read string
		)
		if err := rows.Scan(&s.Checkout, &s.Path, &s.Digest, &read); err != nil {
			return nil, fmt.Errorf("workspace: list import stamps: %w", err)
		}
		s.At = parseSessionTime(read)
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workspace: list import stamps: %w", err)
	}
	return out, nil
}

// importTableExists reports whether the import-stamp table has been created.
// Only a read-only workspace asks: every writable one runs the migration first.
func importTableExists(ctx context.Context, db *storage.DB) bool {
	var name string
	err := db.QueryRowContext(ctx,
		`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'workspace_context_imports'`).Scan(&name)
	return err == nil && name != ""
}
