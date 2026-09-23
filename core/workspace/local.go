package workspace

import (
	"context"
	"crypto/sha256"
	"database/sql"
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
}, {
	// Operations carry an id every log agrees on, and a content address for
	// the ones that say something a second recording should not repeat. The
	// log is started afresh rather than migrated: its numbered operations have
	// no id to carry across.
	Version:     2,
	Description: "operation ids and content addresses",
	SQL: `
DROP TABLE IF EXISTS workspace_ops;
CREATE TABLE workspace_ops (
    seq     INTEGER PRIMARY KEY AUTOINCREMENT,
    id      TEXT NOT NULL UNIQUE,
    address TEXT,
    project TEXT NOT NULL DEFAULT '',
    kind    TEXT NOT NULL,
    payload BLOB,
    at      TEXT NOT NULL
);
CREATE UNIQUE INDEX idx_workspace_ops_address ON workspace_ops(address) WHERE address IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_workspace_ops_project ON workspace_ops(project, seq);
CREATE INDEX idx_workspace_ops_kind ON workspace_ops(kind, seq);`,
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

// Forget closes one project's context store and removes its file, together
// with the write-ahead log and shared-memory sidecars beside it. Removing the
// file while a pool is still on it would leave that pool writing into an
// unlinked inode, so the handle goes first.
func (b *LocalBackend) Forget(_ context.Context, key ProjectKey) error {
	if key == "" {
		return ErrNoProjectKey
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.readOnly {
		return ErrReadOnly
	}
	if db, ok := b.projects[key]; ok {
		delete(b.projects, key)
		if err := db.Close(); err != nil {
			return fmt.Errorf("workspace: release the context store of %s: %w", key, err)
		}
	}
	path := b.ProjectPath(key)
	for _, suffix := range []string{"", "-wal", "-shm"} {
		if err := os.Remove(path + suffix); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("workspace: remove %s: %w", path+suffix, err)
		}
	}
	return nil
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

// Record appends operations to the log. An operation that arrives without an
// id is given one; one whose id or content address the log already holds is
// answered with the operation held.
func (b *LocalBackend) Record(ctx context.Context, ops ...Op) ([]Op, error) {
	if len(ops) == 0 {
		return nil, nil
	}
	for _, op := range ops {
		if op.Kind == "" {
			return nil, errors.New("workspace: an operation with no kind")
		}
		if op.ID != "" && !ValidOpID(op.ID) {
			return nil, fmt.Errorf("workspace: %q is not an operation id", op.ID)
		}
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

	// The newest id the log holds, which a minted id must sort after. The
	// transaction is IMMEDIATE (storage.ProjectOptions), so no other writer
	// can slip an id in between this read and the insert.
	var newest string
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(id), '') FROM workspace_ops`).Scan(&newest); err != nil {
		return nil, fmt.Errorf("workspace: read the newest operation id: %w", err)
	}
	for _, op := range ops {
		at := op.At
		if at.IsZero() {
			at = time.Now()
		}
		at = at.UTC()

		held, ok, err := heldOp(ctx, tx, op)
		if err != nil {
			return nil, err
		}
		if ok && !(held.ID != op.ID && op.ID != "" && op.ID < held.ID) {
			out = append(out, held)
			continue
		}
		if ok {
			// Two logs recorded one content address under different ids. The
			// older id stands in every log, so a merge reaches the same
			// operation whichever side it started from.
			if _, err := tx.ExecContext(ctx, `DELETE FROM workspace_ops WHERE id = ?`, held.ID); err != nil {
				return nil, fmt.Errorf("workspace: replace %s: %w", held.ID, err)
			}
		}
		if op.ID == "" {
			op.ID = NewOpID(time.Now(), newest)
		}
		var address any
		if op.Address != "" {
			address = op.Address
		}
		res, err := tx.ExecContext(ctx,
			`INSERT INTO workspace_ops (id, address, project, kind, payload, at) VALUES (?, ?, ?, ?, ?, ?)`,
			op.ID, address, string(op.Project), op.Kind, op.Payload, at.Format(time.RFC3339Nano))
		if err != nil {
			return nil, fmt.Errorf("workspace: record %s: %w", op.Kind, err)
		}
		seq, err := res.LastInsertId()
		if err != nil {
			return nil, fmt.Errorf("workspace: record %s: %w", op.Kind, err)
		}
		op.Seq, op.At = seq, at
		newest = max(newest, op.ID)
		out = append(out, op)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("workspace: record operations: %w", err)
	}
	return out, nil
}

// heldOp looks up the operation a log already holds under an arriving
// operation's id or content address.
func heldOp(ctx context.Context, tx *storage.Tx, op Op) (Op, bool, error) {
	if op.ID == "" && op.Address == "" {
		return Op{}, false, nil
	}
	rows, err := tx.QueryContext(ctx, `SELECT `+opColumns+` FROM workspace_ops
WHERE (? <> '' AND id = ?) OR (? <> '' AND address = ?) LIMIT 1`,
		op.ID, op.ID, op.Address, op.Address)
	if err != nil {
		return Op{}, false, fmt.Errorf("workspace: look up %s: %w", op.Kind, err)
	}
	held, err := scanOps(rows)
	if err != nil || len(held) == 0 {
		return Op{}, false, err
	}
	return held[0], true, nil
}

// opColumns is what a read of the log selects, in the order scanOps reads.
const opColumns = `seq, id, COALESCE(address, ''), project, kind, payload, at`

// scanOps reads rows selected with opColumns and closes them.
func scanOps(rows *sql.Rows) ([]Op, error) {
	defer func() { _ = rows.Close() }()
	var out []Op
	for rows.Next() {
		var (
			op      Op
			project string
			at      string
		)
		if err := rows.Scan(&op.Seq, &op.ID, &op.Address, &project, &op.Kind, &op.Payload, &at); err != nil {
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

// Since returns the operations this log received after a local position, in
// the order it received them.
func (b *LocalBackend) Since(ctx context.Context, after int64, limit int) ([]Op, error) {
	return b.Select(ctx, OpQuery{After: after, Limit: limit})
}

// Select returns the operations a query names, in the order this log received
// them.
func (b *LocalBackend) Select(ctx context.Context, q OpQuery) ([]Op, error) {
	db, err := b.Registry(ctx)
	if err != nil {
		return nil, err
	}
	query := `SELECT ` + opColumns + ` FROM workspace_ops WHERE seq > ?`
	args := []any{q.After}
	if q.KindPrefix != "" {
		// A range over the kind index rather than LIKE, which SQLite answers
		// with a scan unless the column is declared case-insensitive.
		query += ` AND kind >= ? AND kind < ?`
		args = append(args, q.KindPrefix, prefixEnd(q.KindPrefix))
	}
	if q.Project != "" {
		query += ` AND project = ?`
		args = append(args, string(q.Project))
	}
	query += ` ORDER BY seq`
	if q.Limit > 0 {
		query += ` LIMIT ?`
		args = append(args, q.Limit)
	}
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("workspace: read operations: %w", err)
	}
	return scanOps(rows)
}

// prefixEnd is the smallest string that sorts after every string starting
// with prefix, for a range scan. A prefix of bytes that cannot be incremented
// has no end, which the caller never meets: kinds are ASCII.
func prefixEnd(prefix string) string {
	b := []byte(prefix)
	for i := len(b) - 1; i >= 0; i-- {
		if b[i] < 0xff {
			b[i]++
			return string(b[:i+1])
		}
	}
	return "\xff"
}

// Head returns the local position of the last operation this log received.
func (b *LocalBackend) Head(ctx context.Context) (int64, error) {
	db, err := b.Registry(ctx)
	if err != nil {
		return 0, err
	}
	var head int64
	if err := db.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(seq), 0) FROM workspace_ops`).Scan(&head); err != nil {
		return 0, fmt.Errorf("workspace: read the operation log head: %w", err)
	}
	return head, nil
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
