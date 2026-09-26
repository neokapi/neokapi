package workspace

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/neokapi/neokapi/core/storage"
)

// Sync moves one project's operations between this workspace and a remote.
//
// Pull reads the segments of the remote this workspace has not seen, merges
// their operations into the log by id and hands them to the Applier; push
// writes the project's operations the remote does not hold as one new segment
// under this workspace's writer id. What the workspace knows about a remote
// (the ids the remote holds, the segments it has read) is kept in the registry
// database, keyed by the project and the remote, so switching a project to
// another remote starts that remote's knowledge afresh.
//
// Only one project's operations travel: a remote belongs to one project. The
// kinds named in SyncOptions.LocalKinds stay on the machine that recorded
// them: the project's registration, its checkpoints, and rules a person
// widened to the whole workspace.

// syncMigrationsTable is the sync state's own migration ledger inside
// `workspace.db`.
const syncMigrationsTable = "workspace_sync_migrations"

var syncMigrations = []storage.Migration{{
	Version:     1,
	Description: "context sync state",
	SQL: `
CREATE TABLE IF NOT EXISTS workspace_sync_meta (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS workspace_sync_known (
    remote TEXT NOT NULL,
    id     TEXT NOT NULL,
    PRIMARY KEY (remote, id)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS workspace_sync_segments (
    remote  TEXT NOT NULL,
    name    TEXT NOT NULL,
    pending INTEGER NOT NULL,
    applied INTEGER NOT NULL,
    blob    TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (remote, name)
) WITHOUT ROWID;
CREATE TABLE IF NOT EXISTS workspace_sync_contact (
    remote TEXT PRIMARY KEY,
    at     TEXT NOT NULL,
    error  TEXT NOT NULL DEFAULT ''
);`,
}}

// SegmentOps bounds the operations one segment carries, so a push after a
// long time offline writes several moderate files rather than one large one.
const SegmentOps = 1000

// CheckpointEvery is how many operations a remote may gain after its newest
// checkpoint before a push adds another, for a fast first pull.
const CheckpointEvery = 5000

// Applier brings a project's stores up to date with its log, and reads and
// writes the checkpoints a remote carries. core/projector implements it.
type Applier interface {
	// Apply applies the operations merged into the log. rebuild asks for the
	// stores to be written again from the log in id order, which is needed
	// when a merged operation sorts before one already applied.
	Apply(ctx context.Context, rebuild bool) error
	// Checkpoint writes the project's projections as a checkpoint file that
	// includes the operations of the segments named, and reports the last
	// operation it includes.
	Checkpoint(ctx context.Context, segments []string) (data []byte, through string, err error)
	// CheckpointMark reads which operation a checkpoint file stands at and the
	// segments it includes.
	CheckpointMark(data []byte) (through string, segments []string, err error)
	// InstallCheckpoint records a checkpoint file in the log, as of the
	// operations the log holds now, so the next rebuild starts from it.
	InstallCheckpoint(ctx context.Context, data []byte) error
}

// SyncOptions configures a Sync.
type SyncOptions struct {
	// LocalKinds are the operation kinds that never leave this machine.
	LocalKinds []string
	// CheckpointEvery overrides CheckpointEvery; zero keeps it, a negative
	// value never writes one.
	CheckpointEvery int
}

// Sync is one project's link to one remote.
type Sync struct {
	w       *Workspace
	remote  Remote
	key     ProjectKey
	applier Applier
	opts    SyncOptions
	// rid keys the sync state: the project and the remote.
	rid string
}

// SyncStatus says how far this workspace and the remote are apart.
type SyncStatus struct {
	// Remote describes the remote.
	Remote RemoteDescriptor `json:"remote"`
	// ToPush counts the project's operations the remote is not known to hold.
	ToPush int `json:"to_push"`
	// ToPull counts the operations in segments read from the remote and not
	// yet merged.
	ToPull int `json:"to_pull"`
	// Contacted is when the remote was last reached; zero when never.
	Contacted time.Time `json:"contacted,omitzero"`
	// Error is what the last attempt to reach it reported, empty when it
	// succeeded.
	Error string `json:"error,omitempty"`
}

