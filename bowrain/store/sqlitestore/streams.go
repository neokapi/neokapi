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

func (s *SQLiteStore) GetStream(ctx context.Context, projectID, name string) (*platstore.Stream, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by, locked, locked_by, locked_at, properties
		 FROM streams WHERE project_id = ? AND name = ?`,
		projectID, name)
	return scanStream(row)
}

// ListStreams returns all streams for a project.
func (s *SQLiteStore) ListStreams(ctx context.Context, projectID string, includeArchived bool) ([]*platstore.Stream, error) {
	var query string
	var args []any
	if includeArchived {
		query = `SELECT project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by, locked, locked_by, locked_at, properties
				 FROM streams WHERE project_id = ? ORDER BY name`
		args = []any{projectID}
	} else {
		query = `SELECT project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by, locked, locked_by, locked_at, properties
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

	propsJSON, err := json.Marshal(st.Properties)
	if err != nil {
		return fmt.Errorf("marshal stream properties: %w", err)
	}
	res, err := s.db.ExecContext(ctx,
		`UPDATE streams SET description = ?, visibility = ?, archived = ?, locked = ?, locked_by = ?, locked_at = ?, properties = ?
		 WHERE project_id = ? AND name = ?`,
		st.Description, string(st.Visibility), archived, locked, st.LockedBy, lockedAt, string(propsJSON), st.ProjectID, st.Name)
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
func (s *SQLiteStore) DeleteStream(ctx context.Context, projectID, name string) error {
	if name == "main" {
		return errors.New("cannot delete the main stream")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	res, err := tx.ExecContext(ctx,
		`DELETE FROM streams WHERE project_id = ? AND name = ?`,
		projectID, name)
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
			`DELETE FROM `+table+` WHERE project_id=? AND stream=?`, projectID, name); err != nil {
			return fmt.Errorf("delete %s for stream %q: %w", table, name, err)
		}
	}

	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Stream membership
//
// Branching — CreateStream, MergeStream, DiffStream — is deliberately absent.
// It is a server concern (store.StreamBranchStore): this store backs the
// desktop app's offline and cached editing, which works on one stream and calls
// no stream verb at all.
// ---------------------------------------------------------------------------

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
	var propsJSON string
	err := row.Scan(&st.ProjectID, &st.Name, &st.Parent, &st.BaseCursor,
		&archived, &visibility, &st.Description, &createdStr, &st.CreatedBy,
		&locked, &lockedBy, &lockedAtStr, &propsJSON)
	if err != nil {
		return nil, fmt.Errorf("scan stream: %w", err)
	}
	if propsJSON != "" {
		if err := json.Unmarshal([]byte(propsJSON), &st.Properties); err != nil {
			return nil, fmt.Errorf("unmarshal stream properties: %w", err)
		}
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
		if err := json.Unmarshal([]byte(metaStr), &tag.Metadata); err != nil {
			return nil, fmt.Errorf("unmarshal stream tag metadata: %w", err)
		}
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
