package sqlitestore

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
// Stream CRUD
// ---------------------------------------------------------------------------

// CreateStream inserts a new stream. If the stream has a parent, the BaseCursor
// is automatically set to the parent's latest cursor position.
func (s *SQLiteStore) CreateStream(ctx context.Context, st *platstore.Stream) error {
	if st.Name == "" {
		return errors.New("stream name cannot be empty")
	}
	// "main" can now be created explicitly (e.g. during project setup).
	if st.Visibility == "" {
		st.Visibility = platstore.StreamPublic
	}
	now := time.Now().UTC()
	st.CreatedAt = now

	// Auto-set base cursor from parent's latest cursor.
	if st.Parent != "" {
		parent := storeutil.DefaultStream(st.Parent)
		cursor, err := s.LatestCursor(ctx, st.ProjectID, parent)
		if err != nil {
			return fmt.Errorf("get parent cursor: %w", err)
		}
		st.BaseCursor = cursor
	}

	archived := 0
	if st.Archived {
		archived = 1
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO streams (project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		st.ProjectID, st.Name, st.Parent, st.BaseCursor, archived,
		string(st.Visibility), st.Description,
		now.Format(time.RFC3339), st.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert stream: %w", err)
	}

	// Copy items from the parent stream into the new stream.
	parentStream := storeutil.DefaultStream(st.Parent)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO items (id, project_id, stream, name, format, item_type, block_index, preview_html, properties, collection_id, created_at, updated_at)
		 SELECT lower(hex(randomblob(4))), project_id, ?, name, format, item_type, block_index, preview_html, properties, collection_id, ?, ?
		 FROM items WHERE project_id = ? AND stream = ?`,
		st.Name, now.Format(time.RFC3339), now.Format(time.RFC3339),
		st.ProjectID, parentStream)
	if err != nil {
		return fmt.Errorf("copy parent items: %w", err)
	}

	return nil
}

// GetStream returns a stream by project and name.
func (s *SQLiteStore) GetStream(ctx context.Context, projectID, name string) (*platstore.Stream, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by, locked, locked_by, locked_at
		 FROM streams WHERE project_id = ? AND name = ?`,
		projectID, name)
	return scanStream(row)
}

// ListStreams returns all streams for a project.
func (s *SQLiteStore) ListStreams(ctx context.Context, projectID string, includeArchived bool) ([]*platstore.Stream, error) {
	var query string
	var args []any
	if includeArchived {
		query = `SELECT project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by, locked, locked_by, locked_at
				 FROM streams WHERE project_id = ? ORDER BY name`
		args = []any{projectID}
	} else {
		query = `SELECT project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by, locked, locked_by, locked_at
				 FROM streams WHERE project_id = ? AND archived = 0 ORDER BY name`
		args = []any{projectID}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list streams: %w", err)
	}
	defer rows.Close()

	var result []*platstore.Stream
	for rows.Next() {
		st, err := scanStream(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, st)
	}
	return result, rows.Err()
}