// PullReport says what a pull merged.
type PullReport struct {
	SyncStatus
	// Segments counts the segments read.
	Segments int `json:"segments"`
	// Merged counts the operations the log did not hold before.
	Merged int `json:"merged"`
	// Rebuilt reports that the stores were written again from the log.
	Rebuilt bool `json:"rebuilt,omitempty"`
	// Checkpoint is the operation a first pull started from, when the remote
	// held a checkpoint it could use.
	Checkpoint string `json:"checkpoint,omitempty"`
	// Empty reports that the remote lists no segments at all: nothing has
	// been pushed to it yet.
	Empty bool `json:"empty,omitempty"`
}

// PushReport says what a push wrote.
type PushReport struct {
	SyncStatus
	// Pushed counts the operations written.
	Pushed int `json:"pushed"`
	// Segments counts the segment files written.
	Segments int `json:"segments"`
	// Blobs counts the blobs written.
	Blobs int `json:"blobs"`
	// Checkpoint is the operation of the checkpoint written, when one was.
	Checkpoint string `json:"checkpoint,omitempty"`

	written []string
}

// NewSync links a project to a remote.
func (w *Workspace) NewSync(remote Remote, key ProjectKey, applier Applier, opts SyncOptions) *Sync {
	return &Sync{
		w: w, remote: remote, key: key, applier: applier, opts: opts,
		rid: string(key) + " " + remote.Describe().ID(),
	}
}

// sync brings the sync state's schema up to date on first use.
func (w *Workspace) sync() (*storage.DB, error) {
	if w.backend.Describe().ReadOnly {
		return w.registry, nil
	}
	w.syncOnce.Do(func() {
		w.syncErr = storage.Migrate(w.registry, syncMigrationsTable, syncMigrations)
	})
	if w.syncErr != nil {
		return nil, fmt.Errorf("workspace: migrate sync state: %w", w.syncErr)
	}
	return w.registry, nil
}

// WriterID returns the id this workspace writes its segments under, minting
// one the first time it is asked for.
func (w *Workspace) WriterID(ctx context.Context) (string, error) {
	db, err := w.sync()
	if err != nil {
		return "", err
	}
	var id string
	err = db.QueryRowContext(ctx, `SELECT value FROM workspace_sync_meta WHERE key = 'writer'`).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("workspace: read the writer id: %w", err)
	}
	var raw [10]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	id = encodeCrockford(raw[:])
	if _, err := db.ExecContext(ctx,
		`INSERT INTO workspace_sync_meta (key, value) VALUES ('writer', ?) ON CONFLICT(key) DO NOTHING`, id); err != nil {
		return "", fmt.Errorf("workspace: record the writer id: %w", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT value FROM workspace_sync_meta WHERE key = 'writer'`).Scan(&id); err != nil {
		return "", fmt.Errorf("workspace: read the writer id: %w", err)
	}
	return id, nil
}

// encodeCrockford writes bytes in the alphabet operation ids use.
func encodeCrockford(b []byte) string {
	var sb strings.Builder
	var acc uint64
	bits := 0
	for _, c := range b {
		acc = acc<<8 | uint64(c)
		bits += 8
		for bits >= 5 {
			sb.WriteByte(crockford[(acc>>(bits-5))&31])
			bits -= 5
		}
	}
	if bits > 0 {
		sb.WriteByte(crockford[(acc<<(5-bits))&31])
	}
	return sb.String()
}

// Status reports how far apart this workspace and the remote were at the last
// contact, without reaching the remote.
func (s *Sync) Status(ctx context.Context) (SyncStatus, error) {
	st := SyncStatus{Remote: s.remote.Describe()}
	db, err := s.w.sync()
	if err != nil {
		return st, err
	}
	if st.ToPush, err = s.countToPush(ctx, db); err != nil {
		return st, err
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COALESCE(SUM(pending), 0) FROM workspace_sync_segments WHERE remote = ? AND applied = 0`,
		s.rid).Scan(&st.ToPull); err != nil {
		return st, fmt.Errorf("workspace: read sync state: %w", err)
	}
	var at string
	switch err := db.QueryRowContext(ctx,
		`SELECT at, error FROM workspace_sync_contact WHERE remote = ?`, s.rid).Scan(&at, &st.Error); {
	case errors.Is(err, sql.ErrNoRows):
	case err != nil:
		return st, fmt.Errorf("workspace: read sync state: %w", err)
	default:
		st.Contacted, _ = time.Parse(time.RFC3339Nano, at)
	}
	return st, nil
}

