package workspace

import (
	"context"
	"crypto/sha256"
	"encoding/base32"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/neokapi/neokapi/core/storage"
)

// RegistryFileName is the workspace-wide database inside a local workspace
// directory: the project registry, the context graph and the operation log.
const RegistryFileName = "workspace.db"

// ProjectsDirName holds one context store per project, inside a local
// workspace directory.
const ProjectsDirName = "projects"

// opsMigrationsTable is the local backend's own migration ledger. Registry
// callers keep their own, which is what lets the graph and the registry share
// `workspace.db` without replaying each other's migrations.
const opsMigrationsTable = "workspace_ops_migrations"

var opsMigrations = []storage.Migration{{
	Version:     1,
	Description: "workspace operation log",
	SQL: `
CREATE TABLE IF NOT EXISTS workspace_ops (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    project TEXT NOT NULL DEFAULT '',
    kind    TEXT NOT NULL,
    payload BLOB,
    at      TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_workspace_ops_project ON workspace_ops(project, seq);`,
}}

// LocalBackend keeps a workspace as a directory of SQLite files on this
// machine.
//
// Every handle it opens is memoized, so a process that touches one project
// twice holds one pool for it. The pools carry the project store's write
// discipline (storage.ProjectOptions): IMMEDIATE transactions, so a second kapi
// process queues on the file lock rather than being refused, and an in-process
// FIFO permit, so the writers this process controls take their turn in arrival
// order. The permit is per file, which is what makes a project's writers
// independent of every other project's.
type LocalBackend struct {
	root string

	mu       sync.Mutex
	readOnly bool
	registry *storage.DB
	projects map[ProjectKey]*storage.DB
	closed   bool
}

// Local names a workspace kept in a directory on this machine. The directory is
// created on first use; nothing touches the filesystem here.
func Local(root string) *LocalBackend {
	return &LocalBackend{root: root, projects: map[ProjectKey]*storage.DB{}}
}

// Describe reports where this workspace is and whether it can be written. The
// read-only answer is only known once something has been opened, so a
// descriptor read before the first open reports a writable workspace.
func (b *LocalBackend) Describe() Descriptor {
	b.mu.Lock()
	defer b.mu.Unlock()
	return Descriptor{Kind: KindLocal, Location: b.root, ReadOnly: b.readOnly}
}

// Root returns the workspace directory.
func (b *LocalBackend) Root() string { return b.root }

// RegistryPath returns the workspace-wide database file.
func (b *LocalBackend) RegistryPath() string { return filepath.Join(b.root, RegistryFileName) }

// ProjectPath returns one project's context store file.
func (b *LocalBackend) ProjectPath(key ProjectKey) string {
	return filepath.Join(b.root, ProjectsDirName, FileNameFor(key)+".db")
}

// Registry opens the workspace-wide database, creating it on first use.
func (b *LocalBackend) Registry(ctx context.Context) (*storage.DB, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, errors.New("workspace: backend is closed")
	}
	if b.registry != nil {
		return b.registry, nil
	}
	db, err := b.open(ctx, b.root, b.RegistryPath())
	if err != nil {
		return nil, err
	}
	if err := storage.Migrate(db, opsMigrationsTable, opsMigrations); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("workspace: migrate operation log: %w", err)
	}
	b.registry = db
	return db, nil
}

// Project opens one project's context store, creating it on first use.
func (b *LocalBackend) Project(ctx context.Context, key ProjectKey) (*storage.DB, error) {
	if key == "" {
		return nil, ErrNoProjectKey
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, errors.New("workspace: backend is closed")
	}
	if db, ok := b.projects[key]; ok {
		return db, nil
	}
	db, err := b.open(ctx, filepath.Join(b.root, ProjectsDirName), b.ProjectPath(key))
	if err != nil {
		return nil, err
	}
	b.projects[key] = db
	return db, nil
}

// open creates dir and opens path inside it, falling back to a read-only handle
// where the directory refuses to be written.
//
// The caller holds b.mu.
func (b *LocalBackend) open(_ context.Context, dir, path string) (*storage.DB, error) {
	if segment, product, found := CloudSyncedDir(b.root); found {
		return nil, errCloudSynced(b.root, segment, product)
	}
	mkErr := os.MkdirAll(dir, 0o755)
	if mkErr == nil {
		db, err := storage.OpenWith(path, storage.ProjectOptions())
		if err == nil {
			return db, nil
		}
		if !storage.IsPermissionErr(err) {
			return nil, fmt.Errorf("workspace: open %s: %w", path, err)
		}
	} else if !storage.IsPermissionErr(mkErr) {
		return nil, fmt.Errorf("workspace: create %s: %w", dir, mkErr)
	}

	// The directory will not take a write. A workspace that is not there at all
	// has nothing to read, and saying so is more useful than reporting a
	// missing database file.
	if _, statErr := os.Stat(path); statErr != nil {
		return nil, fmt.Errorf(
			"workspace: %s cannot be created and %s is not there to read: %w", dir, path, statErr)
	}
	db, err := storage.OpenReadOnly(path)
	if err != nil {
		return nil, fmt.Errorf("workspace: open %s for reading: %w", path, err)
	}
	b.readOnly = true
	return db, nil
}