// UpdateStream updates a stream's description, visibility, and archived status.
func (s *SQLiteStore) UpdateStream(ctx context.Context, st *platstore.Stream) error {
	archived := 0
	if st.Archived {
		archived = 1
	}
	locked := 0
	if st.Locked {
		locked = 1
	}
	var lockedAt *string
	if st.LockedAt != nil {
		s := st.LockedAt.UTC().Format(time.RFC3339)
		lockedAt = &s
	}

	res, err := s.db.ExecContext(ctx,
		`UPDATE streams SET description = ?, visibility = ?, archived = ?, locked = ?, locked_by = ?, locked_at = ?
		 WHERE project_id = ? AND name = ?`,
		st.Description, string(st.Visibility), archived, locked, st.LockedBy, lockedAt, st.ProjectID, st.Name)
	if err != nil {
		return fmt.Errorf("update stream: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("stream %q not found in project %s", st.Name, st.ProjectID)
	}
	return nil
}

// DeleteStream removes a stream and its associated data.
func (s *SQLiteStore) DeleteStream(ctx context.Context, projectID, name string) error {
	if name == "main" {
		return errors.New("cannot delete the main stream")
	}

	res, err := s.db.ExecContext(ctx,
		`DELETE FROM streams WHERE project_id = ? AND name = ?`,
		projectID, name)
	if err != nil {
		return fmt.Errorf("delete stream: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("stream %q not found in project %s", name, projectID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Stream operations
// ---------------------------------------------------------------------------

// MergeStream applies a stream's changes to its parent stream.
func (s *SQLiteStore) MergeStream(ctx context.Context, projectID, streamName string, opts platstore.MergeOptions) (*platstore.MergeResult, error) {
	stream, err := s.GetStream(ctx, projectID, streamName)
	if err != nil {
		return nil, fmt.Errorf("get stream: %w", err)
	}
	if stream.Parent == "" {
		return nil, fmt.Errorf("stream %q has no parent to merge into", streamName)
	}

	parentStream := storeutil.DefaultStream(stream.Parent)

	// Get all change log entries for this stream since the base cursor.
	changes, err := s.GetChanges(ctx, projectID, streamName, stream.BaseCursor, nil, MaxChangesPerRequest)
	if err != nil {
		return nil, fmt.Errorf("get stream changes: %w", err)
	}

	result := &platstore.MergeResult{}

	// Collect unique block IDs and categorize changes.
	blockChanges := map[string]string{} // blockID -> latest change type
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
		result.Changes = append(result.Changes, platstore.BlockChange{
			BlockID:    blockID,
			ChangeType: ct,
		})
	}
	result.MergedBlocks = len(blockChanges)

	if opts.DryRun {
		return result, nil
	}

	// Apply changes: copy block targets from stream to parent.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	for blockID, changeType := range blockChanges {
		if changeType == "source_removed" {
			continue
		}

		// Verify the block exists. The former path pulled targets_json
		// for existence; targets now live in the translations table so
		// we probe the id column directly.
		var exists string
		err := tx.QueryRowContext(ctx,
			`SELECT id FROM blocks WHERE project_id = ? AND id = ?`,
			projectID, blockID).Scan(&exists)
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, fmt.Errorf("get block: %w", err)
		}

		// Log the change in the parent stream.
		if err := logChange(ctx, tx, projectID, parentStream, blockID, changeType, "", ""); err != nil {
			return nil, fmt.Errorf("log merge change: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit merge: %w", err)
	}

	return result, nil
}

// DiffStream compares a stream's blocks against its parent's state at the branch point.
func (s *SQLiteStore) DiffStream(ctx context.Context, projectID, streamName string) (*platstore.StreamDiff, error) {
	stream, err := s.GetStream(ctx, projectID, streamName)
	if err != nil {
		return nil, fmt.Errorf("get stream: %w", err)
	}

	parentName := storeutil.DefaultStream(stream.Parent)

	// Get all changes in this stream since the base cursor.
	changes, err := s.GetChanges(ctx, projectID, streamName, stream.BaseCursor, nil, MaxChangesPerRequest)
	if err != nil {
		return nil, fmt.Errorf("get stream changes: %w", err)
	}

	diff := &platstore.StreamDiff{
		StreamName: streamName,
		ParentName: parentName,
	}

	// Deduplicate by block ID, keeping the latest change type.
	blockChanges := map[string]string{}
	for _, c := range changes.Changes {
		blockChanges[c.BlockID] = c.ChangeType
	}

	for blockID, changeType := range blockChanges {
		var ct platstore.ChangeType
		switch {
		case changeType == "source_added":
			ct = platstore.ChangeAdded
		case changeType == "source_removed":
			ct = platstore.ChangeRemoved
		default:
			ct = platstore.ChangeModified
		}
		diff.Changes = append(diff.Changes, platstore.BlockChange{
			BlockID:    blockID,
			ChangeType: ct,
		})
	}

	return diff, nil
}

// ---------------------------------------------------------------------------
// Stream membership
// ---------------------------------------------------------------------------

// AddStreamMember adds a user to a stream's member list.
func (s *SQLiteStore) AddStreamMember(ctx context.Context, projectID, streamName, userID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO stream_members (project_id, stream, user_id, added_at)
		 VALUES (?, ?, ?, ?)
		 ON CONFLICT(project_id, stream, user_id) DO NOTHING`,
		projectID, streamName, userID, now)
	if err != nil {
		return fmt.Errorf("add stream member: %w", err)
	}
	return nil
}

// RemoveStreamMember removes a user from a stream's member list.
func (s *SQLiteStore) RemoveStreamMember(ctx context.Context, projectID, streamName, userID string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM stream_members WHERE project_id = ? AND stream = ? AND user_id = ?`,
		projectID, streamName, userID)
	if err != nil {
		return fmt.Errorf("remove stream member: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("member %s not found in stream %s", userID, streamName)
	}
	return nil
}

// ListStreamMembers returns all user IDs that are members of a stream.
func (s *SQLiteStore) ListStreamMembers(ctx context.Context, projectID, streamName string) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT user_id FROM stream_members WHERE project_id = ? AND stream = ? ORDER BY added_at`,
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
// Scan helpers
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// Stream lock
// ---------------------------------------------------------------------------

// LockStream locks a stream, preventing further content changes.
func (s *SQLiteStore) LockStream(ctx context.Context, projectID, streamName, userID string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := s.db.ExecContext(ctx,
		`UPDATE streams SET locked = 1, locked_by = ?, locked_at = ?
		 WHERE project_id = ? AND name = ? AND locked = 0`,
		userID, now, projectID, streamName)
	if err != nil {
		return fmt.Errorf("lock stream: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		// Either not found or already locked.
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

// UnlockStream unlocks a previously locked stream.
func (s *SQLiteStore) UnlockStream(ctx context.Context, projectID, streamName string) error {
	res, err := s.db.ExecContext(ctx,
		`UPDATE streams SET locked = 0, locked_by = '', locked_at = NULL
		 WHERE project_id = ? AND name = ?`,
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
// Stream tags
// ---------------------------------------------------------------------------

// CreateStreamTag creates a new immutable tag on a stream.
func (s *SQLiteStore) CreateStreamTag(ctx context.Context, tag *platstore.StreamTag) error {
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
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tag.ID, tag.ProjectID, tag.Stream, tag.Name, string(tag.Kind),
		tag.Cursor, string(metaJSON), tag.CreatedBy, now.Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("insert stream tag: %w", err)
	}
	return nil
}

// ListStreamTags returns all tags for a given stream.
func (s *SQLiteStore) ListStreamTags(ctx context.Context, projectID, stream string) ([]*platstore.StreamTag, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, project_id, stream, name, kind, cursor, metadata, created_by, created_at
		 FROM stream_tags WHERE project_id = ? AND stream = ? ORDER BY created_at DESC`,
		projectID, stream)
	if err != nil {
		return nil, fmt.Errorf("list stream tags: %w", err)
	}
	defer rows.Close()
	return scanStreamTags(rows)
}