// localKindsClause renders the kinds that stay here as a SQL exclusion.
func (s *Sync) localKindsClause() (string, []any) {
	if len(s.opts.LocalKinds) == 0 {
		return "", nil
	}
	args := make([]any, len(s.opts.LocalKinds))
	for i, k := range s.opts.LocalKinds {
		args[i] = k
	}
	return " AND o.kind NOT IN (" + strings.TrimSuffix(strings.Repeat("?,", len(args)), ",") + ")", args
}

func (s *Sync) countToPush(ctx context.Context, db *storage.DB) (int, error) {
	clause, kindArgs := s.localKindsClause()
	args := append([]any{string(s.key)}, kindArgs...)
	args = append(args, s.rid)
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_ops o WHERE o.project = ?`+clause+`
AND NOT EXISTS (SELECT 1 FROM workspace_sync_known k WHERE k.remote = ? AND k.id = o.id)`, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("workspace: count operations to push: %w", err)
	}
	return n, nil
}

func (s *Sync) opsToPush(ctx context.Context, db *storage.DB) ([]Op, error) {
	clause, kindArgs := s.localKindsClause()
	args := append([]any{string(s.key)}, kindArgs...)
	args = append(args, s.rid)
	rows, err := db.QueryContext(ctx, `SELECT `+opColumns+` FROM workspace_ops o WHERE o.project = ?`+clause+`
AND NOT EXISTS (SELECT 1 FROM workspace_sync_known k WHERE k.remote = ? AND k.id = o.id) ORDER BY o.id`, args...)
	if err != nil {
		return nil, fmt.Errorf("workspace: read operations to push: %w", err)
	}
	return scanOps(rows)
}

// noteContact records a reach of the remote, or the failure to reach it.
func (s *Sync) noteContact(ctx context.Context, db *storage.DB, cause error) {
	msg := ""
	if cause != nil {
		msg = cause.Error()
	}
	_, _ = db.ExecContext(ctx, `INSERT INTO workspace_sync_contact (remote, at, error) VALUES (?, ?, ?)
ON CONFLICT(remote) DO UPDATE SET at = excluded.at, error = excluded.error`,
		s.rid, time.Now().UTC().Format(time.RFC3339Nano), msg)
}

// segmentLine is one operation as a segment carries it.
type segmentLine struct {
	ID      string          `json:"id"`
	Address string          `json:"address,omitempty"`
	Project string          `json:"project"`
	Kind    string          `json:"kind"`
	At      string          `json:"at"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// EncodeSegment writes operations as a segment file, one JSON object a line.
func EncodeSegment(ops []Op) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	for _, op := range ops {
		line := segmentLine{
			ID: op.ID, Address: op.Address, Project: string(op.Project), Kind: op.Kind,
			At: op.At.UTC().Format(time.RFC3339Nano),
		}
		if len(op.Payload) > 0 {
			if !json.Valid(op.Payload) {
				return nil, fmt.Errorf("workspace: %s %s carries a payload that is not JSON", op.Kind, ShortOpID(op.ID))
			}
			line.Payload = op.Payload
		}
		if err := enc.Encode(line); err != nil {
			return nil, err
		}
	}
	return buf.Bytes(), nil
}

// DecodeSegment reads a segment file back into operations, refusing one that
// holds anything but operations of the project named.
func DecodeSegment(data []byte, project ProjectKey) ([]Op, error) {
	var out []Op
	for n, raw := range bytes.Split(data, []byte("\n")) {
		if len(bytes.TrimSpace(raw)) == 0 {
			continue
		}
		var line segmentLine
		if err := json.Unmarshal(raw, &line); err != nil {
			return nil, fmt.Errorf("line %d: %w", n+1, err)
		}
		if !ValidOpID(line.ID) || line.Kind == "" {
			return nil, fmt.Errorf("line %d: not an operation", n+1)
		}
		if ProjectKey(line.Project) != project {
			return nil, fmt.Errorf("line %d: an operation of project %q, not %q", n+1, line.Project, project)
		}
		at, err := time.Parse(time.RFC3339Nano, line.At)
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", n+1, err)
		}
		out = append(out, Op{
			ID: line.ID, Address: line.Address, Project: project, Kind: line.Kind,
			Payload: []byte(line.Payload), At: at.UTC(),
		})
	}
	return out, nil
}

