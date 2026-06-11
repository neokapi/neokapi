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

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO streams (project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		st.ProjectID, st.Name, st.Parent, st.BaseCursor, st.Archived,
		string(st.Visibility), st.Description, now, st.CreatedBy)
	if err != nil {
		return fmt.Errorf("insert stream: %w", err)
	}

	// Copy items from the parent stream into the new stream.
	parentStream := storeutil.DefaultStream(st.Parent)
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO items (id, project_id, stream, name, format, item_type, block_index, preview_html, properties, collection_id, created_at, updated_at)
		 SELECT substr(md5(random()::text), 1, 8), project_id, $1, name, format, item_type, block_index, preview_html, properties, collection_id, $2, $2
		 FROM items WHERE project_id = $3 AND stream = $4`,
		st.Name, now, st.ProjectID, parentStream)
	if err != nil {
		return fmt.Errorf("copy parent items: %w", err)
	}

	return nil
}

func (s *PostgresStore) GetStream(ctx context.Context, projectID, name string) (*platstore.Stream, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by, locked, locked_by, locked_at
		 FROM streams WHERE project_id = $1 AND name = $2`, projectID, name)
	return scanStreamPg(row)
}

func (s *PostgresStore) ListStreams(ctx context.Context, projectID string, includeArchived bool) ([]*platstore.Stream, error) {
	var query string
	var args []any
	if includeArchived {
		query = `SELECT project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by, locked, locked_by, locked_at
				 FROM streams WHERE project_id = $1 ORDER BY name`
		args = []any{projectID}
	} else {
		query = `SELECT project_id, name, parent, base_cursor, archived, visibility, description, created_at, created_by, locked, locked_by, locked_at
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
	res, err := s.db.ExecContext(ctx,
		`UPDATE streams SET parent=$1, base_cursor=$2, archived=$3, visibility=$4, description=$5, locked=$6, locked_by=$7, locked_at=$8
		 WHERE project_id=$9 AND name=$10`,
		st.Parent, st.BaseCursor, st.Archived, string(st.Visibility),
		st.Description, st.Locked, st.LockedBy, st.LockedAt, st.ProjectID, st.Name)
	if err != nil {
		return fmt.Errorf("update stream: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("stream %q not found in project %s", st.Name, st.ProjectID)
	}
	return nil
}

func (s *PostgresStore) DeleteStream(ctx context.Context, projectID, name string) error {
	if name == "main" {
		return errors.New("cannot delete the main stream")
	}
	res, err := s.db.ExecContext(ctx,
		`DELETE FROM streams WHERE project_id=$1 AND name=$2`, projectID, name)
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

func (s *PostgresStore) MergeStream(ctx context.Context, projectID, streamName string, opts platstore.MergeOptions) (*platstore.MergeResult, error) {
	stream, err := s.GetStream(ctx, projectID, streamName)
	if err != nil {
		return nil, fmt.Errorf("get stream: %w", err)
	}
	if stream.Parent == "" {
		return nil, fmt.Errorf("stream %q has no parent to merge into", streamName)
	}

	parentStream := storeutil.DefaultStream(stream.Parent)

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
		result.Changes = append(result.Changes, platstore.BlockChange{
			BlockID:    blockID,
			ChangeType: ct,
		})
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

	for blockID, changeType := range blockChanges {
		if changeType == "source_removed" {
			continue
		}
		if err := logChange(ctx, tx, projectID, parentStream, blockID, changeType, "", ""); err != nil {
			return nil, fmt.Errorf("log merge change: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit merge: %w", err)
	}

	return result, nil
}

func (s *PostgresStore) DiffStream(ctx context.Context, projectID, streamName string) (*platstore.StreamDiff, error) {
	stream, err := s.GetStream(ctx, projectID, streamName)
	if err != nil {
		return nil, fmt.Errorf("get stream: %w", err)
	}

	parentName := storeutil.DefaultStream(stream.Parent)

	changes, err := s.GetChanges(ctx, projectID, streamName, stream.BaseCursor, nil, MaxChangesPerRequest)
	if err != nil {
		return nil, fmt.Errorf("get stream changes: %w", err)
	}

	diff := &platstore.StreamDiff{
		StreamName: streamName,
		ParentName: parentName,
	}

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
	err := row.Scan(&st.ProjectID, &st.Name, &st.Parent, &st.BaseCursor,
		&st.Archived, &visibility, &st.Description, &st.CreatedAt, &st.CreatedBy,
		&st.Locked, &st.LockedBy, &st.LockedAt)
	if err != nil {
		return nil, fmt.Errorf("scan stream: %w", err)
	}
	st.Visibility = platstore.StreamVisibility(visibility)
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
		_ = json.Unmarshal([]byte(metaStr), &tag.Metadata)
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
