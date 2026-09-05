package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/internal/storeutil"
	"github.com/neokapi/neokapi/core/id"
)

// ---------------------------------------------------------------------------
// Stream CRUD (PostgreSQL)
// ---------------------------------------------------------------------------

func (s *PostgresStore) CreateStream(ctx context.Context, st *platstore.Stream) error {
	if st.Name == "" {
		return errors.New("stream name cannot be empty")
	}
	// "main" can now be created explicitly (e.g. during project setup).
	if st.Visibility == "" {
		st.Visibility = platstore.StreamPublic
	}
	now := time.Now().UTC()
	st.CreatedAt = now

	if st.Parent != "" {
		parent := storeutil.DefaultStream(st.Parent)
		cursor, err := s.LatestCursor(ctx, st.ProjectID, parent)
		if err != nil {
			return fmt.Errorf("get parent cursor: %w", err)
		}
		st.BaseCursor = cursor
	}

	propsJSON, err := json.Marshal(st.Properties)
	if err != nil {
		return fmt.Errorf("marshal stream properties: %w", err)
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO streams (project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by, properties)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		st.ProjectID, st.Name, st.Parent, st.BaseCursor, st.Archived,
		string(st.Visibility), st.Description, now, st.CreatedBy, string(propsJSON))
	if err != nil {
		return fmt.Errorf("insert stream: %w", err)
	}

	// A branch starts as its parent: the same content, the same translations,
	// the same approvals. Everything is copied under the new stream and KEEPS
	// ITS IDS — that is what makes the same unit the same unit on both sides,
	// so a diff compares by key rather than guessing by content, and a branch
	// inherits its governance instead of starting unreviewed.
	//
	// Ids used to be minted per stream here, which is why the same file on two
	// streams read as two unrelated units and why nothing could be merged.
	parentStream := storeutil.DefaultStream(st.Parent)
	if err := branchStreamContent(ctx, s.db.DB, st.ProjectID, parentStream, st.Name, now); err != nil {
		return fmt.Errorf("branch %q from %q: %w", st.Name, parentStream, err)
	}

	return nil
}

// branchStreamCopies names each stream-scoped table and the columns to carry
// across, with the stream itself supplied by the caller. Written out per table
// rather than derived, because a column list is what a copy IS: a table that
// gains a column and is not named here would silently drop it on every branch,
// and the compiler cannot see into SQL.
//
// change_log is deliberately absent. A branch inherits content, not history:
// its base_cursor already records where it started, and copying its parent's
// log would make every unit read as changed on the branch the moment it was
// created.
// stamp names the column set to the branch time rather than copied, or "" for
// a table with no such column. A copied row is new here, so its own clock
// starts now; the content it carries is the parent's verbatim.
//
// mintID marks a table whose id is the ROW's identity rather than the unit's.
// items and blocks keep theirs — the same unit having the same id on both sides
// is what makes a branch comparable to its parent — but a note is its own row,
// keyed globally, so a copy is a new note and needs a new id.
var branchStreamCopies = []struct {
	table, columns, stamp string
	mintID                bool
}{
	{table: "items", columns: "id, project_id, name, format, item_type, block_index, preview_html, properties, collection_id, created_at", stamp: "updated_at"},
	{table: "blocks", columns: "id, project_id, item_name, item_id, source_id, name, type, mime_type, translatable, " +
		"content_hash, context_hash, source_json, overlays, word_count, properties, owner_id, access, stored_at", stamp: "updated_at"},
	{table: "translations", columns: "project_id, block_id, locale, text, target_json, provider, metadata", stamp: "updated_at"},
	{table: "annotations", columns: "project_id, block_id, kind, payload", stamp: "updated_at"},
	{table: "overlays_ext", columns: "project_id, block_id, kind, payload", stamp: "updated_at"},
	{table: "block_notes", columns: "project_id, block_id, author, text, created_at", mintID: true},
	{table: "unit_decisions", columns: "project_id, item_id, item_name, unit, variant, status, target_hash, content_hash, " +
		"review_state, decided_by, decided_at, note, parked, assignee, governing_fingerprint, updated", stamp: "updated_at"},
}