// BlobRefs returns the blobs an operation names. An operation names a blob in
// its payload's top-level "blob" field.
func BlobRefs(op Op) []string {
	if len(op.Payload) == 0 || !bytes.Contains(op.Payload, []byte(`"blob"`)) {
		return nil
	}
	var body struct {
		Blob string `json:"blob"`
	}
	if json.Unmarshal(op.Payload, &body) != nil || !validBlobAddress(body.Blob) {
		return nil
	}
	return []string{body.Blob}
}

// Fetch reaches the remote, reads the segments this workspace has not seen,
// and records what they hold, without merging anything.
func (s *Sync) Fetch(ctx context.Context) (SyncStatus, error) {
	db, err := s.w.sync()
	if err != nil {
		return SyncStatus{Remote: s.remote.Describe()}, err
	}
	_, ferr := s.fetch(ctx, db)
	s.noteContact(ctx, db, ferr)
	st, err := s.Status(ctx)
	if ferr != nil {
		return st, ferr
	}
	return st, err
}

// fetched is one segment read and not yet merged.
type fetched struct {
	name string
	ops  []Op
}

// fetch lists the remote's segments and reads the ones not seen before,
// keeping their bytes as local blobs until a pull merges them. It returns the
// names of every segment the remote lists.
func (s *Sync) fetch(ctx context.Context, db *storage.DB) ([]string, error) {
	names, err := s.remote.List(ctx, RemoteLogDir)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	rows, err := db.QueryContext(ctx, `SELECT name FROM workspace_sync_segments WHERE remote = ?`, s.rid)
	if err != nil {
		return nil, fmt.Errorf("workspace: read sync state: %w", err)
	}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			_ = rows.Close()
			return nil, err
		}
		seen[name] = true
	}
	_ = rows.Close()
	for _, name := range names {
		if seen[name] {
			continue
		}
		data, err := s.remote.Get(ctx, name)
		if err != nil {
			return nil, err
		}
		if len(data) > MaxBlobSize {
			return nil, fmt.Errorf("workspace: %s holds %d bytes, over the %d a segment may", name, len(data), MaxBlobSize)
		}
		ops, err := DecodeSegment(data, s.key)
		if err != nil {
			return nil, fmt.Errorf("workspace: read %s from %s: %w", name, s.remote.Describe().Location, err)
		}
		address, err := s.w.PutBlob(ctx, data)
		if err != nil {
			return nil, err
		}
		pending, err := s.notHeld(ctx, db, ops)
		if err != nil {
			return nil, err
		}
		if err := s.noteKnown(ctx, db, ops); err != nil {
			return nil, err
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO workspace_sync_segments (remote, name, pending, applied, blob)
VALUES (?, ?, ?, ?, ?) ON CONFLICT(remote, name) DO NOTHING`, s.rid, name, pending, boolInt(pending == 0), address); err != nil {
			return nil, fmt.Errorf("workspace: record %s: %w", name, err)
		}
	}
	return names, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// notHeld counts the operations the log does not hold, by id or content
// address.
func (s *Sync) notHeld(ctx context.Context, db *storage.DB, ops []Op) (int, error) {
	n := 0
	for _, op := range ops {
		var one int
		err := db.QueryRowContext(ctx, `SELECT 1 FROM workspace_ops WHERE id = ? OR (? <> '' AND address = ?) LIMIT 1`,
			op.ID, op.Address, op.Address).Scan(&one)
		if errors.Is(err, sql.ErrNoRows) {
			n++
			continue
		}
		if err != nil {
			return 0, fmt.Errorf("workspace: look up %s: %w", op.ID, err)
		}
	}
	return n, nil
}

// noteKnown records that the remote holds these operations.
func (s *Sync) noteKnown(ctx context.Context, db *storage.DB, ops []Op) error {
	if len(ops) == 0 {
		return nil
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	for _, op := range ops {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO workspace_sync_known (remote, id) VALUES (?, ?) ON CONFLICT DO NOTHING`, s.rid, op.ID); err != nil {
			return fmt.Errorf("workspace: record sync state: %w", err)
		}
	}
	return tx.Commit()
}