// GetStreamTag returns a single tag by stream and tag name.
func (s *SQLiteStore) GetStreamTag(ctx context.Context, projectID, stream, tagName string) (*platstore.StreamTag, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT id, project_id, stream, name, kind, cursor, metadata, created_by, created_at
		 FROM stream_tags WHERE project_id = ? AND stream = ? AND name = ?`,
		projectID, stream, tagName)
	return scanStreamTag(row)
}

// DeleteStreamTag removes a tag.
func (s *SQLiteStore) DeleteStreamTag(ctx context.Context, projectID, stream, tagName string) error {
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM stream_tags WHERE project_id = ? AND stream = ? AND name = ?`,
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

// ListProjectTags returns all tags across all streams in a project, optionally filtered by kind.
func (s *SQLiteStore) ListProjectTags(ctx context.Context, projectID string, kind platstore.StreamTagKind) ([]*platstore.StreamTag, error) {
	var query string
	var args []any
	if kind != "" {
		query = `SELECT id, project_id, stream, name, kind, cursor, metadata, created_by, created_at
				 FROM stream_tags WHERE project_id = ? AND kind = ? ORDER BY created_at DESC`
		args = []any{projectID, string(kind)}
	} else {
		query = `SELECT id, project_id, stream, name, kind, cursor, metadata, created_by, created_at
				 FROM stream_tags WHERE project_id = ? ORDER BY created_at DESC`
		args = []any{projectID}
	}

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list project tags: %w", err)
	}
	defer rows.Close()
	return scanStreamTags(rows)
}

// ---------------------------------------------------------------------------
// Scan helpers
// ---------------------------------------------------------------------------

func scanStream(row scanner) (*platstore.Stream, error) {
	var st platstore.Stream
	var archived, locked int
	var visibility, createdStr, lockedBy string
	var lockedAtStr *string
	err := row.Scan(&st.ProjectID, &st.Name, &st.Parent, &st.BaseCursor,
		&archived, &visibility, &st.Description, &createdStr, &st.CreatedBy,
		&locked, &lockedBy, &lockedAtStr)
	if err != nil {
		return nil, fmt.Errorf("scan stream: %w", err)
	}
	st.Archived = archived != 0
	st.Locked = locked != 0
	st.LockedBy = lockedBy
	if lockedAtStr != nil {
		t, _ := time.Parse(time.RFC3339, *lockedAtStr)
		st.LockedAt = &t
	}
	st.Visibility = platstore.StreamVisibility(visibility)
	st.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	return &st, nil
}

func scanStreamTag(row scanner) (*platstore.StreamTag, error) {
	var tag platstore.StreamTag
	var kindStr, metaStr, createdStr string
	err := row.Scan(&tag.ID, &tag.ProjectID, &tag.Stream, &tag.Name,
		&kindStr, &tag.Cursor, &metaStr, &tag.CreatedBy, &createdStr)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errors.New("stream tag not found")
		}
		return nil, fmt.Errorf("scan stream tag: %w", err)
	}
	tag.Kind = platstore.StreamTagKind(kindStr)
	tag.CreatedAt, _ = time.Parse(time.RFC3339, createdStr)
	if metaStr != "" && metaStr != "{}" {
		_ = json.Unmarshal([]byte(metaStr), &tag.Metadata)
	}
	return &tag, nil
}

func scanStreamTags(rows *sql.Rows) ([]*platstore.StreamTag, error) {
	var result []*platstore.StreamTag
	for rows.Next() {
		tag, err := scanStreamTag(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, tag)
	}
	return result, rows.Err()
}