// branchStreamContent copies one stream's content into another, ids intact.
// Takes an Execer so a branch (its own statement) and a fast-forward merge
// (inside the transaction that just cleared the target) share one definition
// of what copying a stream means.
func branchStreamContent(ctx context.Context, ex Execer, projectID, from, to string, now time.Time) error {
	for _, c := range branchStreamCopies {
		cols, sel := "stream, "+c.columns, "$1, "+c.columns
		args := []any{to}
		next := 2
		if c.mintID {
			cols = "id, " + cols
			sel = "substr(md5(random()::text || " + c.table + ".id), 1, 12), " + sel
		}
		if c.stamp != "" {
			cols += ", " + c.stamp
			sel += fmt.Sprintf(", $%d", next)
			args = append(args, now)
			next++
		}
		q := fmt.Sprintf(`INSERT INTO %s (%s) SELECT %s FROM %s WHERE project_id = $%d AND stream = $%d`,
			c.table, cols, sel, c.table, next, next+1)
		args = append(args, projectID, from)
		if _, err := ex.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("copy %s: %w", c.table, err)
		}
	}
	return nil
}

func (s *PostgresStore) GetStream(ctx context.Context, projectID, name string) (*platstore.Stream, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by, locked, locked_by, locked_at, properties
		 FROM streams WHERE project_id = $1 AND name = $2`, projectID, name)
	return scanStreamPg(row)
}

func (s *PostgresStore) ListStreams(ctx context.Context, projectID string, includeArchived bool) ([]*platstore.Stream, error) {
	var query string
	var args []any
	if includeArchived {
		query = `SELECT project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by, locked, locked_by, locked_at, properties
				 FROM streams WHERE project_id = $1 ORDER BY name`
		args = []any{projectID}
	} else {
		query = `SELECT project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by, locked, locked_by, locked_at, properties
				 FROM streams WHERE project_id = $1 AND archived = FALSE ORDER BY name`
		args = []any{projectID}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list streams: %w", err)
	}
	defer rows.Close()

	var result []*platstore.Stream
	for rows.Next() {
		st, err := scanStreamPg(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, st)
	}
	return result, rows.Err()
}

func (s *PostgresStore) UpdateStream(ctx context.Context, st *platstore.Stream) error {
	propsJSON, err := json.Marshal(st.Properties)
	if err != nil {
		return fmt.Errorf("marshal stream properties: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE streams SET parent=$1, base_cursor=$2, archived=$3, visibility=$4, description=$5, locked=$6, locked_by=$7, locked_at=$8, properties=$9
		 WHERE project_id=$10 AND name=$11`,
		st.Parent, st.BaseCursor, st.Archived, string(st.Visibility),
		st.Description, st.Locked, st.LockedBy, st.LockedAt, string(propsJSON), st.ProjectID, st.Name)
	if err != nil {
		return fmt.Errorf("update stream: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("stream %q not found in project %s", st.Name, st.ProjectID)
	}
	return nil
}