// unapplied reads the fetched segments not yet merged, in name order.
func (s *Sync) unapplied(ctx context.Context, db *storage.DB) ([]fetched, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT name, blob FROM workspace_sync_segments WHERE remote = ? AND applied = 0 ORDER BY name`, s.rid)
	if err != nil {
		return nil, fmt.Errorf("workspace: read sync state: %w", err)
	}
	type pair struct{ name, blob string }
	var pairs []pair
	for rows.Next() {
		var p pair
		if err := rows.Scan(&p.name, &p.blob); err != nil {
			_ = rows.Close()
			return nil, err
		}
		pairs = append(pairs, p)
	}
	_ = rows.Close()
	out := make([]fetched, 0, len(pairs))
	for _, p := range pairs {
		data, err := s.w.Blob(ctx, p.blob)
		if err != nil {
			return nil, err
		}
		ops, err := DecodeSegment(data, s.key)
		if err != nil {
			return nil, fmt.Errorf("workspace: read %s: %w", p.name, err)
		}
		out = append(out, fetched{name: p.name, ops: ops})
	}
	return out, nil
}

// Pull fetches the segments this workspace has not seen, merges their
// operations into the log and applies them.
func (s *Sync) Pull(ctx context.Context) (PullReport, error) {
	var report PullReport
	report.Remote = s.remote.Describe()
	db, err := s.w.sync()
	if err != nil {
		return report, err
	}
	first, err := s.isFirstPull(ctx, db)
	if err != nil {
		return report, err
	}
	names, ferr := s.fetch(ctx, db)
	s.noteContact(ctx, db, ferr)
	if ferr != nil {
		report.SyncStatus, _ = s.Status(ctx)
		return report, ferr
	}
	report.Empty = len(names) == 0
	segs, err := s.unapplied(ctx, db)
	if err != nil {
		return report, err
	}
	report.Segments = len(segs)
	if len(segs) > 0 {
		if first {
			report.Checkpoint = s.firstPull(ctx, db, names, segs, &report)
		} else if err := s.merge(ctx, db, segs, &report); err != nil {
			return report, err
		}
	}
	report.SyncStatus, err = s.Status(ctx)
	return report, err
}

// isFirstPull reports whether the log holds none of the project's operations
// that travel, which is when a remote checkpoint can stand in for replaying
// the whole log.
func (s *Sync) isFirstPull(ctx context.Context, db *storage.DB) (bool, error) {
	clause, kindArgs := s.localKindsClause()
	var one int
	err := db.QueryRowContext(ctx, `SELECT 1 FROM workspace_ops o WHERE o.project = ?`+clause+` LIMIT 1`,
		append([]any{string(s.key)}, kindArgs...)...).Scan(&one)
	if errors.Is(err, sql.ErrNoRows) {
		return true, nil
	}
	return false, err
}

// merge records the segments' operations and applies them, rebuilding the
// stores when one sorts before an operation already applied.
func (s *Sync) merge(ctx context.Context, db *storage.DB, segs []fetched, report *PullReport) error {
	var newest string
	clause, kindArgs := s.localKindsClause()
	if err := db.QueryRowContext(ctx, `SELECT COALESCE(MAX(o.id), '') FROM workspace_ops o WHERE o.project = ?`+clause,
		append([]any{string(s.key)}, kindArgs...)...).Scan(&newest); err != nil {
		return fmt.Errorf("workspace: read the newest operation: %w", err)
	}
	var ops []Op
	for _, seg := range segs {
		ops = append(ops, seg.ops...)
	}
	if err := s.fetchBlobs(ctx, ops); err != nil {
		return err
	}
	for _, op := range ops {
		if op.ID < newest && !slices.Contains(s.opts.LocalKinds, op.Kind) {
			held, err := s.notHeld(ctx, db, []Op{op})
			if err != nil {
				return err
			}
			if held == 1 {
				report.Rebuilt = true
				break
			}
		}
	}
	added, err := Merge(ctx, s.w.backend, ops)
	if err != nil {
		return err
	}
	report.Merged += added
	if err := s.markApplied(ctx, db, segs); err != nil {
		return err
	}
	if added == 0 && !report.Rebuilt {
		return nil
	}
	return s.applier.Apply(ctx, report.Rebuilt)
}

// firstPull merges a remote into a log that holds nothing of the project,
// starting from the remote's newest checkpoint when it covers every
// operation it claims to. It returns the checkpoint used, empty when none.
func (s *Sync) firstPull(ctx context.Context, db *storage.DB, names []string, segs []fetched, report *PullReport) string {
	var used string
	err := func() error {
		cps, err := s.remote.List(ctx, RemoteCheckpointsDir)
		if err != nil || len(cps) == 0 {
			return err
		}
		newest := cps[len(cps)-1]
		data, err := s.remote.Get(ctx, newest)
		if err != nil {
			return err
		}
		through, included, err := s.applier.CheckpointMark(data)
		if err != nil {
			return err
		}
		in := map[string]bool{}
		for _, n := range included {
			in[n] = true
		}
		for _, n := range included {
			if !slices.Contains(names, n) {
				return nil
			}
		}
		var before, after []fetched
		for _, seg := range segs {
			if in[seg.name] {
				before = append(before, seg)
				continue
			}
			for _, op := range seg.ops {
				if op.ID <= through {
					// The checkpoint does not include an operation it would
					// have to: replay everything instead.
					return nil
				}
			}
			after = append(after, seg)
		}
		var ops []Op
		for _, seg := range before {
			ops = append(ops, seg.ops...)
		}
		if err := s.fetchBlobs(ctx, ops); err != nil {
			return err
		}
		added, err := Merge(ctx, s.w.backend, ops)
		if err != nil {
			return err
		}
		report.Merged += added
		if err := s.markApplied(ctx, db, before); err != nil {
			return err
		}
		if err := s.applier.InstallCheckpoint(ctx, data); err != nil {
			return err
		}
		used = through
		segs = after
		return nil
	}()
	if err != nil {
		// A checkpoint that cannot be read or installed costs a full replay,
		// never the pull.
		used = ""
	}
	report.Rebuilt = true
	var ops []Op
	for _, seg := range segs {
		ops = append(ops, seg.ops...)
	}
	if ferr := s.fetchBlobs(ctx, ops); ferr != nil {
		report.Error = ferr.Error()
		return used
	}
	added, merr := Merge(ctx, s.w.backend, ops)
	if merr != nil {
		report.Error = merr.Error()
		return used
	}
	report.Merged += added
	if err := s.markApplied(ctx, db, segs); err != nil {
		report.Error = err.Error()
		return used
	}
	if err := s.applier.Apply(ctx, true); err != nil {
		report.Error = err.Error()
	}
	return used
}

func (s *Sync) markApplied(ctx context.Context, db *storage.DB, segs []fetched) error {
	for _, seg := range segs {
		if _, err := db.ExecContext(ctx,
			`UPDATE workspace_sync_segments SET applied = 1 WHERE remote = ? AND name = ?`, s.rid, seg.name); err != nil {
			return fmt.Errorf("workspace: record sync state: %w", err)
		}
	}
	return nil
}

// fetchBlobs reads from the remote every blob the operations name that the
// workspace does not hold.
func (s *Sync) fetchBlobs(ctx context.Context, ops []Op) error {
	for _, op := range ops {
		for _, ref := range BlobRefs(op) {
			if _, err := s.w.Blob(ctx, ref); err == nil {
				continue
			} else if !errors.Is(err, ErrNoBlob) {
				return err
			}
			data, err := s.remote.Get(ctx, BlobObjectName(ref))
			if err != nil {
				return fmt.Errorf("workspace: %s %s names blob %s: %w", op.Kind, ShortOpID(op.ID), ref, err)
			}
			if BlobAddress(data) != ref {
				return fmt.Errorf("workspace: blob %s on the remote does not hold the bytes its name says", ref)
			}
			if _, err := s.w.PutBlob(ctx, data); err != nil {
				return err
			}
		}
	}
	return nil
}

// Push writes the project's operations the remote does not hold as new
// segments under this workspace's writer id, with the blobs they name, and a
// checkpoint when the remote has gained CheckpointEvery operations since its
// newest one.
func (s *Sync) Push(ctx context.Context) (PushReport, error) {
	var report PushReport
	report.Remote = s.remote.Describe()
	db, err := s.w.sync()
	if err != nil {
		return report, err
	}
	writer, err := s.w.WriterID(ctx)
	if err != nil {
		return report, err
	}
	// Reading what the remote holds first means an interrupted push, whose
	// segment landed before this workspace recorded it, is not written twice.
	names, ferr := s.fetch(ctx, db)
	if ferr == nil {
		ferr = s.push(ctx, db, writer, &report)
	}
	if ferr == nil {
		ferr = s.maybeCheckpoint(ctx, db, append(names, report.written...), &report)
	}
	s.noteContact(ctx, db, ferr)
	report.SyncStatus, err = s.Status(ctx)
	if ferr != nil {
		return report, ferr
	}
	return report, err
}

func (s *Sync) push(ctx context.Context, db *storage.DB, writer string, report *PushReport) error {
	ops, err := s.opsToPush(ctx, db)
	if err != nil || len(ops) == 0 {
		return err
	}
	held, err := s.remote.List(ctx, RemoteBlobDir)
	if err != nil {
		return err
	}
	onRemote := map[string]bool{}
	for _, n := range held {
		onRemote[n] = true
	}
	var objs []Object
	for _, op := range ops {
		for _, ref := range BlobRefs(op) {
			name := BlobObjectName(ref)
			if onRemote[name] {
				continue
			}
			data, err := s.w.Blob(ctx, ref)
			if err != nil {
				return fmt.Errorf("workspace: %s %s names blob %s: %w", op.Kind, ShortOpID(op.ID), ref, err)
			}
			objs = append(objs, Object{Name: name, Data: data})
			onRemote[name] = true
			report.Blobs++
		}
	}
	var segs []Object
	for chunk := range slices.Chunk(ops, SegmentOps) {
		data, err := EncodeSegment(chunk)
		if err != nil {
			return err
		}
		segs = append(segs, Object{Name: SegmentName(writer, chunk[0].ID), Data: data})
	}
	// Blobs go first, so a remote that writes object by object never shows a
	// segment whose blobs are not there yet.
	if err := s.remote.Put(ctx, append(objs, segs...)...); err != nil {
		return err
	}
	if err := s.noteKnown(ctx, db, ops); err != nil {
		return err
	}
	for _, seg := range segs {
		address, err := s.w.PutBlob(ctx, seg.Data)
		if err != nil {
			return err
		}
		if _, err := db.ExecContext(ctx, `INSERT INTO workspace_sync_segments (remote, name, pending, applied, blob)
VALUES (?, ?, 0, 1, ?) ON CONFLICT(remote, name) DO UPDATE SET applied = 1, pending = 0`, s.rid, seg.Name, address); err != nil {
			return fmt.Errorf("workspace: record %s: %w", seg.Name, err)
		}
		report.written = append(report.written, seg.Name)
	}
	report.Pushed, report.Segments = len(ops), len(segs)
	return nil
}

// maybeCheckpoint writes a checkpoint when the remote has gained enough
// operations since its newest one.
func (s *Sync) maybeCheckpoint(ctx context.Context, db *storage.DB, names []string, report *PushReport) error {
	every := s.opts.CheckpointEvery
	if every == 0 {
		every = CheckpointEvery
	}
	if every < 0 || s.applier == nil {
		return nil
	}
	cps, err := s.remote.List(ctx, RemoteCheckpointsDir)
	if err != nil {
		return err
	}
	since := ""
	if len(cps) > 0 {
		since = CheckpointThrough(cps[len(cps)-1])
	}
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM workspace_sync_known WHERE remote = ? AND id > ?`,
		s.rid, since).Scan(&n); err != nil {
		return fmt.Errorf("workspace: read sync state: %w", err)
	}
	if n < every {
		return nil
	}
	applied, err := s.appliedSegments(ctx, db, names)
	if err != nil {
		return err
	}
	data, through, err := s.applier.Checkpoint(ctx, applied)
	if err != nil || through == "" || through <= since {
		return err
	}
	if err := s.remote.Put(ctx, Object{Name: CheckpointName(through), Data: data}); err != nil &&
		!errors.Is(err, ErrObjectExists) {
		return err
	}
	report.Checkpoint = through
	return nil
}

// appliedSegments narrows segment names to the ones merged here.
func (s *Sync) appliedSegments(ctx context.Context, db *storage.DB, names []string) ([]string, error) {
	rows, err := db.QueryContext(ctx,
		`SELECT name FROM workspace_sync_segments WHERE remote = ? AND applied = 1 ORDER BY name`, s.rid)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	listed := map[string]bool{}
	for _, n := range names {
		listed[n] = true
	}
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if listed[name] {
			out = append(out, name)
		}
	}
	return out, rows.Err()
}

// Forget discards what this workspace knows about the remote.
func (s *Sync) Forget(ctx context.Context) error {
	db, err := s.w.sync()
	if err != nil {
		return err
	}
	for _, table := range []string{"workspace_sync_known", "workspace_sync_segments", "workspace_sync_contact"} {
		if _, err := db.ExecContext(ctx, `DELETE FROM `+table+` WHERE remote = ?`, s.rid); err != nil {
			return fmt.Errorf("workspace: forget sync state: %w", err)
		}
	}
	return nil
}