// Record appends operations to the log, assigning each a sequence number.
func (b *LocalBackend) Record(ctx context.Context, ops ...Op) ([]Op, error) {
	if len(ops) == 0 {
		return nil, nil
	}
	db, err := b.Registry(ctx)
	if err != nil {
		return nil, err
	}
	if b.Describe().ReadOnly {
		return nil, ErrReadOnly
	}
	out := make([]Op, 0, len(ops))
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("workspace: record operations: %w", err)
	}
	defer func() { _ = tx.Rollback() }()
	for _, op := range ops {
		if op.Kind == "" {
			return nil, errors.New("workspace: an operation with no kind")
		}
		at := op.At
		if at.IsZero() {
			at = time.Now()
		}
		at = at.UTC()
		res, err := tx.ExecContext(ctx,
			`INSERT INTO workspace_ops (project, kind, payload, at) VALUES (?, ?, ?, ?)`,
			string(op.Project), op.Kind, op.Payload, at.Format(time.RFC3339Nano))
		if err != nil {
			return nil, fmt.Errorf("workspace: record %s: %w", op.Kind, err)
		}
		seq, err := res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("workspace: record %s: %w", op.Kind, err)
		}
		op.Seq, op.At = seq, at
		out = append(out, op)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("workspace: record operations: %w", err)
	}
	return out, nil
}

// Since returns the operations after a sequence number, oldest first.
func (b *LocalBackend) Since(ctx context.Context, after int64, limit int) ([]Op, error) {
	db, err := b.Registry(ctx)
	if err != nil {
		return nil, err
	}
	query := `SELECT seq, project, kind, payload, at FROM workspace_ops WHERE seq > ? ORDER BY seq`
	args := []any{after}
	if limit > 0 {
		query += ` LIMIT ?`
		args = append(args, limit)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("workspace: read operations: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []Op
	for rows.Next() {
		var (
			op      Op
			project string
			at      string
		)
		if err := rows.Scan(&op.Seq, &project, &op.Kind, &op.Payload, &at); err != nil {
			return nil, fmt.Errorf("workspace: read operations: %w", err)
		}
		op.Project = ProjectKey(project)
		if parsed, perr := time.Parse(time.RFC3339Nano, at); perr == nil {
			op.At = parsed.UTC()
		}
		out = append(out, op)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("workspace: read operations: %w", err)
	}
	return out, nil
}

// Close releases every pool this backend opened.
func (b *LocalBackend) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.closed = true
	var first error
	for key, db := range b.projects {
		if err := db.Close(); err != nil && first == nil {
			first = err
		}
		delete(b.projects, key)
	}
	if b.registry != nil {
		if err := b.registry.Close(); err != nil && first == nil {
			first = err
		}
		b.registry = nil
	}
	return first
}

// FileNameFor renders a project key as a filename that is stable, unique and
// legible.
//
// A key is a project id (`prj_` and lowercase base32, which is already a safe
// filename) or a recipe's `name:`, which is a person's text and may hold
// anything. A key that is safe as it stands is used verbatim, so a directory
// listing reads as the projects it holds. Anything else is rendered as its
// sanitized prefix plus a digest of the whole key, which keeps two keys that
// sanitize alike apart.
func FileNameFor(key ProjectKey) string {
	s := string(key)
	if s == "" {
		return "unidentified"
	}
	if safeFileName(s) {
		return s
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
		if b.Len() >= 32 {
			break
		}
	}
	sum := sha256.Sum256([]byte(s))
	return strings.Trim(b.String(), "-") + "-" + hex.EncodeToString(sum[:8])
}

// safeFileName reports whether a key can be a filename as it stands: short,
// and built only of characters every filesystem this runs on treats as
// ordinary.
func safeFileName(s string) bool {
	if len(s) == 0 || len(s) > 64 || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9', r == '-', r == '_':
		default:
			return false
		}
	}
	return true
}

// keyEncoding renders a path digest in the same alphabet a project id uses, so
// a key derived from a checkout reads like one that was minted.
var keyEncoding = base32.StdEncoding.WithPadding(base32.NoPadding)

// KeyForCheckout derives a project key from the place a project was found, for
// a recipe that states neither an `id:` nor a `name:`.
//
// Such a project has no identity to share, so its context store belongs to the
// checkout it was found in, and a second checkout of the same tree gets a
// second store. Giving the recipe an `id:` is what joins them.
func KeyForCheckout(normalizedRoot string) ProjectKey {
	sum := sha256.Sum256([]byte(filepath.ToSlash(normalizedRoot)))
	return ProjectKey("at_" + strings.ToLower(keyEncoding.EncodeToString(sum[:10])))
}