// DeleteStream removes a stream and everything filed under it. A deleted stream
// is ERASED, not retained: its items, targets, annotations, overlays, history,
// notes, change log, decision ledger and source proposals go with it, so a
// stream created again under the same name starts empty instead of inheriting a
// dead one's work. stream_members and stream_tags follow on their foreign key.
func (s *PostgresStore) DeleteStream(ctx context.Context, projectID, name string) error {
	if name == "main" {
		return errors.New("cannot delete the main stream")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`DELETE FROM streams WHERE project_id=$1 AND name=$2`, projectID, name)
	if err != nil {
		return fmt.Errorf("delete stream: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("stream %q not found in project %s", name, projectID)
	}

	for _, table := range storeutil.StreamScopedTables() {
		//nolint:gosec // table is a fixed literal from storeutil, never user input
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM `+table+` WHERE project_id=$1 AND stream=$2`, projectID, name); err != nil {
			return fmt.Errorf("delete %s for stream %q: %w", table, name, err)
		}
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Stream operations
// ---------------------------------------------------------------------------

// MergeStream fast-forwards a stream's parent onto it.
//
// FAST-FORWARD ONLY. The parent takes the branch's state wholesale, and the
// merge is refused if the parent has moved since the branch was taken. Nothing
// is combined: a merge that had to choose between two edits of one unit would
// be choosing between two people's approved wording, and doing that silently is
// the outcome this platform exists to prevent. A diverged parent is reported so
// a person can decide.
//
// The fast-forward is a copy rather than a pointer move because the store holds
// rows, not commits: the parent's content for this stream is replaced by the
// branch's, ids intact, so every translation and approval arrives attached to
// the content it belongs to.
//
// It used to move nothing at all — it counted the change log and wrote entries
// into the parent, reporting a MergeResult that described work no row had done.
func (s *PostgresStore) MergeStream(ctx context.Context, projectID, streamName string, opts platstore.MergeOptions) (*platstore.MergeResult, error) {
	stream, err := s.GetStream(ctx, projectID, streamName)
	if err != nil {
		return nil, fmt.Errorf("get stream: %w", err)
	}
	if stream.Parent == "" {
		return nil, fmt.Errorf("stream %q has no parent to merge into", streamName)
	}
	parentStream := storeutil.DefaultStream(stream.Parent)

	// Has the parent moved since this branch was taken? base_cursor is where it
	// stood then; anything past it is work the branch never saw.
	parentCursor, err := s.LatestCursor(ctx, projectID, parentStream)
	if err != nil {
		return nil, fmt.Errorf("read parent cursor: %w", err)
	}
	if parentCursor != stream.BaseCursor {
		return nil, fmt.Errorf(
			"%q has moved since %q branched from it (branched at %d, now at %d): merge is fast-forward only, so re-branch or bring the changes across deliberately",
			parentStream, streamName, stream.BaseCursor, parentCursor)
	}

	changes, err := s.GetChanges(ctx, projectID, streamName, stream.BaseCursor, nil, MaxChangesPerRequest)
	if err != nil {
		return nil, fmt.Errorf("get stream changes: %w", err)
	}

	result := &platstore.MergeResult{}
	blockChanges := map[string]string{}
	for _, c := range changes.Changes {
		blockChanges[c.BlockID] = c.ChangeType
	}
	for blockID, changeType := range blockChanges {
		var ct platstore.ChangeType
		switch {
		case changeType == "source_added":
			ct = platstore.ChangeAdded
			result.AddedBlocks++
		case changeType == "source_removed":
			ct = platstore.ChangeRemoved
			result.RemovedBlocks++
		default:
			ct = platstore.ChangeModified
			result.ModifiedBlocks++
		}
		result.Changes = append(result.Changes, platstore.BlockChange{BlockID: blockID, ChangeType: ct})
	}
	result.MergedBlocks = len(blockChanges)

	if opts.DryRun {
		return result, nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	// The parent becomes the branch. Its rows go first — a fast-forward
	// replaces state rather than merging into it — and all of it is one
	// transaction, so a failure leaves the parent exactly as it was.
	for _, table := range storeutil.StreamScopedTables() {
		if table == "change_log" {
			continue // the parent's history is appended to below, not replaced
		}
		//nolint:gosec // table is a fixed literal from storeutil, never user input
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM `+table+` WHERE project_id=$1 AND stream=$2`, projectID, parentStream); err != nil {
			return nil, fmt.Errorf("clear %s on %q: %w", table, parentStream, err)
		}
	}
	now := time.Now().UTC()
	if err := branchStreamContent(ctx, tx, projectID, streamName, parentStream, now); err != nil {
		return nil, fmt.Errorf("fast-forward %q onto %q: %w", parentStream, streamName, err)
	}

	for blockID, changeType := range blockChanges {
		if err := logChange(ctx, tx, projectID, parentStream, blockID, changeType, "", ""); err != nil {
			return nil, fmt.Errorf("log merge change: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit merge: %w", err)
	}

	return result, nil
}

// DiffStream compares a stream against its parent, by content.
//
// It reads both sides rather than replaying the change log. The log records
// what a stream DID; a diff has to answer what the two streams ARE, and those
// stop agreeing the moment a branch edits a unit back to the value its parent
// already held — the log has two entries and the streams have no difference.
//
// The comparison is by key, which is what stable ids across streams buy: the
// same unit carries the same id on both sides, so a difference is a hash
// inequality rather than a guess about which block corresponds to which.
func (s *PostgresStore) DiffStream(ctx context.Context, projectID, streamName string) (*platstore.StreamDiff, error) {
	stream, err := s.GetStream(ctx, projectID, streamName)
	if err != nil {
		return nil, fmt.Errorf("get stream: %w", err)
	}
	parentName := storeutil.DefaultStream(stream.Parent)

	diff := &platstore.StreamDiff{StreamName: streamName, ParentName: parentName}

	// A full outer join over the two sides: present on one only is an add or a
	// remove, present on both with differing content is a modification, and
	// present on both alike contributes nothing.
	rows, err := s.db.QueryContext(ctx,
		`SELECT COALESCE(b.id, p.id),
			CASE WHEN p.id IS NULL THEN 'added'
			     WHEN b.id IS NULL THEN 'removed'
			     ELSE 'modified' END
		 FROM (SELECT id, content_hash FROM blocks WHERE project_id=$1 AND stream=$2) b
		 FULL OUTER JOIN
		     (SELECT id, content_hash FROM blocks WHERE project_id=$1 AND stream=$3) p
		 ON p.id = b.id
		 WHERE p.id IS NULL OR b.id IS NULL OR p.content_hash <> b.content_hash
		 ORDER BY 1`,
		projectID, streamName, parentName)
	if err != nil {
		return nil, fmt.Errorf("diff stream %q against %q: %w", streamName, parentName, err)
	}
	defer rows.Close()

	for rows.Next() {
		var blockID, kind string
		if err := rows.Scan(&blockID, &kind); err != nil {
			return nil, fmt.Errorf("scan stream diff: %w", err)
		}
		var ct platstore.ChangeType
		switch kind {
		case "added":
			ct = platstore.ChangeAdded
		case "removed":
			ct = platstore.ChangeRemoved
		default:
			ct = platstore.ChangeModified
		}
		diff.Changes = append(diff.Changes, platstore.BlockChange{BlockID: blockID, ChangeType: ct})
	}
	return diff, rows.Err()
}

// ---------------------------------------------------------------------------
// Stream membership
// ---------------------------------------------------------------------------

func (s *PostgresStore) AddStreamMember(ctx context.Context, projectID, streamName, userID string) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO stream_members (project_id, stream, user_id, added_at) VALUES ($1, $2, $3, $4)
		 ON CONFLICT(project_id, stream, user_id) DO NOTHING`,
		projectID, streamName, userID, now)
	if err != nil {
		return fmt.Errorf("add stream member: %w", err)
	}
	return nil
}

func (s *PostgresStore) RemoveStreamMember(ctx context.Context, projectID, streamName, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM stream_members WHERE project_id=$1 AND stream=$2 AND user_id=$3`,
		projectID, streamName, userID)
	if err != nil {
		return fmt.Errorf("remove stream member: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("member %s not found in stream %s/%s", userID, projectID, streamName)
	}
	return nil
}

func (s *PostgresStore) ListStreamMembers(ctx context.Context, projectID, streamName string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id FROM stream_members WHERE project_id=$1 AND stream=$2 ORDER BY user_id`,
		projectID, streamName)
	if err != nil {
		return nil, fmt.Errorf("list stream members: %w", err)
	}
	defer rows.Close()

	var members []string
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return nil, fmt.Errorf("scan stream member: %w", err)
		}
		members = append(members, uid)
	}
	return members, rows.Err()
}

// ---------------------------------------------------------------------------
// Scan helper
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Stream lock (PostgreSQL)
// ---------------------------------------------------------------------------

func (s *PostgresStore) LockStream(ctx context.Context, projectID, streamName, userID string) error {
	now := time.Now().UTC()
	res, err := s.db.ExecContext(ctx,
		`UPDATE streams SET locked = TRUE, locked_by = $1, locked_at = $2
		 WHERE project_id = $3 AND name = $4 AND locked = FALSE`,
		userID, now, projectID, streamName)
	if err != nil {
		return fmt.Errorf("lock stream: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		st, err := s.GetStream(ctx, projectID, streamName)
		if err != nil {
			return fmt.Errorf("stream %q not found in project %s", streamName, projectID)
		}
		if st.Locked {
			return fmt.Errorf("stream %q is already locked", streamName)
		}
	}
	return nil
}

func (s *PostgresStore) UnlockStream(ctx context.Context, projectID, streamName string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE streams SET locked = FALSE, locked_by = '', locked_at = NULL
		 WHERE project_id = $1 AND name = $2`,
		projectID, streamName)
	if err != nil {
		return fmt.Errorf("unlock stream: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("stream %q not found in project %s", streamName, projectID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Stream tags (PostgreSQL)
// ---------------------------------------------------------------------------

func (s *PostgresStore) CreateStreamTag(ctx context.Context, tag *platstore.StreamTag) error {
	if tag.ID == "" {
		tag.ID = id.New()
	}
	if tag.Kind == "" {
		tag.Kind = platstore.TagKindCustom
	}
	now := time.Now().UTC()
	tag.CreatedAt = now

	metaJSON, err := json.Marshal(tag.Metadata)
	if err != nil {
		return fmt.Errorf("marshal tag metadata: %w", err)
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO stream_tags (id, project_id, stream, name, kind, cursor, metadata, created_by, created_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		tag.ID, tag.ProjectID, tag.Stream, tag.Name, string(tag.Kind),
		tag.Cursor, string(metaJSON), tag.CreatedBy, now)
	if err != nil {
		return fmt.Errorf("insert stream tag: %w", err)
	}
	return nil
}

func (s *PostgresStore) ListStreamTags(ctx context.Context, projectID, stream string) ([]*platstore.StreamTag, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, stream, name, kind, cursor, metadata, created_by, created_at
		 FROM stream_tags WHERE project_id = $1 AND stream = $2 ORDER BY created_at DESC`,
		projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("list stream tags: %w", err)
	}
	defer rows.Close()
	return scanStreamTagsPg(rows)
}

func (s *PostgresStore) GetStreamTag(ctx context.Context, projectID, stream, tagName string) (*platstore.StreamTag, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, stream, name, kind, cursor, metadata, created_by, created_at
		 FROM stream_tags WHERE project_id = $1 AND stream = $2 AND name = $3`,
		projectID, stream, tagName)
	return scanStreamTagPg(row)
}

func (s *PostgresStore) DeleteStreamTag(ctx context.Context, projectID, stream, tagName string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM stream_tags WHERE project_id = $1 AND stream = $2 AND name = $3`,
		projectID, stream, tagName)
	if err != nil {
		return fmt.Errorf("delete stream tag: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("tag %q not found on stream %s", tagName, stream)
	}
	return nil
}

func (s *PostgresStore) ListProjectTags(ctx context.Context, projectID string, kind platstore.StreamTagKind) ([]*platstore.StreamTag, error) {
	var query string
	var args []any
	if kind != "" {
		query = `SELECT id, project_id, stream, name, kind, cursor, metadata, created_by, created_at
				 FROM stream_tags WHERE project_id = $1 AND kind = $2 ORDER BY created_at DESC`
		args = []any{projectID, string(kind)}
	} else {
		query = `SELECT id, project_id, stream, name, kind, cursor, metadata, created_by, created_at
				 FROM stream_tags WHERE project_id = $1 ORDER BY created_at DESC`
		args = []any{projectID}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list project tags: %w", err)
	}
	defer rows.Close()
	return scanStreamTagsPg(rows)
}

// ---------------------------------------------------------------------------
// Scan helpers
// ---------------------------------------------------------------------------

func scanStreamPg(row scanner) (*platstore.Stream, error) {
	var st platstore.Stream
	var visibility string
	var propsJSON string
	err := row.Scan(&st.ProjectID, &st.Name, &st.Parent, &st.BaseCursor,
		&st.Archived, &visibility, &st.Description, &st.CreatedAt, &st.CreatedBy,
		&st.Locked, &st.LockedBy, &st.LockedAt, &propsJSON)
	if err != nil {
		return nil, fmt.Errorf("scan stream: %w", err)
	}
	st.Visibility = platstore.StreamVisibility(visibility)
	if propsJSON != "" {
		if err := json.Unmarshal([]byte(propsJSON), &st.Properties); err != nil {
			return nil, fmt.Errorf("unmarshal stream properties: %w", err)
		}
	}
	return &st, nil
}

func scanStreamTagPg(row scanner) (*platstore.StreamTag, error) {
	var tag platstore.StreamTag
	var kindStr, metaStr string
	err := row.Scan(&tag.ID, &tag.ProjectID, &tag.Stream, &tag.Name,
		&kindStr, &tag.Cursor, &metaStr, &tag.CreatedBy, &tag.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("stream tag not found")
		}
		return nil, fmt.Errorf("scan stream tag: %w", err)
	}
	tag.Kind = platstore.StreamTagKind(kindStr)
	if metaStr != "" && metaStr != "{}" {
		if err := json.Unmarshal([]byte(metaStr), &tag.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal stream tag metadata: %w", err)
		}
	}
	return &tag, nil
}

func scanStreamTagsPg(rows *sql.Rows) ([]*platstore.StreamTag, error) {
	var result []*platstore.StreamTag
	for rows.Next() {
		tag, err := scanStreamTagPg(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, tag)
	}
	return result, rows.Err()
}
